package fhirserver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

func TestSplitNonEmpty(t *testing.T) {
	got := splitNonEmpty(" Patient, Observation ,,Condition", ",")
	want := []string{"Patient", "Observation", "Condition"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := splitNonEmpty("", ","); got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
}

func TestGroupQueryFileName(t *testing.T) {
	name := "SIM patients with a final Observation"
	id := "abc-123"
	if got := groupQueryFileName(&fhir.Group{Name: &name, Id: &id}); got != "Group_SIM_patients_with_a_final_Observation.sql" {
		t.Fatalf("got %q", got)
	}
	if got := groupQueryFileName(&fhir.Group{Id: &id}); got != "Group_abc-123.sql" {
		t.Fatalf("got %q", got)
	}
	if got := groupQueryFileName(&fhir.Group{}); got != "Group.sql" {
		t.Fatalf("got %q", got)
	}
}

func TestCreateExportDir(t *testing.T) {
	parent := t.TempDir()
	name := "SIM patients with a final Observation"
	g := &fhir.Group{Name: &name}
	at := time.Date(2026, 10, 13, 14, 30, 5, 0, time.Local)

	first, err := createExportDir(parent, at, g)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(first), "20261013_143005_SIM_patients_with_a_final_Observation"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	second, err := createExportDir(parent, at, g)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(second), "20261013_143005_SIM_patients_with_a_final_Observation_2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseElements(t *testing.T) {
	got := parseElements("Patient.id,Patient.gender,Observation.status")
	want := map[string][]string{
		"Patient":     {"id", "gender"},
		"Observation": {"status"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := parseElements(""); got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
	// An entry with no "." (no resourceType prefix) is silently dropped
	// rather than guessed at.
	got = parseElements("id,Patient.gender")
	want = map[string][]string{"Patient": {"gender"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseTypeFilters(t *testing.T) {
	got, err := parseTypeFilters([]string{"Observation?status=final", "Patient?gender=male"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[string]map[string]string{
		"Observation": {"status": "final"},
		"Patient":     {"gender": "male"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	if got, err := parseTypeFilters(nil); got != nil || err != nil {
		t.Fatalf("expected (nil, nil) for no filters, got (%v, %v)", got, err)
	}

	if _, err := parseTypeFilters([]string{"?status=final"}); err == nil {
		t.Fatal("expected error for a filter missing its resource type")
	}
}

// handleGroupExport's own request validation (method, _type, unknown group)
// doesn't need a real compiler/converter, so it's testable with the same
// lightweight *Server newTestServer already uses in group_test.go. Resolving
// members and actually fetching resources does need a real searcher — that
// path is covered by the live end-to-end run documented in the conversation
// (POST Group -> POST $export -> GET status -> GET NDJSON against the real
// "sim" data), not by this unit test.
func TestHandleGroupExport_Validation(t *testing.T) {
	s := newTestServer()

	body := []byte(`{"resourceType":"Group","type":"person","actual":true,"name":"Export test"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/r4/Group", bytes.NewReader(body))
	createRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("group creation failed: %d %s", createRec.Code, createRec.Body.String())
	}
	location := createRec.Header().Get("Location")

	t.Run("wrong method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/r4"+location[len("/r4"):]+"/$export?_type=Patient", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("missing _type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/r4"+location[len("/r4"):]+"/$export", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("unknown group", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/r4/Group/does-not-exist/$export?_type=Patient", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestHandleExportStatus_UnknownJob(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/r4/$export-status/does-not-exist", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleExportFile_UnknownJob(t *testing.T) {
	s := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/r4/$export-files/does-not-exist/Patient.ndjson", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestExportRequestFindingsKey(t *testing.T) {
	a := exportRequest{
		resourceTypes: []string{"Patient", "Observation"},
		typeFilters:   map[string]map[string]string{"Observation": {"status": "final", "code": "x"}},
	}
	b := exportRequest{
		resourceTypes: []string{"Observation", "Patient"},
		typeFilters:   map[string]map[string]string{"Observation": {"code": "x", "status": "final"}},
	}
	if a.findingsKey("g1") != b.findingsKey("g1") {
		t.Errorf("same request, different keys: %q vs %q", a.findingsKey("g1"), b.findingsKey("g1"))
	}
	want := "Group/g1 $export _type=Observation,Patient _typeFilter=Observation?code=x&status=final"
	if got := a.findingsKey("g1"); got != want {
		t.Errorf("key = %q, want %q", got, want)
	}
	if a.findingsKey("g1") == a.findingsKey("g2") {
		t.Error("different groups must differ")
	}
	a.typeFilters["Observation"]["status"] = "amended"
	if a.findingsKey("g1") == b.findingsKey("g1") {
		t.Error("a changed filter must change the key")
	}
}
