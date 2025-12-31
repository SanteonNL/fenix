package hierarchy

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// Loader loads hierarchical data from SQL and assembles it into Go structs.
//
// The SQL uses comment-based path annotations to define the hierarchy:
//
//	-- CarePlan
//	SELECT id, status FROM care_plans WHERE patient_id = @patientId;
//
//	-- CarePlan.Activity
//	SELECT care_plan_id AS _parent, id, description FROM activities;
//
//	-- CarePlan.Activity.Detail
//	SELECT activity_id AS _parent, id, status FROM activity_details;
//
// The loader:
// 1. Parses SQL into separate queries by path comment
// 2. Executes all queries in a single batch
// 3. Assembles results into nested Go structs using reflection
type Loader struct {
	db *sql.DB
}

// NewLoader creates a new hierarchy loader
func NewLoader(db *sql.DB) *Loader {
	return &Loader{db: db}
}

// Query represents a parsed query with its hierarchy path
type Query struct {
	Path   string // e.g., "CarePlan.Activity.Detail"
	SQL    string // The actual SQL query
	Params map[string]any
}

// Load executes the SQL and assembles results into the target slice.
// The target must be a pointer to a slice of structs.
func (l *Loader) Load(ctx context.Context, sqlBatch string, params map[string]any, target any) error {
	// Parse SQL into queries
	queries := ParseSQL(sqlBatch)

	// Execute all queries
	results, err := l.executeQueries(ctx, queries, params)
	if err != nil {
		return err
	}

	// Assemble into target struct
	return Assemble(results, target)
}

// ParseSQL splits a SQL batch into individual queries based on path comments.
// Format: "-- Path.To.Element" followed by the SQL query
func ParseSQL(sqlBatch string) []Query {
	var queries []Query

	// Match: -- Path (with optional whitespace)
	pathRegex := regexp.MustCompile(`(?m)^--\s*([A-Z][A-Za-z0-9.]*)\s*$`)

	// Find all path comments
	matches := pathRegex.FindAllStringSubmatchIndex(sqlBatch, -1)

	for i, match := range matches {
		path := sqlBatch[match[2]:match[3]]

		// Get SQL between this comment and the next (or end)
		sqlStart := match[1]
		var sqlEnd int
		if i+1 < len(matches) {
			sqlEnd = matches[i+1][0]
		} else {
			sqlEnd = len(sqlBatch)
		}

		sqlText := strings.TrimSpace(sqlBatch[sqlStart:sqlEnd])
		if sqlText != "" {
			queries = append(queries, Query{
				Path: path,
				SQL:  sqlText,
			})
		}
	}

	return queries
}

// executeQueries runs all queries and returns results indexed by path
func (l *Loader) executeQueries(ctx context.Context, queries []Query, params map[string]any) (map[string][]map[string]any, error) {
	results := make(map[string][]map[string]any)

	for _, q := range queries {
		rows, err := l.executeQuery(ctx, q.SQL, params)
		if err != nil {
			return nil, fmt.Errorf("query for %s: %w", q.Path, err)
		}
		results[q.Path] = rows
	}

	return results, nil
}

// executeQuery runs a single query with named parameters
func (l *Loader) executeQuery(ctx context.Context, query string, params map[string]any) ([]map[string]any, error) {
	// Convert named params to positional for database/sql
	args := make([]any, 0)
	for name, value := range params {
		args = append(args, sql.Named(name, value))
	}

	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]any

	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}

		row := make(map[string]any)
		for i, col := range columns {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	return results, rows.Err()
}

// Assemble builds hierarchical Go structs from flat query results.
// It uses the _parent column to establish relationships.
func Assemble(results map[string][]map[string]any, target any) error {
	targetVal := reflect.ValueOf(target)
	if targetVal.Kind() != reflect.Ptr || targetVal.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("target must be a pointer to a slice")
	}

	sliceVal := targetVal.Elem()
	elemType := sliceVal.Type().Elem()

	// Determine root path from type name
	rootPath := elemType.Name()

	rootRows, ok := results[rootPath]
	if !ok {
		return nil // No data
	}

	// Build index of all rows by path and ID for parent lookup
	indexes := buildIndexes(results)

	// Create root elements
	for _, row := range rootRows {
		elem := reflect.New(elemType).Elem()
		if err := populateStruct(elem, row, rootPath, results, indexes); err != nil {
			return err
		}
		sliceVal = reflect.Append(sliceVal, elem)
	}

	targetVal.Elem().Set(sliceVal)
	return nil
}

