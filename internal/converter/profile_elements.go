package converter

import (
	"sort"
	"strings"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// ElementRule is the part of a StructureDefinition snapshot element that
// query validation needs: cardinality, allowed types, binding and fixed
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
// narrows the unsliced element, which is registered.
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

// ChildElements returns the rules one segment below parentPath, ordered by
// path, so validation can walk required elements level by level.
func (p *ProfileService) ChildElements(parentPath string) []ElementRule {
	prefix := parentPath + "."
	var out []ElementRule
	for path, r := range p.elements {
		if strings.HasPrefix(path, prefix) && !strings.Contains(path[len(prefix):], ".") {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func (p *ProfileService) registerElement(e fhir.ElementDefinition) {
	if e.SliceName != nil {
		return
	}
	r := &ElementRule{Path: e.Path}
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
