package main

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// Part 3: Filter System
func (rp *ProcessorService) checkFilter(path string, field reflect.Value) (*FilterResult, error) {
	param, exists := rp.searchParams[path]
	if !exists {
		return &FilterResult{Passed: true}, nil
	}

	rp.log.Debug().
		Str("path", path).
		Str("fieldType", field.Type().String()).
		Str("fieldKind", field.Kind().String()).
		Str("paramType", param.Type).
		Msg("Checking filter")

	// Handle pointer types
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return &FilterResult{Passed: false}, nil
		}
		field = field.Elem()
	}

	switch param.Type {
	case "token":
		return rp.checkTokenFilter(field, param)
	case "date":
		return rp.checkDateFilter(field, param)
	default:
		return &FilterResult{Passed: true}, nil
	}
}

func (rp *ProcessorService) checkDateFilter(field reflect.Value, param SearchParameter) (*FilterResult, error) {
	rp.log.Debug().
		Str("fieldType", field.Type().String()).
		Msg("Checking date filter")

	if field.Type().String() != "fhir.Date" {
		rp.log.Debug().Msg("Field is not a date")
		return &FilterResult{Passed: false, Message: "field is not a date"}, nil
	}

	dateVal := field.Interface().(fhir.Date)
	filterDate, err := time.Parse("2006-01-02", param.Value)
	if err != nil {
		rp.log.Error().Err(err).Msg("Error parsing filter date")
		return nil, err
	}

	fieldDate := dateVal.Time()
	rp.log.Debug().
		Str("fieldDate", fieldDate.String()).
		Str("filterDate", filterDate.String()).
		Str("comparator", param.Comparator).
		Msg("Comparing dates")

	passed := false
	switch param.Comparator {
	case "eq", "":
		passed = fieldDate.Equal(filterDate)
	case "gt":
		passed = fieldDate.After(filterDate)
	case "lt":
		passed = fieldDate.Before(filterDate)
	case "ge":
		passed = !fieldDate.Before(filterDate)
	case "le":
		passed = !fieldDate.After(filterDate)
	}

	rp.log.Debug().Bool("passed", passed).Msg("Date filter result")
	return &FilterResult{Passed: passed}, nil
}

func (rp *ProcessorService) checkStringFilter(field reflect.Value, param SearchParameter) (*FilterResult, error) {
	if field.Kind() != reflect.String &&
		(field.Kind() != reflect.Ptr || field.Elem().Kind() != reflect.String) {
		return &FilterResult{Passed: false}, nil
	}

	var fieldValue string
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return &FilterResult{Passed: false}, nil
		}
		fieldValue = field.Elem().String()
	} else {
		fieldValue = field.String()
	}

	passed := fieldValue == param.Value
	return &FilterResult{Passed: passed}, nil
}

func parseTokenValue(value string) (string, string) {
	parts := strings.Split(value, "|")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}

// Token filter main entry point
func (rp *ProcessorService) checkTokenFilter(field reflect.Value, param SearchParameter) (*FilterResult, error) {
	if IsValueSetReference(param.Value) {
		return rp.checkValueSetFilter(field, param)
	}

	system, code := parseTokenValue(param.Value)
	return rp.checkTokenWithSystemCode(field, system, code)
}

// Helper function to log available codes in ValueSet
func (rp *ProcessorService) logValueSetContents(validCombinations map[string]map[string]bool) {
	for system, codes := range validCombinations {
		var validCodes []string
		for code := range codes {
			validCodes = append(validCodes, code)
		}
		rp.log.Debug().
			Str("system", system).
			Strs("availableCodes", validCodes).
			Int("codeCount", len(codes)).
			Msg("ValueSet system contents")
	}
}

