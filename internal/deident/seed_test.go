package deident

import (
	"testing"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

func TestHashDeterministicWithinRun(t *testing.T) {
	ctx := RunContext{Key: []byte("secret"), RunID: "run-1"}
	if ctx.Hash("patient-123") != ctx.Hash("patient-123") {
		t.Fatal("expected the same (key, runID, value) to hash identically")
	}
}

func TestHashUnlinkableAcrossRuns(t *testing.T) {
	a := RunContext{Key: []byte("secret"), RunID: "run-1"}
	b := RunContext{Key: []byte("secret"), RunID: "run-2"}
	if a.Hash("patient-123") == b.Hash("patient-123") {
		t.Fatal("expected the same patient to hash differently across runs (same key, different runID)")
	}
}

func TestHashDiffersByValue(t *testing.T) {
	ctx := RunContext{Key: []byte("secret"), RunID: "run-1"}
	if ctx.Hash("patient-123") == ctx.Hash("patient-456") {
		t.Fatal("expected different values to hash differently")
	}
}

func TestShiftOffsetDeterministicAndBounded(t *testing.T) {
	ctx := RunContext{Key: []byte("secret"), RunID: "run-1"}
	for _, linkID := range []string{"patient-1", "patient-2", "another-patient"} {
		off := ctx.ShiftOffsetDays(linkID, 15)
		if off == 0 {
			t.Fatalf("offset for %q must never be zero", linkID)
		}
		if off < -15 || off > 15 || off == 0 {
			t.Fatalf("offset %d for %q out of [-15,-1]∪[1,15]", off, linkID)
		}
		if ctx.ShiftOffsetDays(linkID, 15) != off {
			t.Fatalf("expected deterministic offset for %q within a run", linkID)
		}
	}
}

func TestShiftOffsetDiffersAcrossRuns(t *testing.T) {
	a := RunContext{Key: []byte("secret"), RunID: "run-1"}
	b := RunContext{Key: []byte("secret"), RunID: "run-2"}
	// Not guaranteed to differ for every possible linkID, but should differ
	// for at least one of a handful — proves the seed actually participates.
	same := 0
	for _, id := range []string{"p1", "p2", "p3", "p4", "p5"} {
		if a.ShiftOffsetDays(id, 15) == b.ShiftOffsetDays(id, 15) {
			same++
		}
	}
	if same == 5 {
		t.Fatal("expected at least one offset to differ across runs")
	}
}

func TestLinkIDSharedAcrossRelatedPatients(t *testing.T) {
	motherID := "mother-1"
	childID := "child-1"
	sharedIdentifier := "family-42"

	related := []fhir.RelatedPerson{
		{
			Patient:    fhir.Reference{Reference: strPtr("Patient/" + childID)},
			Identifier: []fhir.Identifier{{Value: strPtr(sharedIdentifier)}},
		},
		{
			Patient:    fhir.Reference{Reference: strPtr("Patient/" + motherID)},
			Identifier: []fhir.Identifier{{Value: strPtr(sharedIdentifier)}},
		},
	}

	if got := LinkID(motherID, related); got != sharedIdentifier {
		t.Fatalf("mother linkID = %q, want %q", got, sharedIdentifier)
	}
	if got := LinkID(childID, related); got != sharedIdentifier {
		t.Fatalf("child linkID = %q, want %q", got, sharedIdentifier)
	}
}

func TestLinkIDFallsBackToOwnID(t *testing.T) {
	if got := LinkID("solo-patient", nil); got != "solo-patient" {
		t.Fatalf("LinkID with no relations = %q, want the patient's own id", got)
	}
}

func strPtr(s string) *string { return &s }
