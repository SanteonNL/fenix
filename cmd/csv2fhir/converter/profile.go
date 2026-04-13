package converter

import (
	"fmt"
	"os"
	"strings"

	"github.com/SanteonNL/fenix/models/fhir"
	"github.com/rs/zerolog"
)

// ProfileService loads FHIR StructureDefinition profiles and provides the
// valueset binding URI for any FHIR path (e.g. "Patient.gender" →
// "http://hl7.org/fhir/ValueSet/administrative-gender").
type ProfileService struct {
	pathToValueset map[string]string // "Patient.gender" → valueset URI (no version)
	logger         zerolog.Logger
}

// NewProfileService creates an empty service.
func NewProfileService(logger zerolog.Logger) *ProfileService {
	return &ProfileService{
		pathToValueset: make(map[string]string),
		logger:         logger,
	}
}

// LoadProfile parses a single StructureDefinition JSON file and registers all
// code-type element bindings found in the snapshot.
func (p *ProfileService) LoadProfile(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", filePath, err)
	}

	sd, err := fhir.UnmarshalStructureDefinition(data)
	if err != nil {
		return fmt.Errorf("unmarshal %s: %w", filePath, err)
	}
	if sd.Snapshot == nil {
		return nil
	}

	count := 0
	for _, element := range sd.Snapshot.Element {
		if element.Binding == nil || element.Binding.ValueSet == nil {
			continue
		}
		for _, t := range element.Type {
			if t.Code == "code" || t.Code == "Coding" || t.Code == "CodeableConcept" {
				path := element.Path
				vs := stripVersion(*element.Binding.ValueSet)
				p.pathToValueset[path] = vs
				count++
				break
			}
		}
	}

	p.logger.Info().
		Str("file", filePath).
		Int("bindings", count).
		Msg("Loaded FHIR profile")
	return nil
}

// LoadDir loads all .json StructureDefinition files from a directory.
func (p *ProfileService) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		if err := p.LoadProfile(dir + "/" + e.Name()); err != nil {
			p.logger.Warn().Err(err).Str("file", e.Name()).Msg("Skipping profile")
		}
	}
	return nil
}

// ValuesetURI returns the binding valueset URI for a given FHIR path,
// e.g. ValuesetURI("Patient.gender") → "http://hl7.org/fhir/ValueSet/administrative-gender".
// Returns "" if no binding is registered for the path.
func (p *ProfileService) ValuesetURI(fhirPath string) string {
	return p.pathToValueset[fhirPath]
}

// applyConceptMappings recursively walks a raw FHIR resource map.
// currentPath is the FHIR path of the current map level (e.g. "Encounter" at root,
// "Encounter.statusHistory" when recursing into statusHistory items).
// For each string field it looks up the valueset binding from the profile and
// translates the code via the concept map service if a mapping exists.
func applyConceptMappings(raw map[string]any, currentPath string, profile *ProfileService, concepts *ConceptMapService) {
	if profile == nil || concepts == nil {
		return
	}

	for field, val := range raw {
		childPath := currentPath + "." + field

		switch v := val.(type) {
		case string:
			if vsURI := profile.ValuesetURI(childPath); vsURI != "" {
				if mapped, changed := concepts.Translate(vsURI, v); changed {
					raw[field] = mapped
				}
			}
		case []byte:
			strVal := string(v)
			if vsURI := profile.ValuesetURI(childPath); vsURI != "" {
				if mapped, changed := concepts.Translate(vsURI, strVal); changed {
					raw[field] = mapped
				}
			}
		case map[string]any:
			applyConceptMappings(v, childPath, profile, concepts)
		case []any:
			for _, elem := range v {
				if m, ok := elem.(map[string]any); ok {
					applyConceptMappings(m, childPath, profile, concepts)
				}
			}
		}
	}
}

func rawToString(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}

