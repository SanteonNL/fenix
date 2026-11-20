package fhirserver

import (
	"encoding/json"
	"strings"

	"github.com/SanteonNL/fenix/internal/conformance"
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
//
// A token param may carry the :in / :not-in modifier ("code:in=<ValueSet url>"):
// the coding must (not) be a member of the ValueSet, expanded by the
// conformance store (see valueSetMembersFunc).
//
// A reference param may be chained ("condition:Condition.code=<token>"): the
// referenced resource must match the search after the dot. The chain is
// resolved by searching the target type (see chainFunc) and keeping the
// resources whose reference points at one of the matches. Other modifiers are
// left alone.
type paramMatcher struct {
	field     string
	paramType string
	value     string
	modifier  string               // "" | "in" | "not-in" | "chain"
	members   *conformance.Members // the union of the ValueSets of an :in / :not-in value
	target    string               // chain: the referenced resource type, e.g. Condition
	targetArg string               // chain: the search param on the target, e.g. code
	ids       map[string]bool      // chain: ids of the target resources that match
}

// valueSetMembersFunc expands a ValueSet given by canonical url for :in / :not-in.
type valueSetMembersFunc func(ref string) (conformance.Members, error)

// chainFunc returns the ids of the targetType resources matching targetParam=value.
type chainFunc func(targetType, targetParam, value string) (map[string]bool, error)

// filterDeps are what filterUnwiredParams needs beyond the resources: both optional.
type filterDeps struct {
	expand valueSetMembersFunc // :in / :not-in
	chain  chainFunc           // chained reference params
}

// valueSetModifiers are the token modifiers evaluated against a ValueSet.
var valueSetModifiers = map[string]bool{"in": true, "not-in": true}

// chainedParam splits a chained param "condition:Condition.code" into
// (condition, Condition, code). ok is false for anything else.
func chainedParam(code string) (base, target, targetParam string, ok bool) {
	base, modifier, found := strings.Cut(code, ":")
	if !found {
		return "", "", "", false
	}
	target, targetParam, ok = strings.Cut(modifier, ".")
	if !ok || target == "" || targetParam == "" || !validResourceTypeName(target) {
		return "", "", "", false
	}
	return base, target, targetParam, true
}

// unwiredMatchers are the params SQL did not narrow that filterUnwiredParams
// must check in Go.
func unwiredMatchers(resourceType string, fhirParams map[string]string, pushed map[string]bool, lookup searchParamLookup) []paramMatcher {
	var matchers []paramMatcher
	for code, value := range fhirParams {
		if pushed[code] {
			continue // SQL already scoped this one
		}
		if base, target, targetParam, ok := chainedParam(code); ok {
			if field, paramType, ok := lookup(resourceType, base); ok && paramType == "reference" {
				matchers = append(matchers, paramMatcher{field: field, paramType: paramType, value: value,
					modifier: "chain", target: target, targetArg: targetParam})
			}
			continue
		}
		base, modifier, _ := strings.Cut(code, ":")
		field, paramType, ok := lookup(resourceType, base)
		if !ok {
			continue // not a known search param for this resource — leave unfiltered
		}
		if modifier != "" && !(paramType == "token" && valueSetModifiers[modifier]) {
			continue // a modifier FENIX does not evaluate — leave unfiltered, as before
		}
		matchers = append(matchers, paramMatcher{field: field, paramType: paramType, value: value, modifier: modifier})
	}
	return matchers
}

// valueSetParams returns the ValueSet urls of every :in / :not-in param, so a
// search can be refused up front when one of them cannot be expanded.
func valueSetParams(fhirParams map[string]string) map[string][]string {
	out := map[string][]string{}
	for code, value := range fhirParams {
		if _, modifier, _ := strings.Cut(code, ":"); valueSetModifiers[modifier] {
			out[code] = splitUnescaped(value, ',')
		}
	}
	return out
}

// valueSetMembers expands every ValueSet of an :in / :not-in value into one membership.
func valueSetMembers(value string, expand valueSetMembersFunc) conformance.Members {
	all := conformance.Members{Pairs: map[string]bool{}, Codes: map[string]bool{}}
	if expand == nil {
		return all
	}
	for _, url := range splitUnescaped(value, ',') {
		m, err := expand(url)
		if err != nil {
			continue // refused up front (see valueSetParams); nothing to add here
		}
		for k := range m.Pairs {
			all.Pairs[k] = true
		}
		for k := range m.Codes {
			all.Codes[k] = true
		}
	}
	return all
}

// matches reports whether one resource field satisfies the matcher.
func (m paramMatcher) matches(fieldValue interface{}) bool {
	switch m.modifier {
	case "in":
		return fieldValue != nil && inValueSet(fieldValue, *m.members)
	case "not-in":
		return fieldValue == nil || !inValueSet(fieldValue, *m.members)
	case "chain":
		for _, id := range referencedIDs(fieldValue) {
			if m.ids[id] {
				return true
			}
		}
		return false
	}
	return matchParam(fieldValue, m.paramType, m.value)
}

// referencedIDs returns the id of every Reference anywhere inside v, e.g.
// EpisodeOfCare.diagnosis[].condition.reference "Condition/42" -> "42".
func referencedIDs(v interface{}) []string {
	var out []string
	switch val := v.(type) {
	case []interface{}:
		for _, item := range val {
			out = append(out, referencedIDs(item)...)
		}
	case map[string]interface{}:
		if ref, ok := val["reference"].(string); ok && ref != "" {
			if i := strings.LastIndex(ref, "/"); i != -1 {
				ref = ref[i+1:]
			}
			out = append(out, ref)
		}
		for k, item := range val {
			if k != "reference" {
				out = append(out, referencedIDs(item)...)
			}
		}
	}
	return out
}

// inValueSet reports whether any code in a token field (a plain code, a
// Coding, a CodeableConcept, or an array of these) is a member.
func inValueSet(v interface{}, members conformance.Members) bool {
	switch val := v.(type) {
	case string:
		return members.Contains("", val)
	case []interface{}:
		for _, item := range val {
			if inValueSet(item, members) {
				return true
			}
		}
	case map[string]interface{}:
		if codings, ok := val["coding"].([]interface{}); ok {
			return inValueSet(codings, members)
		}
		code, _ := val["code"].(string)
		system, _ := val["system"].(string)
		return code != "" && members.Contains(system, code)
	}
	return false
}

// hasUnwiredParams reports whether filterUnwiredParams would filter anything,
// i.e. whether the result of the SQL is not yet the final result.
func hasUnwiredParams(resourceType string, fhirParams map[string]string, pushed map[string]bool, lookup searchParamLookup) bool {
	return len(unwiredMatchers(resourceType, fhirParams, pushed, lookup)) > 0
}

func filterUnwiredParams(resourceType string, fhirParams map[string]string, pushed map[string]bool, resources []interface{}, lookup searchParamLookup, deps filterDeps) []interface{} {
	matchers := unwiredMatchers(resourceType, fhirParams, pushed, lookup)
	if len(matchers) == 0 {
		return resources
	}
	for i := range matchers {
		switch m := &matchers[i]; m.modifier {
		case "in", "not-in":
			members := valueSetMembers(m.value, deps.expand)
			m.members = &members
		case "chain":
			m.ids = map[string]bool{}
			if deps.chain != nil {
				if ids, err := deps.chain(m.target, m.targetArg, m.value); err == nil {
					m.ids = ids // an error was refused up front (see Server.checkChainParams)
				}
			}
		}
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
			if !m.matches(resolveField(fields, m.field)) {
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
//
// A comma-separated value is an OR of tokens. FHIR escapes a literal ',', '|',
// '$' or '\' inside a value with a backslash (e.g. the DBC code "0313\|922").
func matchToken(fieldValue interface{}, want string) bool {
	for _, token := range splitUnescaped(want, ',') {
		parts := splitUnescaped(token, '|')
		wantSystem, wantCode := "", ""
		if len(parts) == 1 {
			wantCode = unescape(parts[0])
		} else {
			wantSystem = unescape(parts[0])
			wantCode = unescape(strings.Join(parts[1:], `\|`))
		}
		if tokenMatches(fieldValue, wantSystem, wantCode) {
			return true
		}
	}
	return false
}

// splitUnescaped splits s on sep where sep is not escaped with a backslash.
// The parts keep their escapes; see unescape.
func splitUnescaped(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++ // skip the escaped character
		case sep:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// unescape removes the FHIR search escapes: \, \| \$ and \\.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
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
//
// A comma-separated value is an OR. A canonical field (a plain string, e.g.
// QuestionnaireResponse.questionnaire) matches the full canonical (a
// |version on either side is ignored) or, like a Reference, its last segment.
func matchReference(fieldValue interface{}, want string) bool {
	if strings.Contains(want, ",") {
		for _, w := range splitUnescaped(want, ',') {
			if matchReference(fieldValue, unescape(w)) {
				return true
			}
		}
		return false
	}
	wantID := want
	if i := strings.LastIndex(want, "/"); i != -1 {
		wantID = want[i+1:]
	}
	switch val := fieldValue.(type) {
	case string:
		canonical, _, _ := strings.Cut(val, "|")
		wantCanonical, _, _ := strings.Cut(want, "|")
		if canonical == wantCanonical {
			return true
		}
		id, _, _ := strings.Cut(wantID, "|")
		return canonical == id || strings.HasSuffix(canonical, "/"+id)
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
	if period, ok := fieldValue.(map[string]interface{}); ok {
		return matchPeriod(period, want)
	}
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

// matchPeriod compares a date search value with a Period ({"start","end"}),
// as FHIR does for a range: ge/gt match when the period ends at or after the
// date (or has no end), le/lt when it starts at or before it (or has no
// start), and no prefix / eq when the date falls inside the period.
func matchPeriod(period map[string]interface{}, want string) bool {
	start, _ := period["start"].(string)
	end, _ := period["end"].(string)
	if start == "" && end == "" {
		return false
	}
	prefix, date := "", want
	if len(want) >= 2 {
		switch strings.ToLower(want[:2]) {
		case "ge", "gt", "le", "lt", "eq":
			prefix, date = strings.ToLower(want[:2]), want[2:]
		}
	}
	cmp := func(v string) int {
		n := len(date)
		if n > len(v) {
			n = len(v)
		}
		return strings.Compare(v[:n], date)
	}
	switch prefix {
	case "ge":
		return end == "" || cmp(end) >= 0
	case "gt":
		return end == "" || cmp(end) > 0
	case "le":
		return start == "" || cmp(start) <= 0
	case "lt":
		return start == "" || cmp(start) < 0
	default:
		return (start == "" || cmp(start) <= 0) && (end == "" || cmp(end) >= 0)
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
