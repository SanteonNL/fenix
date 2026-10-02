package converter

import (
	"testing"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// Regression test: coerceSQLiteTypes used to coerce ANY string value of
// "1"/"0"/"true"/"false" to a bool, regardless of which field it was in.
// An id of "1" (a very ordinary value — see queries/sim/fhir/observation.sql's
// MetingID) was silently turned into the boolean true, which then failed
// struct validation and got the whole resource dropped. The fix makes the
// coercion type-aware: only a field the FHIR struct actually declares bool
// gets coerced.
func TestValidateThroughStruct_IDLooksLikeBoolean(t *testing.T) {
	raw := map[string]interface{}{
		"resourceType": "Observation",
		"id":           "1",
		"status":       "final",
		"subject":      map[string]interface{}{"reference": "Patient/123"},
	}

	result, err := validateThroughStruct(raw, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	obs, ok := result.(*fhir.Observation)
	if !ok {
		t.Fatalf("expected *fhir.Observation, got %T", result)
	}
	if obs.Id == nil || *obs.Id != "1" {
		t.Fatalf("expected Id %q (string), got %v", "1", obs.Id)
	}
}

// A genuinely boolean field ("active") with the same kind of SQLite string
// value ("true"/"1") must still coerce correctly — the fix narrows the
// coercion, it doesn't disable it.
func TestValidateThroughStruct_GenuineBooleanStillCoerces(t *testing.T) {
	raw := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "0", // also boolean-looking; must stay the string "0"
		"active":       "true",
		"gender":       "male",
	}

	result, err := validateThroughStruct(raw, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p, ok := result.(*fhir.Patient)
	if !ok {
		t.Fatalf("expected *fhir.Patient, got %T", result)
	}
	if p.Id == nil || *p.Id != "0" {
		t.Fatalf("expected Id %q (string), got %v", "0", p.Id)
	}
	if p.Active == nil || !*p.Active {
		t.Fatalf("expected Active=true, got %v", p.Active)
	}
}
