package fhirserver

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SanteonNL/fenix/internal/conformance"
	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/SanteonNL/fenix/internal/datasets"
	"github.com/SanteonNL/fenix/internal/findings"
	"github.com/rs/zerolog"
)

func datasetCatalog(t *testing.T) *datasets.Catalog {
	t.Helper()
	c := datasets.NewCatalog()
	for _, def := range []string{
		`{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/dataset-definition", "id": "sb-ibd",
		  "include": [{"resource": "Patient", "element": ["gender", "birthDate"]}],
		  "requirement": ["k312-scopie", "k363-calprotectin-before-scopie"]}`,
		`{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/data-requirement-definition", "id": "k312-scopie",
		  "criteria": [{"resource": "Procedure", "element": ["code", "performed[x]", "subject"]}]}`,
		`{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/data-requirement-definition", "id": "k363-calprotectin-before-scopie",
		  "criteria": [{"resource": "Procedure", "element": ["performed[x]"]},
		               {"resource": "Observation", "element": ["code", "value[x]"]}]}`,
	} {
		bin, _ := json.Marshal(map[string]string{"resourceType": "Binary", "contentType": "application/fhir+json",
			"data": base64.StdEncoding.EncodeToString([]byte(def))})
		if ok, err := c.Add(bin); !ok || err != nil {
			t.Fatalf("Add = %v, %v", ok, err)
		}
	}
	return c
}

// A Group posted with the id the IG gives a dataset's cohort is linked to that
// dataset; any other Group is not.
func TestGroupCreateLinksTheDataset(t *testing.T) {
	s := newTestServer()
	s.SetDatasets(datasetCatalog(t))

	post := func(body string) string {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r4/Group", bytes.NewReader([]byte(body))))
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST status = %d: %s", rec.Code, rec.Body.String())
		}
		return strings.TrimPrefix(rec.Header().Get("Location"), "/r4/Group/")
	}
	cohort := post(`{"resourceType":"Group","id":"sb-ibd-cohort","type":"person","actual":true,"name":"sb-ibd Group"}`)
	other := post(`{"resourceType":"Group","id":"something-else","type":"person","actual":true}`)

	if ds := s.datasetForGroup(cohort); ds == nil || ds.ID != "sb-ibd" {
		t.Errorf("Group %s (posted as sb-ibd-cohort) is linked to %v, want sb-ibd", cohort, ds)
	}
	if ds := s.datasetForGroup(other); ds != nil {
		t.Errorf("Group %s is linked to dataset %s", other, ds.ID)
	}
}

func TestCheckDatasetElementsReportsMissingElements(t *testing.T) {
	fc := converter.NewFHIRConverter(nil, zerolog.Nop(), nil, nil)
	c := findings.NewCollector()
	fc.SetFindings(c)
	s := &Server{converter: fc, log: zerolog.Nop()}
	ds, _ := datasetCatalog(t).Get("sb-ibd")

	var procs []interface{}
	for _, p := range []string{
		`{"resourceType":"Procedure","id":"V1","code":{"text":"scopie"},"performedDateTime":"2021-01-01","subject":{"reference":"Patient/2001"}}`,
		`{"resourceType":"Procedure","id":"V2","code":{"text":"scopie"},"subject":{"reference":"Patient/2002"}}`,
		`{"resourceType":"Procedure","id":"V3","subject":{"reference":"Patient/2002"}}`,
	} {
		var m map[string]interface{}
		_ = json.Unmarshal([]byte(p), &m)
		procs = append(procs, m)
	}
	for _, id := range []string{"V1", "V2", "V3"} {
		c.Clean("Procedure", id, nil) // as the converter counts them, before the dataset check
	}
	s.checkDatasetElements(ds, "Procedure", procs)
	s.checkDatasetElements(nil, "Procedure", procs) // no dataset: nothing checked
	s.reportMissingDatasetTypes(ds, []string{"Procedure", "Patient"})

	var clean *int
	got := map[string]findings.Record{}
	for _, r := range c.Drain("run", "sim", "q", time.Time{}) {
		if r.Kind == "summary" && r.Resource == "Procedure" {
			clean = r.Clean
		}
		if r.Kind == "finding" {
			got[r.Code+" "+r.Path] = r
		}
	}
	if clean == nil || *clean != 1 {
		t.Errorf("Procedure clean = %v, want 1 (V2 and V3 miss required elements)", clean)
	}
	if len(got) != 3 {
		t.Fatalf("findings = %v, want performed[x] and code missing, and Observation not exported", got)
	}
	perf := got[findings.CodeDataDatasetElement+" Procedure.performed[x]"]
	if perf.Count != 2 || perf.Severity != findings.Warning || strings.Join(perf.Examples, ",") != "V2,V3" {
		t.Errorf("performed[x]: count %d, severity %v, examples %v; want 2 warnings for V2,V3", perf.Count, perf.Severity, perf.Examples)
	}
	if !strings.Contains(perf.Message, "sb-ibd") || !strings.Contains(perf.Message, "k312-scopie, k363-calprotectin-before-scopie") {
		t.Errorf("performed[x] message %q names neither the dataset nor both requirements", perf.Message)
	}
	if code := got[findings.CodeDataDatasetElement+" Procedure.code"]; code.Count != 1 || !strings.Contains(code.Message, "(k312-scopie)") {
		t.Errorf("code: %+v, want V3 only, needed by k312-scopie", code)
	}
	if obs, ok := got[findings.CodeDataDatasetType+" Observation"]; !ok || obs.Resource != "Observation" {
		t.Errorf("Observation (needed by k363, not in _type) is not reported: %v", got)
	}
}