// Update the checkValueSetFilter to include content logging
func (rp *ProcessorService) checkValueSetFilter(field reflect.Value, param SearchParameter) (*FilterResult, error) {
	valueSetURL := strings.TrimPrefix(param.Value, "ValueSet/")

	rp.log.Debug().
		Str("valueSetURL", valueSetURL).
		Str("fieldType", field.Type().String()).
		Msg("Fetching ValueSet")

	valueSet, err := rp.valueSetCache.GetValueSet(valueSetURL)
	if err != nil {
		rp.log.Error().
			Err(err).
			Str("valueSetURL", valueSetURL).
			Msg("Failed to fetch ValueSet")
		return nil, fmt.Errorf("failed to fetch ValueSet: %w", err)
	}

	// Create a map to store all valid system/code combinations
	validCombinations := make(map[string]map[string]bool)

	// Process all includes in the ValueSet
	for i, include := range valueSet.Compose.Include {
		system := ""
		if include.System != nil {
			system = *include.System
		}

		// Initialize the system map if it doesn't exist
		if _, exists := validCombinations[system]; !exists {
			validCombinations[system] = make(map[string]bool)
		}

		// Add all concepts for this system
		for _, concept := range include.Concept {
			validCombinations[system][concept.Code] = true
		}

		rp.log.Debug().
			Str("system", system).
			Int("conceptCount", len(include.Concept)).
			Int("includeIndex", i).
			Msg("Processed system concepts")
	}

	// Log the contents of the ValueSet
	rp.logValueSetContents(validCombinations)

	if field.Kind() == reflect.Slice {
		rp.log.Debug().Msg("Checking slice against ValueSet")
		return rp.checkSliceAgainstValueSet(field, validCombinations)
	}

	rp.log.Debug().Msg("Checking single value against ValueSet")
	return rp.checkSingleAgainstValueSet(field, validCombinations)
}

// Also update the checking functions with more logging:

func (rp *ProcessorService) checkSingleAgainstValueSet(field reflect.Value, validCombinations map[string]map[string]bool) (*FilterResult, error) {
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			rp.log.Debug().Msg("Field is nil pointer")
			return &FilterResult{Passed: false}, nil
		}
		field = field.Elem()
	}

	fieldType := field.Type().String()
	rp.log.Debug().
		Str("fieldType", fieldType).
		Msg("Checking field against ValueSet")

	switch fieldType {
	case "fhir.CodeableConcept":
		codings := field.FieldByName("Coding")
		if !codings.IsValid() {
			rp.log.Debug().Msg("Invalid Coding field in CodeableConcept")
			return &FilterResult{Passed: false}, nil
		}
		rp.log.Debug().Msg("Checking CodeableConcept codings")
		return rp.checkSliceAgainstValueSet(codings, validCombinations)

	case "fhir.Coding":
		var system, code string
		if systemField := field.FieldByName("System"); systemField.IsValid() && !systemField.IsNil() {
			system = systemField.Elem().String()
		}
		if codeField := field.FieldByName("Code"); codeField.IsValid() && !codeField.IsNil() {
			code = codeField.Elem().String()
		}

		// If no code is present, can't match
		if code == "" {
			rp.log.Debug().Msg("No code present in Coding")
			return &FilterResult{Passed: false}, nil
		}

		// If system is empty, check if the code exists in any system
		if system == "" {
			rp.log.Debug().
				Str("code", code).
				Msg("Checking code across all systems in ValueSet")

			// Check each system for the code
			for sys, codes := range validCombinations {
				if codes[code] {
					rp.log.Debug().
						Str("matchedSystem", sys).
						Str("code", code).
						Msg("Found matching code in ValueSet (system-agnostic match)")
					return &FilterResult{Passed: true}, nil
				}
			}

			rp.log.Debug().
				Str("code", code).
				Msg("No matching code found in any system in ValueSet")
			return &FilterResult{Passed: false}, nil
		}

		// If system is specified, check only that system
		if systemCodes, exists := validCombinations[system]; exists {
			if systemCodes[code] {
				rp.log.Debug().
					Str("system", system).
					Str("code", code).
					Msg("Found matching code in specified system")
				return &FilterResult{Passed: true}, nil
			}
		}

		rp.log.Debug().
			Str("system", system).
			Str("code", code).
			Interface("availableSystems", validCombinations).
			Msg("No matching code found in specified system")
		return &FilterResult{Passed: false}, nil

	default:
		rp.log.Debug().
			Str("fieldType", fieldType).
			Msg("Unsupported field type for ValueSet matching")
		return &FilterResult{Passed: false}, nil
	}
}

