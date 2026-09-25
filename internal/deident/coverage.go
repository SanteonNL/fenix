package deident

import (
	"sort"
	"strings"
)

// Coverage records the rule(s) that cover one walked field, already ordered
// for execution.
type Coverage struct {
	Path  string
	Rules []Rule
}

// actionOrder is the canonical execution order when more than one rule
// targets the same element (docs/fenix_architecture.md, "Applying multiple
// rules to one element"): shift → first-of-month → clamp-age. Actions
// outside this list (hash, none) never stack with these on a realistic
// path, so they keep their relative authoring order (stable sort).
var actionOrder = map[string]int{
	ActionShift:        0,
	ActionFirstOfMonth: 1,
	ActionClampAge:     2,
}

func sortForExecution(rules []Rule) []Rule {
	sorted := append([]Rule{}, rules...)
	sort.SliceStable(sorted, func(i, j int) bool {
		oi, oki := actionOrder[sorted[i].Action]
		oj, okj := actionOrder[sorted[j].Action]
		if !oki {
			oi = len(actionOrder)
		}
		if !okj {
			oj = len(actionOrder)
		}
		return oi < oj
	})
	return sorted
}

func rulesForPath(rs Ruleset, path string) []Rule {
	var out []Rule
	for _, r := range rs.Rule {
		if r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// bestAncestorPath returns the longest rule path that is a proper ancestor
// of path (i.e. path starts with rulePath+"."), or "" if none.
func bestAncestorPath(rs Ruleset, path string) string {
	best := ""
	for _, r := range rs.Rule {
		if isDateWildcard(r.Path) || r.Path == "" {
			continue
		}
		if strings.HasPrefix(path, r.Path+".") && len(r.Path) > len(best) {
			best = r.Path
		}
	}
	return best
}

// rulesHashingResourceType returns every hash rule whose own resource type
// (from its path, e.g. "Patient" from "Patient.id") matches resourceType.
// A reference field pointing at that type is covered by these rules
// automatically — there is no separate declaration (e.g. a propagateTo
// path list) to author or maintain, because there is never a legitimate
// reason to hash a resource's id but leave some references to it
// unrewritten: that would either leak the original id or break referential
// integrity. Every "Patient/X" reference anywhere, once Patient.id is
// hashed, is rewritten the same way.
func rulesHashingResourceType(rs Ruleset, resourceType string) []Rule {
	var out []Rule
	for _, r := range rs.Rule {
		if r.Action == ActionHash && ruleResourceType(r.Path) == resourceType {
			out = append(out, r)
		}
	}
	return out
}

// CheckCoverage finds, for each field, the rule(s) that cover it:
//
//   - An exact path match wins outright — every rule sharing that exact
//     path stacks (in canonical execution order).
//   - Otherwise, the deepest ancestor rule wins, covering the whole subtree
//     beneath it (a rule on Observation.code covers Observation.code.coding.system).
//   - Otherwise, for a reference field, a hash rule on the id of the
//     resource type it actually points at covers it (see
//     rulesHashingResourceType) — no separate declaration needed.
//   - Otherwise, the **.ofType(date) wildcard applies if the field's Kind is
//     a date.
//   - A field matched by none of the above is uncovered.
func CheckCoverage(rs Ruleset, fields []Field) (report []Coverage, uncovered []string) {
	for _, f := range fields {
		if rules := rulesForPath(rs, f.Path); len(rules) > 0 {
			report = append(report, Coverage{Path: f.Path, Rules: sortForExecution(rules)})
			continue
		}

		if best := bestAncestorPath(rs, f.Path); best != "" {
			report = append(report, Coverage{Path: f.Path, Rules: sortForExecution(rulesForPath(rs, best))})
			continue
		}

		if f.Kind == KindReference {
			if v, ok := f.Get(); ok {
				if rules := rulesHashingResourceType(rs, refType(v)); len(rules) > 0 {
					report = append(report, Coverage{Path: f.Path, Rules: rules})
					continue
				}
			}
		}

		if f.Kind == KindDate || f.Kind == KindDateString {
			if rules := rulesForPath(rs, "**.ofType(date)"); len(rules) > 0 {
				report = append(report, Coverage{Path: f.Path, Rules: sortForExecution(rules)})
				continue
			}
		}

		uncovered = append(uncovered, f.Path)
	}
	return report, uncovered
}
