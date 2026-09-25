package deident

import (
	"strings"
	"testing"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

func mustDate(t *testing.T, s string) fhir.Date {
	t.Helper()
	d, err := fhir.ParseDate(s)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", s, err)
	}
	return d
}

func TestDeidentifyEndToEnd(t *testing.T) {
	const origPatientID = "patient-1"
	const origIdentifier = "bsn-123456789"
	const origBirth = "1990-06-15"
	const origEffective = "2024-03-22T09:15:00+01:00" // a full dateTime, not just a date — the bug this locks in

	birth := mustDate(t, origBirth)
	effective, err := fhir.ParseDateTime(origEffective)
	if err != nil {
		t.Fatalf("ParseDateTime(%q): %v", origEffective, err)
	}

	patient := &fhir.Patient{
		Id:         strPtr(origPatientID),
		Identifier: []fhir.Identifier{{Value: strPtr(origIdentifier)}},
		BirthDate:  &birth,
	}
	obs := &fhir.Observation{
		Id:                strPtr("obs-1"),
		Status:            fhir.ObservationStatusFinal,
		Code:              fhir.CodeableConcept{Text: strPtr("Body weight")},
		Subject:           &fhir.Reference{Reference: strPtr("Patient/" + origPatientID)},
		EffectiveDateTime: &effective,
	}

	// santeon-default only names Patient.id/identifier/birthDate — a real
	// deployment adds its own rule for every other resource type's own id.
	// Observation.subject needs no separate rule: any reference to Patient
	// is covered automatically once Patient.id has a hash rule.
	rs, err := Resolve(baseRuleset(), []Rule{
		{Path: "Observation.id", Action: ActionNone, ExceptionReason: "server-internal id, not linkable without the (hashed) patient reference"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	ctx := RunContext{Key: []byte("test-key"), RunID: "run-1"}
	elements := map[string][]string{
		"Patient":     {"id", "identifier", "birthDate"},
		"Observation": {"id", "subject", "effectiveDateTime"},
	}

	resources := []Resource{{Type: "Patient", Value: patient}, {Type: "Observation", Value: obs}}
	cov, err := Deidentify(resources, rs, elements, ctx, nil)
	if err != nil {
		t.Fatalf("Deidentify() error = %v", err)
	}
	if len(cov) == 0 {
		t.Fatal("expected a non-empty coverage report")
	}

	// id hashed, and the Observation's reference to it rewritten to match.
	if patient.Id == nil || *patient.Id != ctx.Hash(origPatientID) {
		t.Fatalf("Patient.id = %v, want hash of %q", patient.Id, origPatientID)
	}
	if *patient.Id == origPatientID {
		t.Fatal("expected Patient.id to actually change")
	}
	if obs.Subject.Reference == nil || *obs.Subject.Reference != "Patient/"+*patient.Id {
		t.Fatalf("Observation.subject.reference = %v, want Patient/%s", obs.Subject.Reference, *patient.Id)
	}

	// identifier hashed.
	if patient.Identifier[0].Value == nil || *patient.Identifier[0].Value != ctx.Hash(origIdentifier) {
		t.Fatalf("Patient.identifier[0].value = %v, want hash of %q", patient.Identifier[0].Value, origIdentifier)
	}

	// birthDate: shift -> first-of-month -> clamp-age, keyed by the
	// patient's own (pre-hash) id.
	offset := ctx.ShiftOffsetDays(origPatientID, 15)
	wantBirth := ClampAge(FirstOfMonth(ShiftDate(mustParseDate(t, origBirth), offset)), 18, 85, time.Now())
	if patient.BirthDate.String() != wantBirth.Format(dateOnlyLayout) {
		t.Fatalf("Patient.birthDate = %s, want %s", patient.BirthDate.String(), wantBirth.Format(dateOnlyLayout))
	}

	// Observation.effectiveDateTime: shift only, by the SAME offset as its
	// subject's birthDate — proving the linkID resolved via the (pre-hash)
	// subject reference, not the observation's own id. It's a full dateTime
	// (time-of-day + timezone), which must survive the shift unchanged.
	origEffectiveTime, err := time.Parse(time.RFC3339, origEffective)
	if err != nil {
		t.Fatalf("parse origEffective: %v", err)
	}
	wantEffective := ShiftDate(origEffectiveTime, offset)
	gotEffective, err := time.Parse(time.RFC3339, obs.EffectiveDateTime.String())
	if err != nil {
		t.Fatalf("parse obs.EffectiveDateTime after Deidentify: %v", err)
	}
	if !gotEffective.Equal(wantEffective) {
		t.Fatalf("Observation.effectiveDateTime = %s, want %s", gotEffective, wantEffective)
	}
}

// TestDeidentifyHashesReferenceWithoutOwningResourcePresent locks in the
// fix for a real gap found by running the full CLI pipeline: the CLI and
// the live server each de-identify one resource type at a time, so a
// Patient is almost never in the same batch as an Observation referencing
// it. A reference must still be hashed correctly on its own — and its hash
// must equal what hashing that same id directly, in a wholly separate call
// with the same RunContext, would produce, so cross-file/cross-request
// output stays joinable without ever putting Patient and Observation in the
// same batch.
func TestDeidentifyHashesReferenceWithoutOwningResourcePresent(t *testing.T) {
	const patientID = "patient-1"
	rs := baseRuleset()
	ctx := RunContext{Key: []byte("test-key"), RunID: "run-1"}

	obs := &fhir.Observation{
		Id:      strPtr("obs-1"),
		Status:  fhir.ObservationStatusFinal,
		Code:    fhir.CodeableConcept{Text: strPtr("Body weight")},
		Subject: &fhir.Reference{Reference: strPtr("Patient/" + patientID)},
	}
	rsWithObsID, err := Resolve(rs, []Rule{
		{Path: "Observation.id", Action: ActionNone, ExceptionReason: "server-internal id"},
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	elements := map[string][]string{"Observation": {"id", "subject"}}

	// Only the Observation is in this batch — no Patient resource at all.
	if _, err := Deidentify([]Resource{{Type: "Observation", Value: obs}}, rsWithObsID, elements, ctx, nil); err != nil {
		t.Fatalf("Deidentify() error = %v", err)
	}

	want := "Patient/" + ctx.Hash(patientID)
	if obs.Subject.Reference == nil || *obs.Subject.Reference != want {
		t.Fatalf("Observation.subject.reference = %v, want %s", obs.Subject.Reference, want)
	}

	// A wholly separate Deidentify call, hashing Patient.id directly with
	// the same RunContext, must produce the identical value.
	patient := &fhir.Patient{Id: strPtr(patientID)}
	if _, err := Deidentify([]Resource{{Type: "Patient", Value: patient}}, rs, map[string][]string{"Patient": {"id"}}, ctx, nil); err != nil {
		t.Fatalf("Deidentify() error = %v", err)
	}
	if "Patient/"+*patient.Id != want {
		t.Fatalf("Patient.id hashed to %q, but the Observation's reference hashed to %q — these must match", *patient.Id, want)
	}
}

func TestDeidentifyUncoveredFieldsAreRejectedAndNothingMutated(t *testing.T) {
	patient := &fhir.Patient{
		Id:     strPtr("patient-1"),
		Gender: genderPtr(fhir.AdministrativeGenderFemale), // not covered by santeon-default
	}

	rs := baseRuleset()
	ctx := RunContext{Key: []byte("test-key"), RunID: "run-1"}

	_, err := Deidentify([]Resource{{Type: "Patient", Value: patient}}, rs, nil, ctx, nil)
	if err == nil {
		t.Fatal("expected an error for the uncovered Patient.gender field")
	}
	if !strings.Contains(err.Error(), "Patient.gender") {
		t.Fatalf("expected error to name Patient.gender, got: %v", err)
	}
	if *patient.Id != "patient-1" {
		t.Fatalf("expected no mutation when coverage fails, but Patient.id changed to %q", *patient.Id)
	}
}

func TestDeidentifyRefusesUntypedResource(t *testing.T) {
	rs := baseRuleset()
	ctx := RunContext{Key: []byte("test-key"), RunID: "run-1"}
	resources := []Resource{{Type: "RelatedPerson", Value: map[string]interface{}{"resourceType": "RelatedPerson"}}}

	_, err := Deidentify(resources, rs, nil, ctx, nil)
	if err == nil {
		t.Fatal("expected an error for a resource type with no typed FHIR model")
	}
	if !strings.Contains(err.Error(), "no typed FHIR model") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeidentifyLinkedPatientsShareShiftOffset(t *testing.T) {
	const motherID, childID = "mother-1", "child-1"
	const sharedAnchor = "family-42"
	const motherBirth, childBirth = "1985-01-10", "2020-05-20"

	mb := mustDate(t, motherBirth)
	cb := mustDate(t, childBirth)
	mother := &fhir.Patient{Id: strPtr(motherID), BirthDate: &mb}
	child := &fhir.Patient{Id: strPtr(childID), BirthDate: &cb}

	related := []fhir.RelatedPerson{
		{Patient: fhir.Reference{Reference: strPtr("Patient/" + motherID)}, Identifier: []fhir.Identifier{{Value: strPtr(sharedAnchor)}}},
		{Patient: fhir.Reference{Reference: strPtr("Patient/" + childID)}, Identifier: []fhir.Identifier{{Value: strPtr(sharedAnchor)}}},
	}

	rs := baseRuleset()
	ctx := RunContext{Key: []byte("test-key"), RunID: "run-1"}
	elements := map[string][]string{"Patient": {"id", "birthDate"}}

	resources := []Resource{{Type: "Patient", Value: mother}, {Type: "Patient", Value: child}}
	if _, err := Deidentify(resources, rs, elements, ctx, related); err != nil {
		t.Fatalf("Deidentify() error = %v", err)
	}

	// Both patients must have been shifted by the same offset (derived from
	// their shared RelatedPerson anchor, not their own ids) before
	// first-of-month/clamp-age reshape the result further.
	offset := ctx.ShiftOffsetDays(sharedAnchor, 15)
	wantMother := ClampAge(FirstOfMonth(ShiftDate(mustParseDate(t, motherBirth), offset)), 18, 85, time.Now())
	wantChild := ClampAge(FirstOfMonth(ShiftDate(mustParseDate(t, childBirth), offset)), 18, 85, time.Now())

	if mother.BirthDate.String() != wantMother.Format(dateOnlyLayout) {
		t.Fatalf("mother.BirthDate = %s, want %s", mother.BirthDate.String(), wantMother.Format(dateOnlyLayout))
	}
	if child.BirthDate.String() != wantChild.Format(dateOnlyLayout) {
		t.Fatalf("child.BirthDate = %s, want %s (offset %d shared with mother)", child.BirthDate.String(), wantChild.Format(dateOnlyLayout), offset)
	}
}
