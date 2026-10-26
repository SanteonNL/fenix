package queryvalidator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/rs/zerolog"
)

// has reports whether the report contains a finding with the given code whose
// column or path contains where and whose message contains msg.
func has(r *FileReport, code, where, msg string) bool {
	for _, f := range r.Findings {
		if f.Code == code && strings.Contains(f.Path+" "+f.Column, where) && strings.Contains(f.Message, msg) {
			return true
		}
	}
	return false
}

func validate(t *testing.T, sql string) *FileReport {
	t.Helper()
	return (&Validator{}).ValidateSQL("test.sql", sql)
}

func TestValidSimplePatient(t *testing.T) {
	r := validate(t, `SELECT id AS resource_id, id AS id, '' AS parent_id, 'Patient' AS fhir_path,
		gender, birth_date AS birthDate FROM p;`)
	for _, f := range r.Findings {
		if f.Severity >= Warning {
			t.Errorf("unexpected finding: %+v", f)
		}
	}
}

func TestMetadataAndShape(t *testing.T) {
	r := validate(t, `SELECT * FROM p;
		SELECT id AS resource_id, 'Patient' AS fhir_path, a, a FROM p;
		SELECT id AS resource_id, id, '' AS parent_id, 'Patient' AS fhir_path, UPPER(x) FROM p;`)
	for _, w := range []struct{ code, where string }{
		{CodeSQLSelectStar, ""}, {CodeSQLMeta, "id"}, {CodeSQLDupCol, "a"}, {CodeSQLUnnamed, ""},
	} {
		if !has(r, w.code, w.where, "") {
			t.Errorf("missing %s (%s): %+v", w.code, w.where, r.Findings)
		}
	}
}

func TestPathAndColumnErrors(t *testing.T) {
	r := validate(t, `SELECT id AS resource_id, id, '' AS parent_id, 'Patient' AS fhir_path,
			birthdate, gendr, name, deceasedBoolean, deceasedDateTime FROM p;
		SELECT id AS resource_id, id, pid AS parent_id, 'Patient.nme' AS fhir_path, family FROM n;
		SELECT id AS resource_id, id, '' AS parent_id, 'Patiënt' AS fhir_path, x FROM n;`)
	cases := []struct{ code, where, msg string }{
		{CodeColCase, "birthdate", "birthDate"},
		{CodeColUnknown, "gendr", "gender"},
		{CodeColComplex, "name", "complex"},
		{CodeColChoiceMulti, "deceasedBoolean", "deceased[x]"},
		{CodePathUnknown, "Patient.nme", "name"},
		{CodePathUnknownType, "Patiënt", "not a FHIR R4 resource type"},
	}
	for _, c := range cases {
		if !has(r, c.code, c.where, c.msg) {
			t.Errorf("missing %s %q %q in %+v", c.code, c.where, c.msg, r.Findings)
		}
	}
}

func TestLiterals(t *testing.T) {
	r := validate(t, `SELECT id AS resource_id, id, '' AS parent_id, 'Observation' AS fhir_path,
		'finished' AS status,
		'2024-01-02 10:00:00' AS effectiveDateTime,
		'12,5' AS "valueQuantity.value",
		CAST(x AS INTEGER) AS "valueDateTime",
		'Patiant/' || pid AS "subject.reference",
		NULL AS "code.text"
		FROM o;`)
	cases := []struct{ code, where, msg string }{
		{CodeLitCode, "status", "registered"},
		{CodeLitValue, "effectiveDateTime", "timezone"},
		{CodeLitValue, "valueQuantity.value", "decimal"},
		{CodeLitCast, "valueDateTime", "casts to int"},
		{CodeLitReference, "subject.reference", "not a FHIR resource type"},
	}
	for _, c := range cases {
		if !has(r, c.code, c.where, c.msg) {
			t.Errorf("missing %s %q %q in %+v", c.code, c.where, c.msg, r.Findings)
		}
	}
}

func TestRequiredElementsFromBaseModel(t *testing.T) {
	// Observation.status and Observation.code are mandatory in R4.
	r := validate(t, `SELECT id AS resource_id, id, '' AS parent_id, 'Observation' AS fhir_path, x AS effectiveDateTime FROM o;`)
	for _, w := range []string{"status", "code"} {
		if !has(r, CodeProfRequired, w, "requires") {
			t.Errorf("missing required %s: %+v", w, r.Findings)
		}
	}
	// Providing code through a child statement counts as provided.
	r = validate(t, `SELECT id AS resource_id, id, '' AS parent_id, 'Observation' AS fhir_path, s AS status FROM o;
		SELECT id AS resource_id, id, id AS parent_id, 'Observation.code.coding' AS fhir_path, c AS code FROM o;`)
	if has(r, CodeProfRequired, "code", "") {
		t.Errorf("code is provided by the child statement: %+v", r.Findings)
	}
}

