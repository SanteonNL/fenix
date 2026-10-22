package fhirserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/SanteonNL/fenix/internal/querycompiler"
	"github.com/SanteonNL/fenix/internal/deident"
	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Server serves FHIR search requests by compiling SQL via the query compiler
// and converting the results using the fenix FHIRConverter.
//
// Endpoint: GET /r4/{resourceType}?<fhir-params>
// Response: FHIR Bundle (searchset)
type Server struct {
	compiler  *querycompiler.Compiler
	converter *converter.FHIRConverter
	source    string
	groupID   string
	outputDir string // if non-empty, compiled queries are written here
	log       zerolog.Logger

	conceptMapsDir string                       // directory of FHIR ConceptMap .json files served/edited by the /conceptmaps UI
	conceptMapSvc  *converter.ConceptMapService // reloaded in place whenever the editor saves/deletes a file

	deidentRuleset *deident.Ruleset // nil disables de-identification entirely
	deidentKey     []byte

	groupsMu sync.RWMutex
	groups   map[string]*fhir.Group // Bulk Export Group definitions posted via POST /r4/Group, keyed by id

	exportsMu sync.RWMutex
	exports   map[string]*exportJob // $export jobs kicked off via POST /r4/Group/{id}/$export, keyed by job id
}

// New creates a Server.
//   - compiler       query compiler initialised with config/queries and the repo root
//   - conv           FHIRConverter already wired to the staging/source database
//   - source         source name from config/queries/sources/ (e.g. "hix-test")
//   - groupID        optional group override (e.g. "geboortezorg-2024"), empty for none
//   - outputDir      directory to write compiled queries into; empty disables writing
//   - deidentRuleset the effective de-identification ruleset to apply to every
//     response, or nil to disable de-identification (existing behaviour)
//   - deidentKey     the HMAC key backing deidentRuleset; ignored when deidentRuleset is nil
//   - conceptMapsDir directory of FHIR ConceptMap .json files backing the /conceptmaps editor UI;
//     empty disables the editor's API routes (list/get/save/delete still 404-free but inert)
//   - conceptMapSvc  the same ConceptMapService instance conv's FHIRConverter was built with,
//     so edits made through the UI take effect on the next request without a restart
func New(compiler *querycompiler.Compiler, conv *converter.FHIRConverter, source, groupID, outputDir string, deidentRuleset *deident.Ruleset, deidentKey []byte, conceptMapsDir string, conceptMapSvc *converter.ConceptMapService, log zerolog.Logger) *Server {
	compiler.SetParamTranslator(conv.SourceCodes)
	return &Server{
		compiler:       compiler,
		converter:      conv,
		source:         source,
		groupID:        groupID,
		outputDir:      outputDir,
		deidentRuleset: deidentRuleset,
		deidentKey:     deidentKey,
		conceptMapsDir: conceptMapsDir,
		conceptMapSvc:  conceptMapSvc,
		log:            log,
		groups:         make(map[string]*fhir.Group),
		exports:        make(map[string]*exportJob),
	}
}

