package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/SanteonNL/fenix/internal/models/fhir" // for use of the conceptMap.go file
	"github.com/rs/zerolog"
)

// Per resource:
// - Mapping van FHIRPath naar Valueset
// - Mapping van Valueset naar ConceptMap

// Mapping van FHIRPath naar ValueSet
// TODO: Add possibility to overrirde Valueset from resource structure definition (R4) with Valueset from searchparameter (url)

var FhirPathToValueset = make(map[string]string)

// Mapping van Valueset naar ConceptMap (gebaseerd op targetUri)
// TODO: Check if targetCanonical should also work or make an agreement taht we always use the more generic targetUri
// TODO: Add the possibitlity to read all ConceptMaps in the case a code is requested without a valueset?
// (Maybe read the entire ConceptMap resource and filter on targetUri, depending on how memory intensive that is)
// The ConceptMaps have to be read in anyways to get the mappings, but the complete conceptmaps that are needed
// might be limited to the ones that are relevant for the searchparameters.
// So maybe best to do it in 2 stesps: first make a map with name and targetUri, then read the complete ConceptMaps
// that are relevant for the searchparameters.
// Per Conceptmap there is only one targetUri (the valueset uri), so that is the key to use.
// How to handle the fact that multiple conceptmaps can be used for the same valueset?
// Solution: make a map with valueset uri as key and conceptmap as value
// Then there is also some other data needed to know which conceptmap to use, like the organization and version
// Maybe that has to be collected already in the first step, so that the conceptmaps can be filtered on that
var ValueSetToConceptMap = make(map[string]*fhir.ConceptMap)

// Functie om ConceptMap op te halen voor een gegeven FHIRPath
// This is the fhirPath that in structureDefinition is used to define the valueset, nl. element.binding.valueSet
func getConceptMapForFhirPath(valuesetBindingPath string, log zerolog.Logger) (fhir.ConceptMap, error) {
	valueSet, exists := FhirPathToValueset[valuesetBindingPath]
	if !exists {
		return fhir.ConceptMap{}, fmt.Errorf("no valueset found voor FHIR valuesetBindingPath: %s", valuesetBindingPath)
	}
	log.Debug().Msgf("valueSet for FHIR valuesetBindingPath %s: %s", valuesetBindingPath, valueSet)

	conceptMap, exists := ValueSetToConceptMap[valueSet]
	if !exists {
		return fhir.ConceptMap{}, fmt.Errorf("no ConceptMap found voor ValueSet: %s", valueSet)
	}
	log.Debug().Msgf("ConceptMap for valueset %s: %s", valueSet, *conceptMap.Id)

	return *conceptMap, nil
}

// TODO add something that makes the programme does not exit when default mappingn is lacking
// and an unknown code is encountered (for coding this should be fine however)
func TranslateCode(conceptMap fhir.ConceptMap, sourceCode *string, typeIsCode bool, log zerolog.Logger) (*string, *string, error) {
	log.Debug().Msgf("sourceCode: %v", *sourceCode)
	for _, group := range conceptMap.Group {
		log.Debug().Msgf("Group.Source: %v", *group.Source)
		log.Debug().Msgf("Source: %v", *conceptMap.SourceUri)
		//if group.Source == conceptMap.SourceUri {
		for _, element := range group.Element {
			if *element.Code == *sourceCode {
				for _, target := range element.Target {
					log.Debug().Msgf("Returning targetCode: %s, targetDisplay: %s", *target.Code, *target.Display)
					return target.Code, target.Display, nil
				}
			}
		}
	}
	log.Debug().Msgf("Code not found in ConceptMap")
	if typeIsCode {
		log.Debug().Msgf("Type is code, default mapping with *")
		for _, group := range conceptMap.Group {
			log.Debug().Msgf("Group.Source: %v", *group.Source)
			log.Debug().Msgf("Source: %v", *conceptMap.SourceUri)
			for _, element := range group.Element {
				if *element.Code == "*" {
					for _, target := range element.Target {
						log.Debug().Msgf("Returning targetCode: %s, targetDisplay: %s", *target.Code, *target.Display)
						return target.Code, target.Display, nil
					}
				}
			}
		}
	}
	return nil, nil, nil
}

func LoadConceptMaps(log zerolog.Logger) error {
	files, err := os.ReadDir("config/conceptmaps")
	if err != nil {
		return fmt.Errorf("failed to read conceptmaps directory: %v", err)
	}

	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".json" {
			filePath := filepath.Join("config/conceptmaps", file.Name())
			conceptMap, err := ReadFHIRResource(filePath, fhir.UnmarshalConceptMap)
			if err != nil {
				return fmt.Errorf("failed to read Conceptmap from file: %v", err)
			}
			log.Debug().Str("conceptMap", file.Name()).Msg("Loaded conceptMap")
			ValueSetToConceptMap[*conceptMap.TargetUri] = conceptMap
		}
	}

	return nil
}

// TODO for the 3 funcitons below:
// - Check when and when not to reutrn errors
// - Check in what cases they will go wrong (eg struct withoud code?)
// - See if other chekcs are needed
// - For a code make sure always everything is mapped to the bound valueset, for coding also unmapped codes can
// be correct
// - What to do with system and display fields? (not yet implemented)
// - Let cocneptmappingn for codes not in conceptmpa map to unknwon oid (or should this be done in conceptmap?)

