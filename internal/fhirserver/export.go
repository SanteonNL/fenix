package fhirserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/google/uuid"
)

// Lifecycle of an exportJob.
const (
	exportRunning  = "in-progress"
	exportComplete = "complete"
	exportFailed   = "failed"
)

// exportJob is one $export run. POST .../$export validates the request and
// returns 202 straight away; the export itself runs in the background and the
// client polls the status endpoint (Bulk Data Access IG): 202 + X-Progress
// while running, the manifest when complete, the failure's own status and an
// OperationOutcome when it failed.
type exportJob struct {
	id              string
	groupID         string
	transactionTime time.Time

	cancel    context.CancelFunc // stops the background run (DELETE on the status URL)
	spoolDir  string             // where the NDJSON files are spooled
	ownsSpool bool               // spoolDir is a temp folder of this job, removed with it

	mu        sync.Mutex
	state     string
	progress  string            // human-readable, served as X-Progress while running
	errMsg    string            // why it failed
	errStatus int               // HTTP status to report for the failure
	files     map[string]string // resourceType -> path of its NDJSON file (spooled to disk, not held in memory)
	counts    map[string]int    // resourceType -> resource count
}

func (j *exportJob) setProgress(p string) {
	j.mu.Lock()
	j.progress = p
	j.mu.Unlock()
}

// removeSpool deletes the job's temp folder, if it owns one. The folder of an
// export that was given an output directory is kept: it is there to be inspected.
func (j *exportJob) removeSpool() {
	if j.ownsSpool {
		_ = os.RemoveAll(j.spoolDir)
	}
}

// exportRequest is everything the background run needs, validated at kickoff.
type exportRequest struct {
	resourceTypes  []string
	elementsByType map[string][]string
	typeFilters    map[string]map[string]string
	membersSQL     string
	runID          string
	queriesDir     string
	groupFile      string
}

