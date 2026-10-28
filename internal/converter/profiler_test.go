package converter

import (
	"testing"
	"time"

	"github.com/SanteonNL/fenix/internal/findings"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

// The profile counts the values as the source delivered them: a gender the
// concept map would rewrite is still counted under its source code.
func TestProfileCountsSourceValuesBeforeMapping(t *testing.T) {
	db := sqlx.MustConnect("sqlite", ":memory:")
	db.SetMaxOpenConns(1)
	defer db.Close()
	db.MustExec(`CREATE TABLE p (id TEXT, g TEXT, ident TEXT)`)
	db.MustExec(`INSERT INTO p VALUES ('1','man','a'), ('2','man','b'), ('3','F','c'), ('4',NULL,'d')`)

	fc := NewFHIRConverter(db, zerolog.Nop(), NewProfileService(zerolog.Nop()), nil)
	c := findings.NewCollector()
	c.Profile = true
	fc.SetFindings(c)

	_, err := fc.ConvertSQL(`
SELECT id AS resource_id, id AS id, '' AS parent_id, 'Patient' AS fhir_path, g AS gender FROM p;
SELECT id AS resource_id, id || '_i' AS id, id AS parent_id, 'Patient.identifier' AS fhir_path, ident AS "value" FROM p;`)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]findings.Record{}
	for _, r := range c.Drain("run", "src", "q", time.Time{}) {
		if r.Kind == "profile" {
			got[r.Path] = r
		}
	}
	g := got["Patient.gender"]
	if g.Rows != 4 || g.Empty != 1 || g.Distinct != 2 || len(g.Values) != 2 || g.Values[0].Value != "man" || g.Values[0].Count != 2 {
		t.Errorf("gender: %+v", g)
	}
	if v := got["Patient.identifier.value"]; v.Rows != 4 || v.Distinct != 4 {
		t.Errorf("identifier.value: %+v", v)
	}
	if _, ok := got["Patient.id"]; ok {
		t.Error("the metadata columns are not profiled")
	}
}

func TestProfileOffByDefault(t *testing.T) {
	db := sqlx.MustConnect("sqlite", ":memory:")
	defer db.Close()
	fc := NewFHIRConverter(db, zerolog.Nop(), nil, nil)
	c := findings.NewCollector()
	fc.SetFindings(c)
	if _, err := fc.ConvertSQL(`SELECT 'a' AS resource_id, 'a' AS id, '' AS parent_id, 'Patient' AS fhir_path, 'M' AS gender`); err != nil {
		t.Fatal(err)
	}
	for _, r := range c.Drain("r", "s", "q", time.Time{}) {
		if r.Kind == "profile" {
			t.Errorf("profile collected while off: %+v", r)
		}
	}
}
