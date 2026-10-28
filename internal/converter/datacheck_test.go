package converter

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SanteonNL/fenix/internal/findings"
	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

func rule(path string, min int, max string, types ...string) fhir.ElementDefinition {
	e := fhir.ElementDefinition{Path: path, Min: &min, Max: &max}
	for _, t := range types {
		e.Type = append(e.Type, fhir.ElementDefinitionType{Code: t})
	}
	return e
}

// obsProfile is a small Observation profile: status, code (with a required
// coding), subject and a Quantity/string value are required or constrained,
// category is prohibited and status has a fixed value.
func obsProfile() *ProfileService {
	p := NewProfileService(zerolog.Nop())
	fixed := "final"
	status := rule("Observation.status", 1, "1", "code")
	status.FixedCode = &fixed
	for _, e := range []fhir.ElementDefinition{
		rule("Observation", 0, "*"),
		status,
		rule("Observation.code", 1, "1", "CodeableConcept"),
		rule("Observation.code.coding", 1, "*", "Coding"),
		rule("Observation.subject", 1, "1", "Reference"),
		rule("Observation.category", 0, "0", "CodeableConcept"),
		rule("Observation.performer", 0, "1", "Reference"),
		rule("Observation.value[x]", 0, "1", "Quantity", "string"),
	} {
		p.registerElement("http://example.org/StructureDefinition/obs", e)
	}
	return p
}

func collect(t *testing.T, p *ProfileService, raw map[string]interface{}, id string, structErr error) []findings.Record {
	t.Helper()
	fc := NewFHIRConverter(nil, zerolog.Nop(), p, nil)
	c := findings.NewCollector()
	fc.SetFindings(c)
	fc.reportFindings(id, "Observation", raw, structErr)
	return c.Drain("run", "src", "q", time.Time{})
}

func codes(recs []findings.Record) map[string]string {
	m := map[string]string{}
	for _, r := range recs {
		if r.Kind == "finding" {
			m[r.Code+" "+r.Path] = r.Message
		}
	}
	return m
}

func TestProfileFindings(t *testing.T) {
	raw := map[string]interface{}{
		"resourceType": "Observation",
		"status":       "preliminary", // fixed to "final"
		"code":         map[string]interface{}{"text": "x"},
		// subject missing
		"category":     []interface{}{map[string]interface{}{"text": "c"}}, // prohibited
		"performer":    []interface{}{map[string]interface{}{"reference": "a"}, map[string]interface{}{"reference": "b"}},
		"valueBoolean": true, // not Quantity or string
	}
	got := codes(collect(t, obsProfile(), raw, "obs-1", nil))

	for _, want := range []string{
		"FX-DATA-013 Observation.status",
		"FX-DATA-010 Observation.code.coding",
		"FX-DATA-010 Observation.subject",
		"FX-DATA-011 Observation.category",
		"FX-DATA-012 Observation.performer",
		"FX-DATA-014 Observation.value[x]",
	} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing finding %q; got %v", want, got)
		}
	}
}

func TestProfileFindings_ConformingResourceHasNone(t *testing.T) {
	raw := map[string]interface{}{
		"resourceType":  "Observation",
		"status":        "final",
		"code":          map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "1"}}},
		"subject":       map[string]interface{}{"reference": "Patient/1"},
		"valueQuantity": map[string]interface{}{"value": 1.0},
	}
	if got := codes(collect(t, obsProfile(), raw, "obs-1", nil)); len(got) != 0 {
		t.Errorf("expected no findings, got %v", got)
	}
}

// An absent optional parent must not make its required children show up.
func TestProfileFindings_RequiredChildOfAbsentParent(t *testing.T) {
	p := NewProfileService(zerolog.Nop())
	for _, e := range []fhir.ElementDefinition{
		rule("Observation", 0, "*"),
		rule("Observation.referenceRange", 0, "*"),
		rule("Observation.referenceRange.low", 1, "1", "Quantity"),
	} {
		p.registerElement("u", e)
	}
	if got := codes(collect(t, p, map[string]interface{}{"resourceType": "Observation"}, "o", nil)); len(got) != 0 {
		t.Errorf("expected no findings, got %v", got)
	}
}

func TestStructErrorsAreClassified(t *testing.T) {
	_, err := validateThroughStruct(map[string]interface{}{"resourceType": "Patient", "gender": "man"}, zerolog.Nop())
	e := classifyStructError(err)
	if e.code != findings.CodeDataValue || e.msg != "unknown AdministrativeGender code" || e.value != "man" || !e.locate {
		t.Errorf("invalid code: got %+v", e)
	}

	_, err = validateThroughStruct(map[string]interface{}{"resourceType": "Patient", "active": "maybe"}, zerolog.Nop())
	e = classifyStructError(err)
	if e.code != findings.CodeDataType || e.path != "active" || !strings.HasPrefix(e.msg, "expected bool") {
		t.Errorf("wrong type: got %+v", e)
	}

	_, err = validateThroughStruct(map[string]interface{}{"resourceType": "Patient", "birthDate": "31-12-2021"}, zerolog.Nop())
	if e = classifyStructError(err); e.msg != "invalid date, expected YYYY-MM-DD" || e.value != "31-12-2021" {
		t.Errorf("date: got %+v", e)
	}
}

