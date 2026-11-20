package fhirserver

import (
	"encoding/json"
	"strings"

	"github.com/SanteonNL/fenix/internal/converter"
)

// subsettedTag marks a resource that does not hold all its elements, as FHIR
// requires for _elements (http://hl7.org/fhir/R4/search.html#elements).
var subsettedTag = map[string]interface{}{
	"system": "http://terminology.hl7.org/CodeSystem/v3-ObservationValue",
	"code":   "SUBSETTED",
}

// trimElements keeps only the elements an _elements list asks for, for the
// resources of one type: the declared elements (a choice element "value[x]"
// keeps whichever value... variant is present), the elements FHIR requires
// for the type (converter.MandatoryElements), and resourceType, id and meta.
// Every trimmed resource gets the SUBSETTED tag. declared holds the element
// names without the type prefix ("code", "value[x]"); empty leaves the
// resources as they are.
func trimElements(resourceType string, resources []interface{}, declared []string) []interface{} {
	if len(declared) == 0 {
		return resources
	}
	keep := map[string]bool{"resourceType": true, "id": true, "meta": true}
	for name := range converter.MandatoryElements(resourceType) {
		keep[name] = true
	}
	var choices []string
	for _, d := range declared {
		if base, ok := strings.CutSuffix(d, "[x]"); ok {
			choices = append(choices, base)
		} else {
			keep[d] = true
		}
	}

	out := make([]interface{}, 0, len(resources))
	for _, r := range resources {
		b, err := json.Marshal(r)
		if err != nil {
			out = append(out, r)
			continue
		}
		var fields map[string]interface{}
		if err := json.Unmarshal(b, &fields); err != nil {
			out = append(out, r)
			continue
		}
		trimmed := false
		for k := range fields {
			if keep[k] || isChoiceOf(k, choices) {
				continue
			}
			delete(fields, k)
			trimmed = true
		}
		if trimmed {
			addSubsettedTag(fields)
		}
		out = append(out, fields)
	}
	return out
}

// isChoiceOf reports whether key is a typed variant of one of the choice
// elements: valueQuantity, valueCodeableConcept, ... for "value".
func isChoiceOf(key string, choices []string) bool {
	for _, c := range choices {
		if rest, ok := strings.CutPrefix(key, c); ok && rest != "" && rest[0] >= 'A' && rest[0] <= 'Z' {
			return true
		}
	}
	return false
}

func addSubsettedTag(fields map[string]interface{}) {
	meta, _ := fields["meta"].(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
	}
	tags, _ := meta["tag"].([]interface{})
	for _, t := range tags {
		if m, ok := t.(map[string]interface{}); ok && m["code"] == "SUBSETTED" {
			return
		}
	}
	meta["tag"] = append(tags, subsettedTag)
	fields["meta"] = meta
}