// TODO: check if some code from processresources.go can moved to here
func performConceptMapping(valuesetBindingPath string, inputValue string, typeIsCode bool, log zerolog.Logger) (string, string, error) {
	// Check if fhirPath is in FhirPathToValueset
	_, exists := FhirPathToValueset[valuesetBindingPath]
	if !exists {
		log.Debug().Msgf("FHIR valuesetBindingPath %s is not in FhirPathToValueset", valuesetBindingPath)
		return inputValue, "", nil
	}

	log.Debug().Msgf("FHIR valuesetBindingPath %s is in FhirPathToValueset", valuesetBindingPath)

	// Collect the ConceptMap for the FHIR path
	conceptMap, err := getConceptMapForFhirPath(valuesetBindingPath, log)
	if err != nil {
		log.Debug().Msgf("Failed to get ConceptMap for FHIR Path: %v", err)
		// No error return needed in this case
		return inputValue, "", nil
	}

	log.Debug().Msgf("ConceptMap for FHIR valuesetBindingPath %s: %v", valuesetBindingPath, *conceptMap.Id)

	// Perform concept mapping using the ConceptMap
	targetCode, targetDisplay, err := TranslateCode(conceptMap, &inputValue, typeIsCode, log)
	if err != nil {
		return "", "", fmt.Errorf("Failed to map concept code: %v", err)
	}
	// TODO check if this goes well when targetDisplay is nil, if that case can happen
	if targetCode != nil {
		log.Debug().Msgf("Mapped concept code: %v, display: %v", *targetCode, *targetDisplay)
		// Return the mapped code and display
		return *targetCode, *targetDisplay, nil
	}
	return "", "", nil

}

// TODO: check if this function is still needed, probably not because performConceptMapping is used directly
func applyConceptMappingForStruct(structPath string, structType reflect.Type, structPointer interface{}, log zerolog.Logger) error {
	// Get the FHIR path for ValueSet binding
	fhirPath := getValueSetBindingPath(structPath, structType.Name())
	log.Debug().Msgf("FHIR Path to determine ValueSet: %s", fhirPath)

	// Check for system, code, or display fields in the struct
	structValue := reflect.ValueOf(structPointer).Elem()
	for i := 0; i < structType.NumField(); i++ {
		fieldName := structType.Field(i).Name
		fieldNameLower := strings.ToLower(fieldName)

		if fieldNameLower == "system" || fieldNameLower == "code" || fieldNameLower == "display" {
			log.Debug().Msgf("This field might need concept mapping: %s", fieldNameLower)
		}
	}

	// Get the code field value
	fieldValue := structValue.FieldByName("Code")
	log.Debug().Msgf("Field value: %v", fieldValue)
	fieldValueStr := getStringValue(fieldValue.Elem())

	// Perform concept mapping using the shared function
	mappedCode, _, err := performConceptMapping(fhirPath, fieldValueStr, false, log)
	if err != nil {
		return err
	}
	log.Debug().Msgf("Mapped code: %s", mappedCode)

	// Set the mapped code back to the struct's Code field
	// TODO make working with current set functions, SetField does not exist anymore
	/*if fieldValue.IsValid() && fieldValue.CanSet() {
		if err := SetField(structPath, structPointer, "Code", mappedCode, log); err != nil {
			return err
		}
	}*/

	return nil
}

// TODO: check if this function is still needed, probably not because performConceptMapping is used directly
func applyConceptMappingForField(structPath string, structFieldName string, inputValue interface{}, log zerolog.Logger) (interface{}, error) {
	// Construct the FHIR path
	fhirPath := structPath + "." + strings.ToLower(structFieldName)
	fhirPath = extractAndCapitalizeLastTwoParts(fhirPath)
	log.Debug().Msgf("FHIR Path: %s", fhirPath)

	// Convert inputValue to a string for concept mapping
	stringInputValue := getStringValue(reflect.ValueOf(inputValue))

	// Perform concept mapping using the shared function
	mappedCode, _, err := performConceptMapping(fhirPath, stringInputValue, true, log)
	if err != nil {
		return nil, err
	}

	// Return the mapped value
	return mappedCode, nil
}

func prepareForConceptMappingCode(structValue reflect.Value, structPath string, fieldName string, value interface{}, log zerolog.Logger) (string, string, error) {
	// Convert inputValue to a string for concept mapping
	stringInputValue := getStringValue(reflect.ValueOf(value))
	log.Debug().Msgf("Converted inputValue to string: %s", stringInputValue)

	// Determine paths
	valuesetBindingPath := structPath + "." + strings.ToLower(fieldName)
	structValueType := structValue.Type().String()
	valuesetBindingPathAlternative := AlternativePath(structValueType, strings.ToLower(fieldName))

	// Log paths for debugging
	log.Debug().Msgf("Default valueSetBindingPath: %s", valuesetBindingPath)
	log.Debug().Msgf("ValueSetBindingPathAlternative: %s", valuesetBindingPathAlternative)

	// Determine which binding path to use
	if _, exists := FhirPathToValueset[valuesetBindingPath]; exists {
		return valuesetBindingPath, stringInputValue, nil // Return the normal path if it exists
	}

	return valuesetBindingPathAlternative, stringInputValue, nil // Return the alternative path if normal doesn't exist
}

func findColumnIndex(headers []string, name string) int {
	for i, h := range headers {
		if strings.ToLower(strings.TrimSpace(h)) == strings.ToLower(name) {
			return i
		}
	}
	return -1
}

// getStringValue converts a reflect.Value to a string
func getStringValue(value reflect.Value) string {
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}
	if value.Kind() == reflect.String {
		return value.String()
	}
	return ""
}
