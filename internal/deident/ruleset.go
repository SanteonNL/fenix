// Package deident implements the SIM on FHIR de-identification model: a
// versioned ruleset of hash/shift/clamp-age/first-of-month/none actions,
// resolved from a base ruleset plus per-export overrides, applied to FHIR
// resources with coverage checking (see docs/fenix_architecture.md, ❻
// De-identification).
package deident

import "fmt"

// Action names, as they appear in a Rule's Action field and in the spec.
const (
	ActionNone         = "none"
	ActionHash         = "hash"
	ActionShift        = "shift"
	ActionClampAge     = "clamp-age"
	ActionFirstOfMonth = "first-of-month"
)

// Rule is one de-identification rule, matching the DeidentificationRuleset
// Logical Model's rule element. JSON field names are camelCase to match the
// spec exactly, so a resolved Ruleset is spec-conformant on disk.
type Rule struct {
	Path            string `json:"path"`
	Action          string `json:"action"`
	ExceptionReason string `json:"exceptionReason,omitempty"`
	Algorithm       string `json:"algorithm,omitempty"`
	MaxDays         *int   `json:"maxDays,omitempty"`
	MinAge          *int   `json:"minAge,omitempty"`
	MaxAge          *int   `json:"maxAge,omitempty"`
}

// Ruleset is a fully-resolved (effective) or a base set of de-identification
// rules, matching the DeidentificationRuleset Logical Model.
type Ruleset struct {
	Id      string `json:"id"`
	Version string `json:"version"`
	URL     string `json:"url,omitempty"`
	Rule    []Rule `json:"rule"`
}

// Validate enforces the per-action invariants from the spec:
//   - none-requires-reason
//   - hash-requires-algorithm (and no date/age params)
//   - shift-requires-maxdays (and no hash/age params)
//   - clampage-requires-bounds (both minAge and maxAge)
func (r Rule) Validate() error {
	if r.Path == "" {
		return fmt.Errorf("rule: path is required")
	}
	switch r.Action {
	case ActionNone:
		if r.ExceptionReason == "" {
			return fmt.Errorf("rule %q: action %q requires exceptionReason", r.Path, r.Action)
		}
		if r.Algorithm != "" || r.MaxDays != nil || r.MinAge != nil || r.MaxAge != nil {
			return fmt.Errorf("rule %q: action %q must carry no transform parameters", r.Path, r.Action)
		}
	case ActionHash:
		if r.Algorithm == "" {
			return fmt.Errorf("rule %q: action %q requires algorithm", r.Path, r.Action)
		}
		if r.MaxDays != nil || r.MinAge != nil || r.MaxAge != nil {
			return fmt.Errorf("rule %q: action %q must carry no date/age parameters", r.Path, r.Action)
		}
	case ActionShift:
		if r.MaxDays == nil {
			return fmt.Errorf("rule %q: action %q requires maxDays", r.Path, r.Action)
		}
		if *r.MaxDays <= 0 {
			return fmt.Errorf("rule %q: maxDays must be positive", r.Path)
		}
		if r.Algorithm != "" || r.MinAge != nil || r.MaxAge != nil {
			return fmt.Errorf("rule %q: action %q must carry no hash/age parameters", r.Path, r.Action)
		}
	case ActionClampAge:
		if r.MinAge == nil || r.MaxAge == nil {
			return fmt.Errorf("rule %q: action %q requires both minAge and maxAge", r.Path, r.Action)
		}
		if *r.MinAge < 0 || *r.MaxAge < *r.MinAge {
			return fmt.Errorf("rule %q: invalid age bounds [%d,%d]", r.Path, *r.MinAge, *r.MaxAge)
		}
		if r.Algorithm != "" || r.MaxDays != nil {
			return fmt.Errorf("rule %q: action %q must carry no hash/date parameters", r.Path, r.Action)
		}
	case ActionFirstOfMonth:
		if r.Algorithm != "" || r.MaxDays != nil || r.MinAge != nil || r.MaxAge != nil {
			return fmt.Errorf("rule %q: action %q must carry no parameters", r.Path, r.Action)
		}
	default:
		return fmt.Errorf("rule %q: unknown action %q", r.Path, r.Action)
	}
	return nil
}

// Validate runs Rule.Validate over every rule in the set.
func (rs Ruleset) Validate() error {
	if rs.Id == "" {
		return fmt.Errorf("ruleset: id is required")
	}
	if len(rs.Rule) == 0 {
		return fmt.Errorf("ruleset %q: must carry at least one rule", rs.Id)
	}
	for _, r := range rs.Rule {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("ruleset %q: %w", rs.Id, err)
		}
	}
	return nil
}

// isDateWildcard reports whether path is the special wildcard that matches
// every date-typed element, regardless of its position in the resource tree.
func isDateWildcard(path string) bool {
	return path == "**.ofType(date)"
}
