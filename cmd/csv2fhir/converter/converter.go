package converter

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// RowData represents one SQL row at a given FHIR path level.
// Multiple rows at the same fhir_path + parent_id become FHIR array elements.
type RowData struct {
	ID       string
	ParentID string
	Data     map[string]interface{} // leaf field name → value (no [n] notation)
}

// ResourceResult groups RowData by FHIR path (e.g. "Patient", "Patient.name", "Patient.name.coding")
type ResourceResult map[string][]RowData

// FHIRConverter reads SQL rows and converts them to FHIR resources.
//
// SQL row format (columns):
//
//	resource_id  – identifier of the root resource (e.g. patient id)
//	id           – identifier of this specific row
//	parent_id    – id of the parent row (empty string for root rows)
//	fhir_path    – FHIR path at this level, e.g. "Patient", "Patient.name", "Patient.name.coding"
//	<field>      – any other column is a leaf field value at this path level.
//	               Dot-notation is allowed for simple scalar nesting (e.g. "subject.reference")
//	               Arrays are created by providing multiple rows with the same fhir_path + parent_id.
type FHIRConverter struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

// NewFHIRConverter creates a new converter.
func NewFHIRConverter(db *sqlx.DB, logger zerolog.Logger) *FHIRConverter {
	return &FHIRConverter{db: db, logger: logger}
}

// ConvertSQL executes a SQL string that may contain multiple statements separated by ";".
// Each statement should return rows with: resource_id, id, parent_id, fhir_path, + field columns.
// Multiple rows at the same fhir_path + parent_id become array elements in the FHIR resource.
// Resources that fail FHIR struct validation are logged and skipped.
func (fc *FHIRConverter) ConvertSQL(query string) ([]interface{}, error) {
	fc.logger.Info().Msg("Converting SQL to FHIR")

	resources := make(map[string]ResourceResult)
	rootPaths := make(map[string]string)

	for i, stmt := range splitStatements(query) {
		if err := fc.executeStatement(stmt, i+1, resources, rootPaths); err != nil {
			fc.logger.Error().Err(err).Int("statement", i+1).Msg("Statement failed, continuing")
		}
	}

	return fc.buildResources(resources, rootPaths), nil
}

// executeStatement runs one SQL statement and feeds its rows into resources.
func (fc *FHIRConverter) executeStatement(stmt string, n int, resources map[string]ResourceResult, rootPaths map[string]string) error {
	rows, err := fc.db.Queryx(stmt)
	if err != nil {
		return fmt.Errorf("statement %d: %w", n, err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		raw := make(map[string]interface{})
		if err := rows.MapScan(raw); err != nil {
			fc.logger.Error().Err(err).Msg("Row scan failed")
			continue
		}
		processRow(raw, resources, rootPaths)
		count++
	}
	fc.logger.Debug().Int("statement", n).Int("rows", count).Msg("Statement executed")
	return rows.Err()
}

// buildResources converts the grouped ResourceResult map into validated FHIR structs.
// Each resource is first built as a map, then validated by unmarshaling into the
// matching fhir.* struct. Invalid resources are logged and skipped.
func (fc *FHIRConverter) buildResources(resources map[string]ResourceResult, rootPaths map[string]string) []interface{} {
	var result []interface{}
	for resourceID, resResult := range resources {
		rootPath, ok := rootPaths[resourceID]
		if !ok {
			fc.logger.Warn().Str("resourceID", resourceID).Msg("No root fhir_path, skipping")
			continue
		}

		raw, err := buildFHIRResource(resResult, rootPath)
		if err != nil {
			fc.logger.Error().Err(err).Str("resourceID", resourceID).Msg("Build failed")
			continue
		}

		// Debug logging: show the raw data structure before validation
		rawJSON, _ := json.MarshalIndent(raw, "", "  ")
		fc.logger.Debug().RawJSON("raw", rawJSON).Str("resourceID", resourceID).Msg("Built FHIR resource")

		// Validate by round-tripping through the typed fhir.* struct.
		// This catches wrong codes, wrong types, and missing required fields.
		validated, err := validateThroughStruct(raw, fc.logger)
		if err != nil {
			fc.logger.Warn().
				Err(err).
				Str("resourceID", resourceID).
				Str("resourceType", rootPath).
				RawJSON("raw", rawJSON).
				Msg("FHIR validation failed — resource skipped")
			continue
		}

		result = append(result, validated)
	}
	fc.logger.Info().Int("resources", len(result)).Msg("Conversion completed")
	return result
}

// splitStatements splits a SQL string on ";" into individual non-empty statements.
// Lines that consist only of comments are skipped.
func splitStatements(sql string) []string {
	var statements []string
	for _, raw := range strings.Split(sql, ";") {
		var lines []string
		for _, line := range strings.Split(raw, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
				lines = append(lines, line)
			}
		}
		stmt := strings.TrimSpace(strings.Join(lines, "\n"))
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}
	return statements
}

