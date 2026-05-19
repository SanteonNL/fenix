package luscii

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	lusciiclient "github.com/SanteonNL/fenix/internal/models/luscii/client"
	"github.com/SanteonNL/fenix/internal/source"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// endpointConfig mirrors one entry in the config.yaml endpoints list.
type endpointConfig struct {
	path       string
	table      string
	sinceParam string
	endParam   string
	idField    string
}

// Source loads Luscii API data into the staging database.
// Each configured endpoint is loaded into its own table.
// Fields that are arrays of objects are automatically split into child tables
// named <table>_<fieldname> with a <table>_id foreign key column.
type Source struct {
	name          string
	baseURL       string
	apiKey        string
	endpoints     []endpointConfig
	watermarkPath string
	watermark     map[string]string
	log           zerolog.Logger
}

func (s *Source) Load(ctx context.Context, db *sqlx.DB) error {
	cli := lusciiclient.New(s.baseURL, s.apiKey)

	for _, ep := range s.endpoints {
		since := ""
		if ep.sinceParam != "" {
			since = s.watermark[ep.table]
		}

		params := lusciiclient.FetchParams{
			SinceParam: ep.sinceParam,
			EndParam:   ep.endParam,
			Since:      since,
		}

		s.log.Info().Str("path", ep.path).Str("table", ep.table).Str("since", since).Msg("luscii: fetching")

		records, err := cli.FetchAll(ep.path, params)
		if err != nil {
			s.log.Error().Err(err).Str("path", ep.path).Msg("luscii: fetch failed")
			continue
		}
		if len(records) == 0 {
			s.log.Info().Str("table", ep.table).Msg("luscii: no records returned")
			continue
		}

		s.loadEndpoint(db, ep, records, since)
	}

	if s.watermarkPath != "" {
		now := time.Now().UTC().Format(time.RFC3339)
		updated := make(map[string]string, len(s.watermark))
		for k, v := range s.watermark {
			updated[k] = v
		}
		for _, ep := range s.endpoints {
			if ep.sinceParam != "" {
				updated[ep.table] = now
			}
		}
		if err := source.WriteWatermark(s.watermarkPath, updated); err != nil {
			s.log.Warn().Err(err).Msg("luscii: failed to save watermark")
		}
	}

	return nil
}

// loadEndpoint loads one endpoint's records into the main table and any child tables.
func (s *Source) loadEndpoint(db *sqlx.DB, ep endpointConfig, records []map[string]interface{}, since string) {
	incremental := since != "" && ep.idField != ""
	fkCol := ep.table + "_id"

	// Split every record into flat fields and arrays-of-objects.
	flatRecords := make([]map[string]interface{}, 0, len(records))
	// childTable → accumulated rows
	childBatches := map[string][]map[string]interface{}{}
	// Track which parent IDs were touched (for incremental child cleanup).
	updatedIDs := map[string]bool{}

	for _, rec := range records {
		flat, arrays := splitRecord(rec)
		flatRecords = append(flatRecords, flat)

		if ep.idField != "" {
			if pid := fmt.Sprintf("%v", rec[ep.idField]); pid != "" {
				updatedIDs[pid] = true
			}
		}

		for fieldName, items := range arrays {
			childTable := ep.table + "_" + fieldName
			parentID := fmt.Sprintf("%v", rec[ep.idField])
			for _, item := range items {
				row := flattenObjectFields(item)
				row[fkCol] = parentID
				childBatches[childTable] = append(childBatches[childTable], row)
			}
		}
	}

	// ── Main table ────────────────────────────────────────────────────────────
	mainCols := extractColumns(flatRecords)
	if incremental {
		s.log.Info().Str("table", ep.table).Str("since", since).Int("rows", len(flatRecords)).Msg("luscii: incremental upsert")
		if err := ensureTable(db, ep.table, mainCols, ep.idField); err != nil {
			s.log.Error().Err(err).Str("table", ep.table).Msg("luscii: ensure table failed")
			return
		}
		for _, rec := range flatRecords {
			if err := upsertRow(db, ep.table, mainCols, rec); err != nil {
				s.log.Error().Err(err).Str("table", ep.table).Msg("luscii: upsert failed")
			}
		}
	} else {
		s.log.Info().Str("table", ep.table).Int("rows", len(flatRecords)).Msg("luscii: full load")
		if err := recreateTable(db, ep.table, mainCols, ep.idField); err != nil {
			s.log.Error().Err(err).Str("table", ep.table).Msg("luscii: recreate failed")
			return
		}
		for _, rec := range flatRecords {
			if err := insertRow(db, ep.table, mainCols, rec); err != nil {
				s.log.Error().Err(err).Str("table", ep.table).Msg("luscii: insert failed")
			}
		}
	}

	// ── Child tables ─────────────────────────────────────────────────────────
	for childTable, childRows := range childBatches {
		childCols := extractColumns(childRows)

		if incremental {
			if err := ensureTable(db, childTable, childCols, ""); err != nil {
				s.log.Error().Err(err).Str("table", childTable).Msg("luscii: ensure child table failed")
				continue
			}
			// Delete stale child rows for every parent that was re-fetched.
			for pid := range updatedIDs {
				if _, err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", quoted(childTable), quoted(fkCol)), pid); err != nil {
					s.log.Warn().Err(err).Str("table", childTable).Msg("luscii: delete stale child rows failed")
				}
			}
		} else {
			if err := recreateTable(db, childTable, childCols, ""); err != nil {
				s.log.Error().Err(err).Str("table", childTable).Msg("luscii: recreate child table failed")
				continue
			}
		}

		for _, cr := range childRows {
			if err := insertRow(db, childTable, childCols, cr); err != nil {
				s.log.Error().Err(err).Str("table", childTable).Msg("luscii: child insert failed")
			}
		}

		s.log.Info().Str("table", childTable).Int("rows", len(childRows)).Msg("luscii: child table loaded")
	}
}

