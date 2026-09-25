package deident

import "testing"

func noopField(path string, kind FieldKind) Field {
	return Field{Path: path, Kind: kind, Get: func() (string, bool) { return "", true }, Set: func(string) {}}
}

func TestCheckCoverageExactPathWins(t *testing.T) {
	rs := Ruleset{Id: "x", Rule: []Rule{
		{Path: "Patient.birthDate", Action: ActionFirstOfMonth},
		{Path: "**.ofType(date)", Action: ActionShift, MaxDays: intPtr(15)},
	}}
	report, uncovered := CheckCoverage(rs, []Field{noopField("Patient.birthDate", KindDate)})
	if len(uncovered) != 0 {
		t.Fatalf("expected no uncovered paths, got %v", uncovered)
	}
	if len(report) != 1 || len(report[0].Rules) != 1 || report[0].Rules[0].Action != ActionFirstOfMonth {
		t.Fatalf("expected the exact-path rule to win over the wildcard, got %+v", report)
	}
}

func TestCheckCoverageStacksMultipleRulesOnExactPath(t *testing.T) {
	rs := baseRuleset()
	report, uncovered := CheckCoverage(rs, []Field{noopField("Patient.birthDate", KindDate)})
	if len(uncovered) != 0 {
		t.Fatalf("expected no uncovered paths, got %v", uncovered)
	}
	if len(report[0].Rules) != 2 {
		t.Fatalf("expected 2 stacked rules on Patient.birthDate, got %d", len(report[0].Rules))
	}
	if report[0].Rules[0].Action != ActionFirstOfMonth || report[0].Rules[1].Action != ActionClampAge {
		t.Fatalf("expected canonical execution order [first-of-month, clamp-age], got %+v", report[0].Rules)
	}
}

func TestCheckCoverageAncestorCoversSubtree(t *testing.T) {
	rs := Ruleset{Id: "x", Rule: []Rule{
		{Path: "Observation.code", Action: ActionNone, ExceptionReason: "coded concept"},
	}}
	report, uncovered := CheckCoverage(rs, []Field{
		noopField("Observation.code.coding.system", KindString),
		noopField("Observation.code.text", KindString),
	})
	if len(uncovered) != 0 {
		t.Fatalf("expected the ancestor rule to cover the whole subtree, got uncovered %v", uncovered)
	}
	for _, c := range report {
		if len(c.Rules) != 1 || c.Rules[0].Action != ActionNone {
			t.Fatalf("expected every descendant to resolve to the single none rule, got %+v", c)
		}
	}
}

func TestCheckCoverageMostSpecificAncestorWins(t *testing.T) {
	rs := Ruleset{Id: "x", Rule: []Rule{
		{Path: "Observation", Action: ActionNone, ExceptionReason: "broad"},
		{Path: "Observation.code.coding.code", Action: ActionHash, Algorithm: "hmac-sha256"},
	}}
	report, uncovered := CheckCoverage(rs, []Field{
		noopField("Observation.code.text", KindString),          // covered by the broad Observation rule
		noopField("Observation.code.coding.code", KindString),   // covered by the more specific rule
		noopField("Observation.code.coding.system", KindString), // covered by the broad Observation rule
	})
	if len(uncovered) != 0 {
		t.Fatalf("expected no uncovered paths, got %v", uncovered)
	}
	got := map[string]string{}
	for _, c := range report {
		got[c.Path] = c.Rules[0].Action
	}
	if got["Observation.code.text"] != ActionNone {
		t.Fatalf("expected code.text to fall back to the broad ancestor rule, got %v", got["Observation.code.text"])
	}
	if got["Observation.code.coding.code"] != ActionHash {
		t.Fatalf("expected coding.code to use the more specific rule, got %v", got["Observation.code.coding.code"])
	}
}

func TestCheckCoverageWildcardOnlyMatchesDateKinds(t *testing.T) {
	rs := Ruleset{Id: "x", Rule: []Rule{
		{Path: "**.ofType(date)", Action: ActionShift, MaxDays: intPtr(15)},
	}}
	_, uncovered := CheckCoverage(rs, []Field{noopField("Observation.status", KindString)})
	if len(uncovered) != 1 {
		t.Fatalf("expected a non-date field to be uncovered by the date wildcard, got %v", uncovered)
	}
}

func TestCheckCoverageUncovered(t *testing.T) {
	rs := Ruleset{Id: "x", Rule: []Rule{{Path: "Patient.id", Action: ActionHash, Algorithm: "hmac-sha256"}}}
	_, uncovered := CheckCoverage(rs, []Field{noopField("Patient.gender", KindString)})
	if len(uncovered) != 1 || uncovered[0] != "Patient.gender" {
		t.Fatalf("expected Patient.gender to be uncovered, got %v", uncovered)
	}
}
