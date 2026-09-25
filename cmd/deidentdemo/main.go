// Command deidentdemo is a small, runnable fixture that shows the effect of
// internal/deident on a realistic Patient + Observation pair: it prints the
// resources before de-identification, the coverage report, and the
// resources after — see docs/fenix_architecture.md, "❻ De-identification".
//
// Run it with:
//
//	go run ./cmd/deidentdemo
package main

import (
	"encoding/json"
	"fmt"

	"github.com/SanteonNL/fenix/internal/deident"
	"github.com/SanteonNL/fenix/internal/models/fhir"
)

func strPtr(s string) *string { return &s }

func printJSON(label string, v interface{}) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Printf("--- %s ---\n%s\n\n", label, b)
}

func main() {
	// A realistic Patient + Observation pair, as the converter would produce them.
	// effectiveDateTime is a full FHIR dateTime (time + timezone, not just a
	// date) — fhir.DateTime handles that; fhir.Date (birthDate) is date-only.
	birth, _ := fhir.ParseDate("1990-06-15")
	effective, _ := fhir.ParseDateTime("2024-03-22T09:15:00+01:00")
	gender := fhir.AdministrativeGenderMale

	patient := &fhir.Patient{
		Id:         strPtr("a3f2c1-patient"),
		Identifier: []fhir.Identifier{{System: strPtr("http://fhir.nl/fhir/NamingSystem/bsn"), Value: strPtr("123456789")}},
		BirthDate:  &birth,
		Gender:     &gender,
	}
	obs := &fhir.Observation{
		Id:     strPtr("9b1e4d-obs"),
		Status: fhir.ObservationStatusFinal,
		Code: fhir.CodeableConcept{
			Coding: []fhir.Coding{{System: strPtr("http://loinc.org"), Code: strPtr("29463-7"), Display: strPtr("Body weight")}},
		},
		Subject:           &fhir.Reference{Reference: strPtr("Patient/a3f2c1-patient")},
		EffectiveDateTime: &effective,
	}

	printJSON("Patient (before)", patient)
	printJSON("Observation (before)", obs)

	// santeon-default only names Patient.id/identifier/birthDate. A real
	// deployment adds rules for whatever else its queries select — here,
	// the coded/status fields the Observation carries.
	base, err := deident.DefaultRuleset()
	if err != nil {
		panic(err)
	}
	rs, err := deident.Resolve(base, []deident.Rule{
		{Path: "Observation.id", Action: deident.ActionNone, ExceptionReason: "server-internal id, not linkable without the (hashed) patient reference"},
		{Path: "Observation.status", Action: deident.ActionNone, ExceptionReason: "coded value, no identifying risk"},
		{Path: "Observation.code", Action: deident.ActionNone, ExceptionReason: "LOINC coded concept, exact value required for analysis"},
		{Path: "Patient.gender", Action: deident.ActionNone, ExceptionReason: "low re-identification risk, required for cohort stratification"},
	})
	if err != nil {
		panic(err)
	}

	// A fixed key and RunID so this demo's output is reproducible run to
	// run — a real deployment generates RunID fresh per request/batch and
	// keeps the key secret (FENIX_DEIDENT_KEY), never hardcoded like this.
	ctx := deident.RunContext{Key: []byte("demo-only-not-a-real-secret"), RunID: "run-f3a1c8"}
	resources := []deident.Resource{
		{Type: "Patient", Value: patient},
		{Type: "Observation", Value: obs},
	}

	cov, err := deident.Deidentify(resources, rs, nil, ctx, nil)
	if err != nil {
		fmt.Println("Deidentify error:", err)
		return
	}

	fmt.Println("--- Coverage report ---")
	for _, c := range cov {
		actions := make([]string, len(c.Rules))
		for i, r := range c.Rules {
			actions[i] = r.Action
		}
		fmt.Printf("%-32s -> %v\n", c.Path, actions)
	}
	fmt.Println()

	printJSON("Patient (after)", patient)
	printJSON("Observation (after)", obs)
}
