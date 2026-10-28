package fhirserver

import (
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/rs/zerolog"
)

//go:embed findings_dashboard.html
var findingsDashboardHTML []byte

// ignoredFile holds the source codes the user has decided not to map. It lives
// next to the findings, so it applies to every run, past and future.
const ignoredFile = "ignored-codes.json"

// findingsAPI is the findings dashboard and what it needs from the server:
//
//	GET  /findings               the dashboard
//	GET  /api/findings           every findings-*.jsonl in dir as one stream of JSON lines
//	POST /api/findings/mapping   how the concept map of an element treats a list of source codes
//	POST /api/findings/definitions what the ObservationDefinition of each observation code allows
//	POST /api/findings/answers     how the concept map of an observation code's answer valueset treats an answer
//	GET  /api/findings/ignored   the codes marked "do not map"
//	POST /api/findings/ignored   mark ({"action":"ignore"}) or unmark ({"action":"restore"}) codes
//
// profile and maps may be nil: the mapping endpoint then reports nothing is bound.
type findingsAPI struct {
	dir     string
	profile func() *converter.ProfileService
	maps    *converter.ConceptMapService
	defs    func() *converter.ObservationDefinitions
	mu      sync.Mutex // guards the ignored file
}

// FindingsHandler serves the dashboard and its data from dir, without concept map support.
func FindingsHandler(dir string) http.Handler { return (&findingsAPI{dir: dir}).handler() }

func (a *findingsAPI) handler() http.Handler {
	mux := http.NewServeMux()
	a.mount(mux)
	return mux
}

func (a *findingsAPI) mount(mux *http.ServeMux) {
	mux.HandleFunc("/findings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(findingsDashboardHTML)
	})
	mux.HandleFunc("/api/findings", a.handleData)
	mux.HandleFunc("/api/findings/mapping", a.handleMapping)
	mux.HandleFunc("/api/findings/ignored", a.handleIgnored)
	mux.HandleFunc("/api/findings/definitions", a.handleDefinitions)
	mux.HandleFunc("/api/findings/answers", a.handleAnswers)
}

// answerStatus is the answer to one {"obs_code","answer"} pair of handleAnswers.
type answerStatus struct {
	ObsCode      string   `json:"obs_code"`
	Answer       string   `json:"answer"`
	Found        bool     `json:"found"`                // the observation code has a definition with a loaded answer valueset
	Definition   string   `json:"definition,omitempty"` // its id
	ValueSet     string   `json:"valueset,omitempty"`   // the answer valueset: what a concept map for the answers targets
	ValidAnswers []string `json:"valid_answers,omitempty"`
	HasMap       bool     `json:"has_map"`          // a concept map targets that valueset
	Status       string   `json:"status,omitempty"` // valid | mapped | default | none
	Target       string   `json:"target,omitempty"` // the answer it is mapped to
	File         string   `json:"file,omitempty"`   // the concept map file to edit
}

// handleAnswers answers {"items":[{"obs_code":"MCD_L","answer":"P1"}, ...]}: for each answer, whether it is
// a valid answer for that observation code, mapped onto one, or not mapped, and which concept map to edit.
// An answer is mapped by a concept map that targets the answer valueset of the observation code's definition.
func (a *findingsAPI) handleAnswers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Items []struct {
			ObsCode string `json:"obs_code"`
			Answer  string `json:"answer"`
		} `json:"items"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		apiError(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	var defs *converter.ObservationDefinitions
	if a.defs != nil {
		defs = a.defs()
	}
	out := make([]answerStatus, 0, len(req.Items))
	for _, it := range req.Items {
		st := answerStatus{ObsCode: it.ObsCode, Answer: it.Answer}
		if defs != nil && a.maps != nil {
			if id, vs, valid, ok := defs.AnswerSet(it.ObsCode); ok {
				info := a.maps.Resolve(vs, []string{it.Answer})
				c := info.Codes[0]
				st.Found, st.Definition, st.ValueSet, st.ValidAnswers = true, id, vs, valid
				st.HasMap, st.Status, st.Target, st.File = info.HasMap, c.Status, c.Target, c.File
			}
		}
		out = append(out, st)
	}
	writeJSON(w, http.StatusOK, map[string]any{"answers": out})
}

// handleDefinitions answers {"codes":["MCD_L", ...]} with the ObservationDefinition of each observation code.
func (a *findingsAPI) handleDefinitions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Codes []string `json:"codes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		apiError(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	out := make([]converter.DefinitionInfo, 0, len(req.Codes))
	var defs *converter.ObservationDefinitions
	if a.defs != nil {
		defs = a.defs()
	}
	for _, c := range req.Codes {
		if defs == nil {
			out = append(out, converter.DefinitionInfo{Code: c})
		} else {
			out = append(out, defs.Describe(c))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"definitions": out})
}

