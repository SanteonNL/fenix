package fhirserver

import (
	"encoding/json"
	"testing"
)

func TestTrimElements(t *testing.T) {
	obs := map[string]interface{}{
		"resourceType": "Observation", "id": "101", "status": "final",
		"code":              map[string]interface{}{"coding": []interface{}{map[string]interface{}{"code": "38445-3"}}},
		"subject":           map[string]interface{}{"reference": "Patient/2001"},
		"effectiveDateTime": "2021-03-01",
		"valueQuantity":     map[string]interface{}{"value": 250},
		"method":            map[string]interface{}{"text": "dropped"},
		"note":              []interface{}{map[string]interface{}{"text": "dropped"}},
	}
	got := trimElements("Observation", []interface{}{obs}, []string{"code", "value[x]", "subject"})
	b, _ := json.Marshal(got[0])
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)

	for _, k := range []string{"resourceType", "id", "status", "code", "subject", "valueQuantity"} {
		if _, ok := m[k]; !ok {
			t.Errorf("%s must be kept (declared, mandatory or always kept)", k)
		}
	}
	for _, k := range []string{"effectiveDateTime", "method", "note"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s must be trimmed", k)
		}
	}
	tags := m["meta"].(map[string]interface{})["tag"].([]interface{})
	if len(tags) != 1 || tags[0].(map[string]interface{})["code"] != "SUBSETTED" {
		t.Fatalf("a trimmed resource gets the SUBSETTED tag: %v", m["meta"])
	}

	// nothing to trim: no tag, resource unchanged
	pat := map[string]interface{}{"resourceType": "Patient", "id": "1", "gender": "male"}
	got = trimElements("Patient", []interface{}{pat}, []string{"gender"})
	if _, ok := got[0].(map[string]interface{})["meta"]; ok {
		t.Fatal("an untrimmed resource is not tagged")
	}
	// no _elements: as is
	if out := trimElements("Patient", []interface{}{pat}, nil); out[0].(map[string]interface{})["gender"] != "male" {
		t.Fatal("without _elements nothing changes")
	}
}
