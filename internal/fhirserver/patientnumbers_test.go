package fhirserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A patient with old numbers: the export finds the data recorded under every
// old number, but the output only carries the single active number.
func TestExport_OldPatientNumbersLeadToTheActiveNumber(t *testing.T) {
	s, db := simServerWithDB(t, 2, 1) // P000001, P000002, one final Observation each
	for _, q := range []string{
		// OLD0 -> OLD1 -> P000001 (a chain), P000002 has no old numbers
		`INSERT INTO sim_patientnumbers (Identificatienummer, ReplacedBy) VALUES ('OLD0','OLD1'),('OLD1','P000001'),('P000001',''),('P000002','')`,
		`INSERT INTO sim_patient (Identificatienummer, GeslachtCode) VALUES ('OLD1','male')`,
		`INSERT INTO sim_algemenemeting (MetingID, Identificatienummer, MetingNaamCodeSysteem, MetingNaamCode, MetingNaamOmschrijving, UitslagCodeSysteem, UitslagCode, MetingDatumTijd, Status) VALUES ('MOLD0','OLD0','https://santeon.nl/sct','MCD_1','Meting','https://santeon.nl/sct','A2','2023-10-02T10:00:00+02:00','final'),('MOLD1','OLD1','https://santeon.nl/sct','MCD_1','Meting','https://santeon.nl/sct','A2','2023-10-03T10:00:00+02:00','final')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	out := exportGroup(t, s)

	if strings.Contains(out["Observation"], "Patient/OLD") || strings.Contains(out["Patient"], `"id":"OLD`) {
		t.Errorf("an old patient number reached the output:\nPatient:\n%s\nObservation:\n%s", out["Patient"], out["Observation"])
	}
	if got := strings.Count(strings.TrimSpace(out["Patient"]), "\n") + 1; got != 2 {
		t.Errorf("want 2 Patients (one per active number), got %d:\n%s", got, out["Patient"])
	}
	// The old numbers are listed on the active Patient; P000001 also gets the Observations recorded under them.
	for _, old := range []string{"OLD0", "OLD1"} {
		if !strings.Contains(out["Patient"], `"value":"`+old+`"`) || !strings.Contains(out["Patient"], "patientnumber-old") {
			t.Errorf("old number %s missing as identifier on the active Patient:\n%s", old, out["Patient"])
		}
	}
	if got := strings.Count(out["Observation"], `"Patient/P000001"`); got != 3 {
		t.Errorf("want 3 Observations for P000001, got %d:\n%s", got, out["Observation"])
	}
}

// No patient numbers delivered (no table): the export runs as before.
func TestExport_WithoutPatientNumbersStillWorks(t *testing.T) {
	s, db := simServerWithDB(t, 2, 1)
	if _, err := db.Exec(`DROP TABLE sim_patientnumbers`); err != nil {
		t.Fatal(err)
	}
	out := exportGroup(t, s)
	if got := strings.Count(strings.TrimSpace(out["Patient"]), "\n") + 1; got != 2 {
		t.Errorf("want 2 Patients, got %d:\n%s", got, out["Patient"])
	}
}

// A plain search for one patient also finds the data under its old numbers,
// and shows it under the active number.
func TestSearch_FindsDataUnderOldPatientNumbers(t *testing.T) {
	s, db := simServerWithDB(t, 2, 1)
	for _, q := range []string{
		`INSERT INTO sim_patientnumbers (Identificatienummer, ReplacedBy) VALUES ('OLD0','OLD1'),('OLD1','P000001'),('P000001',''),('P000002','')`,
		`INSERT INTO sim_patient (Identificatienummer, GeslachtCode) VALUES ('OLD1','male')`,
		`INSERT INTO sim_algemenemeting (MetingID, Identificatienummer, MetingNaamCodeSysteem, MetingNaamCode, MetingNaamOmschrijving, UitslagCodeSysteem, UitslagCode, MetingDatumTijd, Status) VALUES ('MOLD0','OLD0','https://santeon.nl/sct','MCD_1','Meting','https://santeon.nl/sct','A2','2023-10-02T10:00:00+02:00','final')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	get := func(url string) string {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", url, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	for _, url := range []string{"/r4/Observation?patient=P000001", "/r4/Observation?patient=OLD0"} {
		body := get(url)
		if !strings.Contains(body, `"id":"MOLD0"`) || strings.Count(body, `"reference":"Patient/P000001"`) != 2 || strings.Contains(body, "Patient/OLD") {
			t.Errorf("%s: want both Observations under Patient/P000001, got:\n%s", url, body)
		}
	}
	for _, url := range []string{"/r4/Patient?_id=P000001", "/r4/Patient?_id=OLD1"} {
		body := get(url)
		if !strings.Contains(body, `"total":1`) || !strings.Contains(body, `"id":"P000001"`) || strings.Contains(body, `"id":"OLD`) {
			t.Errorf("%s: want only Patient P000001, got:\n%s", url, body)
		}
	}
}

// Test patients are left out of the export, as in the DSG.
func TestExport_TestPatientsAreLeftOut(t *testing.T) {
	s, db := simServerWithDB(t, 2, 1) // P000001, P000002
	if _, err := db.Exec(`INSERT INTO sim_patientnumbers (Identificatienummer, ReplacedBy, TestPatient) VALUES ('P000001','','false'),('P000002','','true')`); err != nil {
		t.Fatal(err)
	}
	out := exportGroup(t, s)
	if strings.Contains(out["Patient"], "P000002") || strings.Contains(out["Observation"], "P000002") {
		t.Errorf("test patient P000002 reached the output:\n%s\n%s", out["Patient"], out["Observation"])
	}
	if !strings.Contains(out["Patient"], `"id":"P000001"`) {
		t.Errorf("P000001 missing:\n%s", out["Patient"])
	}
}
