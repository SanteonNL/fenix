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
		wantQueries  int    // expected number of rendered queries
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
				"FROM hix_observations",   // main
				"FROM hix_lab_results",    // lab-results
				"FROM hix_vitals",         // vital-signs
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
				"FROM hix_verloskunde_lab",                       // lab-results → replaced SQL file
				"FROM hix_vitals",                               // vital-signs → unchanged
				"JOIN test_patients tp ON tp.patient_id",        // main → partial replace injected JOIN
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