// processRow places one raw SQL row into the correct ResourceResult bucket.
func processRow(raw map[string]interface{}, resources map[string]ResourceResult, rootPaths map[string]string) {
	resourceID := toString(raw["resource_id"])
	id := toString(raw["id"])
	parentID := toString(raw["parent_id"])
	fhirPath := toString(raw["fhir_path"])

	if resourceID == "" || fhirPath == "" {
		return
	}

	if resources[resourceID] == nil {
		resources[resourceID] = make(ResourceResult)
	}

	// The root path has no dot and no existing root registered
	if !strings.Contains(fhirPath, ".") && rootPaths[resourceID] == "" {
		rootPaths[resourceID] = fhirPath
	}

	// Collect all data columns (skip the four metadata columns)
	data := make(map[string]interface{})
	for k, v := range raw {
		lower := strings.ToLower(k)
		if lower == "resource_id" || lower == "parent_id" || lower == "id" || lower == "fhir_path" {
			continue
		}
		if v != nil {
			data[k] = v
		}
	}

	resources[resourceID][fhirPath] = append(
		resources[resourceID][fhirPath],
		RowData{ID: id, ParentID: parentID, Data: data},
	)
}

// buildFHIRResource converts a ResourceResult into a nested FHIR map.
func buildFHIRResource(result ResourceResult, rootPath string) (map[string]interface{}, error) {
	rootRows := result[rootPath]
	if len(rootRows) == 0 {
		return nil, fmt.Errorf("no root rows for path %s", rootPath)
	}

	resource := make(map[string]interface{})
	resource["resourceType"] = rootPath // e.g. "Patient"

	rootRow := rootRows[0]

	// Set leaf fields from root row; support dot-notation for simple scalar nesting
	for k, v := range rootRow.Data {
		setNestedValue(resource, k, v)
	}

	// Build array field map from the FHIR struct definition
	arrayFields := getArrayFieldsForType(rootPath)

	// Recursively add child paths
	setChildren(resource, result, rootPath, rootRow.ID, arrayFields)

	return resource, nil
}

// setChildren finds all direct child paths of parentPath and populates them.
// Multiple RowData at the same child path with the same parentID become a FHIR array.
// Fields that are defined as array types in the FHIR struct are wrapped in arrays
// even when there's only one element.
func setChildren(parent map[string]interface{}, result ResourceResult, parentPath string, parentID string, arrayFields map[string]bool) {
	for path := range result {
		if !isDirectChild(parentPath, path) {
			continue
		}
		fieldName := path[len(parentPath)+1:] // e.g. "name" from "Patient.name"

		// Collect rows that belong to this parent
		var matching []RowData
		for _, row := range result[path] {
			if row.ParentID == parentID {
				matching = append(matching, row)
			}
		}
		if len(matching) == 0 {
			continue
		}

		// Check if this field should be an array based on the struct definition
		shouldBeArray := arrayFields[fieldName]

		if len(matching) == 1 && !shouldBeArray {
			// Single element and not defined as array in struct → store as single object
			parent[fieldName] = buildChild(matching[0], result, path, arrayFields)
		} else {
			// Multiple elements OR field is defined as array in struct → store as array
			arr := make([]interface{}, len(matching))
			for i, row := range matching {
				arr[i] = buildChild(row, result, path, arrayFields)
			}
			parent[fieldName] = arr
		}
	}
}

