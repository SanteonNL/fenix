package queryvalidator

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// field describes one element of a FHIR struct as seen through its Go model.
type field struct {
	Name     string // JSON name, e.g. "birthDate" or "valueQuantity"
	Elem     reflect.Type
	Repeats  bool
	Required bool // declared without omitempty: the base spec requires it
	Prim     string
	Enum     []string // allowed codes when the field is a required-binding code
	Struct   bool     // complex type, has sub-elements

	ChoiceBase string // "value" for valueQuantity, "" when not a choice variant
	ChoiceType string // "Quantity" for valueQuantity
}

// structInfo is the cached, parsed view of one FHIR struct type.
type structInfo struct {
	typ    reflect.Type
	fields []field
	byName map[string]int
	choice map[string][]int // choice base → indexes of its variants
}

var structCache sync.Map // reflect.Type → *structInfo

// choiceSuffixes are the FHIR data types a value[x] element can take.
var choiceSuffixes = []string{
	"Base64Binary", "Boolean", "Canonical", "Code", "Date", "DateTime", "Decimal", "Id", "Instant", "Integer",
	"Markdown", "Oid", "PositiveInt", "String", "Time", "UnsignedInt", "Uri", "Url", "Uuid",
	"Address", "Age", "Annotation", "Attachment", "CodeableConcept", "Coding", "ContactPoint", "Count", "Distance",
	"Duration", "HumanName", "Identifier", "Money", "Period", "Quantity", "Range", "Ratio", "Reference",
	"SampledData", "Signature", "Timing",
}

func init() {
	sort.Slice(choiceSuffixes, func(i, j int) bool { return len(choiceSuffixes[i]) > len(choiceSuffixes[j]) })
}

func splitChoice(name string) (base, typ string) {
	for _, s := range choiceSuffixes {
		if len(name) > len(s) && strings.HasSuffix(name, s) {
			b := name[:len(name)-len(s)]
			if b[0] >= 'a' && b[0] <= 'z' {
				return b, s
			}
		}
	}
	return "", ""
}

func structOf(t reflect.Type) *structInfo {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	if v, ok := structCache.Load(t); ok {
		return v.(*structInfo)
	}
	si := &structInfo{typ: t, byName: map[string]int{}, choice: map[string][]int{}}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		tag := sf.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" || name == "resourceType" {
			continue
		}
		f := newField(name, sf, strings.Contains(tag, "omitempty"))
		if b, ct := splitChoice(name); b != "" {
			f.ChoiceBase, f.ChoiceType = b, ct
		}
		si.byName[name] = len(si.fields)
		si.fields = append(si.fields, f)
	}
	// Only a base with two or more variants is a real choice element:
	// "birthDate" alone is a plain date, not birth[x].
	groups := map[string][]int{}
	for i, f := range si.fields {
		if f.ChoiceBase != "" {
			groups[f.ChoiceBase] = append(groups[f.ChoiceBase], i)
		}
	}
	for b, idx := range groups {
		if len(idx) >= 2 {
			si.choice[b] = idx
			continue
		}
		si.fields[idx[0]].ChoiceBase, si.fields[idx[0]].ChoiceType = "", ""
	}
	structCache.Store(t, si)
	return si
}

var (
	dateType     = reflect.TypeOf(fhir.Date{})
	dateTimeType = reflect.TypeOf(fhir.DateTime{})
	numberType   = reflect.TypeOf(json.Number(""))
)

