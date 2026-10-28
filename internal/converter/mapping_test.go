package converter

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SanteonNL/fenix/internal/findings"
	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

const genderVS = "http://hl7.org/fhir/ValueSet/administrative-gender"

// a concept map file with M->male, F->female and a fixed fallback "unknown"
const genderMap = `{"resourceType":"ConceptMap","id":"g","status":"active","targetCanonical":"` + genderVS + `",
 "group":[{"target":"http://hl7.org/fhir/administrative-gender",
  "element":[{"code":"M","target":[{"code":"male","equivalence":"equivalent"}]},{"code":"F","target":[{"code":"female","equivalence":"equivalent"}]}],
  "unmapped":{"mode":"fixed","code":"unknown"}}]}`

func conceptMaps(t *testing.T, files map[string]string) *ConceptMapService {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := NewConceptMapService(zerolog.Nop())
	if err := s.LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestResolveMatchesTranslateAndNamesTheFile(t *testing.T) {
	s := conceptMaps(t, map[string]string{"gender.json": genderMap})
	info := s.Resolve(genderVS+"|4.0.1", []string{"M", "male", "man", ""})
	if !info.HasMap || len(info.Files) != 1 || info.Files[0] != "gender.json" || info.Default == nil || info.Default.Code != "unknown" {
		t.Fatalf("info: %+v", info)
	}
	want := map[string]string{"M": MapMapped, "male": MapValid, "man": MapDefault, "": MapDefault}
	for _, c := range info.Codes {
		if c.Status != want[c.Code] || c.File != "gender.json" {
			t.Errorf("%q: %+v, want status %s", c.Code, c, want[c.Code])
		}
		// Resolve and Translate agree on what comes out
		target, _, _, changed := s.Translate(genderVS, c.Code)
		if changed && target != c.Target {
			t.Errorf("%q: Translate gives %q, Resolve %q", c.Code, target, c.Target)
		}
	}
	if none := s.Resolve("http://example.org/vs", []string{"x"}); none.HasMap || none.Codes[0].Status != MapNone {
		t.Errorf("a valueset without a map: %+v", none)
	}
}

func TestResolveWithoutDefaultLeavesTheCodeAlone(t *testing.T) {
	noDefault := `{"resourceType":"ConceptMap","id":"n","status":"active","targetCanonical":"` + genderVS + `","group":[{"element":[{"code":"M","target":[{"code":"male"}]}]}]}`
	s := conceptMaps(t, map[string]string{"n.json": noDefault})
	if c := s.Resolve(genderVS, []string{"zzz"}).Codes[0]; c.Status != MapNone || c.File != "n.json" {
		t.Errorf("got %+v", c)
	}
}

// A code with no mapping is a warning that names the source code, and the resource still converts.
func TestUnmappedSourceCodeIsAWarning(t *testing.T) {
	p := NewProfileService(zerolog.Nop())
	p.pathToValueset["Patient.gender"] = genderVS
	p.registerElement("u", fhir.ElementDefinition{Path: "Patient", Min: ptr(0), Max: ptr("*")})
	cms := conceptMaps(t, map[string]string{"gender.json": genderMap})

	fc := NewFHIRConverter(nil, zerolog.Nop(), p, cms)
	c := findings.NewCollector()
	fc.SetFindings(c)
	for _, g := range []string{"man", "man", "M", "vrouw"} {
		raw := map[string]any{"resourceType": "Patient", "gender": g}
		var unmapped []unmappedCode
		applyConceptMappings(raw, "Patient", p, cms, func(u unmappedCode) { unmapped = append(unmapped, u) })
		_, err := validateThroughStruct(raw, zerolog.Nop())
		fc.reportFindings("p-"+g, "Patient", raw, err, unmapped...)
	}
	var f findings.Record
	for _, r := range c.Drain("run", "src", "q", time.Time{}) {
		if r.Kind == "finding" {
			f = r
		}
	}
	if f.Code != findings.CodeDataUnmapped || f.Severity != findings.Warning || f.Path != "Patient.gender" || f.Count != 3 {
		t.Fatalf("finding: %+v", f)
	}
	if len(f.Values) != 2 || f.Values[0].Value != "man" || f.Values[0].Count != 2 || f.Values[1].Value != "vrouw" {
		t.Errorf("values: %+v", f.Values)
	}
}

func ptr[T any](v T) *T { return &v }

func TestBindingForFindsTheBindingOnTheCodeableConcept(t *testing.T) {
	p := NewProfileService(zerolog.Nop())
	p.pathToValueset["Observation.code"] = "http://example.org/vs"
	p.pathToValueset["Patient.gender"] = genderVS
	for path, want := range map[string]string{
		"Observation.code.coding.code": "http://example.org/vs",
		"Observation.code":             "http://example.org/vs",
		"Patient.gender":               genderVS,
	} {
		if got, ok := p.BindingFor(path); !ok || got != want {
			t.Errorf("%s: %q %v", path, got, ok)
		}
	}
	if _, ok := p.BindingFor("Observation.status"); ok {
		t.Error("an element without a binding has none")
	}
}

// A code that is a member of the target valueset is valid as it is; only codes
// outside it, with no mapping, are unmapped.
func TestValueSetMembersNeedNoMapping(t *testing.T) {
	const vs = "https://example.org/ValueSet/obs"
	cm := `{"resourceType":"ConceptMap","id":"o","status":"active","targetCanonical":"` + vs + `",
	 "group":[{"element":[{"code":"MCD_A","target":[{"code":"718-7"}]}]}]}`
	s := conceptMaps(t, map[string]string{"o.json": cm})

	before := s.Resolve(vs, []string{"4548-4", "MCD_A", "MCD_B"})
	if before.Codes[0].Status != MapNone {
		t.Fatalf("without the valueset, 4548-4 is unmapped: %+v", before.Codes[0])
	}

	dir := t.TempDir()
	body := `{"resourceType":"ValueSet","url":"` + vs + `","compose":{"include":[{"system":"http://loinc.org","concept":[{"code":"718-7"},{"code":"4548-4"}]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "vs.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// the {"valueSet": {...}} wrapper of terminology/valuesets is read too
	wrapped := `{"originalUrl":"` + vs + `|1","valueSet":` + body + `}`
	if err := os.WriteFile(filepath.Join(dir, "wrapped.json"), []byte(wrapped), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadValueSetsDir(dir); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range s.Resolve(vs, []string{"4548-4", "MCD_A", "MCD_B"}).Codes {
		got[c.Code] = c.Status
	}
	if got["4548-4"] != MapValid || got["MCD_A"] != MapMapped || got["MCD_B"] != MapNone {
		t.Errorf("statuses: %v", got)
	}

	// editor saves reload the concept maps; the valueset stays known
	if err := s.LoadDir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadDir(dirWith(t, "o.json", cm)); err != nil {
		t.Fatal(err)
	}
	if s.Resolve(vs, []string{"4548-4"}).Codes[0].Status != MapValid {
		t.Error("a reload of the concept maps must keep the valuesets")
	}
}

func dirWith(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}
