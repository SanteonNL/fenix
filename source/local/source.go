package local

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SanteonNL/fenix/source"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// Source reads all files from a local directory into the staging database.
// Format is detected by extension: .json and .csv are supported.
// Table name: <sourceName>_<filename_without_extension>.
//
// JSON: unwraps {"data": [...]} or bare array; nested objects are flattened
// with "_" separators; arrays are stored as JSON text.
// CSV: first row is treated as the header.
type Source struct {
	name      string
	dir       string
	delimiter rune
	log       zerolog.Logger
}

func New(name, dir string, delimiter rune, log zerolog.Logger) *Source {
	if delimiter == 0 {
		delimiter = ','
	}
	return &Source{name: name, dir: dir, delimiter: delimiter, log: log}
}

func (s *Source) Load(_ context.Context, db *sqlx.DB) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("local source %q: read dir: %w", s.name, err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		base := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		table := strings.ToLower(s.name + "_" + base)
		path := filepath.Join(s.dir, e.Name())

		var loadErr error
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".json":
			loadErr = s.loadJSON(db, path, table)
		case ".csv":
			loadErr = s.loadCSV(db, path, table)
		default:
			continue
		}
		if loadErr != nil {
			s.log.Error().Err(loadErr).Str("file", e.Name()).Str("table", table).Msg("local: load failed")
		}
	}
	return nil
}

// loadJSON reads a JSON file, unwraps the {"data":[...]} envelope if present,
// flattens each record, and inserts into the staging database.
func (s *Source) loadJSON(db *sqlx.DB, path, table string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	records, err := parseJSONRecords(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if len(records) == 0 {
		s.log.Warn().Str("file", filepath.Base(path)).Msg("local: no records")
		return nil
	}

	flat := make([]map[string]interface{}, 0, len(records))
	for _, r := range records {
		f := make(map[string]interface{})
		flattenMap("", r, f)
		flat = append(flat, f)
	}

	cols := columnSet(flat)
	if err := recreate(db, table, cols); err != nil {
		return err
	}
	for _, row := range flat {
		if err := insertRow(db, table, cols, row); err != nil {
			s.log.Error().Err(err).Str("table", table).Msg("local: insert failed")
		}
	}
	s.log.Info().Str("source", s.name).Str("type", "local").Str("table", table).Str("mode", "full").Int("rows", len(flat)).Msg("source: loaded")
	return nil
}

// loadCSV reads a CSV file (first row = header) and inserts into the staging database.
func (s *Source) loadCSV(db *sqlx.DB, path, table string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = s.delimiter
	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if len(records) < 1 {
		s.log.Warn().Str("file", filepath.Base(path)).Msg("local: empty CSV")
		return nil
	}

	headers := sanitise(records[0])
	rows := records[1:]

	if err := recreate(db, table, headers); err != nil {
		return err
	}
	for _, rec := range rows {
		row := make(map[string]interface{}, len(headers))
		for i, h := range headers {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		if err := insertRow(db, table, headers, row); err != nil {
			s.log.Error().Err(err).Str("table", table).Msg("local: insert failed")
		}
	}
	s.log.Info().Str("table", table).Str("mode", "full").Int("rows", len(rows)).Msg("source: loaded")
	return nil
}

// parseJSONRecords unwraps {"data":[...]} or a bare [...].
func parseJSONRecords(data []byte) ([]map[string]interface{}, error) {
	var wrapper struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapper); err == nil && wrapper.Data != nil {
		return wrapper.Data, nil
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(data, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

// flattenMap inlines nested maps with "_" separators; arrays become JSON text.
func flattenMap(prefix string, src map[string]interface{}, dst map[string]interface{}) {
	for k, v := range src {
		key := k
		if prefix != "" {
			key = prefix + "_" + k
		}
		switch val := v.(type) {
		case map[string]interface{}:
			flattenMap(key, val, dst)
		case []interface{}:
			b, _ := json.Marshal(val)
			dst[key] = string(b)
		default:
			dst[key] = v
		}
	}
}

func sanitise(headers []string) []string {
	out := make([]string, len(headers))
	for i, h := range headers {
		h = strings.TrimSpace(h)
		h = strings.ReplaceAll(h, " ", "_")
		h = strings.ReplaceAll(h, `"`, "")
		out[i] = h
	}
	return out
}

func columnSet(rows []map[string]interface{}) []string {
	seen := make(map[string]struct{})
	for _, r := range rows {
		for k := range r {
			seen[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(seen))
	for k := range seen {
		cols = append(cols, k)
	}
	for i := 1; i < len(cols); i++ {
		for j := i; j > 0 && cols[j] < cols[j-1]; j-- {
			cols[j], cols[j-1] = cols[j-1], cols[j]
		}
	}
	return cols
}

func textType(db *sqlx.DB) string {
	if db.DriverName() == "sqlserver" {
		return "NVARCHAR(MAX)"
	}
	return "TEXT"
}

func recreate(db *sqlx.DB, table string, cols []string) error {
	_, _ = db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoted(table)))
	t := textType(db)
	defs := make([]string, len(cols))
	for i, c := range cols {
		defs[i] = c + " " + t
	}
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (%s)", quoted(table), strings.Join(defs, ", ")))
	return err
}

func insertRow(db *sqlx.DB, table string, cols []string, row map[string]interface{}) error {
	placeholders := make([]string, len(cols))
	vals := make([]interface{}, len(cols))
	for i, c := range cols {
		placeholders[i] = "?"
		if v := row[c]; v == nil {
			vals[i] = nil
		} else {
			vals[i] = fmt.Sprintf("%v", v)
		}
	}
	sql := db.Rebind(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoted(table),
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", ")))
	_, err := db.Exec(sql, vals...)
	return err
}

func quoted(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, ``) + `"`
}

// Constructor for registry-based source instantiation.
// The table prefix is always derived from the last path segment of dir,
// so "test/data/source1" → prefix "source1" regardless of the config key name.
func constructor(name string, config map[string]interface{}, log zerolog.Logger) (source.Source, error) {
	dir, ok := config["dir"].(string)
	if !ok || dir == "" {
		return nil, fmt.Errorf("local source %q: missing or invalid 'dir'", name)
	}

	delimiter := ','
	if delim, ok := config["delimiter"].(string); ok && delim != "" {
		delimiter = rune(delim[0])
	}

	return New(filepath.Base(dir), dir, delimiter, log), nil
}

func init() {
	source.Register("local", constructor)
}
