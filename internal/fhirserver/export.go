package fhirserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/google/uuid"
)

// exportJob is the result of one completed $export run. FENIX computes the
// full export synchronously at kickoff time (small SQLite-backed datasets —
// no real background worker needed) but still exposes the standard Bulk
// Data Access IG shape (kickoff -> status -> NDJSON), so a real async
// client (FLARE, see docs/fenix_architecture.md) works against it
// unmodified: POST .../$export returns 202 immediately, and the status
// endpoint below simply already has the manifest ready on the first poll.
type exportJob struct {
	id              string
	groupID         string
	transactionTime time.Time
	output          map[string][]byte // resourceType -> NDJSON bytes
	counts          map[string]int    // resourceType -> resource count
}

// handleGroupExport kicks off a bulk export of a Group's current members —
// one call that does everything server-side: membership is resolved, each
// requested resource type (_type, required) is fetched with its query
// restricted in SQL to the group's members query (searchInGroup), optionally narrowed by its own _typeFilter (a FHIR search like
// "Observation?status=final", independent of the group's own member-filter
// criteria — see parseTypeFilters), then — when de-identification is
// configured — de-identified under a single shared RunID across every
// resource type, so a
// hashed Observation.subject matches the same patient's hashed Patient.id
// within this job's output (deidentify, by contrast, gives every plain
// search its own RunID, so two separate /r4/{type} calls never cross-
// reference correctly — see its comment in server.go). The caller never
// sees a raw patient id: not in this response, and not in the
// status/NDJSON endpoints it hands back.
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
	elementsByType := parseElements(r.URL.Query().Get("_elements"))
	typeFilters, err := parseTypeFilters(r.URL.Query()["_typeFilter"])
	if err != nil {
		fhirError(w, fmt.Sprintf("invalid _typeFilter: %v", err), http.StatusBadRequest)
		return
	}

	// The group's membership as ONE SQL query (member-filters joined on
	// patient number). It is embedded in every resource's query below, so
	// each type is fetched already scoped to the group — no per-patient
	// queries and no filtering afterwards.
	membersSQL, err := groupMembersSQL(s, group)
	if err != nil {
		s.log.Error().Err(err).Str("groupId", groupID).Msg("Failed to build Group members query for export")
		fhirError(w, fmt.Sprintf("failed to resolve group members: %v", err), http.StatusInternalServerError)
		return
	}

	// One RunID for the whole job — the fix for the cross-resource-type
	// hashing gap described above.
	runID := uuid.NewString()
	job := &exportJob{
		id:              uuid.NewString(),
		groupID:         groupID,
		transactionTime: time.Now().UTC(),
		output:          make(map[string][]byte, len(resourceTypes)),
		counts:          make(map[string]int, len(resourceTypes)),
	}

	// Everything this export ran is written to <outputDir>/exports/
	// <timestamp>_<group_name>/, so the queries and their results can be inspected:
	//   queries/  Group_<group_name>.sql and one <Type>_<queryName>.sql per compiled query
	//   data/     the resulting <Type>.ndjson per resource type
	groupFile := groupQueryFileName(group)
	var queriesDir, dataDir string
	if s.outputDir != "" {
		exportDir, err := createExportDir(filepath.Join(s.outputDir, "exports"), job.transactionTime, group)
		if err == nil {
			queriesDir = filepath.Join(exportDir, "queries")
			dataDir = filepath.Join(exportDir, "data")
			for _, d := range []string{queriesDir, dataDir} {
				if err = os.Mkdir(d, 0755); err != nil {
					break
				}
			}
		}
		if err != nil {
			s.log.Warn().Err(err).Str("dir", exportDir).Msg("Export: failed to create export folders")
			queriesDir, dataDir = "", ""
		} else if err := os.WriteFile(filepath.Join(queriesDir, groupFile), []byte(membersSQL), 0644); err != nil {
			s.log.Warn().Err(err).Msg("Export: failed to write group query")
		}
	}

	for _, resourceType := range resourceTypes {
		scoped, found, err := s.searchInGroup(resourceType, typeFilters[resourceType], membersSQL, queriesDir, groupFile)
		if err != nil {
			s.log.Error().Err(err).Str("resourceType", resourceType).Str("groupId", groupID).Msg("Export: search failed")
			fhirError(w, fmt.Sprintf("exporting %s: %v", resourceType, err), http.StatusInternalServerError)
			return
		}
		if !found {
			fhirError(w, fmt.Sprintf("resource type %q is not configured for source %q", resourceType, s.source), http.StatusBadRequest)
			return
		}

		if s.deidentRuleset != nil {
			var elements map[string][]string
			if decl, ok := elementsByType[resourceType]; ok {
				elements = map[string][]string{resourceType: decl}
			}
			if err := s.deidentifyWithRunID(scoped, elements, runID); err != nil {
				s.log.Error().Err(err).Str("resourceType", resourceType).Str("groupId", groupID).Msg("Export: de-identification failed")
				fhirError(w, fmt.Sprintf("de-identifying %s: %v", resourceType, err), http.StatusUnprocessableEntity)
				return
			}
		}

		ndjson, err := converter.ExportToNDJSON(scoped)
		if err != nil {
			s.log.Error().Err(err).Str("resourceType", resourceType).Msg("Export: NDJSON serialization failed")
			fhirError(w, fmt.Sprintf("serializing %s: %v", resourceType, err), http.StatusInternalServerError)
			return
		}

		if dataDir != "" {
			if err := os.WriteFile(filepath.Join(dataDir, resourceType+".ndjson"), ndjson, 0644); err != nil {
				s.log.Warn().Err(err).Str("resourceType", resourceType).Msg("Export: failed to write result file")
			}
		}

		job.output[resourceType] = ndjson
		job.counts[resourceType] = len(scoped)
	}

	s.exportsMu.Lock()
	s.exports[job.id] = job
	s.exportsMu.Unlock()

	s.log.Info().
		Str("groupId", groupID).
		Str("jobId", job.id).
		Strs("types", resourceTypes).
		Bool("deidentified", s.deidentRuleset != nil).
		Msg("Export job completed")

	w.Header().Set("Content-Location", "/r4/$export-status/"+job.id)
	w.WriteHeader(http.StatusAccepted)
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

// handleExportStatus serves the completed job's manifest. Real async
// servers return 202+X-Progress while work is in flight; FENIX's job is
// always already done by the time it exists (see handleGroupExport), so
// this always returns 200 with the full manifest.
func (s *Server) handleExportStatus(w http.ResponseWriter, jobID string) {
	s.exportsMu.RLock()
	job, ok := s.exports[jobID]
	s.exportsMu.RUnlock()
	if !ok {
		fhirError(w, fmt.Sprintf("export job %q not found", jobID), http.StatusNotFound)
		return
	}

	type manifestOutput struct {
		Type  string `json:"type"`
		URL   string `json:"url"`
		Count int    `json:"count"`
	}
	outputs := make([]manifestOutput, 0, len(job.output))
	for resourceType := range job.output {
		outputs = append(outputs, manifestOutput{
			Type:  resourceType,
			URL:   fmt.Sprintf("/r4/$export-files/%s/%s.ndjson", job.id, resourceType),
			Count: job.counts[resourceType],
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
	data, ok := job.output[resourceType]
	if !ok {
		fhirError(w, fmt.Sprintf("no output file %q for export job %q", filename, jobID), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/fhir+ndjson")
	_, _ = w.Write(data)
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
