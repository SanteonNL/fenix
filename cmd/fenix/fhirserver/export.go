package fhirserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/cmd/fenix/converter"
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
// requested resource type (_type, required) is fetched, optionally narrowed
// by its own _typeFilter (a FHIR search like "Observation?status=final",
// independent of the group's own member-filter criteria — see
// parseTypeFilters), scoped to the group's members internally (by their
// raw, source-system id), then — when de-identification is configured — de-
// identified under a single shared RunID across every resource type, so a
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

	members, err := resolveGroupMembers(s, group)
	if err != nil {
		s.log.Error().Err(err).Str("groupId", groupID).Msg("Failed to resolve Group members for export")
		fhirError(w, fmt.Sprintf("failed to resolve group members: %v", err), http.StatusInternalServerError)
		return
	}
	memberIDs := make(map[string]struct{}, len(members))
	for _, m := range members {
		if m.Entity.Reference != nil {
			memberIDs[strings.TrimPrefix(*m.Entity.Reference, "Patient/")] = struct{}{}
		}
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

	for _, resourceType := range resourceTypes {
		resources, found, err := s.search(resourceType, typeFilters[resourceType])
		if err != nil {
			s.log.Error().Err(err).Str("resourceType", resourceType).Str("groupId", groupID).Msg("Export: search failed")
			fhirError(w, fmt.Sprintf("exporting %s: %v", resourceType, err), http.StatusInternalServerError)
			return
		}
		if !found {
			fhirError(w, fmt.Sprintf("resource type %q is not configured for source %q", resourceType, s.source), http.StatusBadRequest)
			return
		}

		scoped := make([]interface{}, 0, len(resources))
		for _, res := range resources {
			if pid, ok := extractPatientID(resourceType, res); ok {
				if _, isMember := memberIDs[pid]; isMember {
					scoped = append(scoped, res)
				}
			}
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
		Int("members", len(memberIDs)).
		Bool("deidentified", s.deidentRuleset != nil).
		Msg("Export job completed")

	w.Header().Set("Content-Location", "/r4/$export-status/"+job.id)
	w.WriteHeader(http.StatusAccepted)
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