// ── Record splitting ──────────────────────────────────────────────────────────

// splitRecord separates a record into flat scalar fields and arrays-of-objects.
// Arrays of objects become child tables; everything else stays in the main table.
func splitRecord(rec map[string]interface{}) (flat map[string]interface{}, arrays map[string][]map[string]interface{}) {
	flat = make(map[string]interface{})
	arrays = make(map[string][]map[string]interface{})
	for k, v := range rec {
		if arr, ok := asObjectArray(v); ok {
			arrays[k] = arr
		} else {
			flat[k] = v
		}
	}
	return
}

// asObjectArray returns v as a slice of maps if it is a non-empty JSON array
// whose every element is an object. Returns false for mixed or primitive arrays.
func asObjectArray(v interface{}) ([]map[string]interface{}, bool) {
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return nil, false
	}
	result := make([]map[string]interface{}, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}
		result = append(result, m)
	}
	return result, true
}

// flattenObjectFields flattens one level of nested objects using "_" as separator.
// Any remaining nested structures (arrays, maps) are JSON-encoded as strings.
func flattenObjectFields(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		if nested, ok := v.(map[string]interface{}); ok {
			for subk, subv := range nested {
				result[k+"_"+subk] = marshalValue(subv)
			}
		} else {
			result[k] = marshalValue(v)
		}
	}
	return result
}

// marshalValue returns scalars as-is and encodes maps/slices as JSON strings.
func marshalValue(v interface{}) interface{} {
	switch v.(type) {
	case nil, string, bool, float64, int, int64:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// ── Column helpers ────────────────────────────────────────────────────────────

// extractColumns collects all unique keys across records, preserving first-seen order.
func extractColumns(records []map[string]interface{}) []string {
	seen := map[string]bool{}
	var cols []string
	for _, rec := range records {
		for k := range rec {
			if !seen[k] {
				seen[k] = true
				cols = append(cols, k)
			}
		}
	}
	return cols
}

// ── SQLite helpers ────────────────────────────────────────────────────────────

func recreateTable(db *sqlx.DB, table string, cols []string, pk string) error {
	_, _ = db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoted(table)))
	return createTable(db, table, cols, pk)
}

func ensureTable(db *sqlx.DB, table string, cols []string, pk string) error {
	defs := colDefs(cols, pk)
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", quoted(table), defs))
	return err
}

func createTable(db *sqlx.DB, table string, cols []string, pk string) error {
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (%s)", quoted(table), colDefs(cols, pk)))
	return err
}

func colDefs(cols []string, pk string) string {
	defs := make([]string, len(cols))
	for i, c := range cols {
		if c == pk {
			defs[i] = quoted(c) + " TEXT PRIMARY KEY"
		} else {
			defs[i] = quoted(c) + " TEXT"
		}
	}
	return strings.Join(defs, ", ")
}

func insertRow(db *sqlx.DB, table string, cols []string, rec map[string]interface{}) error {
	return writeRow(db, "INSERT INTO", table, cols, rec)
}

func upsertRow(db *sqlx.DB, table string, cols []string, rec map[string]interface{}) error {
	return writeRow(db, "INSERT OR REPLACE INTO", table, cols, rec)
}

func writeRow(db *sqlx.DB, verb, table string, cols []string, rec map[string]interface{}) error {
	placeholders := make([]string, len(cols))
	vals := make([]interface{}, len(cols))
	quotedCols := make([]string, len(cols))
	for i, c := range cols {
		placeholders[i] = "?"
		quotedCols[i] = quoted(c)
		v := rec[c]
		if v == nil {
			vals[i] = nil
		} else {
			vals[i] = marshalValue(v)
		}
	}
	_, err := db.Exec(
		fmt.Sprintf("%s %s (%s) VALUES (%s)",
			verb, quoted(table),
			strings.Join(quotedCols, ", "),
			strings.Join(placeholders, ", ")),
		vals...)
	return err
}

func quoted(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, ``) + `"`
}

// ── Config helpers ────────────────────────────────────────────────────────────

func parseEndpoints(cfg map[string]interface{}) []endpointConfig {
	raw, _ := cfg["endpoints"].([]interface{})
	result := make([]endpointConfig, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, endpointConfig{
			path:       strVal(m, "path"),
			table:      strVal(m, "table"),
			sinceParam: strVal(m, "since_param"),
			endParam:   strVal(m, "end_param"),
			idField:    strVal(m, "id_field"),
		})
	}
	return result
}

func strVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func constructor(name string, cfg map[string]interface{}, log zerolog.Logger) (source.Source, error) {
	baseURL, _ := cfg["base_url"].(string)
	apiKey, _ := cfg["api_key"].(string)
	if baseURL == "" {
		return nil, fmt.Errorf("luscii source %q: missing 'base_url'", name)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("luscii source %q: missing 'api_key'", name)
	}
	endpoints := parseEndpoints(cfg)
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("luscii source %q: no endpoints configured", name)
	}
	watermarkPath, _ := cfg["watermark_path"].(string)
	return &Source{
		name:          name,
		baseURL:       baseURL,
		apiKey:        apiKey,
		endpoints:     endpoints,
		watermarkPath: watermarkPath,
		watermark:     source.ReadWatermark(watermarkPath, log),
		log:           log,
	}, nil
}

func init() {
	source.Register("luscii", constructor)
}