// buildChild converts one RowData into either a scalar, or a nested map with children.
func buildChild(row RowData, result ResourceResult, path string, arrayFields map[string]bool) interface{} {
	// Check whether this path has sub-paths
	hasChildren := false
	for p := range result {
		if isDirectChild(path, p) {
			hasChildren = true
			break
		}
	}

	// Single scalar field, no children → return bare value
	if !hasChildren && len(row.Data) == 1 {
		for _, v := range row.Data {
			return v
		}
	}

	obj := make(map[string]interface{})
	for k, v := range row.Data {
		setNestedValue(obj, k, v) // support dot-notation in data columns
	}

	// Get array fields for the nested type (e.g., if path is "Patient.name", get HumanName array fields)
	nestedArrayFields := getArrayFieldsForType(getTypeNameFromPath(path))
	setChildren(obj, result, path, row.ID, nestedArrayFields)

	// Wrap scalar values in arrays where the struct field expects an array
	normalizeObjectArrayFields(obj, getTypeNameFromPath(path))

	return obj
}

// setNestedValue assigns a value at a dot-separated path inside a map.
// e.g. setNestedValue(m, "subject.reference", "Patient/123")
// produces m["subject"]["reference"] = "Patient/123"
func setNestedValue(obj map[string]interface{}, dotPath string, value interface{}) {
	parts := strings.SplitN(dotPath, ".", 2)
	if len(parts) == 1 {
		obj[dotPath] = value
		return
	}
	key := parts[0]
	child, ok := obj[key].(map[string]interface{})
	if !ok {
		child = make(map[string]interface{})
		obj[key] = child
	}
	setNestedValue(child, parts[1], value)
}

// isDirectChild returns true when child is exactly one level below parent.
func isDirectChild(parent, child string) bool {
	prefix := parent + "."
	if !strings.HasPrefix(child, prefix) {
		return false
	}
	rest := child[len(prefix):]
	return !strings.Contains(rest, ".")
}

// toString converts an interface{} to string, returning "" for nil.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}

// ExportToNDJSON marshals resources to newline-delimited JSON.
func ExportToNDJSON(resources []interface{}) ([]byte, error) {
	var out []byte
	for _, r := range resources {
		b, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
		out = append(out, '\n')
	}
	return out, nil
}

// ExportToJSON marshals resources to a pretty-printed JSON array.
func ExportToJSON(resources []interface{}) ([]byte, error) {
	return json.MarshalIndent(resources, "", "  ")
}

// getArrayFieldsForType inspects the FHIR struct and returns which fields are arrays
// by looking at the struct definition and JSON tags
func getArrayFieldsForType(resourceType string) map[string]bool {
	arrayFields := make(map[string]bool)

	// Get the struct type from validate.go's newFHIRResource
	target, err := newFHIRResource(resourceType)
	if err != nil {
		// Unknown type - return empty map
		return arrayFields
	}

	// Get the reflection type
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Iterate through struct fields
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Check if field is a slice/array
		if field.Type.Kind() == reflect.Slice {
			// Get the JSON tag name
			jsonTag := field.Tag.Get("json")
			if jsonTag != "" {
				// Extract the field name (before comma)
				fieldName := strings.Split(jsonTag, ",")[0]
				if fieldName != "" && fieldName != "-" {
					arrayFields[fieldName] = true
				}
			}
		}
	}

	return arrayFields
}

