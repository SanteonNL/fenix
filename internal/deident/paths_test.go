package deident

import (
	"testing"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

func fieldByPath(fields []Field, path string) (Field, bool) {
	for _, f := range fields {
		if f.Path == path {
			return f, true
		}
	}
	return Field{}, false
}

func TestWalkPatientLeafFields(t *testing.T) {
	bd, _ := fhir.ParseDate("1990-06-15")
	patient := &fhir.Patient{
		Id:        strPtr("abc123"),
		BirthDate: &bd,
		Gender:    genderPtr(fhir.AdministrativeGenderMale),
		Identifier: []fhir.Identifier{
			{Value: strPtr("id-1")},
			{Value: strPtr("id-2")},
		},
		// Envelope fields that must NOT be walked:
		Meta:      &fhir.Meta{VersionId: strPtr("1")},
		Text:      &fhir.Narrative{Div: "<div/>"},
		Extension: []fhir.Extension{{Url: "http://example.org/ext", ValueString: strPtr("x")}},
	}

	fields := Walk("Patient", patient)

	if f, ok := fieldByPath(fields, "Patient.id"); !ok || f.Kind != KindString {
		t.Fatalf("expected Patient.id as KindString, got %+v ok=%v", f, ok)
	}
	if f, ok := fieldByPath(fields, "Patient.birthDate"); !ok || f.Kind != KindDate {
		t.Fatalf("expected Patient.birthDate as KindDate, got %+v ok=%v", f, ok)
	}
	if f, ok := fieldByPath(fields, "Patient.gender"); !ok || f.Kind != KindString {
		t.Fatalf("expected Patient.gender as KindString, got %+v ok=%v", f, ok)
	}

	var idFields []Field
	for _, f := range fields {
		if f.Path == "Patient.identifier" {
			idFields = append(idFields, f)
		}
	}
	if len(idFields) != 2 {
		t.Fatalf("expected 2 Patient.identifier fields (one per Identifier), got %d", len(idFields))
	}
	for _, f := range idFields {
		if f.Kind != KindIdentifierSlice {
			t.Fatalf("expected KindIdentifierSlice, got %v", f.Kind)
		}
	}

	for _, skipped := range []string{"Patient.meta", "Patient.text", "Patient.extension"} {
		if _, ok := fieldByPath(fields, skipped); ok {
			t.Fatalf("expected %q to be excluded from the walk", skipped)
		}
	}
}

func TestWalkNilFieldsAreAbsent(t *testing.T) {
	patient := &fhir.Patient{Id: strPtr("abc123")}
	fields := Walk("Patient", patient)
	if _, ok := fieldByPath(fields, "Patient.birthDate"); ok {
		t.Fatal("expected a nil BirthDate to be absent from the walk")
	}
	if _, ok := fieldByPath(fields, "Patient.gender"); ok {
		t.Fatal("expected a nil Gender to be absent from the walk")
	}
}

func TestWalkRecursesIntoCompositeTypes(t *testing.T) {
	obs := &fhir.Observation{
		Id:     strPtr("obs-1"),
		Status: fhir.ObservationStatusFinal,
		Code: fhir.CodeableConcept{
			Text:   strPtr("Body weight"),
			Coding: []fhir.Coding{{System: strPtr("http://loinc.org"), Code: strPtr("29463-7")}},
		},
		Subject: &fhir.Reference{Reference: strPtr("Patient/abc123")},
	}
	fields := Walk("Observation", obs)

	for _, path := range []string{"Observation.code.text", "Observation.code.coding.system", "Observation.code.coding.code"} {
		if _, ok := fieldByPath(fields, path); !ok {
			t.Fatalf("expected recursion to reach %q, fields=%+v", path, fields)
		}
	}
	f, ok := fieldByPath(fields, "Observation.subject")
	if !ok || f.Kind != KindReference {
		t.Fatalf("expected Observation.subject as an opaque KindReference leaf, got %+v ok=%v", f, ok)
	}
	if _, ok := fieldByPath(fields, "Observation.subject.reference"); ok {
		t.Fatal("expected Reference to be terminal — its own sub-fields must not be walked")
	}
}

func TestWalkClassifiesPeriodStartEndAsDateStrings(t *testing.T) {
	cp := &fhir.CarePlan{
		Id:      strPtr("cp-1"),
		Subject: fhir.Reference{Reference: strPtr("Patient/abc123")},
		Period:  &fhir.Period{Start: strPtr("2024-01-01"), End: strPtr("2024-06-01")},
	}
	fields := Walk("CarePlan", cp)
	for _, path := range []string{"CarePlan.period.start", "CarePlan.period.end"} {
		f, ok := fieldByPath(fields, path)
		if !ok || f.Kind != KindDateString {
			t.Fatalf("expected %q as KindDateString, got %+v ok=%v", path, f, ok)
		}
	}
}

func TestWalkSetRoundTrips(t *testing.T) {
	patient := &fhir.Patient{Id: strPtr("abc123")}
	fields := Walk("Patient", patient)
	f, ok := fieldByPath(fields, "Patient.id")
	if !ok {
		t.Fatal("expected Patient.id to be present")
	}
	f.Set("hashed-value")
	if patient.Id == nil || *patient.Id != "hashed-value" {
		t.Fatalf("expected Set to mutate the underlying struct, got %v", patient.Id)
	}
	if v, ok := f.Get(); !ok || v != "hashed-value" {
		t.Fatalf("expected Get to reflect the mutation, got %q ok=%v", v, ok)
	}
}

func genderPtr(g fhir.AdministrativeGender) *fhir.AdministrativeGender { return &g }

// TestWalkSurfacesCodedEnums locks in a real bug found while testing: FHIR
// code enums (AdministrativeGender, ObservationStatus, ...) are generated as
// named int types with custom JSON marshaling, not plain strings — a naive
// reflect.Kind()==String check silently drops them, meaning a field like
// Patient.gender would never be coverage-checked or transformable at all.
func TestWalkSurfacesCodedEnums(t *testing.T) {
	patient := &fhir.Patient{
		Id:     strPtr("abc123"),
		Gender: genderPtr(fhir.AdministrativeGenderFemale),
	}
	fields := Walk("Patient", patient)
	f, ok := fieldByPath(fields, "Patient.gender")
	if !ok {
		t.Fatal("expected Patient.gender (a code enum) to be surfaced by Walk")
	}
	if v, ok := f.Get(); !ok || v != "female" {
		t.Fatalf("Get() = %q, %v; want \"female\", true", v, ok)
	}
	f.Set("male")
	if patient.Gender == nil || *patient.Gender != fhir.AdministrativeGenderMale {
		t.Fatalf("expected Set(\"male\") to mutate the underlying enum, got %v", patient.Gender)
	}
}
