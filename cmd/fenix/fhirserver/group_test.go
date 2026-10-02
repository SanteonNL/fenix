package fhirserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// fakeSearcher simulates the query-compiler+converter pipeline for a fixed
// set of resourceType -> resources, so Group membership resolution can be
// tested without a real database.
type fakeSearcher struct {
	byType map[string][]interface{}
}

func (f fakeSearcher) search(resourceType string, _ map[string]string) ([]interface{}, bool, error) {
	resources, ok := f.byType[resourceType]
	if !ok {
		return nil, false, nil
	}
	return resources, true, nil
}

func condition(subjectID string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Condition",
		"subject":      map[string]interface{}{"reference": "Patient/" + subjectID},
	}
}

func encounter(subjectID string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Encounter",
		"subject":      map[string]interface{}{"reference": "Patient/" + subjectID},
	}
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

func TestExtractPatientID(t *testing.T) {
	if id, ok := extractPatientID("Patient", map[string]interface{}{"resourceType": "Patient", "id": "p1"}); !ok || id != "p1" {
		t.Fatalf("Patient extraction = %q, %v", id, ok)
	}
	if id, ok := extractPatientID("Condition", condition("p2")); !ok || id != "p2" {
		t.Fatalf("Condition extraction = %q, %v", id, ok)
	}
	if _, ok := extractPatientID("Condition", map[string]interface{}{"resourceType": "Condition"}); ok {
		t.Fatal("expected no match for Condition without subject")
	}
}

func TestResolveGroupMembers_NoFilters_ReturnsStaticMembers(t *testing.T) {
	ref := "Patient/static-1"
	group := &fhir.Group{Member: []fhir.GroupMember{{Entity: fhir.Reference{Reference: &ref}}}}

	members, err := resolveGroupMembers(fakeSearcher{}, group)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 1 || *members[0].Entity.Reference != ref {
		t.Fatalf("members = %+v", members)
	}
}

func TestResolveGroupMembers_SingleFilter(t *testing.T) {
	group := &fhir.Group{
		Extension: []fhir.Extension{memberFilterExt("Condition?code=X")},
	}
	fs := fakeSearcher{byType: map[string][]interface{}{
		"Condition": {condition("p1"), condition("p2")},
	}}

	members, err := resolveGroupMembers(fs, group)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("got %d members, want 2: %+v", len(members), members)
	}
}

func TestResolveGroupMembers_MultipleFilters_Intersect(t *testing.T) {
	group := &fhir.Group{
		Extension: []fhir.Extension{
			memberFilterExt("Condition?code=X"),
			memberFilterExt("Encounter?date=ge2023-01-01"),
		},
	}
	fs := fakeSearcher{byType: map[string][]interface{}{
		"Condition": {condition("p1"), condition("p2"), condition("p3")},
		"Encounter": {encounter("p2"), encounter("p3"), encounter("p4")},
	}}

	members, err := resolveGroupMembers(fs, group)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := map[string]bool{}
	for _, m := range members {
		got[*m.Entity.Reference] = true
	}
	want := map[string]bool{"Patient/p2": true, "Patient/p3": true}
	if len(got) != len(want) {
		t.Fatalf("members = %v, want %v", got, want)
	}
	for ref := range want {
		if !got[ref] {
			t.Fatalf("missing %s in %v", ref, got)
		}
	}
}

func TestResolveGroupMembers_UnconfiguredResourceType_Errors(t *testing.T) {
	group := &fhir.Group{Extension: []fhir.Extension{memberFilterExt("Observation?code=X")}}

	if _, err := resolveGroupMembers(fakeSearcher{}, group); err == nil {
		t.Fatal("expected error for unconfigured resource type")
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

	body := []byte(`{"resourceType":"Group","type":"person","actual":true,"name":"Test cohort"}`)
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
	if fetched.Name == nil || *fetched.Name != "Test cohort" {
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