// buildIndexes creates lookup maps for parent relationships
func buildIndexes(results map[string][]map[string]any) map[string]map[any][]map[string]any {
	indexes := make(map[string]map[any][]map[string]any)

	for path, rows := range results {
		indexes[path] = make(map[any][]map[string]any)
		for _, row := range rows {
			if parentID, ok := row["_parent"]; ok {
				indexes[path][parentID] = append(indexes[path][parentID], row)
			}
		}
	}

	return indexes
}

// populateStruct fills a struct from a row and recursively populates children
func populateStruct(structVal reflect.Value, row map[string]any, path string, results map[string][]map[string]any, indexes map[string]map[any][]map[string]any) error {
	structType := structVal.Type()

	// Get this element's ID for child lookup
	var thisID any
	if id, ok := row["id"]; ok {
		thisID = id
	}

	// Build a map of possible child paths to their struct field indices
	// e.g., "Activity" -> field index for Activities []Activity
	childFieldMap := make(map[string]int)
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if field.Type.Kind() == reflect.Slice {
			elemType := field.Type.Elem()
			// Map the element type name to this field
			childFieldMap[elemType.Name()] = i
		}
	}

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldVal := structVal.Field(i)

		if !fieldVal.CanSet() {
			continue
		}

		// Skip slice fields - we handle them via child path lookup below
		if field.Type.Kind() == reflect.Slice {
			continue
		}

		// Map column to field
		colName := toSnakeCase(field.Name)
		if val, ok := row[colName]; ok && val != nil {
			if err := setFieldValue(fieldVal, val); err != nil {
				return fmt.Errorf("field %s: %w", field.Name, err)
			}
		}
	}

	// Now handle children by finding matching paths in results
	// Look for paths that start with our path + "."
	for childPath := range results {
		if !strings.HasPrefix(childPath, path+".") {
			continue
		}

		// Get the immediate child name (e.g., "Activity" from "CarePlan.Activity")
		remainder := strings.TrimPrefix(childPath, path+".")
		if strings.Contains(remainder, ".") {
			// This is a deeper nested path, skip for now
			continue
		}

		// Find the struct field that matches this child type
		fieldIdx, ok := childFieldMap[remainder]
		if !ok {
			continue
		}

		field := structType.Field(fieldIdx)
		fieldVal := structVal.Field(fieldIdx)
		childType := field.Type.Elem()

		// Find children by parent ID
		if childIndex, ok := indexes[childPath]; ok && thisID != nil {
			if childRows, ok := childIndex[thisID]; ok {
				childSlice := reflect.MakeSlice(field.Type, 0, len(childRows))
				for _, childRow := range childRows {
					childElem := reflect.New(childType).Elem()
					if err := populateStruct(childElem, childRow, childPath, results, indexes); err != nil {
						return err
					}
					childSlice = reflect.Append(childSlice, childElem)
				}
				fieldVal.Set(childSlice)
			}
		}
	}

	return nil
}

// setFieldValue converts and sets a value to a struct field
func setFieldValue(field reflect.Value, value any) error {
	if value == nil {
		return nil
	}

	fieldType := field.Type()
	valueVal := reflect.ValueOf(value)

	// Direct assignment if types match
	if valueVal.Type().AssignableTo(fieldType) {
		field.Set(valueVal)
		return nil
	}

	// Type conversions
	switch field.Kind() {
	case reflect.String:
		field.SetString(fmt.Sprintf("%v", value))
	case reflect.Int, reflect.Int64:
		switch v := value.(type) {
		case int64:
			field.SetInt(v)
		case int:
			field.SetInt(int64(v))
		case float64:
			field.SetInt(int64(v))
		}
	case reflect.Float64:
		switch v := value.(type) {
		case float64:
			field.SetFloat(v)
		case int64:
			field.SetFloat(float64(v))
		}
	case reflect.Bool:
		switch v := value.(type) {
		case bool:
			field.SetBool(v)
		case int64:
			field.SetBool(v != 0)
		}
	case reflect.Ptr:
		// Handle pointer types
		elemType := fieldType.Elem()
		ptr := reflect.New(elemType)
		if err := setFieldValue(ptr.Elem(), value); err != nil {
			return err
		}
		field.Set(ptr)
	}

	return nil
}

// toSnakeCase converts PascalCase to snake_case
// Handles common abbreviations like ID, URL, etc.
func toSnakeCase(s string) string {
	// Handle common abbreviations
	if s == "ID" {
		return "id"
	}

	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			// Don't add underscore if previous char was also uppercase (e.g., "ID" in "UserID")
			prev := s[i-1]
			if prev < 'A' || prev > 'Z' {
				result.WriteByte('_')
			} else if i+1 < len(s) {
				// Check if next char is lowercase (end of abbreviation)
				next := s[i+1]
				if next >= 'a' && next <= 'z' {
					result.WriteByte('_')
				}
			}
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}