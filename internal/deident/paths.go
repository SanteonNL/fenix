package deident

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// FieldKind classifies a walked Field so callers (coverage matching, action
// application) can dispatch without re-inspecting Go reflection types.
type FieldKind int

const (
	KindString          FieldKind = iota
	KindIdentifierSlice           // an Identifier's own .Value (singular, *Identifier, or []Identifier alike)
	KindReference                 // a Reference's own .Reference (singular or []Reference alike) — an opaque leaf
	KindDate                      // fhir.Date / *fhir.Date
	KindDateString                // *string holding a FHIR date/dateTime/instant value
)

// Field is one populated, walkable leaf of a FHIR resource.
type Field struct {
	Path string
	Kind FieldKind
	Get  func() (string, bool) // current serialized value; ok=false if absent
	Set  func(string)          // writes back a serialized value
}

var (
	dateType       = reflect.TypeOf(fhir.Date{})
	dateTimeType   = reflect.TypeOf(fhir.DateTime{})
	referenceType  = reflect.TypeOf(fhir.Reference{})
	identifierType = reflect.TypeOf(fhir.Identifier{})
)

// skipAlways are BackboneElement envelope fields excluded from the walk at
// every depth: extensions, whose value is a polymorphic Value[x] union with
// no fixed path shape (see the implementation plan for this documented
// limitation).
var skipAlways = map[string]bool{
	"extension":         true,
	"modifierExtension": true,
}

// skipAtRoot are DomainResource envelope fields excluded only at the
// resource root (depth 0) — administrative metadata with no clinical or
// identifying content. They are deliberately NOT skipped at deeper levels,
// since the same JSON name is reused for real clinical content elsewhere
// (e.g. CodeableConcept.text, Annotation.text are unrelated to the
// resource-level narrative Text and must still be walked).
var skipAtRoot = map[string]bool{
	"meta":          true,
	"implicitRules": true,
	"language":      true,
	"text":          true,
}

var (
	jsonMarshalerType   = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	jsonUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
)

// maxWalkDepth is a safety net against an unforeseen cycle in the generated
// fhir.* types, not an intended limit — FHIR resources are trees, so a real
// path should never approach this depth.
const maxWalkDepth = 20

// Walk recursively enumerates every populated field of resource (a pointer
// to a concrete fhir.* struct), building dotted paths from the struct's json
// tags and prefixing them with resourceType. Depth is not capped to today's
// ruleset — see the paths.go design notes in the plan for why.
func Walk(resourceType string, resource interface{}) []Field {
	v := reflect.ValueOf(resource)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	var fields []Field
	walkStruct(resourceType, v, 0, &fields)
	return fields
}

func jsonName(tag string) (name string, ok bool) {
	if tag == "" {
		return "", false
	}
	name = strings.Split(tag, ",")[0]
	if name == "-" || name == "" {
		return "", false
	}
	return name, true
}

// walkStruct walks every exported field of an addressable struct value v,
// appending discovered Fields to out.
func walkStruct(path string, v reflect.Value, depth int, out *[]Field) {
	if depth > maxWalkDepth {
		return
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" { // unexported
			continue
		}
		name, ok := jsonName(sf.Tag.Get("json"))
		if !ok || skipAlways[name] || (depth == 0 && skipAtRoot[name]) {
			continue
		}
		// A nested element's own "id" attribute (Reference.Id, Identifier.Id,
		// CodeableConcept.Id, ...) is an internal fragment identifier, not
		// clinical/identifying data — only the resource-root id matters.
		if depth > 0 && name == "id" {
			continue
		}
		walkValue(path+"."+name, v.Field(i), depth, out)
	}
}

// walkValue classifies fv (from an addressable parent, so mutations stick)
// and, unless it's a terminal kind, recurses into it.
func walkValue(path string, fv reflect.Value, depth int, out *[]Field) {
	for fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return // absent — not a "present" field
		}
		fv = fv.Elem()
	}

	switch fv.Type() {
	case dateType:
		appendDateField(path, fv, out)
		return
	case dateTimeType:
		appendDateTimeField(path, fv, out)
		return
	case referenceType:
		appendReferenceField(path, fv, out)
		return
	case identifierType:
		appendIdentifierField(path, fv, out)
		return
	}

	// FHIR code enums (AdministrativeGender, ObservationStatus, ...) are
	// generated as named int types with custom JSON string marshaling —
	// detect them generically via the json.Marshaler/Unmarshaler interfaces
	// rather than hardcoding every enum type.
	if fv.Kind() != reflect.String && fv.CanAddr() &&
		fv.Type().Implements(jsonMarshalerType) && fv.Addr().Type().Implements(jsonUnmarshalerType) {
		appendCodedField(path, fv, out)
		return
	}

	switch fv.Kind() {
	case reflect.String:
		appendStringField(path, fv, out)
	case reflect.Struct:
		walkStruct(path, fv, depth+1, out)
	case reflect.Slice, reflect.Array:
		for i := 0; i < fv.Len(); i++ {
			walkValue(path, fv.Index(i), depth+1, out)
		}
	default:
		// Bool, numeric, and other leaf kinds carry no de-identification-
		// relevant content on their own and are intentionally not surfaced —
		// see the "explicitly out of scope" note on boolean/numeric coverage.
	}
}

