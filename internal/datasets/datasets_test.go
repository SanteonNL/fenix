package datasets

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// binary wraps a definition the way the sim-on-fhir IG ships it: a Binary with the JSON base64-encoded.
func binary(t *testing.T, id, def string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]string{
		"resourceType": "Binary", "id": id, "contentType": "application/fhir+json",
		"data": base64.StdEncoding.EncodeToString([]byte(def)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const (
	sbIBD = `{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/dataset-definition",
	  "id": "sb-ibd", "title": "sb-ibd", "version": "0.1.0",
	  "cohort": {"criteria": [{"resource": "Patient", "param": [{"name": "birthdate", "value": "le{today-18y}"}]}]},
	  "include": [{"resource": "Patient", "element": ["gender", "birthDate"]}],
	  "requirement": ["k312-scopie", "k363-calprotectin-before-scopie", "not-in-package"]}`
	k312 = `{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/data-requirement-definition",
	  "id": "k312-scopie",
	  "criteria": [{"resource": "Procedure", "element": ["code", "performed[x]", "subject"]}]}`
	k363 = `{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/data-requirement-definition",
	  "id": "k363-calprotectin-before-scopie",
	  "criteria": [{"resource": "Procedure", "element": ["code", "performed[x]", "subject"]},
	               {"resource": "Observation", "element": ["code", "value[x]", "effective[x]", "subject"]}]}`
	ruleset = `{"resourceType": "https://ig.santeon.nl/sim-on-fhir/StructureDefinition/deidentification-ruleset", "id": "santeon-default"}`
)

func TestCatalogResolvesTheRequirementsOfADataset(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for name, def := range map[string]string{"sb-ibd": sbIBD, "k312-scopie": k312, "k363": k363, "santeon-default": ruleset} {
		p := filepath.Join(dir, "Binary-"+name+".json")
		if err := os.WriteFile(p, binary(t, name, def), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	c := NewCatalog()
	n, err := c.AddFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("added %d definitions, want 3 (the de-identification ruleset is not one)", n)
	}
	if got := c.IDs(); !reflect.DeepEqual(got, []string{"sb-ibd"}) {
		t.Errorf("IDs = %v", got)
	}

	ds, ok := c.ForGroup("sb-ibd-cohort")
	if !ok {
		t.Fatal("the IG's cohort Group id sb-ibd-cohort does not find dataset sb-ibd")
	}
	want := map[string]map[string][]string{
		"Patient": {"gender": {"dataset sb-ibd"}, "birthDate": {"dataset sb-ibd"}},
		"Procedure": {
			"code":         {"k312-scopie", "k363-calprotectin-before-scopie"},
			"performed[x]": {"k312-scopie", "k363-calprotectin-before-scopie"},
			"subject":      {"k312-scopie", "k363-calprotectin-before-scopie"},
		},
		"Observation": {
			"code": {"k363-calprotectin-before-scopie"}, "value[x]": {"k363-calprotectin-before-scopie"},
			"effective[x]": {"k363-calprotectin-before-scopie"}, "subject": {"k363-calprotectin-before-scopie"},
		},
	}
	if !reflect.DeepEqual(ds.Elements, want) {
		t.Errorf("Elements = %v\nwant       %v", ds.Elements, want)
	}
	if !reflect.DeepEqual(ds.Missing, []string{"not-in-package"}) {
		t.Errorf("Missing = %v", ds.Missing)
	}
	if !reflect.DeepEqual(ds.Types(), []string{"Observation", "Patient", "Procedure"}) {
		t.Errorf("Types = %v", ds.Types())
	}

	if _, ok := c.ForGroup("other-cohort"); ok {
		t.Error("a Group of no dataset is linked to one")
	}
	var none *Catalog
	if _, ok := none.ForGroup("sb-ibd-cohort"); ok {
		t.Error("a nil catalog finds a dataset")
	}
}

func TestPresent(t *testing.T) {
	var proc map[string]interface{}
	_ = json.Unmarshal([]byte(`{
	  "resourceType": "Procedure", "id": "V1",
	  "code": {"coding": [{"system": "s", "code": "038853"}]},
	  "performedDateTime": "2021-01-01",
	  "note": [],
	  "status": "",
	  "category": {},
	  "reasonCode": [{"coding": [{"code": "x"}]}]
	}`), &proc)
	for element, want := range map[string]bool{
		"code":                    true,
		"code.coding.code":        true,
		"code.coding.display":     false,
		"performed[x]":            true,
		"subject":                 false,
		"value[x]":                false,
		"note":                    false, // [] is absent
		"status":                  false, // "" is absent
		"category":                false, // {} is absent
		"reasonCode.coding.code":  true,
		"performedDateTime.thing": false,
	} {
		if got := Present(proc, element); got != want {
			t.Errorf("Present(%q) = %v, want %v", element, got, want)
		}
	}
}