// A code a data requirement filters on that no exported resource carries is
// reported, per filter, with the missing codes as values; a ValueSet stands for its members.
func TestMissingDatasetCodesAreReported(t *testing.T) {
	c := datasets.NewCatalog()
	for _, def := range []string{
		`{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/dataset-definition", "id": "sb-ibd",
		  "requirement": ["k312-scopie", "k363-calprotectin-before-scopie"]}`,
		`{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/data-requirement-definition", "id": "k312-scopie",
		  "criteria": [{"resource": "Procedure", "param": [{"name": "code:in", "value": "{vs:ibd}"}, {"name": "date", "value": "ge2018-01-01"}]}]}`,
		`{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/data-requirement-definition", "id": "k363-calprotectin-before-scopie",
		  "criteria": [{"resource": "Observation", "param": [{"name": "code", "value": "{od:od-calprotectin.code}"}]}]}`,
	} {
		if ok, err := c.Add([]byte(def)); !ok || err != nil {
			t.Fatalf("Add = %v, %v", ok, err)
		}
	}
	ds, _ := c.Get("sb-ibd")
	lookup := func(resourceType, code string) (string, string, bool) {
		switch code {
		case "code":
			return "code", "token", true
		case "date":
			return "performed", "date", true
		}
		return "", "", false
	}
	expand := func(ref string) (conformance.Members, error) {
		return conformance.Members{Pairs: map[string]bool{"nza|038853": true, "nza|035121": true, "nza|999999": true}, Codes: map[string]bool{}}, nil
	}
	res := func(js string) []interface{} {
		var m map[string]interface{}
		_ = json.Unmarshal([]byte(js), &m)
		return []interface{}{m}
	}

	fc := converter.NewFHIRConverter(nil, zerolog.Nop(), nil, nil)
	col := findings.NewCollector()
	fc.SetFindings(col)
	s := &Server{converter: fc, log: zerolog.Nop()}

	procs := newCodeTracker(ds, "Procedure", map[string]string{"code:in": "https://ig/ValueSet/ibd", "date": "ge2018-01-01"}, lookup, expand)
	procs.observe(res(`{"id":"V1","code":{"coding":[{"system":"nza","code":"038853"}]}}`)) // batch 1
	procs.observe(res(`{"id":"V2","code":{"coding":[{"system":"nza","code":"035121"}]}}`)) // batch 2
	s.reportMissingDatasetCodes(ds, procs)

	obs := newCodeTracker(ds, "Observation", map[string]string{"code": "http://loinc.org|38445-3"}, lookup, expand)
	obs.observe(res(`{"id":"104","code":{"coding":[{"system":"https://santeon.nl/sct","code":"CALP"}]}}`))
	s.reportMissingDatasetCodes(ds, obs)

	if tr := newCodeTracker(ds, "Patient", map[string]string{"gender": "male"}, lookup, expand); tr != nil {
		t.Error("a type the requirements do not filter gets a tracker")
	}

	got := map[string]findings.Record{}
	for _, r := range col.Drain("run", "sim", "q", time.Time{}) {
		got[r.Code+" "+r.Path] = r
	}
	if len(got) != 2 {
		t.Fatalf("findings = %+v, want one for Procedure.code and one for Observation.code", got)
	}
	p := got[findings.CodeDataDatasetCode+" Procedure.code.coding.code"] // the binding's path, for the concept map
	if p.Count != 1 || len(p.Values) != 1 || p.Values[0].Value != "nza|999999" || !strings.Contains(p.Message, "k312-scopie") {
		t.Errorf("Procedure.code: %+v, want only nza|999999 missing, needed by k312-scopie", p)
	}
	o := got[findings.CodeDataDatasetCode+" Observation.code.coding.code"]
	if len(o.Values) != 1 || o.Values[0].Value != "http://loinc.org|38445-3" || !strings.Contains(o.Message, "k363-calprotectin-before-scopie") {
		t.Errorf("Observation.code: %+v, want http://loinc.org|38445-3 missing, needed by k363", o)
	}
}
