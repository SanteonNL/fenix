package fhirserver

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/SanteonNL/fenix/internal/datasets"
	"github.com/SanteonNL/fenix/internal/findings"
)

// codeFilter is one param of an export's _typeFilter that a data requirement
// of the dataset filters on, with every code it asks for.
type codeFilter struct {
	param    string   // as in the _typeFilter: "code", "code:in", "questionnaire"
	field    string   // the resource element the param searches, e.g. "code"
	refParam bool     // reference/canonical param: compared as text, not as a coding
	coded    bool     // the codes are system|code pairs of a CodeableConcept (a plain code field like gender is not)
	want     []string // "system|code", "code", or a reference/canonical url
	by       []string // the requirements that filter on it
}

// codeTracker collects, over every batch of one exported type, which of the
// codes the dataset filters on occur in the data.
type codeTracker struct {
	resourceType string
	filters      []codeFilter
	mu           sync.Mutex
	seen         []map[string]bool // per filter: the wanted codes found
}

// datasetCodeTracker returns the tracker for one exported type, nil when the
// dataset filters that type on no code. The codes come from the export's own
// _typeFilter (the IG resolves the requirements' {od:...} / {vs:...}
// placeholders into it), limited to the token and reference params a data
// requirement declares; dates and numbers are not codes. A ":in" ValueSet
// stands for each of its members.
func (s *Server) datasetCodeTracker(ds *datasets.Dataset, resourceType string, typeFilter map[string]string) *codeTracker {
	if s.compiler == nil {
		return nil
	}
	return newCodeTracker(ds, resourceType, typeFilter, s.compiler.SearchParamField, s.valueSetMembers())
}

// newCodeTracker is datasetCodeTracker with the search param lookup and the
// ValueSet expander passed in.
func newCodeTracker(ds *datasets.Dataset, resourceType string, typeFilter map[string]string, lookup searchParamLookup, expand valueSetMembersFunc) *codeTracker {
	if ds == nil || len(ds.Filters[resourceType]) == 0 || len(typeFilter) == 0 {
		return nil
	}
	t := &codeTracker{resourceType: resourceType}
	params := make([]string, 0, len(typeFilter))
	for p := range typeFilter {
		params = append(params, p)
	}
	sort.Strings(params)
	for _, param := range params {
		by := ds.Filters[resourceType][param]
		if len(by) == 0 {
			continue // not a filter of the data requirements
		}
		if _, _, _, chained := chainedParam(param); chained {
			continue
		}
		base, modifier, _ := strings.Cut(param, ":")
		field, paramType, ok := lookup(resourceType, base)
		if !ok {
			continue
		}
		f := codeFilter{param: param, field: field, by: by}
		switch {
		case paramType == "token" && modifier == "in":
			members := valueSetMembers(typeFilter[param], expand)
			for pair := range members.Pairs {
				f.want = append(f.want, pair)
			}
			sort.Strings(f.want)
		case paramType == "token" && modifier == "":
			f.want = splitUnescaped(typeFilter[param], ',')
		case paramType == "reference" && modifier == "":
			f.refParam = true
			f.want = splitUnescaped(typeFilter[param], ',')
		default:
			continue // date, number, quantity, string, other modifiers: no codes
		}
		if !f.refParam {
			for _, w := range f.want {
				if strings.Contains(w, "|") {
					f.coded = true
				}
			}
		}
		if len(f.want) > 0 {
			t.filters = append(t.filters, f)
		}
	}
	if len(t.filters) == 0 {
		return nil
	}
	t.seen = make([]map[string]bool, len(t.filters))
	for i := range t.seen {
		t.seen[i] = map[string]bool{}
	}
	return t
}

// observe records the wanted codes that occur in one batch of resources.
func (t *codeTracker) observe(resources []interface{}) {
	if t == nil {
		return
	}
	found := make([]map[string]bool, len(t.filters))
	for i := range found {
		found[i] = map[string]bool{}
	}
	for _, r := range resources {
		b, err := json.Marshal(r)
		if err != nil {
			continue
		}
		var fields map[string]interface{}
		if json.Unmarshal(b, &fields) != nil {
			continue
		}
		for i, f := range t.filters {
			v := resolveField(fields, f.field)
			if v == nil {
				continue
			}
			for _, want := range f.want {
				if f.refParam {
					if matchReference(v, want) {
						found[i][want] = true
					}
				} else if matchToken(v, want) {
					found[i][want] = true
				}
			}
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i := range found {
		for code := range found[i] {
			t.seen[i][code] = true
		}
	}
}

// reportMissingDatasetCodes reports, once the type is exported, every code
// the dataset filters on that no exported resource carried: one finding per
// filter, the missing codes as its values.
func (s *Server) reportMissingDatasetCodes(ds *datasets.Dataset, t *codeTracker) {
	c := s.converter.Findings()
	if t == nil || c == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, f := range t.filters {
		path := datasetFindingPath(t.resourceType, f.field)
		if f.coded {
			// the path the profile binding and the unmapped-code findings use, so the
			// dashboard can offer the concept map that would deliver the missing code
			path += ".coding.code"
		}
		msg := datasetMessage(ds, f.by, "code filtered on ("+f.param+") occurs in no exported resource, needed")
		for _, want := range f.want {
			if !t.seen[i][want] {
				c.Add(findings.Warning, findings.CodeDataDatasetCode, t.resourceType, "", path, msg, "", want)
			}
		}
	}
}
