package fhirserver

import (
	"testing"
)

func TestMatchPeriod(t *testing.T) {
	open := map[string]interface{}{"start": "2020-03-01"}
	closed := map[string]interface{}{"start": "2015-01-01", "end": "2016-12-31"}
	for _, c := range []struct {
		period map[string]interface{}
		want   string
		match  bool
	}{
		{open, "ge2018-01-01", true},    // no end: still running
		{closed, "ge2018-01-01", false}, // ended before
		{closed, "le2015-06-01", true},  // started before
		{open, "le2019-01-01", false},   // started after
		{closed, "2016-05", true},       // inside
		{closed, "2017-05", false},      // outside
		{map[string]interface{}{}, "ge2018-01-01", false},
	} {
		if got := matchDate(c.period, c.want); got != c.match {
			t.Errorf("matchDate(%v, %s) = %v, want %v", c.period, c.want, got, c.match)
		}
	}
}

func TestMatchReferenceCanonicalAndOr(t *testing.T) {
	q := "http://decor.nictiz.nl/fhir/Questionnaire/2.16.840.1--20250714064425"
	if !matchReference(q, q) || !matchReference(q, q+"|1.0") || !matchReference(q, "Questionnaire/2.16.840.1--20250714064425") {
		t.Fatal("canonical string field")
	}
	if matchReference(q, "http://other/Questionnaire/x") {
		t.Fatal("other canonical must not match")
	}
	ref := map[string]interface{}{"reference": "Patient/2001"}
	if !matchReference(ref, "Patient/1,Patient/2001") || matchReference(ref, "Patient/1,Patient/2") {
		t.Fatal("comma is OR on references")
	}
}

func TestChainedParam(t *testing.T) {
	lookup := func(rt, code string) (string, string, bool) {
		if rt == "EpisodeOfCare" && code == "condition" {
			return "diagnosis", "reference", true
		}
		return "", "", false
	}
	eoc := func(id, cond string) interface{} {
		return map[string]interface{}{"resourceType": "EpisodeOfCare", "id": id,
			"diagnosis": []interface{}{map[string]interface{}{"condition": map[string]interface{}{"reference": "Condition/" + cond}, "rank": 1}}}
	}
	resources := []interface{}{eoc("S1", "c1"), eoc("S2", "c2"), map[string]interface{}{"resourceType": "EpisodeOfCare", "id": "S3"}}
	var asked string
	deps := filterDeps{chain: func(targetType, targetParam, value string) (map[string]bool, error) {
		asked = targetType + "?" + targetParam + "=" + value
		return map[string]bool{"c2": true}, nil
	}}
	got := filterUnwiredParams("EpisodeOfCare", map[string]string{"condition:Condition.code": "sys|922"}, nil, resources, lookup, deps)
	if len(got) != 1 || got[0].(map[string]interface{})["id"] != "S2" {
		t.Fatalf("got %v", got)
	}
	if asked != "Condition?code=sys|922" {
		t.Fatalf("the chain must search the target type, asked %q", asked)
	}
	if !hasUnwiredParams("EpisodeOfCare", map[string]string{"condition:Condition.code": "x"}, nil, lookup) {
		t.Fatal("a chained param is filtered in Go, so the SQL result is not final")
	}
	if _, _, _, ok := chainedParam("code:in"); ok {
		t.Fatal(":in is not a chain")
	}
}

func TestExtractPatientIDFromPatientElement(t *testing.T) {
	if id, ok := extractPatientID("EpisodeOfCare", map[string]interface{}{"patient": map[string]interface{}{"reference": "Patient/2001"}}); !ok || id != "2001" {
		t.Fatalf("EpisodeOfCare.patient: %q %v", id, ok)
	}
	if id, ok := extractPatientID("Observation", map[string]interface{}{"subject": map[string]interface{}{"reference": "Patient/7"}}); !ok || id != "7" {
		t.Fatalf("subject: %q %v", id, ok)
	}
}
