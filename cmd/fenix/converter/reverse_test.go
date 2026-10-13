package converter

import (
	"reflect"
	"testing"

	"github.com/rs/zerolog"
)

// sim-observation-code.json maps MayoEndoscopicScore -> LA6751-7 for the
// Observation.code valueset; a search for the target code must find the
// source rows that hold the source code.
func TestReverseTranslate_SimObservationCode(t *testing.T) {
	svc := NewConceptMapService(zerolog.Nop())
	if err := svc.LoadJSON("../../../terminology/conceptmaps/fhir/sim-observation-code.json"); err != nil {
		t.Fatal(err)
	}
	const vs = "https://decor.nictiz.nl/fhir/4.0/sansa-/ValueSet/2.16.840.1.113883.2.4.3.11.60.909.11.2--20241203090354"

	got, exact, mapped := svc.ReverseTranslate(vs, "LA6751-7")
	if !mapped || !exact || !reflect.DeepEqual(got, []string{"MayoEndoscopicScore", "LA6751-7"}) {
		t.Fatalf("got (%v, exact=%v, mapped=%v)", got, exact, mapped)
	}

	// MCD_B is mapped to the made-up DEMO_B, which the group-demo uses
	got, _, _ = svc.ReverseTranslate(vs, "DEMO_B")
	if !reflect.DeepEqual(got, []string{"MCD_B", "DEMO_B"}) {
		t.Fatalf("got %v", got)
	}

	// an unmapped source code stays as is
	got, _, _ = svc.ReverseTranslate(vs, "MCD_A")
	if !reflect.DeepEqual(got, []string{"MCD_A"}) {
		t.Fatalf("got %v", got)
	}

	// no concept map for the valueset: not mapped, code compared as is
	if _, _, mapped := svc.ReverseTranslate("http://example.org/other", "X"); mapped {
		t.Fatal("expected mapped=false")
	}
}