// handleGroupExport kicks off a bulk export of a Group's current members —
// membership is resolved, each requested resource type (_type, required) is
// fetched with its query restricted in SQL to the group's members query
// (searchInGroup), optionally narrowed by its own _typeFilter (a FHIR search like
// "Observation?status=final", independent of the group's own member-filter
// criteria — see parseTypeFilters), then — when de-identification is
// configured — de-identified under a single shared RunID across every
// resource type, so a hashed Observation.subject matches the same patient's
// hashed Patient.id within this job's output (deidentify, by contrast, gives
// every plain search its own RunID, so two separate /r4/{type} calls never
// cross-reference correctly — see its comment in server.go). The caller never
// sees a raw patient id: not in this response, and not in the status/NDJSON
// endpoints it hands back.
//
// Everything that can be checked cheaply (group, parameters, that every type
// is configured, the members query) is checked here and answered with a plain
// 4xx/5xx. The data work happens in runExport, after the 202.
func (s *Server) handleGroupExport(w http.ResponseWriter, r *http.Request, groupID string) {
	if r.Method != http.MethodPost {
		fhirError(w, "method not allowed; POST to kick off $export", http.StatusMethodNotAllowed)
		return
	}

	s.groupsMu.RLock()
	group, ok := s.groups[groupID]
	s.groupsMu.RUnlock()
	if !ok {
		fhirError(w, fmt.Sprintf("Group %q not found", groupID), http.StatusNotFound)
		return
	}

	rawType := r.URL.Query().Get("_type")
	if rawType == "" {
		fhirError(w, `_type is required, e.g. "?_type=Patient,Observation"`, http.StatusBadRequest)
		return
	}
	resourceTypes := splitNonEmpty(rawType, ",")
	for _, rt := range resourceTypes {
		if !validResourceTypeName(rt) {
			fhirError(w, fmt.Sprintf("invalid resource type %q in _type", rt), http.StatusBadRequest)
			return
		}
	}
	elementsByType := parseElements(r.URL.Query().Get("_elements"))
	typeFilters, err := parseTypeFilters(r.URL.Query()["_typeFilter"])
	if err != nil {
		fhirError(w, fmt.Sprintf("invalid _typeFilter: %v", err), http.StatusBadRequest)
		return
	}

	// Compiling is cheap (no query runs), so an unconfigured type or a bad
	// filter is still answered with a 400 here instead of failing the job later.
	for _, rt := range resourceTypes {
		rendered, err := s.render(rt, typeFilters[rt], "")
		if err != nil {
			fhirError(w, fmt.Sprintf("resource type %q: %v", rt, err), http.StatusBadRequest)
			return
		}
		if len(rendered) == 0 {
			fhirError(w, fmt.Sprintf("resource type %q is not configured for source %q", rt, s.source), http.StatusBadRequest)
			return
		}
	}

	// The group's membership as ONE SQL query (member-filters joined on
	// patient number). It is embedded in every resource's query, so each type
	// is fetched already scoped to the group — no per-patient queries and no
	// filtering afterwards.
	membersSQL, err := groupMembersSQL(s, group)
	if err != nil {
		s.log.Error().Err(err).Str("groupId", groupID).Msg("Failed to build Group members query for export")
		fhirError(w, fmt.Sprintf("failed to resolve group members: %v", err), http.StatusInternalServerError)
		return
	}

	job := &exportJob{
		id:              uuid.NewString(),
		groupID:         groupID,
		transactionTime: time.Now().UTC(),
		state:           exportRunning,
		progress:        "starting",
		files:           make(map[string]string, len(resourceTypes)),
		counts:          make(map[string]int, len(resourceTypes)),
	}
	req := exportRequest{
		resourceTypes:  resourceTypes,
		elementsByType: elementsByType,
		typeFilters:    typeFilters,
		membersSQL:     membersSQL,
		// One RunID for the whole job — the fix for the cross-resource-type
		// hashing gap described above.
		runID:     uuid.NewString(),
		groupFile: groupQueryFileName(group),
	}

	// Everything this export runs is written to <outputDir>/exports/
	// <timestamp>_<group_name>/, so the queries and their results can be inspected:
	//   queries/  Group_<group_name>.sql and one <Type>_<queryName>.sql per compiled query
	//   data/     the resulting <Type>.ndjson per resource type
	var dataDir string
	if s.outputDir != "" {
		exportDir, err := createExportDir(filepath.Join(s.outputDir, "exports"), job.transactionTime, group)
		if err == nil {
			req.queriesDir = filepath.Join(exportDir, "queries")
			dataDir = filepath.Join(exportDir, "data")
			for _, d := range []string{req.queriesDir, dataDir} {
				if err = os.Mkdir(d, 0755); err != nil {
					break
				}
			}
		}
		if err != nil {
			s.log.Warn().Err(err).Str("dir", exportDir).Msg("Export: failed to create export folders")
			req.queriesDir, dataDir = "", ""
		} else if err := os.WriteFile(filepath.Join(req.queriesDir, req.groupFile), []byte(membersSQL), 0644); err != nil {
			s.log.Warn().Err(err).Msg("Export: failed to write group query")
		}
	}

	// The NDJSON files are spooled to disk as they are produced, never held
	// whole in memory: in the export folder when there is one, else a temp
	// folder owned by this job.
	job.spoolDir = dataDir
	if job.spoolDir == "" {
		job.spoolDir, err = os.MkdirTemp("", "fenix-export-")
		if err != nil {
			fhirError(w, fmt.Sprintf("creating export spool directory: %v", err), http.StatusInternalServerError)
			return
		}
		job.ownsSpool = true
	}

	// The run outlives this request, so it gets its own context; DELETE on the
	// status URL cancels it.
	ctx, cancel := context.WithCancel(context.Background())
	job.cancel = cancel

	s.exportsMu.Lock()
	s.exports[job.id] = job
	s.exportsMu.Unlock()

	go s.runExport(ctx, job, req)

	w.Header().Set("Content-Location", "/r4/$export-status/"+job.id)
	w.WriteHeader(http.StatusAccepted)
}

