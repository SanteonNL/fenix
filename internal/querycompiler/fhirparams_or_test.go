package querycompiler

import "testing"

func TestTokenAlternatives(t *testing.T) {
	got := tokenAlternatives(`a|1,b|2,3,s|0313\|922`)
	want := [][2]string{{"a", "1"}, {"b", "2"}, {"", "3"}, {"s", "0313|922"}}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("alternative %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestBuildTemplateVarsOrList(t *testing.T) {
	idx := searchParamIndex{"Observation": {"code": {paramType: "token", expression: "Observation.code"}}}
	vars := buildTemplateVars("Observation", map[string]string{"code": "MCD_A,MCD_B"}, []string{"code"}, idx, nil)
	if vars["code_in"] != "'MCD_A', 'MCD_B'" {
		t.Fatalf("code_in = %v", vars["code_in"])
	}
	if _, ok := vars["code"]; ok {
		t.Fatal("an OR list must not set the single-value var")
	}
	if _, ok := vars["code_system"]; ok {
		t.Fatal("an OR list must not set a single system")
	}
	single := buildTemplateVars("Observation", map[string]string{"code": "http://loinc.org|38445-3"}, []string{"code"}, idx, nil)
	if single["code_in"] != "'38445-3'" || single["code_system"] != "http://loinc.org" {
		t.Fatalf("single value unchanged: %v", single)
	}
}
