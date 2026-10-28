package converter

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SanteonNL/fenix/internal/findings"
	"github.com/rs/zerolog"
)

// two observation codes, each with its own valid answers, and a plain numeric one
func obsDefsFixture(t *testing.T) *ObservationDefinitions {
	t.Helper()
	cms := conceptMaps(t, nil)
	vsDir, odDir := t.TempDir(), t.TempDir()
	write := func(dir, name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(vsDir, "l.json", `{"resourceType":"ValueSet","id":"ans-l","url":"https://x/vs/ans-l","compose":{"include":[{"concept":[{"code":"L1"},{"code":"L2"}]}]}}`)
	write(vsDir, "p.json", `{"resourceType":"ValueSet","id":"ans-p","url":"https://x/vs/ans-p","compose":{"include":[{"concept":[{"code":"P1"}]}]}}`)
	write(odDir, "l.json", `{"resourceType":"ObservationDefinition","id":"od-l","code":{"coding":[{"system":"https://s","code":"MCD_L"}]},"permittedDataType":["CodeableConcept"],"validCodedValueSet":{"reference":"https://x/vs/ans-l"}}`)
	// a relative reference "ValueSet/<id>" works as well as a canonical URL
	write(odDir, "p.json", `{"resourceType":"ObservationDefinition","id":"od-p","code":{"coding":[{"system":"https://s","code":"MCD_P"}]},"permittedDataType":["CodeableConcept"],"validCodedValueSet":{"reference":"ValueSet/ans-p"}}`)
	write(odDir, "hb.json", `{"resourceType":"ObservationDefinition","id":"od-hb","code":{"coding":[{"system":"http://loinc.org","code":"718-7"}]},"permittedDataType":["Quantity"]}`)
	if err := cms.LoadValueSetsDir(vsDir); err != nil {
		t.Fatal(err)
	}
	od := NewObservationDefinitions(cms, zerolog.Nop())
	if err := od.LoadDir(odDir); err != nil {
		t.Fatal(err)
	}
	return od
}

func TestAnAnswerIsValidForOneObservationCodeAndWrongForAnother(t *testing.T) {
	od := obsDefsFixture(t)
	l := []codeRef{{"https://s", "MCD_L"}}
	p := []codeRef{{"https://s", "MCD_P"}}

	if v := od.Check(l, "CodeableConcept", []string{"L1"}); len(v) != 0 {
		t.Errorf("L1 is a valid answer for MCD_L: %+v", v)
	}
	// P1 is a fine answer for MCD_P, not for MCD_L
	if v := od.Check(p, "CodeableConcept", []string{"P1"}); len(v) != 0 {
		t.Errorf("P1 is a valid answer for MCD_P: %+v", v)
	}
	v := od.Check(l, "CodeableConcept", []string{"P1"})
	if len(v) != 1 || v[0].Kind != ObsViolationAnswer || v[0].ObsCode != "MCD_L" || v[0].Detail != "P1" || v[0].Allowed != "L1, L2" {
		t.Errorf("P1 for MCD_L: %+v", v)
	}
	// the wrong kind of value
	if v := od.Check(l, "Quantity", nil); len(v) != 1 || v[0].Kind != ObsViolationType || v[0].Detail != "Quantity" {
		t.Errorf("a number for MCD_L: %+v", v)
	}
	if v := od.Check([]codeRef{{"http://loinc.org", "718-7"}}, "CodeableConcept", []string{"A2"}); len(v) != 1 || v[0].Kind != ObsViolationType {
		t.Errorf("a coded answer for a numeric code: %+v", v)
	}
	// no definition for the code: nothing is checked
	if v := od.Check([]codeRef{{"", "OTHER"}}, "CodeableConcept", []string{"X"}); len(v) != 0 {
		t.Errorf("no definition: %+v", v)
	}
	if info := od.Describe("MCD_L"); !info.Found || info.ID != "od-l" || len(info.Answers) != 2 || info.Types[0] != "CodeableConcept" {
		t.Errorf("describe: %+v", info)
	}
}

func TestObservationFindingsNameTheCombination(t *testing.T) {
	fc := NewFHIRConverter(nil, zerolog.Nop(), nil, nil)
	fc.SetObservationDefinitions(obsDefsFixture(t))
	c := findings.NewCollector()
	fc.SetFindings(c)
	obs := func(id, code, answerCode string) {
		raw := map[string]any{"resourceType": "Observation", "id": id, "status": "final",
			"code":                 map[string]any{"coding": []any{map[string]any{"system": "https://s", "code": code}}},
			"valueCodeableConcept": map[string]any{"coding": []any{map[string]any{"code": answerCode}}}}
		fc.reportFindings(id, "Observation", raw, nil)
	}
	obs("1", "MCD_L", "P1")
	obs("2", "MCD_L", "P1")
	obs("3", "MCD_L", "L2") // fine
	obs("4", "MCD_P", "L1")
	obs("5", "MCD_P", "P1") // fine

	var f findings.Record
	n := 0
	for _, r := range c.Drain("run", "src", "q", time.Time{}) {
		if r.Kind == "finding" {
			f, n = r, n+1
		}
	}
	if n != 1 || f.Code != findings.CodeDataObsAnswer || f.Severity != findings.Warning || f.Count != 3 {
		t.Fatalf("%d findings, got %+v", n, f)
	}
	if len(f.Values) != 2 || f.Values[0].Value != "MCD_L: P1" || f.Values[0].Count != 2 || f.Values[1].Value != "MCD_P: L1" {
		t.Errorf("values: %+v", f.Values)
	}
}

// A concept map that targets the answer valueset of a definition turns a source answer into a valid
// one before the check, so it is no longer reported; answers without a mapping still are.
func TestAnAnswerCanBeMappedOntoAValidAnswer(t *testing.T) {
	od := obsDefsFixture(t)
	cmDir := t.TempDir()
	body := `{"resourceType":"ConceptMap","id":"a","status":"active","targetCanonical":"https://x/vs/ans-l",
	 "group":[{"target":"https://s","element":[{"code":"P1","target":[{"code":"L1","equivalence":"equivalent"}]}]}]}`
	if err := os.WriteFile(filepath.Join(cmDir, "a.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := od.maps.LoadDir(cmDir); err != nil {
		t.Fatal(err)
	}
	fc := NewFHIRConverter(nil, zerolog.Nop(), nil, od.maps)
	fc.SetObservationDefinitions(od)
	c := findings.NewCollector()
	fc.SetFindings(c)

	obs := func(id, answerCode string) map[string]any {
		raw := map[string]any{"resourceType": "Observation", "id": id, "status": "final",
			"code":                 map[string]any{"coding": []any{map[string]any{"system": "https://s", "code": "MCD_L"}}},
			"valueCodeableConcept": map[string]any{"coding": []any{map[string]any{"code": answerCode}}}}
		fc.mapObservationAnswers("Observation", raw)
		fc.reportFindings(id, "Observation", raw, nil)
		return raw
	}
	mapped := obs("1", "P1") // mapped: P1 -> L1
	obs("2", "P9")           // no mapping: reported

	coding := mapped["valueCodeableConcept"].(map[string]any)["coding"].([]any)[0].(map[string]any)
	if coding["code"] != "L1" {
		t.Errorf("P1 is mapped onto L1, got %v", coding["code"])
	}
	var got []findings.Record
	for _, r := range c.Drain("run", "src", "q", time.Time{}) {
		if r.Kind == "finding" {
			got = append(got, r)
		}
	}
	if len(got) != 1 || got[0].Count != 1 || got[0].Values[0].Value != "MCD_L: P9" {
		t.Errorf("only the unmapped answer is reported: %+v", got)
	}
	if info := od.maps.ValueSetURL("ValueSet/ans-l"); info != "https://x/vs/ans-l" {
		t.Errorf("a reference by id resolves to the canonical url: %q", info)
	}
}