// getTypeNameFromPath extracts the type name from a FHIR path by inspecting struct definitions
// e.g., "Patient.name" -> "HumanName", "Patient.address" -> "Address"
func getTypeNameFromPath(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return ""
	}

	// Get the resource type (first part)
	resourceType := parts[0]

	// Get the field name (last part)
	fieldName := parts[len(parts)-1]

	// Use reflection to find the actual type of this field
	target, err := newFHIRResource(resourceType)
	if err != nil {
		return ""
	}

	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Find the struct field
	field, ok := t.FieldByNameFunc(func(name string) bool {
		// Match JSON tag to field name
		if f, found := t.FieldByName(name); found {
			jsonTag := f.Tag.Get("json")
			tagName := strings.Split(jsonTag, ",")[0]
			return tagName == fieldName
		}
		return false
	})

	if ok && field.Type.Kind() == reflect.Slice {
		// Get the element type name
		elemType := field.Type.Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		return elemType.Name()
	}

	// Fallback: capitalize the field name
	if len(fieldName) > 0 {
		return strings.ToUpper(fieldName[:1]) + fieldName[1:]
	}
	return ""
}

// normalizeObjectArrayFields wraps scalar values in arrays where the struct definition expects arrays
// This handles nested types like Address.line which should be []string but comes in as string
func normalizeObjectArrayFields(obj map[string]interface{}, typeName string) {
	// Get array fields for this type
	arrayFields := getArrayFieldsForType(typeName)

	// Wrap scalar values in arrays
	for fieldName, shouldBeArray := range arrayFields {
		if !shouldBeArray {
			continue
		}

		val, ok := obj[fieldName]
		if !ok || val == nil {
			continue
		}

		// Check if it's already an array
		if _, isArray := val.([]interface{}); isArray {
			// Already an array, recurse into elements
			for _, elem := range val.([]interface{}) {
				if m, ok := elem.(map[string]interface{}); ok {
					// Recursively normalize nested objects
					normalizeObjectArrayFields(m, getTypeNameForField(typeName, fieldName))
				}
			}
			continue
		}

		// Scalar value - wrap in array
		if _, isMap := val.(map[string]interface{}); isMap {
			// It's an object, wrap it
			obj[fieldName] = []interface{}{val}
			// Recursively normalize the nested object
			if m, ok := val.(map[string]interface{}); ok {
				normalizeObjectArrayFields(m, getTypeNameForField(typeName, fieldName))
			}
		} else {
			// Scalar (string, number, bool) - wrap in array
			obj[fieldName] = []interface{}{val}
		}
	}

	// Recursively process nested objects
	for k, v := range obj {
		if m, ok := v.(map[string]interface{}); ok {
			// This is a nested object - determine its type and normalize it
			nestedType := getTypeNameForField(typeName, k)
			if nestedType != "" {
				normalizeObjectArrayFields(m, nestedType)
			}
		} else if arr, ok := v.([]interface{}); ok {
			// Array of objects - normalize each element
			for _, elem := range arr {
				if m, ok := elem.(map[string]interface{}); ok {
					nestedType := getTypeNameForField(typeName, k)
					if nestedType != "" {
						normalizeObjectArrayFields(m, nestedType)
					}
				}
			}
		}
	}
}

// getTypeNameForField returns the type of a field in a given struct
func getTypeNameForField(structName, fieldName string) string {
	target, err := newFHIRResource(structName)
	if err != nil {
		// Try to get it as a non-resource type using reflection
		// This is a fallback for complex types like Address, HumanName, etc.
		return ""
	}

	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Find the struct field by JSON tag
	field, ok := t.FieldByNameFunc(func(name string) bool {
		if f, found := t.FieldByName(name); found {
			jsonTag := f.Tag.Get("json")
			tagName := strings.Split(jsonTag, ",")[0]
			return tagName == fieldName
		}
		return false
	})

	if ok {
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}
		if fieldType.Kind() == reflect.Slice {
			fieldType = fieldType.Elem()
			if fieldType.Kind() == reflect.Ptr {
				fieldType = fieldType.Elem()
			}
		}
		return fieldType.Name()
	}

	return ""
}

// newFHIRResource is imported from validate.go - defined there
