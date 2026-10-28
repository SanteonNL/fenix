package converter

import (
	"sort"
	"strings"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// ElementRule is the part of a StructureDefinition snapshot element that
// query validation needs: cardinality, allowed types, binding and the fixed
// value, keyed by the element's path (e.g. "Observation.value[x]").
type ElementRule struct {
	Path            string
	Min             int
	Max             string   // "1", "*", another number, or "0" (prohibited)
	Types           []string // type codes, e.g. ["Quantity", "CodeableConcept"]
	TargetProfiles  []string // Reference targets, e.g. .../StructureDefinition/Patient
	BindingStrength string   // required | extensible | preferred | example, "" = no binding
	ValueSet        string   // binding valueset, no version
	MustSupport     bool
	Fixed           string // fixed[x] value of a primitive, "" = none
	Obligations     []Obligation
	ProfileURL      string // StructureDefinition the rule came from
}

// Obligation is one FHIR obligation (R5-style extension, used by the Nictiz IGs):
// a code such as "SHALL:populate" that applies to one actor.
type Obligation struct {
	Code  string // e.g. SHALL:populate, SHOULD:populate-if-known
	Actor string // last segment of the ActorDefinition, e.g. ExchangingSystem
}

// Populate returns the obligation code that tells the producing system
// (actor ExchangingSystem) to fill the element, or "".
func (r ElementRule) Populate() string {
	for _, o := range r.Obligations {
		if o.Actor == "ExchangingSystem" && strings.Contains(o.Code, "populate") {
			return o.Code
		}
	}
	return ""
}

// Repeats reports whether the element may occur more than once.
func (r ElementRule) Repeats() bool { return r.Max != "" && r.Max != "0" && r.Max != "1" }

// Prohibited reports whether the profile forbids the element (max = 0).
func (r ElementRule) Prohibited() bool { return r.Max == "0" }

// Element returns the rule for a snapshot path, or false when no loaded
// profile describes it. Sliced elements are not registered: a slice only
// narrows the unsliced element, which is the one that is.
func (p *ProfileService) Element(path string) (ElementRule, bool) {
	r, ok := p.elements[path]
	if !ok {
		return ElementRule{}, false
	}
	return *r, true
}

// HasProfileFor reports whether any loaded profile describes resourceType.
func (p *ProfileService) HasProfileFor(resourceType string) bool {
	_, ok := p.elements[resourceType]
	return ok
}

// ChildElements returns the rules whose parent path is parentPath (one
// segment deeper), ordered by path, so validation can walk required elements
// level by level.
func (p *ProfileService) ChildElements(parentPath string) []ElementRule {
	paths := p.children[parentPath]
	out := make([]ElementRule, 0, len(paths))
	for _, path := range paths {
		out = append(out, *p.elements[path])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (p *ProfileService) registerElement(sdURL string, e fhir.ElementDefinition) {
	// Slices are skipped, and so is anything below a contentReference or a
	// slice path (these have ':' in the element id).
	if e.SliceName != nil {
		return
	}
	r := &ElementRule{Path: e.Path, Max: "", ProfileURL: sdURL}
	if e.Min != nil {
		r.Min = *e.Min
	}
	if e.Max != nil {
		r.Max = *e.Max
	}
	for _, t := range e.Type {
		r.Types = append(r.Types, t.Code)
		r.TargetProfiles = append(r.TargetProfiles, t.TargetProfile...)
	}
	if e.Binding != nil {
		r.BindingStrength = e.Binding.Strength.Code()
		if e.Binding.ValueSet != nil {
			r.ValueSet = stripVersion(*e.Binding.ValueSet)
		}
	}
	if e.MustSupport != nil {
		r.MustSupport = *e.MustSupport
	}
	switch {
	case e.FixedCode != nil:
		r.Fixed = *e.FixedCode
	case e.FixedUri != nil:
		r.Fixed = *e.FixedUri
	case e.FixedString != nil:
		r.Fixed = *e.FixedString
	}
	r.Obligations = obligationsOf(e)
	if _, known := p.elements[e.Path]; !known {
		if i := strings.LastIndex(e.Path, "."); i != -1 {
			p.children[e.Path[:i]] = append(p.children[e.Path[:i]], e.Path)
		}
	}
	p.elements[e.Path] = r
}

// NewResource returns a pointer to the typed fhir.* struct for resourceType,
// or an error when FENIX has no typed model for it (such resources are passed
// through unvalidated, see validateThroughStruct).
func NewResource(resourceType string) (interface{}, error) { return newFHIRResource(resourceType) }

// obligationsOf reads the http://hl7.org/fhir/StructureDefinition/obligation
// extensions of an element.
func obligationsOf(e fhir.ElementDefinition) []Obligation {
	var out []Obligation
	for _, ext := range e.Extension {
		if !strings.HasSuffix(ext.Url, "/obligation") {
			continue
		}
		var o Obligation
		for _, sub := range ext.Extension {
			switch {
			case sub.Url == "code" && sub.ValueCode != nil:
				o.Code = *sub.ValueCode
			case sub.Url == "actor" && sub.ValueCanonical != nil:
				a := *sub.ValueCanonical
				o.Actor = a[strings.LastIndex(a, "/")+1:]
			}
		}
		if o.Code != "" {
			out = append(out, o)
		}
	}
	return out
}

// ChoicePath rewrites the concrete choice-type keys in a path below root
// ("valueQuantity.value" → "value[x].value") to the profile's [x] form, so a
// problem found through the typed model and one found through the profile are
// reported at the same element. Segments that are not a choice element of a
// loaded profile are kept as they are.
func (p *ProfileService) ChoicePath(root, rel string) string {
	segs := strings.Split(rel, ".")
	cur := root
	for i, seg := range segs {
		for _, child := range p.children[cur] {
			name := child[len(cur)+1:]
			base, isChoice := strings.CutSuffix(name, "[x]")
			if isChoice && len(seg) > len(base) && strings.HasPrefix(seg, base) && seg[len(base)] >= 'A' && seg[len(base)] <= 'Z' {
				segs[i] = name
				seg = name
				break
			}
		}
		cur += "." + seg
	}
	return strings.Join(segs, ".")
}

// BindingFor returns the valueset whose concept map translates the values of
// the element at elemPath, as the dashboard names it (e.g. "Patient.gender",
// "Observation.code.coding.code"). The binding sits on the element itself (a
// code) or on the CodeableConcept/Coding that holds the code.
func (p *ProfileService) BindingFor(elemPath string) (string, bool) {
	candidates := []string{elemPath}
	switch {
	case strings.HasSuffix(elemPath, ".coding.code"):
		candidates = append(candidates, strings.TrimSuffix(elemPath, ".coding.code"))
	case strings.HasSuffix(elemPath, ".code"):
		candidates = append(candidates, strings.TrimSuffix(elemPath, ".code"))
	}
	for _, c := range candidates {
		if vs := p.ValuesetURI(c); vs != "" {
			return vs, true
		}
	}
	return "", false
}
