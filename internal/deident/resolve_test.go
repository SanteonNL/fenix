package deident

import "testing"

func baseRuleset() Ruleset {
	rs, err := DefaultRuleset()
	if err != nil {
		panic(err)
	}
	return rs
}

func hasRule(rs Ruleset, path, action string) bool {
	for _, r := range rs.Rule {
		if r.Path == path && r.Action == action {
			return true
		}
	}
	return false
}

func countRules(rs Ruleset, path string) int {
	n := 0
	for _, r := range rs.Rule {
		if r.Path == path {
			n++
		}
	}
	return n
}

func TestResolveReplacesSamePathAndAction(t *testing.T) {
	rs, err := Resolve(baseRuleset(), []Rule{
		{Path: "Patient.birthDate", Action: ActionClampAge, MinAge: intPtr(0), MaxAge: intPtr(45), ExceptionReason: "maternity cohort"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if countRules(rs, "Patient.birthDate") != 2 {
		t.Fatalf("expected clamp-age to replace in place (2 rules on birthDate), got %d", countRules(rs, "Patient.birthDate"))
	}
	for _, r := range rs.Rule {
		if r.Path == "Patient.birthDate" && r.Action == ActionClampAge {
			if *r.MinAge != 0 || *r.MaxAge != 45 {
				t.Fatalf("clamp-age not replaced, got [%d,%d]", *r.MinAge, *r.MaxAge)
			}
		}
	}
}

func TestResolveNoneDisablesAllRulesOnPath(t *testing.T) {
	rs, err := Resolve(baseRuleset(), []Rule{
		{Path: "Patient.birthDate", Action: ActionNone, ExceptionReason: "not needed for this cohort"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if countRules(rs, "Patient.birthDate") != 1 {
		t.Fatalf("expected none to disable every base rule on the path, got %d rules", countRules(rs, "Patient.birthDate"))
	}
	if !hasRule(rs, "Patient.birthDate", ActionNone) {
		t.Fatal("expected the single remaining rule to be none")
	}
}

func TestResolveAddsNewPath(t *testing.T) {
	rs, err := Resolve(baseRuleset(), []Rule{
		{Path: "Observation.code", Action: ActionNone, ExceptionReason: "coded concept"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !hasRule(rs, "Observation.code", ActionNone) {
		t.Fatal("expected new path to be added")
	}
	if countRules(rs, "Patient.id") != 1 {
		t.Fatal("expected unrelated base rules to be untouched")
	}
}

func TestResolveNewActionStacksOnExistingPath(t *testing.T) {
	// Adding a *different* action on a path that already has rules should
	// stack, not replace — this is what lets shift+first-of-month+clamp-age
	// coexist on Patient.birthDate in the base ruleset itself.
	rs, err := Resolve(baseRuleset(), []Rule{
		{Path: "Patient.birthDate", Action: ActionShift, MaxDays: intPtr(5)},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if countRules(rs, "Patient.birthDate") != 3 {
		t.Fatalf("expected the new shift rule to stack alongside the existing 2, got %d", countRules(rs, "Patient.birthDate"))
	}
}

func TestResolveRejectsInvalidOverride(t *testing.T) {
	_, err := Resolve(baseRuleset(), []Rule{
		{Path: "Observation.code", Action: ActionNone}, // missing exceptionReason
	})
	if err == nil {
		t.Fatal("expected an invalid override to fail resolution")
	}
}