func TestSemanticGroups(t *testing.T) {
	r := validate(t, `SELECT id AS resource_id, id, '' AS parent_id, 'Observation' AS fhir_path,
		s AS status, v AS "valueQuantity.value" FROM o;
		SELECT id AS resource_id, id, id AS parent_id, 'Observation.code.coding' AS fhir_path, c AS code FROM o;
		SELECT id AS resource_id, id, id AS parent_id, 'Observation.identifier' AS fhir_path, v AS value FROM o;`)
	for _, w := range []struct{ code, where string }{
		{CodeSemQuantity, "valueQuantity"}, {CodeSemCoding, "Observation.code.coding"}, {CodeSemIdentifier, "Observation.identifier"},
	} {
		if !has(r, w.code, w.where, "") {
			t.Errorf("missing %s %s: %+v", w.code, w.where, r.Findings)
		}
	}
}

func TestChildWithoutRoot(t *testing.T) {
	r := validate(t, `SELECT id AS resource_id, id, pid AS parent_id, 'Patient.name' AS fhir_path, family FROM n;`)
	if !has(r, CodePathNoRoot, "", "") || !has(r, CodePathNoRootStmt, "Patient.name", "") {
		t.Errorf("expected missing root findings: %+v", r.Findings)
	}
}

func TestSelectListParsing(t *testing.T) {
	items, star, ok := selectList(`-- comment
		WITH x AS (SELECT 1 AS a, 2 AS b)
		SELECT COALESCE(a, b, 'x, y') AS "valueString", CASE WHEN a=1 THEN 'p' END AS status,
		       t.birth_date birthDate, t.id, 'Patient' fhir_path
		FROM x t WHERE a IN (1,2)`)
	if !ok || star {
		t.Fatalf("ok=%v star=%v", ok, star)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.Alias)
	}
	want := "valueString,status,birthDate,id,fhir_path"
	if strings.Join(got, ",") != want {
		t.Errorf("aliases = %v, want %s", got, want)
	}
}

// With a real profile loaded, required and mustSupport elements and
// prohibited/typed choices come from the StructureDefinition.
func TestProfileCoverage(t *testing.T) {
	p := converter.NewProfileService(zerolog.Nop())
	if err := p.LoadDir("../../terminology/profiles/sim-on-fhir"); err != nil {
		t.Fatal(err)
	}
	v := &Validator{Profile: p}
	r := v.ValidateSQL("t.sql", `SELECT id AS resource_id, id, '' AS parent_id, 'Patient' AS fhir_path, gender FROM p;`)
	required := 0
	for _, f := range r.Findings {
		if f.Code == CodeProfRequired || f.Code == CodeProfMustSupport {
			required++
			t.Logf("%s %s: %s", f.Severity, f.Column, f.Message)
		}
	}
	if !p.HasProfileFor("Patient") {
		t.Fatal("Patient profile not loaded")
	}
}

