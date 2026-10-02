package fhirserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/google/uuid"
)

// Group implements the Bulk Export IG's Group-based cohort pattern (see
// docs/fenix_architecture.md, "Generated — Group.json"): a Group is POSTed
// once with member-filter extensions describing its criteria, and every GET
// re-evaluates those criteria against the live source data — membership is
// never cached, so the patient list always reflects the current data.
//
// member-filter / members-refreshed reuse the extension URLs FENIX already
// documents for the generated Bulk Cohort Group, even though the base Bulk
// Data Access IG (https://build.fhir.org/ig/HL7/bulk-data/en/export.html)
// leaves criteria-based group membership entirely up to the data provider.
const (
	memberFilterURL     = "http://hl7.org/fhir/uv/bulkdata/StructureDefinition/member-filter"
	membersRefreshedURL = "http://hl7.org/fhir/uv/bulkdata/StructureDefinition/members-refreshed"
)

// groupJSON marshals a Group with "resourceType" as the first JSON property.
// The FHIR spec doesn't require this (json.html: "parsers cannot assume that
// the resourceType property will come first"), but it's the convention every
// other FENIX response follows — see fhirOutput in cmd/fenix/converter,
// which exists for exactly this reason, because the generated fhir.* models'
// own MarshalJSON always appends resourceType last.
type groupJSON fhir.Group

func (g groupJSON) MarshalJSON() ([]byte, error) {
	inner, err := json.Marshal(fhir.Group(g))
	if err != nil {
		return nil, err
	}
	const suffix = `,"resourceType":"Group"}`
	if !bytes.HasSuffix(inner, []byte(suffix)) {
		return inner, nil // unexpected shape — leave untouched rather than corrupt it
	}
	out := make([]byte, 0, len(inner))
	out = append(out, `{"resourceType":"Group",`...)
	out = append(out, inner[1:len(inner)-len(suffix)]...)
	out = append(out, '}')
	return out, nil
}

// searcher resolves a FHIR search to converted FHIR resources. *Server
// implements it via search (server.go). Group membership resolution depends
// on this interface, not *Server directly, so it can be tested without a
// real query compiler/converter/database.
type searcher interface {
	search(resourceType string, fhirParams map[string]string) (resources []interface{}, found bool, err error)
}

