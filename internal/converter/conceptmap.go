package converter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// translationEntry is one group.element -> group.element.target pairing,
// flattened out of a FHIR ConceptMap resource for fast lookup. targetSystem
// comes from the parent group's own "target" field (the code system the
// group's codes belong to) — every entry from the same group shares it.
type translationEntry struct {
	sourceCode    string
	targetCode    string
	targetDisplay string
	targetSystem  string
	file          string // the ConceptMap file the entry came from, for the editor
}

// valuesetMap holds every translation entry that targets one valueset
// (potentially contributed by several ConceptMap resources/files), the set
// of already-valid target codes, and the fallback for unmapped source codes.
type valuesetMap struct {
	entries    []translationEntry
	validCodes map[string]bool // every code_target value — already-valid codes skip mapping
	unmapped   *translationEntry
	files      []string // ConceptMap files that contribute to this valueset, in load order
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
	byValueset map[string]*valuesetMap    // target_valueset_uri (no version) -> map
	valueSets  map[string]map[string]bool // valueset url (no version) -> its member codes, see LoadValueSetsDir
	vsURLs     map[string]string          // valueset url or "ValueSet/<id>" -> its canonical url (no version)
	vsConcepts map[string][]ValueSetConcept // valueset url or "ValueSet/<id>" -> its listed concepts, for the editor's pick-lists
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
	file := filepath.Base(filePath)
	vm.files = append(vm.files, file)

	for _, group := range cm.Group {
		targetSystem := stringOrEmpty(group.Target)

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
					targetSystem:  targetSystem,
					file:          file,
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
				targetSystem:  targetSystem,
				file:          file,
			}
		}
	}

	return nil
}

// LoadValueSetsDir reads FHIR ValueSet .json files (also the {"valueSet": {...}} wrapper
// the terminology/valuesets files use) and remembers their member codes: the
// concepts of compose.include and the entries of expansion.contains. A code that
// is a member of the valueset a concept map targets is valid as it is, so it needs
// no mapping; only codes outside the valueset can be unmapped. Valuesets that
// include whole code systems or use filters have no listed members and are skipped.
// It does not touch the loaded concept maps, so editor saves (LoadDir) keep it.
func (s *ConceptMapService) LoadValueSetsDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	sets := map[string]map[string]bool{}
	urls := map[string]string{}
	concepts := map[string][]ValueSetConcept{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			s.logger.Warn().Err(err).Str("file", e.Name()).Msg("Skipping valueset")
			continue
		}
		url, id, list := valueSetMembers(data)
		if url == "" || len(list) == 0 {
			continue
		}
		for _, name := range []string{stripVersion(url), "ValueSet/" + id} {
			if name == "ValueSet/" {
				continue
			}
			urls[name] = stripVersion(url)
			if sets[name] == nil {
				sets[name] = map[string]bool{}
			}
			for _, c := range list {
				sets[name][c.Code] = true
			}
			concepts[name] = append(concepts[name], list...)
		}
	}
	s.mu.Lock()
	s.valueSets = sets
	s.vsURLs = urls
	s.vsConcepts = concepts
	s.mu.Unlock()
	s.logger.Info().Str("dir", dir).Int("valuesets", len(sets)).Msg("Loaded valuesets")
	return nil
}

// ValueSetConcept is one listed member of a valueset.
type ValueSetConcept struct {
	System  string `json:"system,omitempty"`
	Code    string `json:"code"`
	Display string `json:"display,omitempty"`
}

// valueSetMembers returns the url, id and listed members of a ValueSet resource.
func valueSetMembers(data []byte) (url, id string, concepts []ValueSetConcept) {
	var vs struct {
		ID       string `json:"id"`
		URL      string `json:"url"`
		ValueSet *struct {
			URL string `json:"url"`
		} `json:"valueSet"`
		Compose struct {
			Include []struct {
				System  string `json:"system"`
				Concept []struct {
					Code    string `json:"code"`
					Display string `json:"display"`
				} `json:"concept"`
			} `json:"include"`
		} `json:"compose"`
		Expansion struct {
			Contains []struct {
				System  string `json:"system"`
				Code    string `json:"code"`
				Display string `json:"display"`
			} `json:"contains"`
		} `json:"expansion"`
	}
	if json.Unmarshal(data, &vs) != nil {
		return "", "", nil
	}
	if vs.URL == "" && vs.ValueSet != nil {
		// the wrapper holds the resource one level down: read it again from there
		var w struct {
			ValueSet json.RawMessage `json:"valueSet"`
		}
		if json.Unmarshal(data, &w) == nil && w.ValueSet != nil {
			return valueSetMembers(w.ValueSet)
		}
	}
	for _, inc := range vs.Compose.Include {
		for _, c := range inc.Concept {
			concepts = append(concepts, ValueSetConcept{System: inc.System, Code: c.Code, Display: c.Display})
		}
	}
	for _, c := range vs.Expansion.Contains {
		concepts = append(concepts, ValueSetConcept{System: c.System, Code: c.Code, Display: c.Display})
	}
	return vs.URL, vs.ID, concepts
}

