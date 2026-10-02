//go:build ignore

// Seeds a small SQLite staging database with just enough rows to exercise
// the "hix" query-compiler source (config/queries/sources/hix/hix.yaml) for
// the Group POST/GET demo described in docs/fenix_architecture.md.
//
// Every table referenced by queries/hix/fhir/*.sql must exist even when the
// demo doesn't use it — the SQL statements reference them unconditionally —
// so this creates the full schema but only puts rows in the tables the demo
// actually reads from (patients, encounters, hix_observations).
//
// Usage: go run scripts/seed_group_demo.go [output/group-demo/staging.db]
package main

import (
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE patients (patient_id TEXT, gender TEXT, birth_date TEXT, active INTEGER);
CREATE TABLE patient_names (patient_id TEXT, name_id TEXT, name_use TEXT, family TEXT, given TEXT);
CREATE TABLE patient_telecom (patient_id TEXT, telecom_id TEXT, telecom_system TEXT, telecom_value TEXT, telecom_use TEXT);
CREATE TABLE patient_address (patient_id TEXT, address_id TEXT, address_use TEXT, line TEXT, city TEXT, postal_code TEXT, country TEXT);
CREATE TABLE patient_identifier (patient_id TEXT, identifier_id TEXT, id_use TEXT, id_system TEXT, id_value TEXT);

CREATE TABLE encounters (encounter_id TEXT, patient_id TEXT, status TEXT, start_time TEXT, end_time TEXT, service_provider TEXT);
CREATE TABLE encounter_type (encounter_id TEXT, type_id TEXT, type_text TEXT);
CREATE TABLE encounter_type_coding (type_id TEXT, coding_id TEXT, coding_system TEXT, coding_code TEXT, coding_display TEXT);
CREATE TABLE encounter_reason (encounter_id TEXT, reason_id TEXT, reason_system TEXT, reason_code TEXT, reason_display TEXT);
CREATE TABLE encounter_status_history (encounter_id TEXT, history_id TEXT, status TEXT, period_start TEXT, period_end TEXT);

CREATE TABLE hix_observations (observation_id TEXT, patient_id TEXT, obs_type TEXT, obs_status TEXT, obs_date TEXT, obs_value TEXT, obs_unit TEXT);
CREATE TABLE hix_obs_category (observation_id TEXT, cat_id TEXT, cat_text TEXT);
CREATE TABLE hix_obs_category_coding (cat_id TEXT, coding_id TEXT, coding_system TEXT, coding_code TEXT, coding_display TEXT);
CREATE TABLE hix_obs_coding (observation_id TEXT, coding_id TEXT, coding_system TEXT, coding_code TEXT, coding_display TEXT);
CREATE TABLE hix_lab_results (lab_id TEXT, patient_id TEXT, lab_status TEXT, result_date TEXT, result_value TEXT, result_unit TEXT, loinc_code TEXT, loinc_display TEXT);
CREATE TABLE hix_vitals (vital_id TEXT, patient_id TEXT, vital_status TEXT, measured_at TEXT, vital_value TEXT, vital_unit TEXT, loinc_code TEXT, loinc_display TEXT);
`

// Three patients so the two-filter demo (Encounter AND Observation) has a
// visible intersection:
//   p1 — has a matching Encounter AND a matching Observation → in the group
//   p2 — has a matching Encounter only                       → excluded by the 2nd filter
//   p3 — has a matching Observation only                     → excluded by the 1st filter
const fixtures = `
-- active left NULL: SQLite has no native boolean and a stray 0/1 INTEGER
-- fails *bool unmarshalling when /r4/Patient round-trips the row through the
-- typed fhir.Patient struct (unrelated to the Group demo, which never
-- queries Patient directly, but worth avoiding if you poke at it anyway).
INSERT INTO patients (patient_id, gender, birth_date) VALUES
  ('p1', 'female', '1980-01-01'),
  ('p2', 'male',   '1975-06-15'),
  ('p3', 'female', '1990-03-22');

INSERT INTO encounters (encounter_id, patient_id, status, start_time, end_time, service_provider) VALUES
  ('e1', 'p1', 'in-progress', '2024-01-05', NULL, 'Ward A'),
  ('e2', 'p2', 'in-progress', '2024-01-06', NULL, 'Ward B'),
  ('e3', 'p3', 'finished',    '2023-11-01', '2023-11-02', 'Ward A');

INSERT INTO hix_observations (observation_id, patient_id, obs_type, obs_status, obs_date, obs_value, obs_unit) VALUES
  ('o1', 'p1', 'CLINICAL', 'final', '2024-02-01', '7.2', 'mmol/L'),
  ('o2', 'p3', 'CLINICAL', 'final', '2024-02-02', '6.9', 'mmol/L');
`

func main() {
	path := "output/group-demo/staging.db"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "create output dir:", err)
		os.Exit(1)
	}
	_ = os.Remove(path) // start clean on re-runs

	db, err := sqlx.Connect("sqlite", path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer db.Close()

	if _, err := db.Exec(schema); err != nil {
		fmt.Fprintln(os.Stderr, "create schema:", err)
		os.Exit(1)
	}
	if _, err := db.Exec(fixtures); err != nil {
		fmt.Fprintln(os.Stderr, "insert fixtures:", err)
		os.Exit(1)
	}

	fmt.Println("Seeded demo staging DB at", path)
	fmt.Println("  Encounter?status=in-progress            -> p1, p2")
	fmt.Println("  Observation?status=final                -> p1, p3")
	fmt.Println("  both (AND)                              -> p1 only")
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
