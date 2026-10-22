package fhirserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/SanteonNL/fenix/internal/querycompiler"
	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// fakeGroupMembers records the filters it is asked to join and answers the group
// query with a fixed id list, so Group membership resolution can be tested
// without a real database.
type fakeGroupMembers struct {
	ids     []string
	filters []querycompiler.GroupFilter
	err     error

	notPushed map[string]bool     // resource types whose filters can't be pushed down
	matching  map[string][]string // resource type -> patients found by the fallback search
}

func (f *fakeGroupMembers) pushedDown(resourceType string, _ map[string]string) bool {
	return !f.notPushed[resourceType]
}

func (f *fakeGroupMembers) matchingPatients(resourceType string, _ map[string]string) ([]string, error) {
	return f.matching[resourceType], nil
}

func (f *fakeGroupMembers) resolveGroupMembersSQL(filters []querycompiler.GroupFilter) (string, error) {
	f.filters = filters
	return "COHORT SQL", f.err
}

func (f *fakeGroupMembers) queryIDs(sql string) ([]string, error) {
	if sql != "COHORT SQL" {
		return nil, fmt.Errorf("unexpected sql %q", sql)
	}
	return f.ids, nil
}

func memberFilterExt(filter string) fhir.Extension {
	v := filter
	return fhir.Extension{Url: memberFilterURL, ValueString: &v}
}

func TestParseMemberFilter(t *testing.T) {
	rt, params, err := parseMemberFilter("Condition?code=363346000&clinical-status=active")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rt != "Condition" {
		t.Fatalf("resourceType = %q, want Condition", rt)
	}
	if params["code"] != "363346000" || params["clinical-status"] != "active" {
		t.Fatalf("params = %v", params)
	}

	if _, _, err := parseMemberFilter("?code=1"); err == nil {
		t.Fatal("expected error for missing resource type")
	}
}

func TestResolveGroupMembers_NoFilters_ReturnsStaticMembers(t *testing.T) {
	ref := "Patient/static-1"
	group := &fhir.Group{Member: []fhir.GroupMember{{Entity: fhir.Reference{Reference: &ref}}}}

	members, err := resolveGroupMembers(&fakeGroupMembers{}, group)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 1 || *members[0].Entity.Reference != ref {
		t.Fatalf("members = %+v", members)
	}
}

func TestResolveGroupMembers_FiltersJoinedIntoOneQuery(t *testing.T) {
	group := &fhir.Group{
		Extension: []fhir.Extension{
			memberFilterExt("Condition?code=X"),
			memberFilterExt("Encounter?date=ge2023-01-01"),
		},
	}
	fc := &fakeGroupMembers{ids: []string{"p3", "p2"}}

	members, err := resolveGroupMembers(fc, group)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// One group query carrying BOTH filters — nothing is intersected in Go.
	want := []querycompiler.GroupFilter{
		{ResourceType: "Condition", Params: map[string]string{"code": "X"}},
		{ResourceType: "Encounter", Params: map[string]string{"date": "ge2023-01-01"}},
	}
	if !reflect.DeepEqual(fc.filters, want) {
		t.Fatalf("filters = %+v, want %+v", fc.filters, want)
	}
	if len(members) != 2 || *members[0].Entity.Reference != "Patient/p2" || *members[1].Entity.Reference != "Patient/p3" {
		t.Fatalf("members = %+v, want sorted p2,p3", members)
	}
}

func TestResolveGroupMembers_UnpushedFilterFallsBackToStaticIDs(t *testing.T) {
	group := &fhir.Group{
		Extension: []fhir.Extension{
			memberFilterExt("Observation?code=X"),
			memberFilterExt("Patient?birthdate=ge2000-01-01"),
		},
	}
	fc := &fakeGroupMembers{
		ids:       []string{"p1"},
		notPushed: map[string]bool{"Observation": true},
		matching:  map[string][]string{"Observation": {"p1", "p2"}},
	}

	if _, err := resolveGroupMembers(fc, group); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []querycompiler.GroupFilter{
		{ResourceType: "Observation", Params: map[string]string{"code": "X"}, Static: true, PatientIDs: []string{"p1", "p2"}},
		{ResourceType: "Patient", Params: map[string]string{"birthdate": "ge2000-01-01"}},
	}
	if !reflect.DeepEqual(fc.filters, want) {
		t.Fatalf("filters = %+v, want %+v", fc.filters, want)
	}
}

func TestResolveGroupMembers_QueryError(t *testing.T) {
	group := &fhir.Group{Extension: []fhir.Extension{memberFilterExt("Observation?code=X")}}

	if _, err := resolveGroupMembers(&fakeGroupMembers{err: fmt.Errorf("not pushed down")}, group); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func newTestServer() *Server {
	return &Server{
		groups: make(map[string]*fhir.Group),
		log:    zerolog.Nop(),
	}
}

func TestHandleGroup_CreateAndReadStaticGroup(t *testing.T) {
	s := newTestServer()

	body := []byte(`{"resourceType":"Group","type":"person","actual":true,"name":"Test group"}`)
	req := httptest.NewRequest(http.MethodPost, "/r4/Group", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body = %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatal("missing Location header")
	}

	var created fhir.Group
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decoding created Group: %v", err)
	}
	if created.Id == nil || *created.Id == "" {
		t.Fatal("expected server-assigned id")
	}
	if !strings.HasPrefix(rec.Body.String(), `{"resourceType":"Group"`) {
		t.Fatalf("expected resourceType first in POST response, got: %s", rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/r4/"+location[len("/r4/"):], nil)
	getRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	if !strings.HasPrefix(getRec.Body.String(), `{"resourceType":"Group"`) {
		t.Fatalf("expected resourceType first in GET response, got: %s", getRec.Body.String())
	}
	var fetched fhir.Group
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decoding fetched Group: %v", err)
	}
	if fetched.Name == nil || *fetched.Name != "Test group" {
		t.Fatalf("fetched Group = %+v", fetched)
	}
}

func TestHandleGroup_ReadUnknownID(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest(http.MethodGet, "/r4/Group/does-not-exist", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
