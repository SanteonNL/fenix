package fhirserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/rs/zerolog"
)

const testGenderVS = "http://hl7.org/fhir/ValueSet/administrative-gender"

func toolsFixture(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	findingsDir, mapsDir, profDir := t.TempDir(), t.TempDir(), t.TempDir()
	cm := `{"resourceType":"ConceptMap","id":"g","status":"active","targetCanonical":"` + testGenderVS + `",
	 "group":[{"element":[{"code":"M","target":[{"code":"male"}]}],"unmapped":{"mode":"fixed","code":"unknown"}}]}`
	if err := os.WriteFile(filepath.Join(mapsDir, "g.json"), []byte(cm), 0o644); err != nil {
		t.Fatal(err)
	}
	sd := `{"resourceType":"StructureDefinition","id":"p","url":"http://x/p","name":"P","status":"draft","kind":"resource","abstract":false,"type":"Patient",
	 "snapshot":{"element":[{"id":"Patient","path":"Patient","min":0,"max":"*"},
	 {"id":"Patient.gender","path":"Patient.gender","min":0,"max":"1","type":[{"code":"code"}],"binding":{"strength":"required","valueSet":"` + testGenderVS + `|4.0.1"}}]}}`
	if err := os.WriteFile(filepath.Join(profDir, "p.json"), []byte(sd), 0o644); err != nil {
		t.Fatal(err)
	}
	profile := converter.NewProfileService(zerolog.Nop())
	if err := profile.LoadDir(profDir); err != nil {
		t.Fatal(err)
	}
	maps := converter.NewConceptMapService(zerolog.Nop())
	if err := maps.LoadDir(mapsDir); err != nil {
		t.Fatal(err)
	}
	return NewToolsServer(findingsDir, mapsDir, profile, maps, nil, zerolog.Nop()), findingsDir, mapsDir
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", path, strings.NewReader(body)))
	return rec
}

func TestMappingEndpointReportsStatusPerCode(t *testing.T) {
	h, _, _ := toolsFixture(t)
	rec := post(h, "/api/findings/mapping", `{"path":"Patient.gender","codes":["M","man"]}`)
	var got struct {
		Bound    bool   `json:"bound"`
		Valueset string `json:"valueset"`
		HasMap   bool   `json:"has_map"`
		Default  struct{ Code string }
		Codes    []struct{ Code, Status, Target, File string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err, rec.Body.String())
	}
	if !got.Bound || !got.HasMap || got.Valueset != testGenderVS || got.Default.Code != "unknown" || len(got.Codes) != 2 {
		t.Fatalf("%+v", got)
	}
	if c := got.Codes[0]; c.Status != "mapped" || c.Target != "male" || c.File != "g.json" {
		t.Errorf("M: %+v", c)
	}
	if c := got.Codes[1]; c.Status != "default" || c.Target != "unknown" || c.File != "g.json" {
		t.Errorf("man: %+v", c)
	}
	// an element without a binding
	if rec := post(h, "/api/findings/mapping", `{"path":"Patient.birthDate","codes":["x"]}`); strings.Contains(rec.Body.String(), `"bound":true`) {
		t.Errorf("unbound element reported bound: %s", rec.Body.String())
	}
}

