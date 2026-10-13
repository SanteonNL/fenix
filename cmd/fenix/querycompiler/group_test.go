package querycompiler_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/SanteonNL/fenix/cmd/fenix/converter"
	"github.com/SanteonNL/fenix/cmd/fenix/querycompiler"
	_ "modernc.org/sqlite"
)

// groupConfig is a throwaway source where Observation has TWO named queries
// (both over the sim observation SQL), to prove group building and
// restriction handle every named query.
func groupConfig(t *testing.T, obsPushdown string) string {
	t.Helper()
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "sources", "grouptest")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	yaml := `source: grouptest
resources:
  Patient:
    queries:
      - name: main
        sql: queries/sim/fhir/patient.sql
        pushdown: [_id]
  Observation:
    queries:
      - name: vitals
        sql: queries/sim/fhir/observation.sql
        pushdown: ` + obsPushdown + `
      - name: labs
        sql: queries/sim/fhir/observation.sql
        pushdown: [status]
`
	if err := os.WriteFile(filepath.Join(srcDir, "grouptest.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func simDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	stmts := []string{
		`CREATE TABLE sim_patient (Identificatienummer, GeslachtCode, GeslachtOmschrijving, Land, Geboortedatum, DatumOverlijden, DatumCheckStatusOverlijden)`,
		`INSERT INTO sim_patient (Identificatienummer, GeslachtCode) VALUES ('123','M'),('456','F'),('789','M'),('999','F')`,
		`CREATE TABLE sim_algemenemeting (MetingID, Identificatienummer, MetingNaamCodeSysteem, MetingNaamCode, MetingNaamOmschrijving,
			UitslagWaarde, UitslagWaardeEenheidSysteem, UitslagWaardeEenheid, UitslagCodeSysteem, UitslagCode, UitslagCodeOmschrijving, MetingDatumTijd, Status)`,
		`INSERT INTO sim_algemenemeting (MetingID, Identificatienummer, MetingNaamCode, UitslagWaarde, Status) VALUES
			('m1','123','A','1','final'),('m2','123','B','2','final'),
			('m3','456','A','3','final'),
			('m4','789','A','4','preliminary')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return db
}

func column(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("query failed: %v\n%s", err, query)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func TestResolveGroupMembersSQL_JoinsFiltersOnPatientNumber(t *testing.T) {
	c, err := querycompiler.New(groupConfig(t, "[status]"), sqlBaseDir)
	if err != nil {
		t.Fatal(err)
	}
	db := simDB(t)

	group, err := c.ResolveGroupMembersSQL("grouptest", "", []querycompiler.GroupFilter{
		{ResourceType: "Observation", Params: map[string]string{"status": "final"}},
		{ResourceType: "Patient"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := column(t, db, group), []string{"123", "456"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("group = %v, want %v\n%s", got, want, group)
	}

	// Restricting both Observation queries to the member_filter: every statement
	// of every named query only returns rows of group patients.
	rendered, err := c.ResolveInGroup("grouptest", "", "Observation", map[string]string{"status": "final"}, group)
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 2 {
		t.Fatalf("got %d queries, want 2 (vitals, labs)", len(rendered))
	}
	for _, rq := range rendered {
		for i, stmt := range converter.SplitStatements(rq.SQL) {
			got := column(t, db, "SELECT DISTINCT resource_id FROM ("+stmt+")")
			if i == 0 {
				// root statement: exactly the final observations of the group
				if want := []string{"m1", "m2", "m3"}; !reflect.DeepEqual(got, want) {
					t.Fatalf("query %s: root resource_ids = %v, want %v", rq.Name, got, want)
				}
				continue
			}
			// child statements: may be empty for this seed data, but never
			// contain a resource outside the group (m4 is patient 789)
			for _, id := range got {
				if id == "m4" {
					t.Fatalf("query %s statement %d leaked resource m4", rq.Name, i+1)
				}
			}
		}
	}

	// Group excluding 456 (no Patient row filter, but a narrower group).
	narrow := querycompiler.StaticGroupMembers([]string{"123"})
	rendered, err = c.ResolveInGroup("grouptest", "", "Observation", nil, narrow)
	if err != nil {
		t.Fatal(err)
	}
	got := column(t, db, "SELECT DISTINCT resource_id FROM ("+converter.SplitStatements(rendered[0].SQL)[0]+")")
	if want := []string{"m1", "m2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("static group resource_ids = %v, want %v", got, want)
	}
}

func TestResolveGroupMembersSQL_StaticFilterJoinsWithSQLFilter(t *testing.T) {
	c, err := querycompiler.New(groupConfig(t, "[status]"), sqlBaseDir)
	if err != nil {
		t.Fatal(err)
	}
	db := simDB(t)

	// Observation filter in SQL, joined with a fixed id list (the fallback
	// for a filter whose param isn't pushed down): 123,456 AND 456,789 -> 456.
	group, err := c.ResolveGroupMembersSQL("grouptest", "", []querycompiler.GroupFilter{
		{ResourceType: "Observation", Params: map[string]string{"status": "final"}},
		{ResourceType: "Patient", Static: true, PatientIDs: []string{"456", "789"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := column(t, db, group), []string{"456"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("group = %v, want %v\n%s", got, want, group)
	}

	if !c.PushedDownByAll("grouptest", "Observation", map[string]string{"status": "final"}) {
		t.Fatal("status should be pushed down by every Observation query")
	}
	if c.PushedDownByAll("grouptest", "Observation", map[string]string{"code": "A"}) {
		t.Fatal("code is not pushed down")
	}
}

func TestResolveGroupMembersSQL_ParamMustBePushedDownByEveryQuery(t *testing.T) {
	// "labs" pushes status down, "vitals" does not: a group filter on
	// status would silently over-match through vitals.
	c, err := querycompiler.New(groupConfig(t, "[]"), sqlBaseDir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.ResolveGroupMembersSQL("grouptest", "", []querycompiler.GroupFilter{
		{ResourceType: "Observation", Params: map[string]string{"status": "final"}},
	})
	if err == nil || !strings.Contains(err.Error(), `"vitals"`) {
		t.Fatalf("expected a pushdown error naming query vitals, got %v", err)
	}
}

func TestStaticGroupMembers_Empty(t *testing.T) {
	db := simDB(t)
	if got := column(t, db, querycompiler.StaticGroupMembers(nil)); len(got) != 0 {
		t.Fatalf("empty static group returned %v", got)
	}
	if got := column(t, db, querycompiler.StaticGroupMembers([]string{"o'brien"})); !reflect.DeepEqual(got, []string{"o'brien"}) {
		t.Fatalf("quote escaping broken: %v", got)
	}
}

func TestResolveGroupMembersSQL_GroupBlockOverridesPushdownAndPatientExpr(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "sources", "grouptest")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Standalone pushdown is empty; as a group the query pushes status down
	// and names its own patient-number expression.
	yaml := `source: grouptest
resources:
  Observation:
    queries:
      - name: main
        sql: queries/sim/fhir/observation.sql
        pushdown: []
        member_filter:
          pushdown: [status]
          patient: REPLACE("subject.reference", 'Patient/', '')
`
	if err := os.WriteFile(filepath.Join(srcDir, "grouptest.yaml"), []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}
	c, err := querycompiler.New(dir, sqlBaseDir)
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{"status": "final"}
	if !c.PushedDownByAll("grouptest", "Observation", status) {
		t.Fatal("status should count as pushed down in the group context")
	}
	group, err := c.ResolveGroupMembersSQL("grouptest", "", []querycompiler.GroupFilter{{ResourceType: "Observation", Params: status}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(group, "REPLACE(") {
		t.Fatalf("custom patient expression not used:\n%s", group)
	}
	if got, want := column(t, simDB(t), group), []string{"123", "456"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("group = %v, want %v", got, want)
	}
}

// The real sim config: Observation by code combined with Patient by
// birthdate, both pushed down, joined into ONE group query.
func TestResolveGroupMembersSQL_SimCodeAndBirthdate(t *testing.T) {
	c, err := querycompiler.New(configDir, sqlBaseDir)
	if err != nil {
		t.Fatal(err)
	}
	db := simDB(t)
	for _, s := range []string{
		`UPDATE sim_patient SET Geboortedatum = '2021-10-30' WHERE Identificatienummer = '123'`,
		`UPDATE sim_patient SET Geboortedatum = '2021-12-31' WHERE Identificatienummer = '456'`,
		`UPDATE sim_patient SET Geboortedatum = '2022-02-14' WHERE Identificatienummer = '789'`,
		`UPDATE sim_algemenemeting SET MetingNaamCode = 'MCD_A', MetingNaamCodeSysteem = 'https://santeon.nl/sct' WHERE MetingID IN ('m1','m3')`,
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name  string
		obs   map[string]string
		birth string
		want  []string
	}{
		{"code and birthdate", map[string]string{"code": "MCD_A"}, "ge2021-11-01", []string{"456"}},
		{"system|code token", map[string]string{"code": "https://santeon.nl/sct|MCD_A"}, "ge2021-01-01", []string{"123", "456"}},
		{"birthdate excludes all", map[string]string{"code": "MCD_A"}, "ge2023-01-01", nil},
	}
	for _, tc := range cases {
		group, err := c.ResolveGroupMembersSQL("sim", "", []querycompiler.GroupFilter{
			{ResourceType: "Observation", Params: tc.obs},
			{ResourceType: "Patient", Params: map[string]string{"birthdate": tc.birth}},
		})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := column(t, db, group); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: group = %v, want %v\n%s", tc.name, got, tc.want, group)
		}
	}
}

// A pushed-down code is a TARGET code: the compiler reverse-maps it to the
// source codes before it reaches the SQL.
func TestResolve_CodeIsReverseConceptMapped(t *testing.T) {
	c, err := querycompiler.New(configDir, sqlBaseDir)
	if err != nil {
		t.Fatal(err)
	}
	c.SetParamTranslator(func(resourceType, field, code string) ([]string, bool, bool) {
		if resourceType == "Observation" && field == "code" && code == "LA6751-7" {
			return []string{"MayoEndoscopicScore", "LA6751-7"}, true, true
		}
		return nil, true, false
	})

	rendered, err := c.Resolve("sim", "", "Observation", map[string]string{"code": "http://loinc.org|LA6751-7"})
	if err != nil {
		t.Fatal(err)
	}
	sql := rendered[0].SQL
	if !strings.Contains(sql, "MetingNaamCode IN ('MayoEndoscopicScore', 'LA6751-7')") {
		t.Fatalf("code not reverse-mapped:\n%s", sql)
	}
	if strings.Contains(sql, "MetingNaamCodeSysteem =") {
		t.Fatalf("target system must not filter the source system column:\n%s", sql)
	}

	// untranslated code keeps its system filter
	rendered, _ = c.Resolve("sim", "", "Observation", map[string]string{"code": "https://santeon.nl/sct|MCD_B"})
	if !strings.Contains(rendered[0].SQL, "MetingNaamCode IN ('MCD_B')") {
		t.Fatalf("plain code:\n%s", rendered[0].SQL)
	}
}
