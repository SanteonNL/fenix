package fhirserver

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/SanteonNL/fenix/internal/config"
	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/SanteonNL/fenix/internal/deident"
	"github.com/SanteonNL/fenix/internal/querycompiler"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

// simServer builds a Server over a SQLite file holding synthetic "sim" data:
// patients patients with obsPerPatient Observations each, served by the real
// sim query config. The file (not :memory:) lets the connection pool open
// several connections, as it would against SQL Server.
func simServer(tb testing.TB, patients, obsPerPatient int) *Server {
	s, _ := simServerWithDB(tb, patients, obsPerPatient)
	return s
}

// simServerWithDB is simServer that also returns the staging database, for tests that add data of their own.
func simServerWithDB(tb testing.TB, patients, obsPerPatient int) (*Server, *sqlx.DB) {
	tb.Helper()

	db, err := sqlx.Open("sqlite", filepath.Join(tb.TempDir(), "staging.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	loadSimData(tb, db.DB, patients, obsPerPatient)
	config.PoolConfig{MaxOpenConns: 8}.Apply(db, false)

	root := filepath.Join("..", "..")
	compiler, err := querycompiler.New(filepath.Join(root, "config", "queries"), root)
	if err != nil {
		tb.Fatal(err)
	}
	log := zerolog.Nop()
	conv := converter.NewFHIRConverter(db, log, converter.NewProfileService(log), converter.NewConceptMapService(log))
	conv.SetConcurrency(8, 0)
	return New(compiler, conv, "sim", "", "", nil, nil, "", nil, log), db
}

func loadSimData(tb testing.TB, db *sql.DB, patients, obsPerPatient int) {
	tb.Helper()
	mustExec := func(q string, args ...any) {
		if _, err := db.Exec(q, args...); err != nil {
			tb.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`CREATE TABLE sim_patientnumbers (Identificatienummer, ReplacedBy, TestPatient)`)
	mustExec(`CREATE TABLE sim_patient (Identificatienummer, GeslachtCode, GeslachtOmschrijving, Land, Geboortedatum, DatumOverlijden, DatumCheckStatusOverlijden)`)
	mustExec(`CREATE TABLE sim_algemenemeting (MetingID, Identificatienummer, MetingNaamCodeSysteem, MetingNaamCode, MetingNaamOmschrijving,
		UitslagWaarde, UitslagWaardeEenheidSysteem, UitslagWaardeEenheid, UitslagCodeSysteem, UitslagCode, UitslagCodeOmschrijving, MetingDatumTijd, Status)`)

	tx, err := db.Begin()
	if err != nil {
		tb.Fatal(err)
	}
	pat, _ := tx.Prepare(`INSERT INTO sim_patient (Identificatienummer, GeslachtCode, Geboortedatum) VALUES (?,?,?)`)
	obs, _ := tx.Prepare(`INSERT INTO sim_algemenemeting VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	metingID := 0
	for p := 1; p <= patients; p++ {
		gender := []string{"male", "female"}[p%2]
		if _, err := pat.Exec(fmt.Sprintf("P%06d", p), gender, fmt.Sprintf("19%02d-%02d-15", 40+p%60, 1+p%12)); err != nil {
			tb.Fatal(err)
		}
		for o := 0; o < obsPerPatient; o++ {
			metingID++
			status := "final"
			if o%10 == 9 {
				status = "preliminary"
			}
			// Alternate between a numeric result and a coded result.
			var value, unit, unitSys, resCode, resSys any
			if o%2 == 0 {
				value, unit, unitSys = fmt.Sprint(60+o%40), "bpm", "http://unitsofmeasure.org"
			} else {
				resCode, resSys = "A2", "https://santeon.nl/sct"
			}
			if _, err := obs.Exec(fmt.Sprintf("M%08d", metingID), fmt.Sprintf("P%06d", p),
				"https://santeon.nl/sct", fmt.Sprintf("MCD_%d", o%7), "Meting", value, unitSys, unit, resSys, resCode, nil,
				fmt.Sprintf("2023-10-%02dT10:%02d:00+02:00", 1+o%28, o%60), status); err != nil {
				tb.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		tb.Fatal(err)
	}
}

// exportGroup creates a group of the patients with a final Observation, runs a
// $export of Patient and Observation, and returns the NDJSON per type.
func exportGroup(tb testing.TB, s *Server) map[string]string {
	tb.Helper()
	h := s.Handler()

	body := `{"resourceType":"Group","type":"person","actual":true,"name":"perf","extension":[{"url":"` + memberFilterURL + `","valueString":"Observation?status=final"}]}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r4/Group", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		tb.Fatalf("create group: %d %s", rec.Code, rec.Body.String())
	}
	var g struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &struct{}{})
	var raw map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	g.ID, _ = raw["id"].(string)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r4/Group/"+g.ID+"/$export?_type=Patient,Observation", nil))
	if rec.Code != http.StatusAccepted {
		tb.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	status := rec.Header().Get("Content-Location")

	rec = pollExport(tb, h, status)
	var manifest struct {
		Output []struct {
			Type  string
			URL   string
			Count int
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &manifest); err != nil {
		tb.Fatalf("manifest: %v", err)
	}
	out := map[string]string{}
	for _, o := range manifest.Output {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, o.URL, nil))
		data, _ := io.ReadAll(rec.Body)
		if n := bytes.Count(data, []byte("\n")); n != o.Count {
			tb.Fatalf("%s: manifest says %d resources, file has %d", o.Type, o.Count, n)
		}
		out[o.Type] = string(data)
	}
	return out
}

func TestExport_BatchedParallelMatchesSingleBatch(t *testing.T) {
	s := simServer(t, 60, 12)

	s.SetExportTuning(1_000_000, 1) // one batch, one worker
	want := exportGroup(t, s)
	if got := strings.Count(want["Patient"], "\n"); got != 60 {
		t.Fatalf("expected 60 patients in the export, got %d", got)
	}
	if got := strings.Count(want["Observation"], "\n"); got != 60*12 {
		t.Fatalf("expected %d observations in the export, got %d", 60*12, got)
	}

	s.SetExportTuning(7, 4) // many small batches, in parallel
	got := exportGroup(t, s)
	for _, rt := range []string{"Patient", "Observation"} {
		if got[rt] != want[rt] {
			t.Fatalf("%s: batched/parallel export differs from the single-batch export", rt)
		}
	}
}

func TestSearch_PagingCoversTheWholeResult(t *testing.T) {
	s := simServer(t, 20, 10)
	h := s.Handler()

	get := func(url string) (ids []string, total int, next string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", url, rec.Code, rec.Body.String())
		}
		var b struct {
			Total int
			Link  []struct{ Relation, URL string }
			Entry []struct{ Resource struct{ ID string } }
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
		for _, e := range b.Entry {
			ids = append(ids, e.Resource.ID)
		}
		for _, l := range b.Link {
			if l.Relation == "next" {
				next = l.URL
			}
		}
		return ids, b.Total, next
	}

	all, total, _ := get("/r4/Observation?status=final")
	if total != len(all) || total != 180 {
		t.Fatalf("unpaged: total %d, %d entries (want 180)", total, len(all))
	}

	var paged []string
	url, pages := "/r4/Observation?status=final&_count=50", 0
	for url != "" {
		ids, tot, next := get(url)
		if tot != 180 {
			t.Fatalf("page total = %d, want 180", tot)
		}
		paged = append(paged, ids...)
		url = next
		pages++
	}
	if pages != 4 {
		t.Fatalf("expected 4 pages of 50 for 180 results, got %d", pages)
	}
	sort.Strings(all)
	if strings.Join(paged, ",") != strings.Join(all, ",") {
		t.Fatal("concatenated pages differ from the unpaged result")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/r4/Observation?_count=abc", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid _count: status %d, want 400", rec.Code)
	}
}

func TestChunk(t *testing.T) {
	if got := chunk(nil, 3); len(got) != 1 || len(got[0]) != 0 {
		t.Fatalf("no ids should give one empty chunk, got %v", got)
	}
	got := chunk([]string{"a", "b", "c", "d", "e"}, 2)
	if len(got) != 3 || len(got[2]) != 1 {
		t.Fatalf("got %v", got)
	}
}

// BenchmarkExport measures a Group $export over synthetic data. Run e.g.
//
//	go test ./internal/fhirserver -run x -bench Export -benchtime 1x
func BenchmarkExport(b *testing.B) {
	const patients, obsPerPatient = 10_000, 10 // 100k Observations
	s := simServer(b, patients, obsPerPatient)
	for _, c := range []struct {
		name           string
		batch, workers int
	}{
		{"single-batch", 1_000_000, 1},
		{"batch1000-x1", 1000, 1},
		{"batch1000-x4", 1000, 4},
		{"batch500-x8", 500, 8},
	} {
		b.Run(c.name, func(b *testing.B) {
			s.SetExportTuning(c.batch, c.workers)
			for i := 0; i < b.N; i++ {
				exportGroup(b, s)
			}
		})
	}
}

// pollExport polls an export's status URL until it stops answering 202 and
// returns the final response.
func pollExport(tb testing.TB, h http.Handler, statusURL string) *httptest.ResponseRecorder {
	tb.Helper()
	deadline := time.Now().Add(10 * time.Minute)
	for {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, statusURL, nil))
		if rec.Code != http.StatusAccepted {
			return rec
		}
		if time.Now().After(deadline) {
			tb.Fatal("export did not finish in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func createPerfGroup(tb testing.TB, h http.Handler, filter string) string {
	tb.Helper()
	body := `{"resourceType":"Group","type":"person","actual":true,"name":"perf","extension":[{"url":"` + memberFilterURL + `","valueString":"` + filter + `"}]}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r4/Group", strings.NewReader(body)))
	var raw map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	id, _ := raw["id"].(string)
	return id
}

func TestExport_KickoffReturnsBeforeTheDataIsReady(t *testing.T) {
	s := simServer(t, 30, 5)
	h := s.Handler()
	id := createPerfGroup(t, h, "Observation?status=final")

	// Hold the run until the test has looked at the in-progress state.
	release := make(chan struct{})
	s.exportGate = func() { <-release }

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r4/Group/"+id+"/$export?_type=Patient", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("kickoff: %d %s", rec.Code, rec.Body.String())
	}
	statusURL := rec.Header().Get("Content-Location")

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, statusURL, nil))
	if rec.Code != http.StatusAccepted || rec.Header().Get("X-Progress") == "" {
		t.Fatalf("running export: want 202 with X-Progress, got %d %v", rec.Code, rec.Header())
	}

	close(release)
	if final := pollExport(t, h, statusURL); final.Code != http.StatusOK {
		t.Fatalf("finished export: %d %s", final.Code, final.Body.String())
	}
}

func TestExport_FailureIsReportedAtTheStatusURL(t *testing.T) {
	s := simServer(t, 10, 3)
	h := s.Handler()
	id := createPerfGroup(t, h, "Observation?status=final")

	// An unconfigured type is rejected up front, with a plain 400.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r4/Group/"+id+"/$export?_type=Nonsense", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unconfigured type: want 400 at kickoff, got %d", rec.Code)
	}

	// A failure inside the run shows up when polling: an empty ruleset covers no
	// field, so de-identification must refuse to release anything.
	s.deidentRuleset = &deident.Ruleset{}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r4/Group/"+id+"/$export?_type=Patient", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("kickoff: want 202, got %d %s", rec.Code, rec.Body.String())
	}
	final := pollExport(t, h, rec.Header().Get("Content-Location"))
	if final.Code != http.StatusUnprocessableEntity {
		t.Fatalf("failed export: want 422 at the status URL, got %d %s", final.Code, final.Body.String())
	}
	if !strings.Contains(final.Body.String(), "OperationOutcome") {
		t.Fatalf("failed export should carry an OperationOutcome, got %s", final.Body.String())
	}
}

func TestExport_DeleteDiscardsTheJob(t *testing.T) {
	s := simServer(t, 10, 3)
	h := s.Handler()
	id := createPerfGroup(t, h, "Observation?status=final")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r4/Group/"+id+"/$export?_type=Patient", nil))
	statusURL := rec.Header().Get("Content-Location")
	pollExport(t, h, statusURL)

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, statusURL, nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("DELETE: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, statusURL, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("after DELETE: want 404, got %d", rec.Code)
	}
}