func newField(name string, sf reflect.StructField, omitempty bool) field {
	f := field{Name: name, Required: !omitempty}
	t := sf.Type
	if t.Kind() == reflect.Slice {
		f.Repeats = true
		t = t.Elem()
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	f.Elem = t
	switch {
	case t == dateType:
		f.Prim = "date"
	case t == dateTimeType:
		f.Prim = "dateTime"
	case t == numberType:
		f.Prim = "decimal"
	case t.Kind() == reflect.Bool:
		f.Prim = "boolean"
	case t.Kind() == reflect.Int:
		if codes := enumCodes(t); len(codes) > 0 {
			f.Prim, f.Enum = "code", codes
		} else {
			f.Prim = "integer"
		}
	case t.Kind() == reflect.String:
		f.Prim = "string"
	case t.Kind() == reflect.Struct:
		f.Struct = true
	}
	return f
}

// enumCodes lists the codes of a generated enum type (an int with a Code()
// method), or nil when t is a plain int.
func enumCodes(t reflect.Type) []string {
	m, ok := reflect.PtrTo(t).MethodByName("Code")
	if !ok {
		if m, ok = t.MethodByName("Code"); !ok {
			return nil
		}
	}
	_ = m
	var codes []string
	v := reflect.New(t).Elem()
	for i := 0; i < 5000; i++ {
		v.SetInt(int64(i))
		out := v.MethodByName("Code").Call(nil)
		if len(out) != 1 {
			return nil
		}
		c := out[0].String()
		if c == "<unknown>" {
			break
		}
		codes = append(codes, c)
	}
	return codes
}

// lookup finds a field by JSON name; exact is false when only a
// case-insensitive match exists (Go's JSON decoder accepts that, FHIR does not).
func (si *structInfo) lookup(name string) (f field, exact, ok bool) {
	if i, ok := si.byName[name]; ok {
		return si.fields[i], true, true
	}
	for _, f := range si.fields {
		if strings.EqualFold(f.Name, name) {
			return f, false, true
		}
	}
	return field{}, false, false
}

func (si *structInfo) names() []string {
	out := make([]string, len(si.fields))
	for i, f := range si.fields {
		out[i] = f.Name
	}
	return out
}

// resourceTypes is the set of FHIR R4 resource type names.
var resourceTypes = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range enumCodes(reflect.TypeOf(fhir.ResourceType(0))) {
		m[c] = true
	}
	return m
}()

// closest returns the candidate within edit distance 2 of name, if any.
func closest(name string, candidates []string) string {
	lower := strings.ToLower(name)
	// A prefix match ("reason" -> "reasonCode") beats edit distance.
	var prefixed []string
	for _, c := range candidates {
		if len(lower) >= 3 && strings.HasPrefix(strings.ToLower(c), lower) {
			prefixed = append(prefixed, c)
		}
	}
	sort.Strings(prefixed)
	if len(prefixed) > 0 {
		if len(prefixed) > 3 {
			prefixed = prefixed[:3]
		}
		return strings.Join(prefixed, " or ")
	}
	best, bestD := "", 3
	for _, c := range candidates {
		if d := editDistance(lower, strings.ToLower(c)); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// leafPrimitives pin down FHIR primitives that the generated models only
// declare as *string. Keyed "<Go struct name>.<json name>".
var leafPrimitives = map[string]string{
	"Coding.system": "uri", "Coding.code": "code", "Identifier.system": "uri",
	"Quantity.system": "uri", "Quantity.code": "code", "Reference.type": "uri",
	"Period.start": "dateTime", "Period.end": "dateTime",
}

// primitiveFromRule maps a profile type code to a FHIR primitive name, or ""
// for complex and unknown types.
func primitiveFromRule(code string) string {
	switch code {
	case "http://hl7.org/fhirpath/System.String":
		return "string"
	case "boolean", "integer", "positiveInt", "unsignedInt", "decimal", "date", "dateTime", "instant", "time",
		"code", "id", "uri", "url", "canonical", "oid", "uuid", "string", "markdown", "base64Binary":
		return code
	}
	return ""
}

// primitiveFromChoice maps a choice suffix (DateTime, Boolean, ...) to a primitive.
func primitiveFromChoice(suffix string) string {
	if suffix == "" {
		return ""
	}
	return primitiveFromRule(strings.ToLower(suffix[:1]) + suffix[1:])
}
