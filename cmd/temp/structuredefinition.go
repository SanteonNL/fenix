package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// StructureDefinitionsMap stores all structure definitions.
var StructureDefinitionsMap = make(map[string]fhir.StructureDefinition)

// LoadStructureDefinitions loads all StructureDefinitions into a global map.
func LoadStructureDefinitions(log zerolog.Logger) error {
	files, err := os.ReadDir("structuredefinitions")
	if err != nil {
		return fmt.Errorf("failed to read StructureDefinitions directory: %v", err)
	}

	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			filePath := filepath.Join("structuredefinitions", file.Name())
			structureDefinition, err := ReadFHIRResource(filePath, fhir.UnmarshalStructureDefinition)
			if err != nil {
				return fmt.Errorf("failed to read StructureDefinition from file: %v", err)
			}
			StructureDefinitionsMap[structureDefinition.Name] = *structureDefinition
			log.Debug().Str("structureDefinition", file.Name()).Msg("Loaded StructureDefinition")
			CollectValuesetBindingsForCodeTypes(structureDefinition, log)
		}
	}

	return nil
}

// TODO: See how a Quantity example might work, as it is not yet implemented
// CollectElementsWithCodeTypes collects elements from the StructureDefinition with code types and their value set bindings.
func CollectValuesetBindingsForCodeTypes(structureDefinition *fhir.StructureDefinition, log zerolog.Logger) {
	// Iterate through the elements in the Snapshot (you can also use Differential if needed)
	for _, element := range structureDefinition.Snapshot.Element {
		for _, t := range element.Type {
			// Choice based on https://www.hl7.org/fhir/search.html#token, CodeableReference is excluded because it is R5
			if t.Code == "code" || t.Code == "Coding" || t.Code == "CodeableConcept" || t.Code == "Quantity" {
				//log.Debug().Msgf("Path: %s, Type: %s, Definition: %s", element.Path, t.Code, *element.Definition)
				if element.Binding != nil {
					//log.Debug().Msgf("  Binding Strength: %s, Value Set URL: %s ", element.Binding.Strength, *element.Binding.ValueSet)
					FhirPathToValueset[element.Path] = *element.Binding.ValueSet
				} else {
					log.Debug().Msgf("No binding for path: %s, code: %s", element.Path, t.Code)
				}
				break
			}
		}
	}
}

// UpdateSearchParameterBindings updates the FhirPathToValueset map with any ValueSet
// references found in search parameters
func UpdateSearchParameterBindings(resourceType string, searchParams SearchParameterMap, log zerolog.Logger) {
	for searchPath, param := range searchParams {
		if param.Type == "token" && IsValueSetReference(param.Value) {
			cleanURL := cleanValueSetURL(param.Value)
			log.Debug().
				Str("path", searchPath).
				Str("valuesetURL", cleanURL).
				Msg("Adding ValueSet binding from search parameter")

			FhirPathToValueset[searchPath] = cleanURL
		}
	}
}

// cleanValueSetURL removes query parameters from ValueSet URLs
func cleanValueSetURL(url string) string {
	if idx := strings.Index(url, "?"); idx != -1 {
		return url[:idx]
	}
	return url
}
