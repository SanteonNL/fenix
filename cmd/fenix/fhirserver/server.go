package fhirserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/SanteonNL/fenix/cmd/fenix/converter"
	"github.com/SanteonNL/fenix/cmd/fenix/querycompiler"
	"github.com/SanteonNL/fenix/internal/deident"
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

	deidentRuleset *deident.Ruleset // nil disables de-identification entirely
	deidentKey     []byte
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
func New(compiler *querycompiler.Compiler, conv *converter.FHIRConverter, source, groupID, outputDir string, deidentRuleset *deident.Ruleset, deidentKey []byte, log zerolog.Logger) *Server {
	return &Server{
		compiler:       compiler,
		converter:      conv,
		source:         source,
		groupID:        groupID,
		outputDir:      outputDir,
		deidentRuleset: deidentRuleset,
		deidentKey:     deidentKey,
		log:            log,
	}
}

// Handler returns an http.Handler for the FHIR API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/r4/", s.handleSearch)
	return mux
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		fhirError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract resource type from /r4/{resourceType}
	resourceType := strings.TrimPrefix(r.URL.Path, "/r4/")
	resourceType = strings.SplitN(resourceType, "/", 2)[0]
	if resourceType == "" {
		fhirError(w, "missing resource type in path", http.StatusBadRequest)
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

	// Compile SQL using the query compiler
	rendered, err := s.compiler.Resolve(s.source, s.groupID, resourceType, fhirParams)
	if err != nil {
		s.log.Error().Err(err).Str("resourceType", resourceType).Msg("Query resolution failed")
		fhirError(w, fmt.Sprintf("query resolution failed: %v", err), http.StatusInternalServerError)
		return
	}
	if len(rendered) == 0 {
		fhirError(w, fmt.Sprintf("no queries configured for resource type %q in source %q", resourceType, s.source), http.StatusNotFound)
		return
	}

	// Write each rendered query to the output folder for inspection
	s.writeCompiledQueries(resourceType, rendered)

	// Join all rendered queries into one multi-statement SQL string.
	// ConvertSQL handles ";" as statement separator, so results from all queries
	// are merged into a single resource map keyed by resource_id.
	sqlParts := make([]string, len(rendered))
	for i, rq := range rendered {
		sqlParts[i] = rq.SQL
	}
	combinedSQL := strings.Join(sqlParts, ";\n")

	// Execute SQL and convert rows to FHIR structs
	resources, err := s.converter.ConvertSQL(combinedSQL)
	if err != nil {
		s.log.Error().Err(err).Str("resourceType", resourceType).Msg("FHIR conversion failed")
		fhirError(w, fmt.Sprintf("conversion failed: %v", err), http.StatusInternalServerError)
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
// don't: each request gets its own. Fixing that properly needs the actual
// bulk $export job concept (one RunID per export run, shared across every
// resource type it returns) — out of scope today, same as the rest of the
// async $export API. The CLI batch path doesn't have this problem: one CLI
// invocation already shares a single RunID across every SQL file/resource
// type it converts (see cmd/fenix/main.go).

func (s *Server) deidentify(resourceType string, resources []interface{}, rawElements string) error {
	deidResources := make([]deident.Resource, len(resources))
	for i, r := range resources {
		rt, val, _ := converter.Unwrap(r)
		deidResources[i] = deident.Resource{Type: rt, Value: val}
	}

	var elements map[string][]string
	if rawElements != "" {
		elements = map[string][]string{resourceType: strings.Split(rawElements, ",")}
	}

	ctx := deident.RunContext{Key: s.deidentKey, RunID: uuid.NewString()}
	_, err := deident.Deidentify(deidResources, *s.deidentRuleset, elements, ctx, nil)
	return err
}

// writeCompiledQueries writes each rendered query to outputDir/compiled/{resourceType}_{name}.sql.
func (s *Server) writeCompiledQueries(resourceType string, rendered []querycompiler.RenderedQuery) {
	if s.outputDir == "" {
		return
	}
	dir := filepath.Join(s.outputDir, "compiled")
	if err := os.MkdirAll(dir, 0755); err != nil {
		s.log.Warn().Err(err).Str("dir", dir).Msg("Failed to create compiled output directory")
		return
	}
	for _, rq := range rendered {
		name := fmt.Sprintf("%s_%s.sql", resourceType, rq.Name)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(rq.SQL), 0644); err != nil {
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
