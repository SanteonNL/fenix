package fhirserver

import (
	"testing"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

func TestExtractMemberFiltersBothForms(t *testing.T) {
	g, err := fhir.UnmarshalGroup([]byte(`{"resourceType":"Group","type":"person","actual":true,
		"modifierExtension":[{"url":"http://hl7.org/fhir/uv/bulkdata/StructureDefinition/member-filter",
			"valueExpression":{"language":"application/x-fhir-query","expression":"Patient?birthdate=le{today-18y}"}}],
		"extension":[{"url":"http://hl7.org/fhir/uv/bulkdata/StructureDefinition/member-filter","valueString":"Observation?status=final"},
			{"url":"http://example.org/other","valueString":"ignored"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := extractMemberFilters(&g)
	if len(got) != 2 || got[0] != "Patient?birthdate=le{today-18y}" || got[1] != "Observation?status=final" {
		t.Fatalf("got %v", got)
	}
}

func TestFillToday(t *testing.T) {
	today := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{
		"Patient?birthdate=le{today-18y}":         "Patient?birthdate=le2008-11-20",
		"Observation?date=ge{today-90d}":          "Observation?date=ge2026-08-22",
		"Observation?date=lt{today+3m}&x={today}": "Observation?date=lt2027-02-20&x=2026-11-20",
		"Patient?birthdate=le2000-01-01":          "Patient?birthdate=le2000-01-01",
	} {
		if got := fillToday(in, today); got != want {
			t.Errorf("fillToday(%q) = %q, want %q", in, got, want)
		}
	}
	rt, params, err := parseMemberFilter("Patient?birthdate=le{today+1y}")
	if err != nil || rt != "Patient" || len(params["birthdate"]) != len("le2006-01-02") {
		t.Fatalf("a '+' placeholder must survive query parsing: %q %v %v", rt, params, err)
	}
}
