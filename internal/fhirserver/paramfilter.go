package fhirserver

import (
	"encoding/json"
	"strings"
)

// searchParamLookup resolves (resourceType, code) to the FHIR field name and
// SearchParameter type to filter on — *querycompiler.Compiler.SearchParamField
// satisfies this. Taking it as a function, rather than the concrete
// compiler type, keeps filterUnwiredParams testable without a real
// terminology/searchparameter/search-parameter.json-backed compiler.
type searchParamLookup func(resourceType, code string) (field, paramType string, ok bool)

// filterUnwiredParams is the correctness backstop for FHIR search params
// that aren't configured as SQL pushdown for (source, resourceType): SQL
// pushdown (see querycompiler.Compiler.Resolve) is the fast path, but a
// param not listed in a query's `pushdown:` config is otherwise silently
// ignored — the SQL never narrows on it, so every row comes back regardless
// of what the caller asked for. This re-checks every such param against the
// already-converted FHIR resources in Go, using the same
// terminology/searchparameter/search-parameter.json index pushdown itself
// is built from (via lookup) to know which field to inspect and how to
// compare it (token/reference/date/string semantics).
//
// Params lookup doesn't recognise at all (lookup's ok is false — e.g. the
// bulk-export-only "_typeFilter"/"_elements", or any param FENIX's
// SearchParameter bundle has no entry for) are left alone, same as before
// this filter existed: FENIX has never rejected an unknown search param,
// and this isn't the place to start.
func filterUnwiredParams(resourceType string, fhirParams map[string]string, pushed map[string]bool, resources []interface{}, lookup searchParamLookup) []interface{} {
	type matcher struct {
		field     string
		paramType string
		value     string
	}

	var matchers []matcher
	for code, value := range fhirParams {
		if pushed[code] {
			continue // SQL already scoped this one
		}
		field, paramType, ok := lookup(resourceType, code)
		if !ok {
			continue // not a known search param for this resource — leave unfiltered
		}
		matchers = append(matchers, matcher{field, paramType, value})
	}
	if len(matchers) == 0 {
		return resources
	}

	out := make([]interface{}, 0, len(resources))
	for _, res := range resources {
		b, err := json.Marshal(res)
		if err != nil {
			continue
		}
		var fields map[string]interface{}
		if err := json.Unmarshal(b, &fields); err != nil {
			continue
		}

		matched := true
		for _, m := range matchers {
			if !matchParam(resolveField(fields, m.field), m.paramType, m.value) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, res)
		}
	}
	return out
}

// resolveField looks up field directly, then falls back to a prefix scan
// for FHIR choice-type ([x]) fields: fieldName strips the "[x]" marker (see
// querycompiler.fieldName), so a SearchParameter expression like
// "Observation.effective[x]" resolves to field "effective", but the
// converted resource's actual JSON key is the concrete type it was
// populated as — "effectiveDateTime", "effectivePeriod", etc.
func resolveField(fields map[string]interface{}, field string) interface{} {
	if v, ok := fields[field]; ok {
		return v
	}
	for k, v := range fields {
		if strings.HasPrefix(k, field) {
			return v
		}
	}
	return nil
}

// matchParam reports whether fieldValue (the JSON value of the field a
// SearchParameter's expression points at) satisfies a FHIR search param
// value of the given SearchParameter type.
func matchParam(fieldValue interface{}, paramType, want string) bool {
	if fieldValue == nil {
		return false
	}
	switch paramType {
	case "token":
		return matchToken(fieldValue, want)
	case "reference":
		return matchReference(fieldValue, want)
	case "date":
		return matchDate(fieldValue, want)
	default:
		// "string", "number", "quantity", "uri", ... — no FHIR-defined
		// structured shape worth special-casing here, so compare as text.
		return matchString(fieldValue, want)
	}
}

// matchToken implements FHIR token search: "system|code" matches both
// exactly, a bare "code" (or "|code") matches on code only. Handles a plain
// string field (e.g. Observation.status), a single Coding/CodeableConcept
// ({"coding":[...]} or {"system","code"}), or an array of either.
func matchToken(fieldValue interface{}, want string) bool {
	wantSystem, wantCode, hasSystem := strings.Cut(want, "|")
	if !hasSystem {
		wantCode = wantSystem
		wantSystem = ""
	}
	return tokenMatches(fieldValue, wantSystem, wantCode)
}

func tokenMatches(v interface{}, wantSystem, wantCode string) bool {
	switch val := v.(type) {
	case string:
		return val == wantCode
	case []interface{}:
		for _, item := range val {
			if tokenMatches(item, wantSystem, wantCode) {
				return true
			}
		}
		return false
	case map[string]interface{}:
		if codings, ok := val["coding"].([]interface{}); ok {
			return tokenMatches(codings, wantSystem, wantCode)
		}
		code, _ := val["code"].(string)
		if code != wantCode {
			return false
		}
		system, _ := val["system"].(string)
		return wantSystem == "" || system == wantSystem
	default:
		return false
	}
}

// matchReference implements FHIR reference search: the search value may be
// a bare id ("123") or "ResourceType/123" — either way, only the trailing
// id segment is compared, against a Reference field ({"reference": "..."})
// or an array of them.
func matchReference(fieldValue interface{}, want string) bool {
	wantID := want
	if i := strings.LastIndex(want, "/"); i != -1 {
		wantID = want[i+1:]
	}
	switch val := fieldValue.(type) {
	case map[string]interface{}:
		ref, _ := val["reference"].(string)
		if ref == "" {
			return false
		}
		refID := ref
		if i := strings.LastIndex(ref, "/"); i != -1 {
			refID = ref[i+1:]
		}
		return refID == wantID
	case []interface{}:
		for _, item := range val {
			if matchReference(item, want) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// matchDate implements the FHIR date prefix comparators (ge/gt/le/lt/eq, or
// an exact/prefix match with none) via lexical comparison on the ISO8601
// field value — the same comparison SQL pushdown already does for dates
// (see buildTemplateVars's doc comment: "effective_date >= '{{.from}}'"),
// so behaviour stays consistent whether or not a given date param happens
// to be wired as pushdown.
func matchDate(fieldValue interface{}, want string) bool {
	val, ok := fieldValue.(string)
	if !ok || val == "" {
		return false
	}

	prefix, date := "", want
	if len(want) >= 2 {
		switch strings.ToLower(want[:2]) {
		case "ge", "gt", "le", "lt", "eq":
			prefix, date = strings.ToLower(want[:2]), want[2:]
		}
	}

	n := len(date)
	if n > len(val) {
		n = len(val)
	}
	cmp := strings.Compare(val[:n], date)

	switch prefix {
	case "ge":
		return cmp >= 0
	case "gt":
		return cmp > 0
	case "le":
		return cmp <= 0
	case "lt":
		return cmp < 0
	default: // "eq" or no recognised prefix: exact/prefix match
		return strings.HasPrefix(val, date)
	}
}

// matchString implements a case-insensitive "starts with" match, the FHIR
// default for the "string" param type, and is also the fallback for any
// other param type with no structured shape to speak of.
func matchString(fieldValue interface{}, want string) bool {
	switch val := fieldValue.(type) {
	case string:
		return strings.HasPrefix(strings.ToLower(val), strings.ToLower(want))
	case []interface{}:
		for _, item := range val {
			if matchString(item, want) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