// runExport does the export's data work in the background and records the
// outcome on job. The batches of a resource type are fetched in parallel (see
// exportType); the job only turns complete if every type succeeded, so a
// failed de-identification never leaves anything servable.
func (s *Server) runExport(ctx context.Context, job *exportJob, req exportRequest) {
	defer job.cancel()
	start := time.Now()
	if s.exportGate != nil {
		s.exportGate()
	}

	fail := func(status int, err error) {
		job.removeSpool()
		job.mu.Lock()
		job.state, job.errMsg, job.errStatus = exportFailed, err.Error(), status
		job.mu.Unlock()
		s.log.Error().Err(err).Str("jobId", job.id).Str("groupId", job.groupID).Msg("Export failed")
	}

	// Resolve the group's patients once, then export them in batches: each
	// batch is a bounded amount of rows to hold in memory, and batches run in
	// parallel over the connection pool. The ids are sorted so every run
	// splits, and writes, the same way.
	job.setProgress("listing group members")
	memberIDs, err := s.converter.QueryStringsContext(ctx, req.membersSQL)
	if err != nil {
		fail(http.StatusInternalServerError, fmt.Errorf("failed to resolve group members: %w", err))
		return
	}
	sort.Strings(memberIDs)
	batches := chunk(memberIDs, s.exportBatch())

	for i, resourceType := range req.resourceTypes {
		var elements map[string][]string
		if decl, ok := req.elementsByType[resourceType]; ok {
			elements = map[string][]string{resourceType: decl}
		}
		path := filepath.Join(job.spoolDir, resourceType+".ndjson")
		n, status, err := s.exportType(ctx, exportTypeJob{
			resourceType: resourceType,
			filters:      req.typeFilters[resourceType],
			batches:      batches,
			elements:     elements,
			runID:        req.runID,
			queriesDir:   req.queriesDir,
			groupFile:    req.groupFile,
			path:         path,
			progress: func(done int) {
				job.setProgress(fmt.Sprintf("%s: %d/%d batches (type %d of %d)", resourceType, done, len(batches), i+1, len(req.resourceTypes)))
			},
		})
		if err != nil {
			fail(status, fmt.Errorf("exporting %s: %w", resourceType, err))
			return
		}
		job.mu.Lock()
		job.files[resourceType] = path
		job.counts[resourceType] = n
		job.mu.Unlock()
	}

	job.mu.Lock()
	job.state, job.progress = exportComplete, "complete"
	job.mu.Unlock()

	s.log.Info().
		Str("groupId", job.groupID).
		Str("jobId", job.id).
		Strs("types", req.resourceTypes).
		Int("patients", len(memberIDs)).
		Bool("deidentified", s.deidentRuleset != nil).
		Dur("took", time.Since(start)).
		Msg("Export job completed")
}

// groupQueryFileName names the file holding a Group's members query:
// "Group_" plus the Group's name with spaces (and anything not safe in a file
// name) replaced by underscores, e.g. "Group_SIM_patients_with_a_final_Observation.sql".
// A Group without a name falls back to its id.
func groupQueryFileName(group *fhir.Group) string {
	slug := groupSlug(group)
	if slug == "" {
		return "Group.sql"
	}
	return "Group_" + slug + ".sql"
}

// groupSlug is the Group's name made safe for a file or folder name: spaces
// and anything else odd become underscores. A Group without a name falls
// back to its id; "" when it has neither.
func groupSlug(group *fhir.Group) string {
	name := ""
	if group.Name != nil {
		name = *group.Name
	}
	if strings.TrimSpace(name) == "" && group.Id != nil {
		name = *group.Id
	}
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		}
		return '_'
	}, name)
	return strings.Trim(safe, "_")
}

// createExportDir creates <parent>/<YYYYMMDD_HHMMSS>_<group_name>, e.g.
// "20261013_143005_SIM_patients_with_a_final_Observation", in local time.
// If that folder already exists (two exports in the same second) a _2, _3, ...
// suffix is added, so one export never writes into another's folder.
func createExportDir(parent string, t time.Time, group *fhir.Group) (string, error) {
	if err := os.MkdirAll(parent, 0755); err != nil {
		return "", err
	}
	name := t.Local().Format("20060102_150405")
	if slug := groupSlug(group); slug != "" {
		name += "_" + slug
	}
	for n := 1; ; n++ {
		dir := filepath.Join(parent, name)
		if n > 1 {
			dir = fmt.Sprintf("%s_%d", filepath.Join(parent, name), n)
		}
		err := os.Mkdir(dir, 0755)
		if err == nil {
			return dir, nil
		}
		if !os.IsExist(err) {
			return "", err
		}
	}
}