// looksLikeDateField reports whether a field name (the last path segment)
// holds a FHIR date/dateTime/instant value, for the mixed fhir.Date/raw-
// string date fields this generated model uses.
func looksLikeDateField(path string) bool {
	name := path
	if i := strings.LastIndex(path, "."); i >= 0 {
		name = path[i+1:]
	}
	switch name {
	case "start", "end", "created", "issued", "dateAsserted", "recordedDate":
		return true
	}
	return strings.HasSuffix(name, "Date") ||
		strings.HasSuffix(name, "DateTime") ||
		strings.HasSuffix(name, "Instant")
}

func appendStringField(path string, fv reflect.Value, out *[]Field) {
	if fv.String() == "" {
		return
	}
	kind := KindString
	if looksLikeDateField(path) {
		kind = KindDateString
	}
	val := fv
	*out = append(*out, Field{
		Path: path,
		Kind: kind,
		Get:  func() (string, bool) { return val.String(), true },
		Set:  func(s string) { val.SetString(s) },
	})
}

func appendDateField(path string, fv reflect.Value, out *[]Field) {
	if fv.Interface().(fhir.Date).IsZero() {
		return
	}
	val := fv
	*out = append(*out, Field{
		Path: path,
		Kind: KindDate,
		Get:  func() (string, bool) { return val.Interface().(fhir.Date).String(), true },
		Set: func(s string) {
			if parsed, err := fhir.ParseDate(s); err == nil {
				val.Set(reflect.ValueOf(parsed))
			}
		},
	})
}

// appendDateTimeField handles fhir.DateTime — unlike fhir.Date, this is a
// flexible-precision value (it may carry a time-of-day and timezone), so it
// is classified KindDateString (parsed/formatted generically by
// actions.go's ParseFHIRDateish), not KindDate (which assumes a fixed
// date-only layout).
func appendDateTimeField(path string, fv reflect.Value, out *[]Field) {
	if fv.Interface().(fhir.DateTime).IsZero() {
		return
	}
	val := fv
	*out = append(*out, Field{
		Path: path,
		Kind: KindDateString,
		Get:  func() (string, bool) { return val.Interface().(fhir.DateTime).String(), true },
		Set: func(s string) {
			if parsed, err := fhir.ParseDateTime(s); err == nil {
				val.Set(reflect.ValueOf(parsed))
			}
		},
	})
}

func appendReferenceField(path string, fv reflect.Value, out *[]Field) {
	ref := fv.FieldByName("Reference")
	if ref.IsNil() {
		return
	}
	ptr := ref
	*out = append(*out, Field{
		Path: path,
		Kind: KindReference,
		Get:  func() (string, bool) { return *ptr.Interface().(*string), true },
		Set:  func(s string) { ptr.Set(reflect.ValueOf(&s)) },
	})
}

func appendCodedField(path string, fv reflect.Value, out *[]Field) {
	b, err := fv.Interface().(json.Marshaler).MarshalJSON()
	if err != nil {
		return
	}
	if strings.Trim(string(b), `"`) == "" {
		return
	}
	val := fv
	*out = append(*out, Field{
		Path: path,
		Kind: KindString,
		Get: func() (string, bool) {
			b, err := val.Interface().(json.Marshaler).MarshalJSON()
			if err != nil {
				return "", false
			}
			return strings.Trim(string(b), `"`), true
		},
		Set: func(s string) {
			_ = val.Addr().Interface().(json.Unmarshaler).UnmarshalJSON([]byte(`"` + s + `"`))
		},
	})
}

func appendIdentifierField(path string, fv reflect.Value, out *[]Field) {
	value := fv.FieldByName("Value")
	if value.IsNil() {
		return
	}
	ptr := value
	*out = append(*out, Field{
		Path: path,
		Kind: KindIdentifierSlice,
		Get:  func() (string, bool) { return *ptr.Interface().(*string), true },
		Set:  func(s string) { ptr.Set(reflect.ValueOf(&s)) },
	})
}