// A profile can tighten the base model: Patient.name mandatory, birthDate
// mustSupport, gender prohibited, Observation.value[x] limited to Quantity.
func TestProfileConstraints(t *testing.T) {
	dir := t.TempDir()
	sd := `{"resourceType":"StructureDefinition","id":"p","url":"http://x/p","name":"P","status":"active","kind":"resource",
	"abstract":false,"type":"Patient","snapshot":{"element":[
	 {"id":"Patient","path":"Patient","min":0,"max":"*"},
	 {"id":"Patient.name","path":"Patient.name","min":1,"max":"*","type":[{"code":"HumanName"}]},
	 {"id":"Patient.birthDate","path":"Patient.birthDate","min":0,"max":"1","mustSupport":true,"type":[{"code":"date"}]},
	 {"id":"Patient.gender","path":"Patient.gender","min":0,"max":"0"},
	 {"id":"Patient.deceased[x]","path":"Patient.deceased[x]","min":0,"max":"1","type":[{"code":"boolean"}]}
	]}}`
	if err := os.WriteFile(filepath.Join(dir, "p.json"), []byte(sd), 0o644); err != nil {
		t.Fatal(err)
	}
	p := converter.NewProfileService(zerolog.Nop())
	if err := p.LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	r := (&Validator{Profile: p}).ValidateSQL("t.sql", `SELECT id AS resource_id, id, '' AS parent_id, 'Patient' AS fhir_path,
		gender, deceasedDateTime FROM p;`)
	cases := []struct {
		code string
		sev  Severity
		col  string
		msg  string
	}{
		{CodeProfRequired, Warning, "name", "the profile requires Patient.name"},
		{CodeProfMustSupport, Info, "birthDate", "mustSupport"},
		{CodeColProhibited, Error, "gender", "prohibits"},
		{CodeColChoiceType, Warning, "deceasedDateTime", "boolean"},
	}
	for _, c := range cases {
		found := false
		for _, f := range r.Findings {
			if f.Code == c.code && f.Severity == c.sev && strings.Contains(f.Column, c.col) && strings.Contains(f.Message, c.msg) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing %s/%s %s: %+v", c.sev, c.code, c.col, r.Findings)
		}
	}
	// Providing name through a child statement satisfies the requirement.
	r = (&Validator{Profile: p}).ValidateSQL("t.sql", `SELECT id AS resource_id, id, '' AS parent_id, 'Patient' AS fhir_path, d AS birthDate FROM p;
		SELECT id AS resource_id, id, id AS parent_id, 'Patient.name' AS fhir_path, f AS family FROM n;`)
	for _, f := range r.Findings {
		if f.Severity >= Info && f.Code != CodeProfNone {
			t.Errorf("unexpected finding: %+v", f)
		}
	}
}

func TestChoiceVariantsInCaseExpressionsAreNotFlagged(t *testing.T) {
	r := validate(t, `SELECT id AS resource_id, id, '' AS parent_id, 'Patient' AS fhir_path,
		CASE WHEN d IS NULL THEN 'false' END AS deceasedBoolean,
		CASE WHEN d IS NOT NULL THEN d END AS deceasedDateTime FROM p;`)
	if has(r, CodeColChoiceMulti, "", "") {
		t.Errorf("mutually exclusive CASE variants flagged: %+v", r.Findings)
	}
}

// Nictiz profiles express "fill this in" as obligations for the
// ExchangingSystem actor instead of min=1.
func TestObligations(t *testing.T) {
	dir := t.TempDir()
	ob := func(code, actor string) string {
		return `{"url":"http://hl7.org/fhir/StructureDefinition/obligation","extension":[
			{"url":"code","valueCode":"` + code + `"},{"url":"actor","valueCanonical":"http://x/ActorDefinition/` + actor + `"}]}`
	}
	sd := `{"resourceType":"StructureDefinition","id":"p","url":"http://x/p","name":"P","status":"active","kind":"resource",
	"abstract":false,"type":"Patient","snapshot":{"element":[
	 {"id":"Patient","path":"Patient","min":0,"max":"*"},
	 {"id":"Patient.gender","path":"Patient.gender","min":0,"max":"1","mustSupport":true,"extension":[` +
		ob("SHOULD:populate-if-known", "ExchangingSystem") + `,` + ob("SHALL:handle", "ConsumingSystem") + `],"type":[{"code":"code"}]},
	 {"id":"Patient.maritalStatus","path":"Patient.maritalStatus","min":0,"max":"1","mustSupport":true,
	  "extension":[` + ob("SHALL:handle", "ConsumingSystem") + `],"type":[{"code":"CodeableConcept"}]}
	]}}`
	if err := os.WriteFile(filepath.Join(dir, "p.json"), []byte(sd), 0o644); err != nil {
		t.Fatal(err)
	}
	p := converter.NewProfileService(zerolog.Nop())
	if err := p.LoadDir(dir); err != nil {
		t.Fatal(err)
	}
	r := (&Validator{Profile: p}).ValidateSQL("t.sql", `SELECT id AS resource_id, id, '' AS parent_id, 'Patient' AS fhir_path, x AS birthDate FROM p;`)
	var gender, marital *Finding
	for i, f := range r.Findings {
		switch f.Column {
		case "gender":
			gender = &r.Findings[i]
		case "maritalStatus":
			marital = &r.Findings[i]
		}
	}
	if gender == nil || gender.Severity != Warning || gender.Code != CodeProfObligation {
		t.Errorf("gender should be an obligation warning: %+v", gender)
	}
	if marital == nil || marital.Severity != Info || marital.Code != CodeProfMustSupport {
		t.Errorf("maritalStatus (consumer-only obligation) should stay info: %+v", marital)
	}
}
