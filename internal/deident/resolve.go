package deident

import "fmt"

// Resolve merges a base ruleset with per-export overrides into a new,
// fully-resolved effective Ruleset, per the documented path+action semantics
// (docs/fenix_architecture.md, "Two-layer configuration"):
//
//   - An override with the same path AND action as a base rule replaces it.
//   - action: "none" on a path disables every existing rule on that path
//     (regardless of their action) and replaces them with the single none
//     rule — this is what lets an override "turn off" a base rule whose
//     action it doesn't otherwise know.
//   - Any other new path+action combination is appended, stacking alongside
//     whatever else already targets that path (e.g. adding a second date
//     rule next to an existing first-of-month rule on the same element).
//
// The result is validated before being returned.
func Resolve(base Ruleset, overrides []Rule) (Ruleset, error) {
	result := Ruleset{
		Id:      base.Id,
		Version: base.Version,
		URL:     base.URL,
		Rule:    append([]Rule{}, base.Rule...),
	}

	for _, ov := range overrides {
		if ov.Action == ActionNone {
			kept := result.Rule[:0]
			for _, r := range result.Rule {
				if r.Path != ov.Path {
					kept = append(kept, r)
				}
			}
			result.Rule = append(kept, ov)
			continue
		}

		replaced := false
		for i, r := range result.Rule {
			if r.Path == ov.Path && r.Action == ov.Action {
				result.Rule[i] = ov
				replaced = true
				break
			}
		}
		if !replaced {
			result.Rule = append(result.Rule, ov)
		}
	}

	if err := result.Validate(); err != nil {
		return Ruleset{}, fmt.Errorf("deident: resolved ruleset is invalid: %w", err)
	}
	return result, nil
}
