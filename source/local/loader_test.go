// Tests for json_options parsing and for primary-key handling in the Loader:
// parent/child linking, and refusing to load tables with missing or duplicate keys.
package local

import (
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

func TestParseJSONOptionsRejectsOldIDFieldName(t *testing.T) {
	config := map[string]any{"json_options": map[string]any{
		"journeys": map[string]any{"id_field": "id"},
	}}
	_, err := ParseJSONOptions(config)
	if err == nil || !strings.Contains(err.Error(), "renamed to primary_key_field") {
		t.Fatalf("want rename hint for id_field, got %v", err)
	}
}

func TestParseJSONOptionsRequiresPrimaryKeyForChildren(t *testing.T) {
	config := map[string]any{"json_options": map[string]any{
		"journeys": map[string]any{"children": map[string]any{"steps": map[string]any{}}},
	}}
	_, err := ParseJSONOptions(config)
	if err == nil || !strings.Contains(err.Error(), "primary_key_field is missing") {
		t.Fatalf("want error for children without primary_key_field, got %v", err)
	}
}

func TestParseJSONOptionsAcceptsEmptyChildOptions(t *testing.T) {
	config := map[string]any{"json_options": map[string]any{
		"Journeys": map[string]any{
			"primary_key_field": "id",
			"children":          map[string]any{"steps": map[string]any{}, "notes": nil},
		},
	}}
	optionsByFile, err := ParseJSONOptions(config)
	if err != nil {
		t.Fatal(err)
	}
	journeys := optionsByFile["journeys"] // file names are matched in lowercase
	if journeys.PrimaryKeyField != "id" || len(journeys.Children) != 2 {
		t.Fatalf("unexpected options: %+v", journeys)
	}
}

func TestLoadLinksChildRowsToParentKey(t *testing.T) {
	database := openTestDatabase(t)
	records := []map[string]any{
		{"id": "j1", "steps": []any{map[string]any{"name": "intake"}, map[string]any{"name": "follow-up"}}},
		{"id": "j2", "steps": []any{map[string]any{"name": "intake"}}},
	}
	err := newTestLoader().Load(database, "pja_journeys", records, journeysWithSteps(), false)
	if err != nil {
		t.Fatal(err)
	}

	var linkedToFirstJourney int
	query := `SELECT COUNT(*) FROM "pja_journeys_steps" WHERE "pja_journeys_id" = 'j1'`
	if err := database.Get(&linkedToFirstJourney, query); err != nil {
		t.Fatal(err)
	}
	if linkedToFirstJourney != 2 {
		t.Errorf("steps linked to j1 = %d, want 2", linkedToFirstJourney)
	}
}

func TestLoadRejectsMissingAndDuplicateKeysWithoutTouchingTable(t *testing.T) {
	database := openTestDatabase(t)
	loader := newTestLoader()
	validRecords := []map[string]any{{"id": "j1"}, {"id": "j2"}}
	if err := loader.Load(database, "pja_journeys", validRecords, journeysWithSteps(), false); err != nil {
		t.Fatal(err)
	}

	badRecords := []map[string]any{{"id": "j3"}, {"id": "j3"}, {"name": "no key"}}
	err := loader.Load(database, "pja_journeys", badRecords, journeysWithSteps(), false)
	if err == nil {
		t.Fatal("want error for missing and duplicate keys, got nil")
	}
	for _, expected := range []string{"1 record(s) without a value", "1 duplicate value(s), e.g. j3"} {
		if !strings.Contains(err.Error(), expected) {
			t.Errorf("error %q does not mention %q", err, expected)
		}
	}

	var rowCount int
	if err := database.Get(&rowCount, `SELECT COUNT(*) FROM "pja_journeys"`); err != nil {
		t.Fatal(err)
	}
	if rowCount != len(validRecords) {
		t.Errorf("table has %d rows after a rejected load, want the previous %d", rowCount, len(validRecords))
	}
}

// journeysWithSteps is the json_options of a journeys file whose steps become a child table.
func journeysWithSteps() JSONFileConfig {
	return JSONFileConfig{PrimaryKeyField: "id", Children: map[string]*JSONFileConfig{"steps": {}}}
}

func newTestLoader() *Loader {
	return NewLoader("pja", nil, zerolog.Nop())
}

// openTestDatabase opens an in-memory SQLite database that lives for the duration of the test.
func openTestDatabase(t *testing.T) *sqlx.DB {
	t.Helper()
	database, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1) // every new connection to :memory: would be a separate, empty database
	t.Cleanup(func() { database.Close() })
	return database
}
