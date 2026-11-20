package fhirserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SanteonNL/fenix/internal/conformance"
	"github.com/rs/zerolog"
)

const testVS = `{"resourceType":"ValueSet","id":"ibd-verrichting","url":"https://ig.santeon.nl/sim-on-fhir/ValueSet/ibd-verrichting","version":"0.1.0","status":"draft",
	"compose":{"include":[{"system":"urn:oid:2.16.840.1.113883.2.4.3.27.15.5","concept":[{"code":"034407","display":"Coloscopie"},{"code":"034408"}]}]}}`

func conformanceServer(t *testing.T) *Server {
	t.Helper()
	st := conformance.NewStore(zerolog.Nop())
	for _, r := range []string{testVS,
		`{"resourceType":"StructureDefinition","id":"observation-san","url":"https://ig.santeon.nl/sim-on-fhir/StructureDefinition/observation-san","name":"ObservationSan"}`} {
		if _, err := st.Add([]byte(r), "test"); err != nil {
			t.Fatal(err)
		}
	}
	return &Server{conformance: st, log: zerolog.Nop()}
}

func get(t *testing.T, s *Server, path string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleSearch(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func TestConformanceEndpoints(t *testing.T) {
	s := conformanceServer(t)

	code, body := get(t, s, "/r4/StructureDefinition?url=https://ig.santeon.nl/sim-on-fhir/StructureDefinition/observation-san")
	if code != 200 || body["total"].(float64) != 1 {
		t.Fatalf("search by url: %d %v", code, body)
	}
	code, body = get(t, s, "/r4/ValueSet/ibd-verrichting")
	if code != 200 || body["url"] != "https://ig.santeon.nl/sim-on-fhir/ValueSet/ibd-verrichting" {
		t.Fatalf("read: %d %v", code, body)
	}
	if code, _ = get(t, s, "/r4/ValueSet/missing"); code != 404 {
		t.Fatalf("read missing: %d", code)
	}

	code, body = get(t, s, "/r4/ValueSet/$expand?url=https://ig.santeon.nl/sim-on-fhir/ValueSet/ibd-verrichting&count=1")
	if code != 200 {
		t.Fatalf("expand: %d %v", code, body)
	}
	exp := body["expansion"].(map[string]any)
	if exp["total"].(float64) != 2 || len(exp["contains"].([]any)) != 1 {
		t.Fatalf("expand paging: %v", exp)
	}

	code, body = get(t, s, "/r4/ValueSet/ibd-verrichting/$validate-code?system=urn:oid:2.16.840.1.113883.2.4.3.27.15.5&code=034407")
	if code != 200 || !strings.Contains(mustJSON(body), `"valueBoolean":true`) || !strings.Contains(mustJSON(body), "Coloscopie") {
		t.Fatalf("validate-code: %d %v", code, body)
	}
	_, body = get(t, s, "/r4/ValueSet/$validate-code?url=https://ig.santeon.nl/sim-on-fhir/ValueSet/ibd-verrichting&system=http://loinc.org&code=034407")
	if !strings.Contains(mustJSON(body), `"valueBoolean":false`) {
		t.Fatalf("validate-code wrong system should be false: %v", body)
	}
	if code, _ = get(t, s, "/r4/ValueSet/$expand?url=https://ex.org/unknown"); code != 404 {
		t.Fatalf("expand unknown: %d", code)
	}
	if code, _ = get(t, s, "/r4/ValueSet?publisher=x"); code != 400 {
		t.Fatalf("unsupported search param: %d", code)
	}
}

func TestInModifier(t *testing.T) {
	s := conformanceServer(t)
	lookup := func(rt, code string) (string, string, bool) {
		if code == "code" {
			return "code", "token", true
		}
		return "", "", false
	}
	proc := func(system, code string) interface{} {
		return map[string]interface{}{"resourceType": "Procedure",
			"code": map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": system, "code": code}}}}
	}
	nza := "urn:oid:2.16.840.1.113883.2.4.3.27.15.5"
	resources := []interface{}{proc(nza, "034407"), proc(nza, "999999"), proc("http://snomed.info/sct", "034408"),
		map[string]interface{}{"resourceType": "Procedure"}}
	vs := "https://ig.santeon.nl/sim-on-fhir/ValueSet/ibd-verrichting"

	in := filterUnwiredParams("Procedure", map[string]string{"code:in": vs}, nil, resources, lookup, s.filterDeps())
	if len(in) != 1 {
		t.Fatalf(":in should keep only the member coding, got %d", len(in))
	}
	notIn := filterUnwiredParams("Procedure", map[string]string{"code:not-in": vs}, nil, resources, lookup, s.filterDeps())
	if len(notIn) != 3 {
		t.Fatalf(":not-in should keep the other three (also the one without code), got %d", len(notIn))
	}
	if err := s.checkValueSetParams(map[string]string{"code:in": "https://ex.org/unknown"}); err == nil {
		t.Fatal("an unknown ValueSet must be refused up front")
	}
	if err := s.checkValueSetParams(map[string]string{"code:in": vs}); err != nil {
		t.Fatal(err)
	}
	if err := (&Server{}).checkValueSetParams(map[string]string{"code:in": vs}); err == nil {
		t.Fatal(":in without any ValueSets loaded must be refused")
	}
}

func TestTokenOrAndEscapes(t *testing.T) {
	field := map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": "https://s", "code": "0313|922"}}}
	if !matchToken(field, `https://s|0313\|922`) {
		t.Fatal("escaped | inside the code")
	}
	if !matchToken(field, `https://x|1,https://s|0313\|922`) {
		t.Fatal("comma is OR")
	}
	if matchToken(field, `https://s|0313`) {
		t.Fatal("partial code must not match")
	}
	if !matchToken("final", "amended,final") {
		t.Fatal("OR on a plain code")
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