// Handler returns an http.Handler for the FHIR API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/r4/", s.handleSearch)
	mux.HandleFunc("/conceptmaps", s.handleConceptMapsUI)
	mux.HandleFunc("/conceptmaps/", s.handleConceptMapsUI)
	mux.HandleFunc("/api/conceptmaps", s.handleConceptMapsAPI)
	mux.HandleFunc("/api/conceptmaps/", s.handleConceptMapsAPI)
	return mux
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	// Extract path segments from /r4/{resourceType}[/{id}[/$export]], or the
	// special /r4/$export-status/{jobId} and /r4/$export-files/{jobId}/{file}
	// paths a kicked-off export's Content-Location/manifest point at.
	rest := strings.TrimPrefix(r.URL.Path, "/r4/")
	segments := strings.Split(rest, "/")
	resourceType := segments[0]
	if resourceType == "" {
		fhirError(w, "missing resource type in path", http.StatusBadRequest)
		return
	}

	switch resourceType {
	case "Group":
		// Group has its own lifecycle (POST to define, GET to dynamically
		// resolve membership, POST .../$export to bulk-export its members)
		// instead of the GET-only search below — see group.go/export.go.
		switch len(segments) {
		case 1:
			s.handleGroup(w, r, "")
		case 2:
			s.handleGroup(w, r, segments[1])
		case 3:
			if segments[2] != "$export" {
				fhirError(w, "not found", http.StatusNotFound)
				return
			}
			s.handleGroupExport(w, r, segments[1])
		default:
			fhirError(w, "not found", http.StatusNotFound)
		}
		return
	case "$export-status":
		if len(segments) != 2 {
			fhirError(w, "not found", http.StatusNotFound)
			return
		}
		s.handleExportStatus(w, segments[1])
		return
	case "$export-files":
		if len(segments) != 3 {
			fhirError(w, "not found", http.StatusNotFound)
			return
		}
		s.handleExportFile(w, segments[1], segments[2])
		return
	}

	if r.Method != http.MethodGet {
		fhirError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Collect FHIR search parameters from the query string
	fhirParams := make(map[string]string)
	for k, vals := range r.URL.Query() {
		if len(vals) > 0 {
			fhirParams[k] = vals[0]
		}
	}

	s.log.Info().
		Str("resourceType", resourceType).
		Str("source", s.source).
		Any("params", fhirParams).
		Msg("FHIR search request")

	resources, found, err := s.search(resourceType, fhirParams)
	if err != nil {
		s.log.Error().Err(err).Str("resourceType", resourceType).Msg("FHIR search failed")
		fhirError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !found {
		fhirError(w, fmt.Sprintf("no queries configured for resource type %q in source %q", resourceType, s.source), http.StatusNotFound)
		return
	}

	if s.deidentRuleset != nil {
		if err := s.deidentify(resourceType, resources, fhirParams["_elements"]); err != nil {
			s.log.Error().Err(err).Str("resourceType", resourceType).Msg("De-identification failed")
			fhirError(w, fmt.Sprintf("de-identification failed: %v", err), http.StatusUnprocessableEntity)
			return
		}
	}

	// Wrap in a FHIR Bundle (searchset)
	entries := make([]map[string]interface{}, len(resources))
	for i, res := range resources {
		entries[i] = map[string]interface{}{"resource": res}
	}
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "searchset",
		"total":        len(resources),
		"entry":        entries,
	}

	w.Header().Set("Content-Type", "application/fhir+json")
	if err := json.NewEncoder(w).Encode(bundle); err != nil {
		s.log.Error().Err(err).Msg("Failed to encode bundle")
	}
}

// search compiles SQL for (resourceType, fhirParams) against s.source/s.groupID
// and converts the resulting rows to FHIR resources. found is false when the
// source has no queries configured for resourceType at all (distinct from a
// configured query that simply matched zero rows).
//
// Not every param in fhirParams necessarily narrowed the SQL above: only
// params listed in a query's `pushdown:` config (config/queries/sources/
// <source>/<source>.yaml) do. filterUnwiredParams is the backstop for the
// rest — it re-checks them against the converted resources in Go, so a
// param still filters correctly even before it's wired as pushdown.
func (s *Server) search(resourceType string, fhirParams map[string]string) (resources []interface{}, found bool, err error) {
	return s.searchInGroup(resourceType, fhirParams, "", "", "")
}

// searchInGroup is search restricted in SQL to the patients of membersSQL (see
// querycompiler.Compiler.ResolveGroupMembersSQL); an empty membersSQL means no restriction.
// Compiled queries are written to queryDir, or outputDir/compiled when empty;
// groupFile names the file holding the group query, for the header of each.
func (s *Server) searchInGroup(resourceType string, fhirParams map[string]string, membersSQL, queryDir, groupFile string) (resources []interface{}, found bool, err error) {
	var rendered []querycompiler.RenderedQuery
	if membersSQL == "" {
		rendered, err = s.compiler.Resolve(s.source, s.groupID, resourceType, fhirParams)
	} else {
		rendered, err = s.compiler.ResolveInGroup(s.source, s.groupID, resourceType, fhirParams, membersSQL)
	}
	if err != nil {
		return nil, false, fmt.Errorf("query resolution failed: %w", err)
	}
	if len(rendered) == 0 {
		return nil, false, nil
	}

	// Write each rendered query to the output folder for inspection
	if queryDir == "" && s.outputDir != "" {
		queryDir = filepath.Join(s.outputDir, "compiled")
	}
	s.writeCompiledQueries(queryDir, resourceType, rendered, groupFile)

	// Join all rendered queries into one multi-statement SQL string.
	// ConvertSQL handles ";" as statement separator, so results from all queries
	// are merged into a single resource map keyed by resource_id.
	sqlParts := make([]string, len(rendered))
	for i, rq := range rendered {
		sqlParts[i] = rq.SQL
	}
	combinedSQL := strings.Join(sqlParts, ";\n")

	resources, err = s.converter.ConvertSQL(combinedSQL)
	if err != nil {
		return nil, true, fmt.Errorf("conversion failed: %w", err)
	}

	pushed := s.compiler.PushdownCodes(s.source, resourceType)
	for code := range s.compiler.InexactParams(resourceType, fhirParams) {
		delete(pushed, code) // not applied in SQL (see InexactParams), so filter it in Go
	}
	resources = filterUnwiredParams(resourceType, fhirParams, pushed, resources, s.compiler.SearchParamField)
	return resources, true, nil
}

