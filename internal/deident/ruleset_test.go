package deident

import "testing"

func intPtr(i int) *int { return &i }

func TestRuleValidate(t *testing.T) {
	tests := []struct {
		name    string
		rule    Rule
		wantErr bool
	}{
		{"none with reason ok", Rule{Path: "Observation.code", Action: ActionNone, ExceptionReason: "coded, no risk"}, false},
		{"none without reason fails", Rule{Path: "Observation.code", Action: ActionNone}, true},
		{"none with stray params fails", Rule{Path: "Observation.code", Action: ActionNone, ExceptionReason: "x", MaxDays: intPtr(1)}, true},

		{"hash with algorithm ok", Rule{Path: "Patient.id", Action: ActionHash, Algorithm: "hmac-sha256"}, false},
		{"hash without algorithm fails", Rule{Path: "Patient.id", Action: ActionHash}, true},
		{"hash with age params fails", Rule{Path: "Patient.id", Action: ActionHash, Algorithm: "hmac-sha256", MinAge: intPtr(1)}, true},

		{"shift with maxDays ok", Rule{Path: "**.ofType(date)", Action: ActionShift, MaxDays: intPtr(15)}, false},
		{"shift without maxDays fails", Rule{Path: "**.ofType(date)", Action: ActionShift}, true},
		{"shift with zero maxDays fails", Rule{Path: "**.ofType(date)", Action: ActionShift, MaxDays: intPtr(0)}, true},
		{"shift with hash params fails", Rule{Path: "**.ofType(date)", Action: ActionShift, MaxDays: intPtr(1), Algorithm: "hmac-sha256"}, true},

		{"clamp-age with bounds ok", Rule{Path: "Patient.birthDate", Action: ActionClampAge, MinAge: intPtr(18), MaxAge: intPtr(85)}, false},
		{"clamp-age missing maxAge fails", Rule{Path: "Patient.birthDate", Action: ActionClampAge, MinAge: intPtr(18)}, true},
		{"clamp-age missing minAge fails", Rule{Path: "Patient.birthDate", Action: ActionClampAge, MaxAge: intPtr(85)}, true},
		{"clamp-age inverted bounds fails", Rule{Path: "Patient.birthDate", Action: ActionClampAge, MinAge: intPtr(85), MaxAge: intPtr(18)}, true},

		{"first-of-month ok", Rule{Path: "Patient.birthDate", Action: ActionFirstOfMonth}, false},
		{"first-of-month with stray param fails", Rule{Path: "Patient.birthDate", Action: ActionFirstOfMonth, MaxDays: intPtr(1)}, true},

		{"unknown action fails", Rule{Path: "Patient.id", Action: "encrypt"}, true},
		{"missing path fails", Rule{Action: ActionNone, ExceptionReason: "x"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRulesetValidate(t *testing.T) {
	valid := Ruleset{Id: "x", Rule: []Rule{{Path: "Patient.id", Action: ActionHash, Algorithm: "hmac-sha256"}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid ruleset, got %v", err)
	}

	if err := (Ruleset{Rule: valid.Rule}).Validate(); err == nil {
		t.Fatal("expected error for missing ruleset id")
	}
	if err := (Ruleset{Id: "x"}).Validate(); err == nil {
		t.Fatal("expected error for empty rule set")
	}
	invalid := Ruleset{Id: "x", Rule: []Rule{{Path: "Patient.id", Action: ActionHash}}}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected error to propagate from an invalid rule")
	}
}

func TestDefaultRulesetEmbedIsValid(t *testing.T) {
	rs, err := DefaultRuleset()
	if err != nil {
		t.Fatalf("DefaultRuleset() error = %v", err)
	}
	if rs.Id != "santeon-default" {
		t.Fatalf("unexpected id %q", rs.Id)
	}
	if len(rs.Rule) != 5 {
		t.Fatalf("expected 5 rules in santeon-default, got %d", len(rs.Rule))
	}
}