func (rp *ProcessorService) checkSliceAgainstValueSet(slice reflect.Value, validCombinations map[string]map[string]bool) (*FilterResult, error) {
	if slice.Len() == 0 {
		rp.log.Debug().Msg("Empty slice, no values to check against ValueSet")
		return &FilterResult{Passed: false}, nil
	}

	rp.log.Debug().
		Int("sliceLength", slice.Len()).
		Msg("Checking slice elements against ValueSet")

	for i := 0; i < slice.Len(); i++ {
		element := slice.Index(i)
		rp.log.Debug().
			Int("elementIndex", i).
			Str("elementType", element.Type().String()).
			Msg("Checking slice element")

		if result, err := rp.checkSingleAgainstValueSet(element, validCombinations); err != nil {
			return nil, err
		} else if result.Passed {
			rp.log.Debug().
				Int("elementIndex", i).
				Msg("Found matching element in slice")
			return &FilterResult{Passed: true}, nil
		}
	}

	rp.log.Debug().Msg("No matching elements found in slice")
	return &FilterResult{Passed: false}, nil
}

// Add this helper function for more detailed logging
func (rp *ProcessorService) logValueSetMatch(coding reflect.Value, validCombinations map[string]map[string]bool) {
	var system, code string
	if systemField := coding.FieldByName("System"); systemField.IsValid() && !systemField.IsNil() {
		system = systemField.Elem().String()
	}
	if codeField := coding.FieldByName("Code"); codeField.IsValid() && !codeField.IsNil() {
		code = codeField.Elem().String()
	}

	// Log all available systems in the ValueSet
	var systems []string
	for sys := range validCombinations {
		systems = append(systems, sys)
	}

	rp.log.Debug().
		Str("checkingSystem", system).
		Str("checkingCode", code).
		Strs("availableSystems", systems).
		Msg("Checking coding against ValueSet systems")
}

func (rp *ProcessorService) checkTokenWithSystemCode(field reflect.Value, system, code string) (*FilterResult, error) {
	rp.log.Debug().
		Str("fieldType", field.Type().String()).
		Str("system", system).
		Str("code", code).
		Msg("Checking token filter")

	if field.Kind() == reflect.Slice {
		return rp.checkSliceToken(field, system, code)
	}
	return rp.checkSingleToken(field, system, code)
}

func (rp *ProcessorService) checkSliceToken(slice reflect.Value, system, code string) (*FilterResult, error) {
	// Empty slice never matches
	if slice.Len() == 0 {
		return &FilterResult{Passed: false}, nil
	}

	// Check each element in the slice
	for i := 0; i < slice.Len(); i++ {
		element := slice.Index(i)
		if result, err := rp.checkSingleToken(element, system, code); err != nil {
			return nil, err
		} else if result.Passed {
			return &FilterResult{Passed: true}, nil
		}
	}

	return &FilterResult{Passed: false}, nil
}

func (rp *ProcessorService) checkSingleToken(field reflect.Value, system, code string) (*FilterResult, error) {
	// Get the field type, handling pointers
	fieldType := field.Type().String()
	if strings.HasPrefix(fieldType, "*") {
		if field.IsNil() {
			return &FilterResult{Passed: false}, nil
		}
		field = field.Elem()
		fieldType = field.Type().String()
	}

	// TODO: check if it works also for *fhir.CodeableConcept
	switch fieldType {
	case "fhir.CodeableConcept":
		codings := field.FieldByName("Coding")
		if !codings.IsValid() {
			return &FilterResult{Passed: false}, nil
		}
		// Recursively check the slice of codings
		return rp.checkSliceToken(codings, system, code)

	case "fhir.Coding":
		return &FilterResult{Passed: rp.matchesCoding(field, system, code)}, nil

	case "fhir.Identifier":
		return rp.checkIdentifierFilter(field, system, code)

	default:
		// Handle simple string comparison
		if field.Kind() == reflect.String {
			return &FilterResult{Passed: field.String() == code}, nil
		}
		return &FilterResult{Passed: false}, nil
	}
}

