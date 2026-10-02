package converter

import (
	"fmt"
	"os"
	"strings"

	"github.com/SanteonNL/fenix/internal/models/fhir"
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
//
// A profile binding can sit on three different shapes of element:
//   - a plain `code` field (e.g. Patient.gender) — the field's own string
//     value is the code to translate.
//   - a `Coding` (e.g. a valueCoding) — the code lives in that object's own
//     "code" field.
//   - a `CodeableConcept` (e.g. Observation.code) — the codes live one level
//     deeper, in its "coding" array's "code" fields. Every coding under one
//     CodeableConcept is translated against the same binding, since FHIR
//     doesn't let a profile bind per-coding-system within one element.
//
// Either way, concepts.Translate is only told the bare code — it has no
// notion of which coding system a given code belongs to (see
// ConceptMapService.Translate), so entries from different source systems
// that happen to share a literal code value cannot be disambiguated.
func applyConceptMappings(raw map[string]any, currentPath string, profile *ProfileService, concepts *ConceptMapService) {
	if profile == nil || concepts == nil {
		return
	}

	for field, val := range raw {
		childPath := currentPath + "." + field
		vsURI := profile.ValuesetURI(childPath)

		switch v := val.(type) {
		case string:
			if vsURI != "" {
				if mapped, _, _, changed := concepts.Translate(vsURI, v); changed {
					raw[field] = mapped
				}
			}
		case []byte:
			if vsURI != "" {
				if mapped, _, _, changed := concepts.Translate(vsURI, string(v)); changed {
					raw[field] = mapped
				}
			}
		case map[string]any:
			if vsURI != "" {
				translateCodingLike(v, vsURI, concepts)
			}
			applyConceptMappings(v, childPath, profile, concepts)
		case []any:
			for _, elem := range v {
				if m, ok := elem.(map[string]any); ok {
					if vsURI != "" {
						translateCodingLike(m, vsURI, concepts)
					}
					applyConceptMappings(m, childPath, profile, concepts)
				}
			}
		}
	}
}

// translateCodingLike translates the code(s) inside a Coding or
// CodeableConcept JSON object bound to vsURI. A CodeableConcept carries its
// codes in a "coding" array; a bare Coding carries "code" directly on v.
//
// "coding" can still be a single bare object rather than a one-element
// array at this point: applyConceptMappings runs before
// validateThroughStruct's normalizeNestedArrays, which is what wraps a
// single row's coding into an array to match the FHIR struct's []Coding
// field — so both shapes have to be handled here.
func translateCodingLike(v map[string]any, vsURI string, concepts *ConceptMapService) {
	if codingsRaw, ok := v["coding"]; ok {
		switch codings := codingsRaw.(type) {
		case []any:
			for _, c := range codings {
				if coding, ok := c.(map[string]any); ok {
					translateCode(coding, vsURI, concepts)
				}
			}
		case map[string]any:
			translateCode(codings, vsURI, concepts)
		}
		return
	}
	translateCode(v, vsURI, concepts)
}

// translateCode translates m["code"] in place, and syncs m["display"] and
// m["system"] to the mapping's target display/system — but only when the
// mapping actually supplied one, so a mapping that left display_target or
// its group's target system unset doesn't blank out what was already there.
// Leaving system stale would otherwise be actively wrong: a translated code
// from a different code system (e.g. a LOINC code) left under the source's
// original system URI misrepresents what the code actually is.
func translateCode(m map[string]any, vsURI string, concepts *ConceptMapService) {
	code, ok := m["code"].(string)
	if !ok {
		return
	}
	mapped, display, system, changed := concepts.Translate(vsURI, code)
	if !changed {
		return
	}
	m["code"] = mapped
	if display != "" {
		m["display"] = display
	}
	if system != "" {
		m["system"] = system
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