// deidentify de-identifies resources (all of resourceType, per this
// handler's single-type search) in place against s.deidentRuleset, using a
// fresh per-request RunID so repeated requests aren't linkable to each
// other. rawElements is the request's own _elements query parameter, if
// any — FENIX has no notion of "an export" to declare elements against
// separately; the resource type and parameters actually posted to this
// endpoint are the only input de-identification needs.
//
// Known limitation: a reference to another resource type (e.g.
// Observation.subject) is hashed by its own id (deident.Deidentify), which
// only matches that other resource's own hashed id — from a *separate*
// request to /r4/{otherType} — if both requests share the same RunID. They
// don't: each request gets its own. $export (group.go/export.go) doesn't
// have this problem: one export job shares a single RunID across every
// resource type it returns, via deidentifyWithRunID below — same fix the
// CLI batch path already has (one RunID per CLI invocation, see
// cmd/fenix/main.go).
func (s *Server) deidentify(resourceType string, resources []interface{}, rawElements string) error {
	var elements map[string][]string
	if rawElements != "" {
		elements = map[string][]string{resourceType: strings.Split(rawElements, ",")}
	}
	return s.deidentifyWithRunID(resources, elements, uuid.NewString())
}

// deidentifyWithRunID is deidentify's shared core, taking an explicit RunID
// and a pre-built elements map (keyed by resourceType, so a caller — e.g.
// $export — can de-identify several resource types under one RunID and
// still restrict _elements per type).
func (s *Server) deidentifyWithRunID(resources []interface{}, elements map[string][]string, runID string) error {
	deidResources := make([]deident.Resource, len(resources))
	for i, r := range resources {
		rt, val, _ := converter.Unwrap(r)
		deidResources[i] = deident.Resource{Type: rt, Value: val}
	}

	ctx := deident.RunContext{Key: s.deidentKey, RunID: runID}
	_, err := deident.Deidentify(deidResources, *s.deidentRuleset, elements, ctx, nil)
	return err
}

// writeCompiledQueries writes each rendered query to dir/{resourceType}_{name}.sql
// (one file per named query, so several Observation queries stay separate).
// An empty dir disables writing.
//
// With groupFile set, the file holds the query as it is on its own (PlainSQL)
// under a header saying it was run restricted to that group's patients.
func (s *Server) writeCompiledQueries(dir, resourceType string, rendered []querycompiler.RenderedQuery, groupFile string) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		s.log.Warn().Err(err).Str("dir", dir).Msg("Failed to create compiled output directory")
		return
	}
	for _, rq := range rendered {
		name := fmt.Sprintf("%s_%s.sql", resourceType, rq.Name)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(compiledFileContent(rq, groupFile)), 0644); err != nil {
			s.log.Warn().Err(err).Str("file", path).Msg("Failed to write compiled query")
			continue
		}
		s.log.Debug().Str("file", path).Msg("Compiled query written")
	}
}

// fhirError writes a minimal FHIR OperationOutcome error response.
func fhirError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"resourceType": "OperationOutcome",
		"issue": []map[string]interface{}{{
			"severity": "error",
			"code":     "processing",
			"details":  map[string]interface{}{"text": msg},
		}},
	})
}

// compiledFileContent is what a compiled-query file contains: the query as it
// is on its own. For a query run inside a group export, a header notes that
// it was executed restricted to the group's patients (the restriction itself
// is only a wrapper, left out here to keep the query readable).
func compiledFileContent(rq querycompiler.RenderedQuery, groupFile string) string {
	if groupFile == "" || rq.PlainSQL == "" {
		return rq.SQL
	}
	return "-- Run for a group export, restricted to the patients selected by " + groupFile + ".\n" +
		"-- Shown below as the query is on its own, without that restriction.\n\n" + rq.PlainSQL
}
