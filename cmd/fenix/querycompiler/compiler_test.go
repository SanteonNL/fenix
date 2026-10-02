package querycompiler_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SanteonNL/fenix/cmd/fenix/querycompiler"
)

const (
	configDir  = "../../../config/queries"
	sqlBaseDir = "../../../"
	outputDir  = "../../../output/query-compile-test"
)

func TestResolve(t *testing.T) {
	c, err := querycompiler.New(configDir, sqlBaseDir)
	if err != nil {
		t.Fatalf("querycompiler.New: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("creating output dir: %v", err)
	}

	tests := []struct {
		name         string
		source       string
		groupID      string
		resourceType string
		params       map[string]string
		wantQueries  int      // expected number of rendered queries
		wantInAll    []string // must appear in every rendered query
		wantInAny    []string // must appear in at least one rendered query
	}{
		{
			name:         "hix_Patient",
			source:       "hix",
			resourceType: "Patient",
			params:       map[string]string{},
			wantQueries:  1,
			wantInAll:    []string{"FROM patients", "'Patient'"},
		},
		{
			// 3 queries: main, lab-results, vital-signs — each filters a different table/column.
			name:         "hix_Observation_date_status",
			source:       "hix",
			resourceType: "Observation",
			params:       map[string]string{"date": "ge2023-01-01", "status": "final"},
			wantQueries:  3,
			wantInAny: []string{
				"FROM hix_observations", // main
				"FROM hix_lab_results",  // lab-results
				"FROM hix_vitals",       // vital-signs
				"obs_date >= '2023-01-01'",
				"result_date >= '2023-01-01'",
				"measured_at >= '2023-01-01'",
			},
		},
		{
			name:         "hix_Observation_date_to",
			source:       "hix",
			resourceType: "Observation",
			params:       map[string]string{"date": "le2023-12-31"},
			wantQueries:  3,
			wantInAny: []string{
				"obs_date <= '2023-12-31'",
				"result_date <= '2023-12-31'",
				"measured_at <= '2023-12-31'",
			},
		},
		{
			// Group WHERE into all 3 queries; lab-results SQL replaced; main gets a JOIN via replace:.
			name:         "hix_geboortezorg_Observation",
			source:       "hix",
			groupID:      "geboortezorg-2024",
			resourceType: "Observation",
			params:       map[string]string{"date": "ge2023-01-01"},
			wantQueries:  3,
			wantInAll:    []string{"category = 'geboortezorg'"},
			wantInAny: []string{
				"FROM hix_verloskunde_lab",               // lab-results → replaced SQL file
				"FROM hix_vitals",                        // vital-signs → unchanged
				"JOIN test_patients tp ON tp.patient_id", // main → partial replace injected JOIN
			},
		},
		{
			// date → Encounter.period → template var period_from; SQL column is start_time.
			name:         "hix_Encounter_date",
			source:       "hix",
			resourceType: "Encounter",
			params:       map[string]string{"date": "ge2024-01-01"},
			wantQueries:  1,
			wantInAll:    []string{"FROM encounters", "start_time >= '2024-01-01'"},
		},
		{
			// sim reuses the CLI batch pipeline's existing SQL (see
			// config/queries/sources/sim/sim.yaml); with no _id given, the
			// {{if ._id}} filter is simply absent, same as "no filter".
			name:         "sim_Patient",
			source:       "sim",
			resourceType: "Patient",
			params:       map[string]string{},
			wantQueries:  1,
			wantInAll:    []string{"FROM sim_patient", "'Patient'"},
		},
		{
			// _id is pushed down (config/queries/sources/sim/sim.yaml) via
			// the generic "Resource" SearchParameter fallback (see
			// querycompiler.lookupSearchParam) and scopes to one patient.
			name:         "sim_Patient_id",
			source:       "sim",
			resourceType: "Patient",
			params:       map[string]string{"_id": "123"},
			wantQueries:  1,
			wantInAll:    []string{"FROM sim_patient", "Identificatienummer = '123'"},
		},
		{
			// status is pushed down (config/queries/sources/sim/sim.yaml) and
			// filters on the real Status column in AlgemeneMeting.csv.
			name:         "sim_Observation_status",
			source:       "sim",
			resourceType: "Observation",
			params:       map[string]string{"status": "final"},
			wantQueries:  1,
			wantInAll:    []string{"FROM sim_algemenemeting", "Status = 'final'"},
		},
		{
			// patient is pushed down too, scoping to one patient's rows —
			// used by Group/$export (cmd/fenix/fhirserver/export.go) to fire
			// this query once per member id.
			name:         "sim_Observation_patient",
			source:       "sim",
			resourceType: "Observation",
			params:       map[string]string{"patient": "456"},
			wantQueries:  1,
			wantInAll:    []string{"FROM sim_algemenemeting", "Identificatienummer = '456'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			queries, err := c.Resolve(tt.source, tt.groupID, tt.resourceType, tt.params)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}

			if len(queries) != tt.wantQueries {
				t.Errorf("expected %d queries, got %d", tt.wantQueries, len(queries))
			}

			// Write each rendered query to the output folder.
			for i, q := range queries {
				name := fmt.Sprintf("%s_%02d_%s", tt.name, i+1, q.Name)
				outFile := filepath.Join(outputDir, name+".sql")
				if err := os.WriteFile(outFile, []byte(q.SQL), 0o644); err != nil {
					t.Fatalf("writing output: %v", err)
				}
				t.Logf("wrote %s", outFile)
			}

			// Assertions that must hold for every rendered query.
			for _, want := range tt.wantInAll {
				for _, q := range queries {
					if !strings.Contains(q.SQL, want) {
						t.Errorf("query %q: expected SQL to contain %q\ngot:\n%s", q.Name, want, q.SQL)
					}
				}
			}

			// Assertions that must hold for at least one rendered query.
			for _, want := range tt.wantInAny {
				found := false
				for _, q := range queries {
					if strings.Contains(q.SQL, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no query contained %q", want)
				}
			}
		})
	}
}

func TestSearchParamField(t *testing.T) {
	c, err := querycompiler.New(configDir, sqlBaseDir)
	if err != nil {
		t.Fatalf("querycompiler.New: %v", err)
	}

	tests := []struct {
		name          string
		resourceType  string
		code          string
		wantField     string
		wantParamType string
		wantOK        bool
	}{
		{
			// Defined directly against Observation's own base in
			// search-parameter.json.
			name:          "Observation_status",
			resourceType:  "Observation",
			code:          "status",
			wantField:     "status",
			wantParamType: "token",
			wantOK:        true,
		},
		{
			// Listed under a multi-resource "patient" SearchParameter whose
			// expression includes "Observation.subject.where(...)".
			name:          "Observation_patient",
			resourceType:  "Observation",
			code:          "patient",
			wantField:     "subject",
			wantParamType: "reference",
			wantOK:        true,
		},
		{
			// _id has no per-resource-type entry at all — only the generic
			// "Resource" base — so the field name falls back to the code
			// itself (see fieldName's "not found for this resource type").
			name:          "Patient_id_fallback",
			resourceType:  "Patient",
			code:          "_id",
			wantField:     "_id",
			wantParamType: "token",
			wantOK:        true,
		},
		{
			name:         "unknown_code",
			resourceType: "Observation",
			code:         "not-a-real-param",
			wantOK:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, paramType, ok := c.SearchParamField(tt.resourceType, tt.code)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if field != tt.wantField || paramType != tt.wantParamType {
				t.Errorf("got (%q, %q), want (%q, %q)", field, paramType, tt.wantField, tt.wantParamType)
			}
		})
	}
}

func TestPushdownCodes(t *testing.T) {
	c, err := querycompiler.New(configDir, sqlBaseDir)
	if err != nil {
		t.Fatalf("querycompiler.New: %v", err)
	}

	got := c.PushdownCodes("sim", "Observation")
	for _, code := range []string{"status", "patient"} {
		if !got[code] {
			t.Errorf("PushdownCodes(sim, Observation) missing %q: %v", code, got)
		}
	}

	if got := c.PushdownCodes("sim", "DoesNotExist"); len(got) != 0 {
		t.Errorf("expected empty set for unconfigured resource type, got %v", got)
	}
	if got := c.PushdownCodes("no-such-source", "Observation"); len(got) != 0 {
		t.Errorf("expected empty set for unconfigured source, got %v", got)
	}
}