func TestIgnoreListSurvivesAndRestores(t *testing.T) {
	h, dir, _ := toolsFixture(t)
	body := `{"action":"ignore","items":[{"path":"Patient.gender","code":"man"},{"path":"Patient.gender","code":"vrouw"},{"path":"Patient.gender","code":"man"}]}`
	if rec := post(h, "/api/findings/ignored", body); rec.Code != 200 || strings.Count(rec.Body.String(), `"code"`) != 2 {
		t.Fatalf("ignore: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ignoredFile)); err != nil {
		t.Fatal("the list is stored next to the findings:", err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/findings/ignored", nil))
	if strings.Count(rec.Body.String(), `"code"`) != 2 {
		t.Errorf("GET: %s", rec.Body.String())
	}
	rec = post(h, "/api/findings/ignored", `{"action":"restore","items":[{"path":"Patient.gender","code":"man"}]}`)
	if strings.Count(rec.Body.String(), `"code"`) != 1 || !strings.Contains(rec.Body.String(), "vrouw") {
		t.Errorf("restore: %s", rec.Body.String())
	}
	if rec := post(h, "/api/findings/ignored", `{"action":"bogus"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad action: %d", rec.Code)
	}
}

func TestDefinitionsEndpointDescribesEachObservationCode(t *testing.T) {
	findingsDir := t.TempDir()
	cms := converter.NewConceptMapService(zerolog.Nop())
	vsDir, odDir := t.TempDir(), t.TempDir()
	_ = os.WriteFile(filepath.Join(vsDir, "a.json"), []byte(`{"resourceType":"ValueSet","id":"a","url":"https://x/a","compose":{"include":[{"concept":[{"code":"L1"},{"code":"L2"}]}]}}`), 0o644)
	_ = os.WriteFile(filepath.Join(odDir, "l.json"), []byte(`{"resourceType":"ObservationDefinition","id":"od-l","code":{"coding":[{"code":"MCD_L"}]},"permittedDataType":["CodeableConcept"],"validCodedValueSet":{"reference":"https://x/a"}}`), 0o644)
	if err := cms.LoadValueSetsDir(vsDir); err != nil {
		t.Fatal(err)
	}
	od := converter.NewObservationDefinitions(cms, zerolog.Nop())
	if err := od.LoadDir(odDir); err != nil {
		t.Fatal(err)
	}
	h := NewToolsServer(findingsDir, "", nil, cms, od, zerolog.Nop())

	rec := post(h, "/api/findings/definitions", `{"codes":["MCD_L","NOPE"]}`)
	var got struct {
		Definitions []struct {
			Code    string
			Found   bool
			ID      string
			Types   []string
			Answers []string
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Definitions) != 2 {
		t.Fatalf("%v %s", err, rec.Body.String())
	}
	if d := got.Definitions[0]; !d.Found || d.ID != "od-l" || len(d.Answers) != 2 || d.Types[0] != "CodeableConcept" {
		t.Errorf("MCD_L: %+v", d)
	}
	if d := got.Definitions[1]; d.Found {
		t.Errorf("NOPE has no definition: %+v", d)
	}
}

func TestAnswersEndpointSaysHowAnAnswerIsMapped(t *testing.T) {
	cms := converter.NewConceptMapService(zerolog.Nop())
	vsDir, odDir, cmDir := t.TempDir(), t.TempDir(), t.TempDir()
	_ = os.WriteFile(filepath.Join(vsDir, "a.json"), []byte(`{"resourceType":"ValueSet","id":"a","url":"https://x/a","compose":{"include":[{"concept":[{"code":"L1"},{"code":"L2"}]}]}}`), 0o644)
	_ = os.WriteFile(filepath.Join(odDir, "l.json"), []byte(`{"resourceType":"ObservationDefinition","id":"od-l","code":{"coding":[{"code":"MCD_L"}]},"permittedDataType":["CodeableConcept"],"validCodedValueSet":{"reference":"ValueSet/a"}}`), 0o644)
	_ = os.WriteFile(filepath.Join(cmDir, "m.json"), []byte(`{"resourceType":"ConceptMap","id":"m","status":"active","targetCanonical":"https://x/a","group":[{"element":[{"code":"P1","target":[{"code":"L1"}]}]}]}`), 0o644)
	if err := cms.LoadValueSetsDir(vsDir); err != nil {
		t.Fatal(err)
	}
	if err := cms.LoadDir(cmDir); err != nil {
		t.Fatal(err)
	}
	od := converter.NewObservationDefinitions(cms, zerolog.Nop())
	if err := od.LoadDir(odDir); err != nil {
		t.Fatal(err)
	}
	h := NewToolsServer(t.TempDir(), "", nil, cms, od, zerolog.Nop())

	rec := post(h, "/api/findings/answers", `{"items":[{"obs_code":"MCD_L","answer":"L2"},{"obs_code":"MCD_L","answer":"P1"},{"obs_code":"MCD_L","answer":"P9"},{"obs_code":"NOPE","answer":"x"}]}`)
	var got struct {
		Answers []struct {
			Answer, Status, Target, File, ValueSet string
			Found                                  bool
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Answers) != 4 {
		t.Fatalf("%v %s", err, rec.Body.String())
	}
	want := []struct{ status, target string }{{"valid", ""}, {"mapped", "L1"}, {"none", ""}}
	for i, w := range want {
		if a := got.Answers[i]; a.Status != w.status || a.Target != w.target || a.ValueSet != "https://x/a" || a.File != "m.json" {
			t.Errorf("answer %d: %+v", i, a)
		}
	}
	if a := got.Answers[3]; a.Status != "" {
		t.Errorf("an observation code without a definition has no status: %+v", a)
	}
}

func TestValueSetEndpointListsConceptsForTheEditorPickLists(t *testing.T) {
	cms := converter.NewConceptMapService(zerolog.Nop())
	vsDir := t.TempDir()
	body := `{"resourceType":"ValueSet","id":"a","url":"https://x/a","compose":{"include":[
	 {"system":"http://loinc.org","concept":[{"code":"718-7","display":"Hemoglobin"},{"code":"4548-4"}]},
	 {"system":"http://snomed.info/sct","concept":[{"code":"1","display":"One"}]}]}}`
	_ = os.WriteFile(filepath.Join(vsDir, "a.json"), []byte(body), 0o644)
	if err := cms.LoadValueSetsDir(vsDir); err != nil {
		t.Fatal(err)
	}
	h := NewToolsServer(t.TempDir(), "", nil, cms, nil, zerolog.Nop())

	get := func(u string) (r struct {
		Found    bool
		Systems  []string
		Concepts []struct{ System, Code, Display string }
	}) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/valuesets?url="+u, nil))
		if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
			t.Fatal(err, rec.Body.String())
		}
		return r
	}
	got := get("https://x/a|1.0")
	if !got.Found || len(got.Concepts) != 3 || len(got.Systems) != 2 || got.Systems[0] != "http://loinc.org" {
		t.Fatalf("%+v", got)
	}
	if c := got.Concepts[0]; c.System != "http://loinc.org" || c.Code != "718-7" || c.Display != "Hemoglobin" {
		t.Errorf("first concept: %+v", c)
	}
	if got := get("https://x/unknown"); got.Found || len(got.Concepts) != 0 {
		t.Errorf("an unknown valueset: %+v", got)
	}
}