// handleGroup dispatches /r4/Group[/{id}] requests: POST to the collection
// defines a new Group, GET on a specific id dynamically resolves and returns
// its current membership.
func (s *Server) handleGroup(w http.ResponseWriter, r *http.Request, id string) {
	switch {
	case r.Method == http.MethodPost && id == "":
		s.handleGroupCreate(w, r)
	case r.Method == http.MethodGet && id != "":
		s.handleGroupRead(w, id)
	case r.Method == http.MethodGet && id == "":
		fhirError(w, "Group search is not supported; GET /r4/Group/{id} to read a specific group", http.StatusNotImplemented)
	default:
		fhirError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGroupCreate stores a posted Group definition (typically carrying one
// or more member-filter extensions) and assigns it a server-generated id.
func (s *Server) handleGroupCreate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fhirError(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}

	group, err := fhir.UnmarshalGroup(body)
	if err != nil {
		fhirError(w, fmt.Sprintf("invalid Group resource: %v", err), http.StatusBadRequest)
		return
	}

	id := uuid.NewString()
	group.Id = &id

	s.groupsMu.Lock()
	s.groups[id] = &group
	s.groupsMu.Unlock()

	s.log.Info().
		Str("groupId", id).
		Int("memberFilters", len(extractMemberFilters(group.Extension))).
		Msg("Group created")

	w.Header().Set("Content-Type", "application/fhir+json")
	w.Header().Set("Location", "/r4/Group/"+id)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(groupJSON(group)); err != nil {
		s.log.Error().Err(err).Msg("Failed to encode created Group")
	}
}

// handleGroupRead looks up a stored Group and returns it with Member freshly
// resolved from its member-filter extensions — every call re-evaluates the
// criteria, so the result always reflects the current source data.
func (s *Server) handleGroupRead(w http.ResponseWriter, id string) {
	s.groupsMu.RLock()
	stored, ok := s.groups[id]
	s.groupsMu.RUnlock()
	if !ok {
		fhirError(w, fmt.Sprintf("Group %q not found", id), http.StatusNotFound)
		return
	}

	members, err := resolveGroupMembers(s, stored)
	if err != nil {
		s.log.Error().Err(err).Str("groupId", id).Msg("Failed to resolve Group members")
		fhirError(w, fmt.Sprintf("failed to resolve group members: %v", err), http.StatusInternalServerError)
		return
	}

	// Build a response copy rather than mutating the stored definition —
	// membership is recomputed fresh on every read, never cached.
	out := *stored
	out.Member = members
	out.Extension = withMembersRefreshed(stored.Extension, time.Now().UTC())

	w.Header().Set("Content-Type", "application/fhir+json")
	if err := json.NewEncoder(w).Encode(groupJSON(out)); err != nil {
		s.log.Error().Err(err).Msg("Failed to encode Group")
	}
}

// resolveGroupMembers evaluates a Group's member-filter extensions and
// returns the resulting member list. Each extension's valueString is a FHIR
// search of the form "ResourceType?params" (e.g.
// "Condition?code=363346000&clinical-status=active"); a patient must match
// every filter to be included (AND semantics across filters), matching the
// multi-filter cohort example in docs/fenix_architecture.md. A Group with no
// member-filter extensions is static — its own Member list is returned
// unchanged.
func resolveGroupMembers(s searcher, group *fhir.Group) ([]fhir.GroupMember, error) {
	filters := extractMemberFilters(group.Extension)
	if len(filters) == 0 {
		return group.Member, nil
	}

	var patientIDs map[string]struct{}
	for i, filter := range filters {
		resourceType, params, err := parseMemberFilter(filter)
		if err != nil {
			return nil, fmt.Errorf("invalid member-filter %q: %w", filter, err)
		}

		resources, found, err := s.search(resourceType, params)
		if err != nil {
			return nil, fmt.Errorf("resolving member-filter %q: %w", filter, err)
		}
		if !found {
			return nil, fmt.Errorf("member-filter %q: resource type %q is not configured for this source", filter, resourceType)
		}

		matched := make(map[string]struct{}, len(resources))
		for _, res := range resources {
			if pid, ok := extractPatientID(resourceType, res); ok {
				matched[pid] = struct{}{}
			}
		}

		if i == 0 {
			patientIDs = matched
			continue
		}
		for pid := range patientIDs {
			if _, ok := matched[pid]; !ok {
				delete(patientIDs, pid)
			}
		}
	}

	members := make([]fhir.GroupMember, 0, len(patientIDs))
	for pid := range patientIDs {
		ref := "Patient/" + pid
		members = append(members, fhir.GroupMember{Entity: fhir.Reference{Reference: &ref}})
	}
	sort.Slice(members, func(i, j int) bool {
		return *members[i].Entity.Reference < *members[j].Entity.Reference
	})
	return members, nil
}

// extractMemberFilters returns the valueString of every member-filter
// extension on a Group, in document order.
func extractMemberFilters(exts []fhir.Extension) []string {
	var out []string
	for _, e := range exts {
		if e.Url == memberFilterURL && e.ValueString != nil {
			out = append(out, *e.ValueString)
		}
	}
	return out
}

// parseMemberFilter splits a member-filter valueString ("ResourceType?params")
// into its resource type and FHIR search params, using the same single-value-
// per-key shape handleSearch builds from a real request's query string.
func parseMemberFilter(filter string) (resourceType string, params map[string]string, err error) {
	resourceType, rawQuery, _ := strings.Cut(filter, "?")
	if resourceType == "" {
		return "", nil, fmt.Errorf("missing resource type before '?'")
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", nil, fmt.Errorf("invalid query string: %w", err)
	}
	params = make(map[string]string, len(values))
	for k, v := range values {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	return resourceType, params, nil
}

// extractPatientID returns the patient id a converted resource belongs to:
// its own id for Patient, or the id referenced by its "subject" element
// (Condition, Encounter, Observation, ...) otherwise. Resources without a
// recognisable subject reference are skipped rather than erroring, since a
// cohort filter may legitimately include resources with no subject.
func extractPatientID(resourceType string, resource interface{}) (string, bool) {
	b, err := json.Marshal(resource)
	if err != nil {
		return "", false
	}

	if resourceType == "Patient" {
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(b, &p); err != nil || p.ID == "" {
			return "", false
		}
		return p.ID, true
	}

	var r struct {
		Subject *struct {
			Reference string `json:"reference"`
		} `json:"subject"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.Subject == nil || r.Subject.Reference == "" {
		return "", false
	}
	return strings.TrimPrefix(r.Subject.Reference, "Patient/"), true
}

// withMembersRefreshed returns exts with any existing members-refreshed
// extension replaced by one carrying t — see the Group.json example in
// docs/fenix_architecture.md ("populated by FENIX at runtime").
func withMembersRefreshed(exts []fhir.Extension, t time.Time) []fhir.Extension {
	out := make([]fhir.Extension, 0, len(exts)+1)
	for _, e := range exts {
		if e.Url != membersRefreshedURL {
			out = append(out, e)
		}
	}
	ts := t.Format(time.RFC3339)
	out = append(out, fhir.Extension{Url: membersRefreshedURL, ValueDateTime: &ts})
	return out
}
