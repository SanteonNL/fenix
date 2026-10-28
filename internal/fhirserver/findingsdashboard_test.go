package fhirserver

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindingsHandlerServesDashboardAndAllRuns(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"findings-a.jsonl": `{"kind":"finding","run_id":"a"}` + "\n",
		"findings-b.jsonl": `{"kind":"finding","run_id":"b"}` + "\n",
		"other.txt":        "ignored\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := FindingsHandler(dir)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/findings", nil))
	got := rec.Body.String()
	if !strings.Contains(got, `"run_id":"a"`) || !strings.Contains(got, `"run_id":"b"`) || strings.Contains(got, "ignored") {
		t.Errorf("api/findings = %q", got)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/findings", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "FENIX Findings") {
		t.Errorf("dashboard not served: %d", rec.Code)
	}

	// An empty or missing folder is an empty stream, not an error.
	rec = httptest.NewRecorder()
	FindingsHandler(filepath.Join(dir, "nope")).ServeHTTP(rec, httptest.NewRequest("GET", "/api/findings", nil))
	if rec.Code != 200 || rec.Body.Len() != 0 {
		t.Errorf("missing dir: %d %q", rec.Code, rec.Body.String())
	}
}
