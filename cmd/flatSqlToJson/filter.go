package main

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

func FilterField(field reflect.Value, sg SearchFilterGroup, fullFieldName string) error {
	if searchFilter, ok := sg[fullFieldName]; ok {
		log.Debug().Str("field", fullFieldName).Interface("searchFilter", searchFilter).Msg("Filtering field")

		// Check if the field is set (non-zero value)
		if field.IsZero() {
			log.Debug().Str("field", fullFieldName).Msg("Field is already zero value, skipping filtering")
			return nil
		}

		switch searchFilter.Type {
		case "token":
			return filterTokenField(field, searchFilter, fullFieldName)
		case "date":
			return filterDateField(field, searchFilter, fullFieldName)
		default:
			log.Warn().Str("field", fullFieldName).Str("type", searchFilter.Type).Msg("Unsupported filter type")
		}
	}
	return nil
}

func filterTokenField(field reflect.Value, searchFilter SearchFilter, fullFieldName string) error {
	system, code := parseFilter(searchFilter.Value)
	log.Debug().Str("field", fullFieldName).Str("system", system).Str("code", code).Msg("Filtering token field")

	switch field.Type().Name() {
	case "Identifier":
		if !fieldMatchesIdentifierFilter(field, system, code) {
			log.Debug().Str("field", fullFieldName).Msg("Field does not match Identifier filter, setting to zero value")
			setFieldToZeroIfNotEmpty(field)
		}
	case "CodeableConcept":
		if !fieldMatchesCodeableConceptFilter(field, system, code) {
			log.Debug().Str("field", fullFieldName).Msg("Field does not match CodeableConcept filter, setting to zero value")
			setFieldToZeroIfNotEmpty(field)
		}
	case "Coding":
		if !fieldMatchesCodingFilter(field, system, code) {
			log.Debug().Str("field", fullFieldName).Msg("Field does not match Coding filter, setting to zero value")
			setFieldToZeroIfNotEmpty(field)
		}
	default:
		log.Warn().Str("field", fullFieldName).Str("type", field.Type().Name()).Msg("Unsupported token field type")
	}

	return nil
}

func filterDateField(field reflect.Value, searchFilter SearchFilter, fullFieldName string) error {
	// Parse the filter date (format: YYYYMMDD)
	filterDate, err := time.Parse("20060102", searchFilter.Value)
	if err != nil {
		return fmt.Errorf("invalid date format for field %s: %v", fullFieldName, err)
	}

	fieldTime, err := getTimeFromField(field)
	if err != nil {
		return fmt.Errorf("error getting time from field %s: %v", fullFieldName, err)
	}

	// Compare dates ignoring the time component
	filterDate = time.Date(filterDate.Year(), filterDate.Month(), filterDate.Day(), 0, 0, 0, 0, time.UTC)
	fieldDate := time.Date(fieldTime.Year(), fieldTime.Month(), fieldTime.Day(), 0, 0, 0, 0, time.UTC)

	switch searchFilter.Comparator {
	case "eq", "":
		if !fieldDate.Equal(filterDate) {
			setFieldToZeroIfNotEmpty(field)
		}
	case "gt":
		if !fieldDate.After(filterDate) {
			setFieldToZeroIfNotEmpty(field)
		}
	case "lt":
		if !fieldDate.Before(filterDate) {
			setFieldToZeroIfNotEmpty(field)
		}
	case "ge":
		if fieldDate.Before(filterDate) {
			setFieldToZeroIfNotEmpty(field)
		}
	case "le":
		if fieldDate.After(filterDate) {
			setFieldToZeroIfNotEmpty(field)
		}
	default:
		return fmt.Errorf("unsupported date comparator for field %s: %s", fullFieldName, searchFilter.Comparator)
	}

	return nil
}

func setFieldToZeroIfNotEmpty(field reflect.Value) {
	if !field.IsZero() {
		field.Set(reflect.Zero(field.Type()))
	}
}

func getTimeFromField(field reflect.Value) (time.Time, error) {

	// Parse the FHIR date string format
	dateString := *field.Interface().(*string)
	return time.Parse(time.RFC3339, dateString)

}

func fieldMatchesCodeableConceptFilter(field reflect.Value, system, code string) bool {
	coding := field.FieldByName("Coding")
	if !coding.IsValid() || coding.IsNil() {
		return false
	}

	for i := 0; i < coding.Len(); i++ {
		if matchesCodingValues(coding.Index(i), system, code) {
			return true
		}
	}
	return false
}

func fieldMatchesCodingFilter(field reflect.Value, system, code string) bool {
	return matchesCodingValues(field, system, code)
}

func matchesCodingValues(v reflect.Value, system, code string) bool {
	systemField := v.FieldByName("System")
	codeField := v.FieldByName("Code")

	if !systemField.IsValid() || !codeField.IsValid() {
		return false
	}

	if systemField.Kind() == reflect.Ptr {
		if systemField.IsNil() {
			return false
		}
		systemField = systemField.Elem()
	}

	if codeField.Kind() == reflect.Ptr {
		if codeField.IsNil() {
			return false
		}
		codeField = codeField.Elem()
	}

	log.Debug().Str("system", systemField.String()).Str("code", codeField.String()).Msg("Matching coding values")

	return (system == "" || systemField.String() == system) && codeField.String() == code
}

func fieldMatchesIdentifierFilter(field reflect.Value, system, code string) bool {
	if field.Kind() == reflect.Slice {
		for i := 0; i < field.Len(); i++ {
			if matchesIdentifierFilter(field.Index(i), system, code) {
				return true
			}
		}
	} else if field.Kind() == reflect.Struct {
		return matchesIdentifierFilter(field, system, code)
	}
	return false
}

func matchesIdentifierFilter(v reflect.Value, system, code string) bool {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return false
	}

	identifierField := v.FieldByName("Identifier")
	if identifierField.IsValid() && identifierField.Kind() == reflect.Slice {
		for i := 0; i < identifierField.Len(); i++ {
			coding := identifierField.Index(i)
			if matchesIndentifierValues(coding, system, code) {
				return true
			}
		}
	} else {
		return matchesIndentifierValues(v, system, code)
	}

	return false
}
func matchesIndentifierValues(v reflect.Value, system, code string) bool {
	systemField := v.FieldByName("System")
	codeField := v.FieldByName("Value")

	if !systemField.IsValid() || !codeField.IsValid() {
		return false
	}

	if systemField.Kind() == reflect.Ptr {
		if systemField.IsNil() {
			return false
		}
		systemField = systemField.Elem()
	}

	if codeField.Kind() == reflect.Ptr {
		if codeField.IsNil() {
			return false
		}
		codeField = codeField.Elem()
	}

	log.Debug().Str("system", systemField.String()).Str("code", codeField.String()).Msg("Matching identifier values")

	return (system == "" || systemField.String() == system) && codeField.String() == code
}

func parseFilter(filter string) (string, string) {
	parts := strings.Split(filter, "|")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", parts[0]
}