func (a *findingsAPI) handleData(w http.ResponseWriter, r *http.Request) {
	files, _ := filepath.Glob(filepath.Join(a.dir, "findings-*.jsonl"))
	sort.Strings(files)
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			continue
		}
		_, _ = io.Copy(w, f)
		f.Close()
	}
}

/* ---------- concept map status of source codes ---------- */

type mappingRequest struct {
	Path  string   `json:"path"`  // element, as the dashboard names it: Patient.gender
	Codes []string `json:"codes"` // source codes
}

type mappingResponse struct {
	Path  string `json:"path"`
	Bound bool   `json:"bound"` // the element has a valueset binding
	converter.MappingInfo
}

func (a *findingsAPI) handleMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req mappingRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		apiError(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	resp := mappingResponse{Path: req.Path}
	if a.profile != nil && a.maps != nil {
		if p := a.profile(); p != nil {
			if vs, ok := p.BindingFor(req.Path); ok {
				resp.Bound, resp.MappingInfo = true, a.maps.Resolve(vs, req.Codes)
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

/* ---------- codes the user does not want to map ---------- */

// IgnoredCode is one source code, at one element, that is not going to be mapped.
type IgnoredCode struct {
	Path string `json:"path"`
	Code string `json:"code"`
	Time string `json:"time,omitempty"`
}

type ignoredRequest struct {
	Action string        `json:"action"` // ignore | restore
	Items  []IgnoredCode `json:"items"`
}

func (a *findingsAPI) handleIgnored(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	file := filepath.Join(a.dir, ignoredFile)
	list := readIgnored(file)

	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		var req ignoredRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
			apiError(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Action != "ignore" && req.Action != "restore" {
			apiError(w, `action must be "ignore" or "restore"`, http.StatusBadRequest)
			return
		}
		list = applyIgnored(list, req.Action, req.Items, time.Now().UTC().Format(time.RFC3339))
		if err := writeIgnored(file, list); err != nil {
			apiError(w, "save the ignore list: "+err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		apiError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": list})
}

func applyIgnored(list []IgnoredCode, action string, items []IgnoredCode, now string) []IgnoredCode {
	key := func(c IgnoredCode) string { return c.Path + "\x00" + c.Code }
	have := map[string]int{}
	for i, c := range list {
		have[key(c)] = i
	}
	switch action {
	case "ignore":
		for _, it := range items {
			if _, ok := have[key(it)]; !ok && it.Path != "" && it.Code != "" {
				it.Time = now
				have[key(it)] = len(list)
				list = append(list, it)
			}
		}
	case "restore":
		drop := map[string]bool{}
		for _, it := range items {
			drop[key(it)] = true
		}
		kept := list[:0:0]
		for _, c := range list {
			if !drop[key(c)] {
				kept = append(kept, c)
			}
		}
		list = kept
	}
	return list
}

func readIgnored(file string) []IgnoredCode {
	list := []IgnoredCode{}
	if b, err := os.ReadFile(file); err == nil {
		var f struct {
			Entries []IgnoredCode `json:"entries"`
		}
		if json.Unmarshal(b, &f) == nil && f.Entries != nil {
			list = f.Entries
		}
	}
	return list
}

// writeIgnored replaces the file atomically, so a reader never sees half a list.
func writeIgnored(file string, list []IgnoredCode) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(map[string]any{"entries": list}, "", "  ")
	if err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

// NewToolsServer serves the findings dashboard together with the ConceptMap
// editor, without the FHIR API or a database: what `-cmd findings` runs. The
// profile and concept maps are what the converter would use; the editor writes
// to conceptMapsDir and reloads maps in place.
func NewToolsServer(findingsDir, conceptMapsDir string, profile *converter.ProfileService, maps *converter.ConceptMapService, defs *converter.ObservationDefinitions, log zerolog.Logger) http.Handler {
	s := &Server{findingsDir: findingsDir, conceptMapsDir: conceptMapsDir, conceptMapSvc: maps, profile: profile, obsDefs: defs, log: log}
	mux := http.NewServeMux()
	s.mountTools(mux)
	return mux
}
