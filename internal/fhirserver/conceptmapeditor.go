package fhirserver

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// conceptMapEditorHTML is the single-page editor served at /conceptmaps. It
// talks to the /api/conceptmaps* routes below to list, read, save and delete
// the FHIR ConceptMap .json files in conceptMapsDir.
//
//go:embed conceptmap_editor.html
var conceptMapEditorHTML []byte

// conceptMapSummary is one row of the editor's list view.
type conceptMapSummary struct {
	Filename string `json:"filename"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	Name     string `json:"name"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Target   string `json:"target"` // targetCanonical or targetUri, whichever is set
	Groups   int    `json:"groups"`
	Elements int    `json:"elements"`
}

func (s *Server) handleConceptMapsUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(conceptMapEditorHTML)
}

// handleConceptMapsAPI implements:
//
//	GET    /api/conceptmaps            -> list summaries
//	POST   /api/conceptmaps            -> create (body is a full ConceptMap resource with "id")
//	GET    /api/conceptmaps/{filename} -> raw ConceptMap JSON
//	PUT    /api/conceptmaps/{filename} -> replace raw ConceptMap JSON
//	DELETE /api/conceptmaps/{filename} -> delete
//
// Every write reloads s.conceptMapSvc from disk so the running converter
// picks up the change immediately, without a server restart.
func (s *Server) handleConceptMapsAPI(w http.ResponseWriter, r *http.Request) {
	if s.conceptMapsDir == "" {
		apiError(w, "concept map editing is not configured (fhir.conceptMapsDir is empty)", http.StatusServiceUnavailable)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/conceptmaps")
	rest = strings.TrimPrefix(rest, "/")

	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			s.listConceptMaps(w)
		case http.MethodPost:
			s.createConceptMap(w, r)
		default:
			apiError(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	filename, err := sanitizeConceptMapFilename(rest)
	if err != nil {
		apiError(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getConceptMap(w, filename)
	case http.MethodPut:
		s.saveConceptMap(w, r, filename)
	case http.MethodDelete:
		s.deleteConceptMap(w, filename)
	default:
		apiError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// sanitizeConceptMapFilename rejects path traversal / nested paths and
// ensures a ".json" suffix, so the API can only ever touch files directly
// inside conceptMapsDir.
func sanitizeConceptMapFilename(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid filename %q", name)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	return name, nil
}

func (s *Server) listConceptMaps(w http.ResponseWriter) {
	entries, err := os.ReadDir(s.conceptMapsDir)
	if err != nil {
		apiError(w, fmt.Sprintf("read concept maps dir: %v", err), http.StatusInternalServerError)
		return
	}

	summaries := []conceptMapSummary{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.conceptMapsDir, e.Name()))
		if err != nil {
			s.log.Warn().Err(err).Str("file", e.Name()).Msg("Failed to read concept map")
			continue
		}
		cm, err := fhir.UnmarshalConceptMap(data)
		if err != nil {
			s.log.Warn().Err(err).Str("file", e.Name()).Msg("Failed to parse concept map")
			continue
		}
		summaries = append(summaries, summarizeConceptMap(e.Name(), cm))
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Filename < summaries[j].Filename })

	writeJSON(w, http.StatusOK, summaries)
}

func summarizeConceptMap(filename string, cm fhir.ConceptMap) conceptMapSummary {
	sum := conceptMapSummary{Filename: filename, Status: cm.Status.Code()}
	if cm.Id != nil {
		sum.ID = *cm.Id
	}
	if cm.Url != nil {
		sum.URL = *cm.Url
	}
	if cm.Name != nil {
		sum.Name = *cm.Name
	}
	if cm.Title != nil {
		sum.Title = *cm.Title
	}
	if cm.TargetCanonical != nil {
		sum.Target = *cm.TargetCanonical
	} else if cm.TargetUri != nil {
		sum.Target = *cm.TargetUri
	}
	sum.Groups = len(cm.Group)
	for _, g := range cm.Group {
		sum.Elements += len(g.Element)
	}
	return sum
}

func (s *Server) getConceptMap(w http.ResponseWriter, filename string) {
	data, err := os.ReadFile(filepath.Join(s.conceptMapsDir, filename))
	if err != nil {
		apiError(w, fmt.Sprintf("read %s: %v", filename, err), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// createConceptMap reads a full ConceptMap resource from the request body,
// derives the filename from its "id" field, and refuses to overwrite an
// existing file (use PUT to update one).
func (s *Server) createConceptMap(w http.ResponseWriter, r *http.Request) {
	cm, raw, err := decodeConceptMap(r.Body)
	if err != nil {
		apiError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if cm.Id == nil || strings.TrimSpace(*cm.Id) == "" {
		apiError(w, "ConceptMap.id is required to derive a filename", http.StatusBadRequest)
		return
	}
	filename, err := sanitizeConceptMapFilename(*cm.Id)
	if err != nil {
		apiError(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := filepath.Join(s.conceptMapsDir, filename)
	if _, err := os.Stat(path); err == nil {
		apiError(w, fmt.Sprintf("%s already exists", filename), http.StatusConflict)
		return
	}

	if err := os.WriteFile(path, raw, 0644); err != nil {
		apiError(w, fmt.Sprintf("write %s: %v", filename, err), http.StatusInternalServerError)
		return
	}
	s.reloadConceptMaps()
	writeJSON(w, http.StatusCreated, summarizeConceptMap(filename, cm))
}

func (s *Server) saveConceptMap(w http.ResponseWriter, r *http.Request, filename string) {
	_, raw, err := decodeConceptMap(r.Body)
	if err != nil {
		apiError(w, err.Error(), http.StatusBadRequest)
		return
	}

	path := filepath.Join(s.conceptMapsDir, filename)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		apiError(w, fmt.Sprintf("write %s: %v", filename, err), http.StatusInternalServerError)
		return
	}
	s.reloadConceptMaps()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteConceptMap(w http.ResponseWriter, filename string) {
	path := filepath.Join(s.conceptMapsDir, filename)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			apiError(w, fmt.Sprintf("%s not found", filename), http.StatusNotFound)
			return
		}
		apiError(w, fmt.Sprintf("delete %s: %v", filename, err), http.StatusInternalServerError)
		return
	}
	s.reloadConceptMaps()
	w.WriteHeader(http.StatusNoContent)
}

// decodeConceptMap reads and validates a request body as a FHIR ConceptMap,
// returning both the typed resource (for deriving metadata) and the
// pretty-printed bytes actually written to disk.
func decodeConceptMap(body io.Reader) (fhir.ConceptMap, []byte, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return fhir.ConceptMap{}, nil, fmt.Errorf("read body: %w", err)
	}
	cm, err := fhir.UnmarshalConceptMap(data)
	if err != nil {
		return fhir.ConceptMap{}, nil, fmt.Errorf("invalid ConceptMap: %w", err)
	}
	pretty, err := json.MarshalIndent(cm, "", "  ")
	if err != nil {
		return fhir.ConceptMap{}, nil, fmt.Errorf("re-encode ConceptMap: %w", err)
	}
	return cm, pretty, nil
}

// reloadConceptMaps rebuilds s.conceptMapSvc from conceptMapsDir in place so
// the next FHIR conversion request picks up the edit immediately.
func (s *Server) reloadConceptMaps() {
	if s.conceptMapSvc == nil {
		return
	}
	if err := s.conceptMapSvc.LoadDir(s.conceptMapsDir); err != nil {
		s.log.Warn().Err(err).Str("dir", s.conceptMapsDir).Msg("Failed to reload concept maps after edit")
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, msg string, code int) {
	writeJSON(w, code, map[string]string{"error": msg})
}
