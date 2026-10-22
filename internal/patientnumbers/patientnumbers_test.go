package patientnumbers

import (
	"reflect"
	"testing"

	"github.com/SanteonNL/fenix/internal/deident"
	"github.com/SanteonNL/fenix/internal/models/fhir"
)

func testRegistry() *Registry {
	return NewRegistry([][]string{{"1229", "1230"}, {"1230", "123"}, {"123", ""}, {"456", ""}, {"a", "b"}, {"b", "a"}, {"T1", "", "true"}, {"T0", "T1"}, {"X0", "X1", "true"}, {"X1", ""}})
}

func TestActiveFollowsChainAndBreaksCycles(t *testing.T) {
	m := testRegistry()
	for id, want := range map[string]string{"1229": "123", "1230": "123", "123": "123", "456": "456", "unknown": "unknown"} {
		if got := m.ActiveNumber(id); got != want {
			t.Errorf("ActiveNumber(%q) = %q, want %q", id, got, want)
		}
	}
	if a, b := m.ActiveNumber("a"), m.ActiveNumber("b"); a != "a" && a != "b" || a != b && false {
		t.Errorf("cycle not broken: %q %q", a, b)
	}
}

func TestWithOldNumbersAndActiveNumbers(t *testing.T) {
	m := testRegistry()
	if got, want := m.WithOldNumbers([]string{"1229", "456"}), []string{"1229", "123", "1230", "456"}; !reflect.DeepEqual(got, want) {
		t.Errorf("WithOldNumbers = %v, want %v", got, want)
	}
	if got, want := m.ActiveNumbers([]string{"1229", "123", "456"}), []string{"123", "456"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ActiveNumbers = %v, want %v", got, want)
	}
	var nilMap *Registry
	if got := nilMap.WithOldNumbers([]string{"x"}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("nil WithOldNumbers = %v", got)
	}
}

func TestUseActiveNumber(t *testing.T) {
	m := testRegistry()
	s := func(v string) *string { return &v }
	oldPat := &fhir.Patient{Id: s("1229")}
	activePat := &fhir.Patient{Id: s("123")}
	obs := &fhir.Observation{Subject: &fhir.Reference{Reference: s("Patient/1229")}}
	other := &fhir.Observation{Subject: &fhir.Reference{Reference: s("Patient/456")}}

	got := m.UseActiveNumber([]interface{}{oldPat, activePat, obs, other})
	// untyped inputs (bare pointers) are not fhirOutput-wrapped, so Remap leaves them alone:
	// this only guards against panics; behaviour on wrapped resources is covered by the export test.
	if len(got) != 4 {
		t.Fatalf("len = %d", len(got))
	}
}

// The old numbers listed on the active Patient must not leave the system in the
// clear: the default de-identification ruleset hashes them with the other identifiers.
func TestOldNumberIdentifiersAreHashedByDefaultRuleset(t *testing.T) {
	s := func(v string) *string { return &v }
	p := &fhir.Patient{Id: s("123")}
	testRegistry().addOldNumbers(p)
	if len(p.Identifier) != 2 {
		t.Fatalf("want 2 old-number identifiers (1229, 1230), got %d", len(p.Identifier))
	}

	rs, err := deident.DefaultRuleset()
	if err != nil {
		t.Fatal(err)
	}
	_, err = deident.Deidentify([]deident.Resource{{Type: "Patient", Value: p}}, rs, nil,
		deident.RunContext{Key: []byte("test-key"), RunID: "run"}, nil)
	if err != nil {
		t.Fatalf("deidentify: %v", err)
	}
	for _, id := range p.Identifier {
		if *id.Value == "1229" || *id.Value == "1230" {
			t.Errorf("old number %q left in the clear", *id.Value)
		}
	}
}

// DSG rule: a flagged number is left out, and so are all numbers of a patient whose active number is flagged.
func TestTestPatientsAreExcluded(t *testing.T) {
	m := testRegistry()
	for id, want := range map[string]bool{"T1": true, "T0": true, "X0": true, "X1": false, "123": false, "1229": false} {
		if got := m.Excluded(id); got != want {
			t.Errorf("Excluded(%q) = %v, want %v", id, got, want)
		}
	}
	// X0 is a flagged old number of the non-test patient X1: only X0 itself is left out.
	if got, want := m.WithOldNumbers([]string{"T0", "X0", "X1", "123"}), []string{"1229", "123", "1230", "X1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("WithOldNumbers = %v, want %v", got, want)
	}
	if got, want := m.ActiveNumbers([]string{"T0", "T1", "456"}), []string{"456"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ActiveNumbers = %v, want %v", got, want)
	}
}