// Every bad field of a resource is reported, each at its own path, although the
// decoder stops at the first one.
func TestAllBadFieldsOfOneResourceAreReported(t *testing.T) {
	raw := map[string]interface{}{
		"resourceType":     "Patient",
		"id":               "p1",
		"gender":           "man",
		"birthDate":        "31-12-2021",
		"deceasedDateTime": "yesterday",
	}
	_, err := validateThroughStruct(raw, zerolog.Nop())
	got := codes(collect2(t, patientProfile(), "Patient", raw, "p1", err))
	for _, want := range []string{"FX-DATA-002 Patient.gender", "FX-DATA-002 Patient.birthDate", "FX-DATA-002 Patient.deceased[x]"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %q in %v", want, got)
		}
	}
}

func collect2(t *testing.T, p *ProfileService, rootPath string, raw map[string]interface{}, id string, structErr error) []findings.Record {
	t.Helper()
	fc := NewFHIRConverter(nil, zerolog.Nop(), p, nil)
	c := findings.NewCollector()
	fc.SetFindings(c)
	fc.reportFindings(id, rootPath, raw, structErr)
	return c.Drain("run", "src", "q", time.Time{})
}

// The same problem in many resources is one finding whose count is the number
// of resources and whose values say how often each offending value occurred.
func TestFindingsAggregateAcrossResources(t *testing.T) {
	fc := NewFHIRConverter(nil, zerolog.Nop(), nil, nil)
	c := findings.NewCollector()
	fc.SetFindings(c)
	for _, v := range []string{"man", "vrouw", "man", "x"} {
		raw := map[string]interface{}{"resourceType": "Patient", "gender": v}
		_, err := validateThroughStruct(raw, zerolog.Nop())
		fc.reportFindings("p-"+v, "Patient", raw, err)
	}
	fc.reportFindings("ok", "Patient", map[string]interface{}{"resourceType": "Patient"}, nil)

	var finding, summary *findings.Record
	recs := c.Drain("run", "src", "q", time.Time{})
	for i := range recs {
		switch recs[i].Kind {
		case "finding":
			finding = &recs[i]
		case "summary":
			summary = &recs[i]
		}
	}
	if finding == nil || finding.Count != 4 || finding.Path != "Patient.gender" || finding.Query != "q" || len(finding.Examples) != 3 {
		t.Fatalf("unexpected finding: %+v", finding)
	}
	if len(finding.Values) != 3 || finding.Values[0].Value != "man" || finding.Values[0].Count != 2 {
		t.Fatalf("unexpected values: %+v", finding.Values)
	}
	if summary == nil || summary.Seen != 5 || summary.Skipped != 4 || summary.Query != "q" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestFlushFindingsWritesJSONLines(t *testing.T) {
	fc := NewFHIRConverter(nil, zerolog.Nop(), nil, nil)
	fc.SetFindings(findings.NewCollector())
	_, err := validateThroughStruct(map[string]interface{}{"resourceType": "Patient", "gender": "man"}, zerolog.Nop())
	fc.reportFindings("p1", "Patient", nil, err)

	dir := t.TempDir()
	path, err := fc.FlushFindings(dir, "20260101_000000", "sim", "q.sql")
	if err != nil || path == "" {
		t.Fatalf("flush: %q %v", path, err)
	}
	f, _ := os.Open(path)
	defer f.Close()
	var kinds []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r map[string]interface{}
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("line is not JSON: %v", err)
		}
		if r["run_id"] != "20260101_000000" || r["source"] != "sim" {
			t.Errorf("record not stamped: %v", r)
		}
		kinds = append(kinds, r["kind"].(string))
	}
	if strings.Join(kinds, ",") != "finding,summary" {
		t.Errorf("kinds = %v", kinds)
	}
	// Flushing again with nothing new writes nothing.
	if again, _ := fc.FlushFindings(dir, "20260101_000000", "sim", "q.sql"); again != "" {
		t.Errorf("second flush wrote %q", again)
	}
}

func patientProfile() *ProfileService {
	p := NewProfileService(zerolog.Nop())
	for _, e := range []fhir.ElementDefinition{
		rule("Patient", 0, "*"),
		rule("Patient.birthDate", 0, "1", "date"),
		rule("Patient.deceased[x]", 0, "1", "boolean", "dateTime"),
	} {
		p.registerElement("http://example.org/StructureDefinition/pat", e)
	}
	return p
}

// With IncludeResources the finding carries the resource as built, offending
// value included, for the dashboard's hover.
func TestFindingsKeepTheResourceOfAnExample(t *testing.T) {
	fc := NewFHIRConverter(nil, zerolog.Nop(), nil, nil)
	c := findings.NewCollector()
	c.IncludeResources = true
	fc.SetFindings(c)
	raw := map[string]interface{}{"resourceType": "Patient", "id": "p1", "gender": "man"}
	_, err := validateThroughStruct(raw, zerolog.Nop())
	fc.reportFindings("p1", "Patient", raw, err)

	var f findings.Record
	for _, r := range c.Drain("run", "src", "q", time.Time{}) {
		if r.Kind == "finding" {
			f = r
		}
	}
	if len(f.Samples) != 1 || f.Samples[0].ID != "p1" || !strings.Contains(string(f.Samples[0].Resource), `"gender":"man"`) {
		t.Fatalf("samples = %+v", f.Samples)
	}
}