// handleExportStatus is the polling endpoint of a kicked-off export (Bulk Data
// Access IG):
//   - running:   202 Accepted with X-Progress (and Retry-After)
//   - complete:  200 with the manifest
//   - failed:    the failure's own status with an OperationOutcome
//   - DELETE:    cancels a running export / discards a finished one (202), after
//     which the job is gone (404)
func (s *Server) handleExportStatus(w http.ResponseWriter, r *http.Request, jobID string) {
	s.exportsMu.RLock()
	job, ok := s.exports[jobID]
	s.exportsMu.RUnlock()
	if !ok {
		fhirError(w, fmt.Sprintf("export job %q not found", jobID), http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
	case http.MethodDelete:
		job.cancel()
		s.exportsMu.Lock()
		delete(s.exports, jobID)
		s.exportsMu.Unlock()
		// A run that is still going removes its own spool when it notices the
		// cancellation; one that already finished is cleaned up here.
		job.mu.Lock()
		finished := job.state != exportRunning
		job.mu.Unlock()
		if finished {
			job.removeSpool()
		}
		w.WriteHeader(http.StatusAccepted)
		return
	default:
		fhirError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	job.mu.Lock()
	state, progress, errMsg, errStatus := job.state, job.progress, job.errMsg, job.errStatus
	files := make(map[string]string, len(job.files))
	for k, v := range job.files {
		files[k] = v
	}
	counts := make(map[string]int, len(job.counts))
	for k, v := range job.counts {
		counts[k] = v
	}
	job.mu.Unlock()

	switch state {
	case exportRunning:
		w.Header().Set("X-Progress", progress)
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusAccepted)
		return
	case exportFailed:
		fhirError(w, errMsg, errStatus)
		return
	}

	type manifestOutput struct {
		Type  string `json:"type"`
		URL   string `json:"url"`
		Count int    `json:"count"`
	}
	outputs := make([]manifestOutput, 0, len(files))
	for resourceType := range files {
		outputs = append(outputs, manifestOutput{
			Type:  resourceType,
			URL:   fmt.Sprintf("/r4/$export-files/%s/%s.ndjson", job.id, resourceType),
			Count: counts[resourceType],
		})
	}
	sort.Slice(outputs, func(i, j int) bool { return outputs[i].Type < outputs[j].Type })

	manifest := map[string]interface{}{
		"transactionTime":     job.transactionTime.Format(time.RFC3339),
		"request":             fmt.Sprintf("Group/%s/$export", job.groupID),
		"requiresAccessToken": false,
		"output":              outputs,
		"error":               []interface{}{},
	}

	// Per the Bulk Data Access IG, the completed manifest is plain
	// application/json — unlike every other FENIX response, which is
	// application/fhir+json (see fhirError, handleSearch) — because the
	// manifest isn't itself a FHIR resource.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(manifest); err != nil {
		s.log.Error().Err(err).Msg("Failed to encode export manifest")
	}
}

// handleExportFile streams one resource type's already-computed (already
// de-identified, if configured) NDJSON output.
func (s *Server) handleExportFile(w http.ResponseWriter, jobID, filename string) {
	s.exportsMu.RLock()
	job, ok := s.exports[jobID]
	s.exportsMu.RUnlock()
	if !ok {
		fhirError(w, fmt.Sprintf("export job %q not found", jobID), http.StatusNotFound)
		return
	}

	resourceType := strings.TrimSuffix(filename, ".ndjson")
	job.mu.Lock()
	path, ok := job.files[resourceType]
	job.mu.Unlock()
	if !ok {
		fhirError(w, fmt.Sprintf("no output file %q for export job %q", filename, jobID), http.StatusNotFound)
		return
	}

	f, err := os.Open(path)
	if err != nil {
		s.log.Error().Err(err).Str("file", path).Msg("Export: cannot open spooled output")
		fhirError(w, "export output is no longer available", http.StatusGone)
		return
	}
	defer f.Close()

	w.Header().Set("Content-Type", "application/fhir+ndjson")
	if st, err := f.Stat(); err == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	}
	_, _ = io.Copy(w, f) // streamed from disk: memory stays flat however large the file
}

// splitNonEmpty splits s on sep, trims whitespace, and drops empty entries.
func splitNonEmpty(s, sep string) []string {
	var out []string
	for _, part := range strings.Split(s, sep) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// parseElements parses the Bulk Data _elements param — a comma-separated
// list of "ResourceType.field" entries (e.g. "Patient.id,Observation.status")
// — into a map keyed by resourceType, the shape deidentifyWithRunID expects.
func parseElements(raw string) map[string][]string {
	if raw == "" {
		return nil
	}
	out := make(map[string][]string)
	for _, entry := range splitNonEmpty(raw, ",") {
		resourceType, field, ok := strings.Cut(entry, ".")
		if !ok {
			continue
		}
		out[resourceType] = append(out[resourceType], field)
	}
	return out
}

// parseTypeFilters parses the Bulk Data _typeFilter param — zero or more
// "ResourceType?params" FHIR search strings, one per resource type that
// needs narrowing beyond plain group membership (e.g.
// "Observation?status=final") — into a map keyed by resourceType, the exact
// shape s.search expects as fhirParams. Reuses member-filter's own
// "ResourceType?params" parser (group.go) since the syntax is identical.
func parseTypeFilters(raw []string) (map[string]map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]map[string]string, len(raw))
	for _, filter := range raw {
		resourceType, params, err := parseMemberFilter(filter)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", filter, err)
		}
		out[resourceType] = params
	}
	return out, nil
}