func (rp *ProcessorService) matchesCoding(coding reflect.Value, system, code string) bool {
	var codingSystem, codingCode string

	if systemField := coding.FieldByName("System"); systemField.IsValid() && !systemField.IsNil() {
		codingSystem = systemField.Elem().String()
	}

	if codeField := coding.FieldByName("Code"); codeField.IsValid() && !codeField.IsNil() {
		codingCode = codeField.Elem().String()
	}

	matches := (system == "" || codingSystem == system) && codingCode == code

	// Add logging
	rp.log.Debug().
		Str("codingSystem", codingSystem).
		Str("codingCode", codingCode).
		Str("filterSystem", system).
		Str("filterCode", code).
		Bool("matches", matches).
		Msg("Coding match result")

	return matches
}

// Helper function to check if a single element matches token criteria
func (rp *ProcessorService) matchesToken(field reflect.Value, system, code string) bool {
	rp.log.Debug().
		Str("fieldType", field.Type().String()).
		Str("fieldKind", field.Kind().String()).
		Str("system", system).
		Str("code", code).
		Msg("Checking token filter")

	// Handle Coding type
	if field.Type().String() == "fhir.Coding" {
		var coding *fhir.Coding
		if field.Kind() == reflect.Ptr {
			if field.IsNil() {
				rp.log.Debug().Msg("Field is nil pointer")
				return false
			}
			coding = field.Interface().(*fhir.Coding)
		} else {
			coding = field.Addr().Interface().(*fhir.Coding)
		}

		var codingSystem, codingCode string
		if coding.System != nil {
			codingSystem = *coding.System
		}
		if coding.Code != nil {
			codingCode = *coding.Code
		}

		matches := (system == "" || codingSystem == system) && codingCode == code
		rp.log.Debug().
			Str("codingSystem", codingSystem).
			Str("codingCode", codingCode).
			Bool("matches", matches).
			Msg("Coding type match result")
		return matches
	}

	// Handle string type
	if field.Kind() == reflect.String {
		matches := field.String() == code
		rp.log.Debug().
			Str("fieldValue", field.String()).
			Bool("matches", matches).
			Msg("String type match result")
		return matches
	}

	rp.log.Debug().Msg("Unsupported field type for token filter")
	return false
}

func (rp *ProcessorService) checkIdentifierFilter(field reflect.Value, system, code string) (*FilterResult, error) {
	var identifier *fhir.Identifier

	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return &FilterResult{Passed: false}, nil
		}
		identifier = field.Interface().(*fhir.Identifier)
	} else {
		identifier = field.Addr().Interface().(*fhir.Identifier)
	}

	// Get system and value, handling nil cases
	var identifierSystem, identifierValue string
	if identifier.System != nil {
		identifierSystem = *identifier.System
	}
	if identifier.Value != nil {
		identifierValue = *identifier.Value
	}

	// Match logic:
	// - If system is provided, both system and code must match
	// - If only code is provided, only value needs to match
	matches := (system == "" || identifierSystem == system) &&
		identifierValue == code

	rp.log.Debug().
		Str("fieldSystem", identifierSystem).
		Str("fieldValue", identifierValue).
		Str("filterSystem", system).
		Str("filterValue", code).
		Bool("matches", matches).
		Msg("Comparing identifier")

	return &FilterResult{Passed: matches}, nil
}

func (rp *ProcessorService) checkCodingFilter(field reflect.Value, system, code string) (*FilterResult, error) {
	if field.IsNil() {
		return &FilterResult{Passed: false}, nil
	}

	coding := field.Interface().(*fhir.Coding)

	var codingSystem, codingCode string
	if coding.System != nil {
		codingSystem = *coding.System
	}
	if coding.Code != nil {
		codingCode = *coding.Code
	}

	matches := (system == "" || codingSystem == system) &&
		codingCode == code

	return &FilterResult{Passed: matches}, nil
}

func IsValueSetReference(value string) bool {
	if strings.Contains(value, "ValueSet/") {
		return true
	}
	return false
}
