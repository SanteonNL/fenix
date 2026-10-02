package converter

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// translationEntry is one group.element -> group.element.target pairing,
// flattened out of a FHIR ConceptMap resource for fast lookup.
type translationEntry struct {
	sourceCode    string
	targetCode    string
	targetDisplay string
}

// valuesetMap holds every translation entry that targets one valueset
// (potentially contributed by several ConceptMap resources/files), the set
// of already-valid target codes, and the fallback for unmapped source codes.
type valuesetMap struct {
	entries    []translationEntry
	validCodes map[string]bool // every code_target value — already-valid codes skip mapping
	unmapped   *translationEntry
}

// ConceptMapService loads real FHIR ConceptMap resources (JSON files
// conforming to http://hl7.org/fhir/StructureDefinition/ConceptMap) and
// indexes them by the valueset they target (ConceptMap.targetCanonical /
// ConceptMap.targetUri, version suffix stripped) so Translate can be called
// with the same valueset URI a FHIR profile binding resolves to.
//
// If the source code is already a valid target code for a valueset it is
// passed through unchanged. A group's "unmapped" element (mode "fixed")
// supplies the fallback for source codes with no explicit mapping — this is
// the standard FHIR way to express what used to be a "*" wildcard row in the
// old flat CSV format.
type ConceptMapService struct {
	mu         sync.RWMutex
	byValueset map[string]*valuesetMap // target_valueset_uri (no version) -> map
	logger     zerolog.Logger
}

// NewConceptMapService creates an empty service.
func NewConceptMapService(logger zerolog.Logger) *ConceptMapService {
	return &ConceptMapService{
		byValueset: make(map[string]*valuesetMap),
		logger:     logger,
	}
}

// LoadJSON loads one FHIR ConceptMap resource from a JSON file and merges
// its mappings into the service, keyed by its target valueset.
// ConceptMap resources with no targetCanonical/targetUri are loaded
// successfully but can't be looked up by Translate (there's no valueset to
// index them under) — this is allowed so an editor can still show/edit them.
func (s *ConceptMapService) LoadJSON(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	cm, err := fhir.UnmarshalConceptMap(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filePath, err)
	}

	vsURI := conceptMapTargetValueset(cm)
	if vsURI == "" {
		s.logger.Warn().Str("file", filePath).Msg("ConceptMap has no targetCanonical/targetUri, skipping (not indexable)")
		return nil
	}
	vsURI = stripVersion(vsURI)

	s.mu.Lock()
	defer s.mu.Unlock()

	vm, ok := s.byValueset[vsURI]
	if !ok {
		vm = &valuesetMap{validCodes: map[string]bool{}}
		s.byValueset[vsURI] = vm
	}

	for _, group := range cm.Group {
		for _, element := range group.Element {
			if element.Code == nil {
				continue
			}
			for _, target := range element.Target {
				if target.Code == nil {
					continue
				}
				entry := translationEntry{
					sourceCode:    *element.Code,
					targetCode:    *target.Code,
					targetDisplay: stringOrEmpty(target.Display),
				}
				vm.entries = append(vm.entries, entry)
				vm.validCodes[entry.targetCode] = true
			}
		}

		if group.Unmapped != nil &&
			group.Unmapped.Mode == fhir.ConceptMapGroupUnmappedModeFixed &&
			group.Unmapped.Code != nil {
			vm.unmapped = &translationEntry{
				targetCode:    *group.Unmapped.Code,
				targetDisplay: stringOrEmpty(group.Unmapped.Display),
			}
		}
	}

	return nil
}

// LoadDir (re)loads all .json FHIR ConceptMap files from a directory,
// replacing any previously loaded mappings. Safe to call again at runtime
// (e.g. after an editor saves a file) to pick up changes without restarting.
func (s *ConceptMapService) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	s.mu.Lock()
	s.byValueset = make(map[string]*valuesetMap)
	s.mu.Unlock()

	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		if err := s.LoadJSON(dir + "/" + e.Name()); err != nil {
			s.logger.Warn().Err(err).Str("file", e.Name()).Msg("Skipping concept map")
		} else {
			count++
		}
	}
	s.logger.Info().Str("dir", dir).Int("files", count).Msg("Loaded concept maps")
	return nil
}

// Translate maps sourceCode using the concept map(s) loaded for the given
// valueset URI. If the code is already a valid target code for this valueset
// it is returned unchanged. Falls back to the group's "unmapped" fixed code
// for unknown codes, if one was loaded. Returns the original code when no
// concept map is loaded for the valueset.
func (s *ConceptMapService) Translate(valuesetURI, sourceCode string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	vm, ok := s.byValueset[stripVersion(valuesetURI)]
	if !ok {
		return sourceCode, false
	}

	// Already a valid FHIR target code — no mapping needed
	if vm.validCodes[sourceCode] {
		return sourceCode, false
	}

	for _, e := range vm.entries {
		if e.sourceCode == sourceCode {
			s.logger.Debug().
				Str("valueset", valuesetURI).
				Str("from", sourceCode).
				Str("to", e.targetCode).
				Msg("Concept mapped (exact)")
			return e.targetCode, true
		}
	}

	if vm.unmapped != nil {
		s.logger.Debug().
			Str("valueset", valuesetURI).
			Str("from", sourceCode).
			Str("to", vm.unmapped.targetCode).
			Msg("Concept mapped (unmapped fallback)")
		return vm.unmapped.targetCode, true
	}
	return sourceCode, false
}

// conceptMapTargetValueset returns the valueset a ConceptMap resource's
// mappings produce codes for, preferring targetCanonical over targetUri.
func conceptMapTargetValueset(cm fhir.ConceptMap) string {
	if cm.TargetCanonical != nil {
		return *cm.TargetCanonical
	}
	if cm.TargetUri != nil {
		return *cm.TargetUri
	}
	return ""
}

// stripVersion removes a version suffix like "|4.0.1" from a valueset URI.
func stripVersion(uri string) string {
	if idx := strings.Index(uri, "|"); idx >= 0 {
		return uri[:idx]
	}
	return uri
}

func stringOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
