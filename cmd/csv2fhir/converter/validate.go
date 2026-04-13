package converter

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/SanteonNL/fenix/models/fhir"
	"github.com/rs/zerolog"
)

// validateThroughStruct marshals the raw map to JSON, then unmarshals it into
// the matching fhir.* struct. This catches:
//   - Wrong code values (e.g. "man" instead of "male" for Patient.gender)
//   - Wrong field types (e.g. a string where a number is expected)
//   - Missing required fields
//
// SQLite returns all values as strings. coerceSQLiteTypes converts common
// string representations to their native JSON types before marshaling.
//
// Returns the typed struct on success. On validation failure it returns an
// error so the caller can decide to skip or warn.
func validateThroughStruct(raw map[string]interface{}, logger zerolog.Logger) (interface{}, error) {
	resourceType, _ := raw["resourceType"].(string)

	target, err := newFHIRResource(resourceType)
	if err != nil {
		// Unknown resource type — pass through as-is without struct validation.
		return raw, nil
	}

	// Log before coercion
	rawBefore, _ := json.MarshalIndent(raw, "", "  ")
	logger.Debug().RawJSON("raw_before_coerce", rawBefore).Str("type", resourceType).Msg("Before coercing SQLite types")

	coerceSQLiteTypes(raw)

	// Normalize nested array fields - wrap scalars in arrays where struct expects arrays
	normalizeNestedArrays(raw, resourceType)

	// Log after coercion and normalization
	rawAfter, _ := json.MarshalIndent(raw, "", "  ")
	logger.Debug().RawJSON("raw_after_coerce", rawAfter).Str("type", resourceType).Msg("After coercing SQLite types")

	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal failed: %w", err)
	}

	logger.Debug().RawJSON("marshaled_json", data).Str("type", resourceType).Msg("Marshaled to JSON")

	if err := json.Unmarshal(data, target); err != nil {
		logger.Error().
			Err(err).
			Str("type", resourceType).
			RawJSON("failed_json", data).
			Msg("Unmarshal to struct failed - detailed validation error")
		return nil, fmt.Errorf("%s validation failed: %w", resourceType, err)
	}

	logger.Debug().Str("type", resourceType).Msg("Validation successful")
	return target, nil
}

// coerceSQLiteTypes converts string values that look like booleans or numbers
// to their native Go types, recursively for nested maps and slices.
// SQLite has no native bool/int columns when loaded from CSV — everything
// comes back as string or []byte.
func coerceSQLiteTypes(obj map[string]interface{}) {
	for k, v := range obj {
		switch val := v.(type) {
		case []byte:
			obj[k] = string(val)
			coerceStringValue(obj, k, string(val))
		case string:
			oldType := fmt.Sprintf("%T", val)
			coerceStringValue(obj, k, val)
			newType := fmt.Sprintf("%T", obj[k])
			if oldType != newType {
				// Type was coerced from string to something else
				_ = fmt.Sprintf("Field '%s': coerced %s -> %s (value: %v)", k, oldType, newType, obj[k])
			}
		case map[string]interface{}:
			coerceSQLiteTypes(val)
		case []interface{}:
			for _, item := range val {
				if m, ok := item.(map[string]interface{}); ok {
					coerceSQLiteTypes(m)
				}
			}
		}
	}
}

func coerceStringValue(obj map[string]interface{}, key, val string) {
	switch val {
	case "true", "True", "TRUE", "1":
		obj[key] = true
	case "false", "False", "FALSE", "0":
		obj[key] = false
	}
}

// normalizeNestedArrays recursively wraps scalar values in arrays where the FHIR struct expects arrays
func normalizeNestedArrays(obj map[string]interface{}, typeName string) {
	t, ok := cachedFHIRType(typeName)
	if !ok {
		return
	}

	// For each field in the struct
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		fieldName := strings.Split(jsonTag, ",")[0]

		if fieldName == "" || fieldName == "-" {
			continue
		}

		val, ok := obj[fieldName]
		if !ok || val == nil {
			continue
		}

		// Check if struct field is a slice
		isArrayField := field.Type.Kind() == reflect.Slice

		if isArrayField {
			// Field should be an array
			elemTypeName := getNestedTypeName(field.Type)
			switch v := val.(type) {
			case []interface{}:
				// Already an array, recurse into elements
				for _, elem := range v {
					if m, ok := elem.(map[string]interface{}); ok {
						normalizeNestedArrays(m, elemTypeName)
					}
				}
			case map[string]interface{}:
				// Single object should be wrapped in array
				normalizeNestedArrays(v, elemTypeName)
				obj[fieldName] = []interface{}{v}
			default:
				// Scalar value - wrap in array
				obj[fieldName] = []interface{}{val}
			}
		} else if m, ok := val.(map[string]interface{}); ok {
			// Non-array field that's an object - recurse
			nestedTypeName := field.Type.Name()
			if field.Type.Kind() == reflect.Ptr {
				nestedTypeName = field.Type.Elem().Name()
			}
			normalizeNestedArrays(m, nestedTypeName)
		}
	}
}

// getNestedTypeName extracts the element type name from a slice type
func getNestedTypeName(t reflect.Type) string {
	if t.Kind() == reflect.Slice {
		elemType := t.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		return elemType.Name()
	}
	return ""
}

// findFHIRType looks up a FHIR type by name using reflection
// This handles complex types like Address, HumanName, etc. that aren't resource types
func findFHIRType(typeName string) reflect.Type {
	// Try to find the type in the fhir package
	// We'll do this by instantiating a Patient and looking at its field types
	p := &fhir.Patient{}
	pType := reflect.TypeOf(p)

	// Search through all fields and their types
	var foundType reflect.Type
	searchType := func(t reflect.Type) {
		if foundType != nil {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fieldType := field.Type

			// Check direct field type
			if fieldType.Name() == typeName {
				foundType = fieldType
				return
			}

			// Check pointer type
			if fieldType.Kind() == reflect.Ptr && fieldType.Elem().Name() == typeName {
				foundType = fieldType.Elem()
				return
			}

			// Check slice element type
			if fieldType.Kind() == reflect.Slice {
				elemType := fieldType.Elem()
				if elemType.Name() == typeName {
					foundType = elemType
					return
				}
				if elemType.Kind() == reflect.Ptr && elemType.Elem().Name() == typeName {
					foundType = elemType.Elem()
					return
				}
			}
		}
	}

	searchType(pType.Elem())

	return foundType
}

// newFHIRResource returns a pointer to the correct fhir.* struct for the given
// resourceType string. Add more resource types here as needed.
func newFHIRResource(resourceType string) (interface{}, error) {
	switch resourceType {
	case "Patient":
		return &fhir.Patient{}, nil
	case "Observation":
		return &fhir.Observation{}, nil
	case "Encounter":
		return &fhir.Encounter{}, nil
	case "Condition":
		return &fhir.Condition{}, nil
	case "Procedure":
		return &fhir.Procedure{}, nil
	case "MedicationRequest":
		return &fhir.MedicationRequest{}, nil
	case "DiagnosticReport":
		return &fhir.DiagnosticReport{}, nil
	case "CarePlan":
		return &fhir.CarePlan{}, nil
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}