// ValueSetConcepts returns the listed concepts of a valueset given by canonical URL or
// "ValueSet/<id>", with their systems and displays, and whether it is known.
func (s *ConceptMapService) ValueSetConcepts(ref string) ([]ValueSetConcept, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.vsConcepts[stripVersion(ref)]
	return append([]ValueSetConcept(nil), c...), ok
}
// ValueSetURL returns the canonical URL (no version) of a valueset given by URL or by
// reference "ValueSet/<id>", or "" when it is not known. Concept maps are indexed by
// that URL, so this is how a reference becomes the valueset a map can target.
func (s *ConceptMapService) ValueSetURL(ref string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vsURLs[stripVersion(ref)]
}

// ValueSetMembers returns the member codes of a valueset given by canonical URL
// (a version suffix is ignored) or by reference "ValueSet/<id>", and whether
// that valueset is known (see LoadValueSetsDir).
func (s *ConceptMapService) ValueSetMembers(ref string) (map[string]bool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.valueSets[stripVersion(ref)]
	return m, ok
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
// for unknown codes, if one was loaded. Returns the original code (and no
// display/system) when no concept map is loaded for the valueset or the code
// is already valid — changed is false in both cases, so callers that also
// carry sibling fields (Coding.display, Coding.system) know not to touch
// them. targetSystem is "" when the matching group left its own "target"
// field unset — callers should likewise leave the existing system alone then,
// rather than blank it out.
func (s *ConceptMapService) Translate(valuesetURI, sourceCode string) (targetCode, targetDisplay, targetSystem string, changed bool) {
	targetCode, targetDisplay, targetSystem, status := s.TranslateStatus(valuesetURI, sourceCode)
	return targetCode, targetDisplay, targetSystem, status == MapMapped || status == MapDefault
}

// TranslateStatus is Translate plus how the code was treated: MapValid, MapMapped
// (an explicit mapping), MapDefault (the fixed fallback replaced it), MapNone
// (a map exists, but neither a mapping nor a fallback), or "" when no concept map
// is loaded for the valueset at all.
func (s *ConceptMapService) TranslateStatus(valuesetURI, sourceCode string) (targetCode, targetDisplay, targetSystem, status string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	vm, ok := s.byValueset[stripVersion(valuesetURI)]
	if !ok {
		return sourceCode, "", "", ""
	}

	// Already a valid FHIR target code, or a member of the target valueset — no mapping needed
	if vm.validCodes[sourceCode] || s.valueSets[stripVersion(valuesetURI)][sourceCode] {
		return sourceCode, "", "", MapValid
	}

	for _, e := range vm.entries {
		if e.sourceCode == sourceCode {
			s.logger.Debug().
				Str("valueset", valuesetURI).
				Str("from", sourceCode).
				Str("to", e.targetCode).
				Msg("Concept mapped (exact)")
			return e.targetCode, e.targetDisplay, e.targetSystem, MapMapped
		}
	}

	if vm.unmapped != nil {
		s.logger.Debug().
			Str("valueset", valuesetURI).
			Str("from", sourceCode).
			Str("to", vm.unmapped.targetCode).
			Msg("Concept mapped (unmapped fallback)")
		return vm.unmapped.targetCode, vm.unmapped.targetDisplay, vm.unmapped.targetSystem, MapDefault
	}
	return sourceCode, "", "", MapNone
}

// ReverseTranslate is Translate backwards: given a code of the TARGET
// valueset (what a FHIR client searches for), it returns the SOURCE codes
// that Translate turns into it, so a search can be pushed down into SQL
// against the untranslated source column.
//   - mapped is false when no concept map is loaded for the valueset (the
//     code is then compared as is).
//   - A source code can map to several targets; Translate uses the first, so
//     only sources whose first target is targetCode are returned.
//   - The code itself is included when Translate would pass it through
//     unchanged.
//   - exact is false when targetCode is also the map's fixed "unmapped"
//     fallback: every unknown source code then maps to it, which can't be
//     listed — callers must not push such a search down into SQL.
func (s *ConceptMapService) ReverseTranslate(valuesetURI, targetCode string) (sourceCodes []string, exact, mapped bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	vm, ok := s.byValueset[stripVersion(valuesetURI)]
	if !ok {
		return nil, true, false
	}

	seen := map[string]bool{}
	firstTarget := map[string]string{}
	for _, e := range vm.entries {
		if _, ok := firstTarget[e.sourceCode]; !ok {
			firstTarget[e.sourceCode] = e.targetCode
		}
	}
	for _, e := range vm.entries {
		if e.targetCode == targetCode && firstTarget[e.sourceCode] == targetCode && !seen[e.sourceCode] {
			seen[e.sourceCode] = true
			sourceCodes = append(sourceCodes, e.sourceCode)
		}
	}
	// Translate passes the code through unchanged when it is a valid target
	// code, or when there is no unmapped fallback to replace it with.
	if (vm.validCodes[targetCode] || vm.unmapped == nil) && !seen[targetCode] {
		sourceCodes = append(sourceCodes, targetCode)
	}
	exact = vm.unmapped == nil || vm.unmapped.targetCode != targetCode
	return sourceCodes, exact, true
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

// How a source code is treated, as reported by TranslateStatus and Resolve.
const (
	MapValid   = "valid"   // already a valid target code: passed through unchanged
	MapMapped  = "mapped"  // an explicit mapping applies
	MapDefault = "default" // no mapping: the group's fixed "unmapped" code replaces it
	MapNone    = "none"    // no mapping and no default: passed through unchanged
)

// CodeResolution says what Translate does with one source code.
type CodeResolution struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Target  string `json:"target,omitempty"`  // the code it becomes (mapped, default)
	Display string `json:"display,omitempty"` // its display, if the map has one
	File    string `json:"file,omitempty"`    // the ConceptMap file to edit for this code
}

// DefaultMapping is a map's fixed fallback for unmapped source codes.
type DefaultMapping struct {
	Code    string `json:"code"`
	Display string `json:"display,omitempty"`
	File    string `json:"file"`
}

// MappingInfo is how the concept maps of one valueset treat a list of source codes.
type MappingInfo struct {
	Valueset string           `json:"valueset"`
	HasMap   bool             `json:"has_map"` // false: no ConceptMap targets this valueset, nothing is translated
	Files    []string         `json:"files,omitempty"`
	Default  *DefaultMapping  `json:"default,omitempty"`
	Codes    []CodeResolution `json:"codes"`
}

// Resolve reports, for each source code, what Translate would do with it and
// which ConceptMap file the answer comes from. It uses TranslateStatus, so what
// the dashboard shows is what the converter does.
func (s *ConceptMapService) Resolve(valuesetURI string, codes []string) MappingInfo {
	info := MappingInfo{Valueset: stripVersion(valuesetURI), Codes: make([]CodeResolution, 0, len(codes))}
	s.mu.RLock()
	vm, ok := s.byValueset[info.Valueset]
	var files []string
	var def *translationEntry
	var firstFile = map[string]string{}
	if ok {
		files = append([]string(nil), vm.files...)
		def = vm.unmapped
		for _, e := range vm.entries {
			if _, seen := firstFile[e.sourceCode]; !seen {
				firstFile[e.sourceCode] = e.file
			}
		}
	}
	s.mu.RUnlock()

	if !ok {
		for _, c := range codes {
			info.Codes = append(info.Codes, CodeResolution{Code: c, Status: MapNone})
		}
		return info
	}
	info.HasMap, info.Files = true, files
	// where a new mapping for an unmapped code belongs: the map that holds the fallback, else the first
	addTo := ""
	if len(files) > 0 {
		addTo = files[0]
	}
	if def != nil {
		info.Default = &DefaultMapping{Code: def.targetCode, Display: def.targetDisplay, File: def.file}
		if def.file != "" {
			addTo = def.file
		}
	}
	for _, c := range codes {
		target, display, _, status := s.TranslateStatus(valuesetURI, c)
		r := CodeResolution{Code: c, Status: status, File: addTo}
		switch status {
		case MapMapped:
			r.Target, r.Display, r.File = target, display, firstFile[c]
		case MapDefault:
			r.Target, r.Display = target, display
		}
		info.Codes = append(info.Codes, r)
	}
	return info
}
