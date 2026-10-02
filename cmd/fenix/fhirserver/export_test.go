package fhirserver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
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
