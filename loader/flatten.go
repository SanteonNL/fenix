package loader

import (
	"encoding/json"
	"reflect"
	"strings"
)

// FlatResult holds the columns for one main-table row plus any child rows
// produced by slice fields.
//
// Child rows are keyed by the JSON field name of the slice (e.g. "identifiers").
// Each child row already has the parent FK column set; see Loader.Load.
type FlatResult struct {
	Row      map[string]interface{}
	Children map[string][]map[string]interface{} // field name → rows
}

// Flattener converts arbitrary structs into FlatResults for SQL insertion.
//
// ScalarTypes lists struct type names that should be stored as a single TEXT
// column rather than recursed into. Use this for API-specific marker types
// (e.g. Luscii's UuidSchema, DateTimeSchema) that wrap a primitive value.
type Flattener struct {
	ScalarTypes map[string]bool
}

// Flatten converts v into a FlatResult.
//
//   - Nested structs are inlined with a "<field>_" prefix.
//   - Types listed in ScalarTypes become a single TEXT column.
//   - Slice fields become child rows in FlatResult.Children.
//   - nil pointers produce a nil (SQL NULL) value.
func (f *Flattener) Flatten(v interface{}) FlatResult {
	res := FlatResult{
		Row:      make(map[string]interface{}),
		Children: make(map[string][]map[string]interface{}),
	}
	f.flattenStruct(reflect.ValueOf(v), "", res.Row, res.Children)
	return res
}

func (f *Flattener) flattenStruct(v reflect.Value, prefix string, row map[string]interface{}, children map[string][]map[string]interface{}) {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		val := v.Field(i)
		colName := jsonKey(field, prefix)

		elem := val
		for elem.Kind() == reflect.Ptr || elem.Kind() == reflect.Interface {
			if elem.IsNil() {
				row[colName] = nil
				goto next
			}
			elem = elem.Elem()
		}

		switch elem.Kind() {
		case reflect.Struct:
			if f.ScalarTypes[elem.Type().Name()] {
				b, _ := json.Marshal(val.Interface())
				row[colName] = strings.Trim(string(b), `"`)
			} else {
				f.flattenStruct(elem, colName+"_", row, children)
			}

		case reflect.Slice:
			fieldName := jsonKey(field, "")
			for j := 0; j < elem.Len(); j++ {
				childRow := make(map[string]interface{})
				childChildren := make(map[string][]map[string]interface{})
				f.flattenStruct(elem.Index(j), "", childRow, childChildren)
				// deeper arrays in a child become JSON — they are rare and
				// the query writer can json_extract() them if needed
				for cf, cv := range childChildren {
					b, _ := json.Marshal(cv)
					childRow[cf] = string(b)
				}
				children[fieldName] = append(children[fieldName], childRow)
			}

		default:
			row[colName] = elem.Interface()
		}
	next:
	}
}

func jsonKey(f reflect.StructField, prefix string) string {
	tag := f.Tag.Get("json")
	name := strings.Split(tag, ",")[0]
	if name == "" || name == "-" {
		name = toSnake(f.Name)
	}
	return prefix + name
}

func toSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

// singular strips a trailing 's' to derive a FK column name.
// "patients" → "patient_id", "conditions" → "condition_id".
func singular(name string) string {
	if strings.HasSuffix(name, "s") {
		return name[:len(name)-1]
	}
	return name
}
