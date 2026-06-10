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

// JSONFileConfig controls how a specific JSON file's records are flattened.
// Fields listed in Children become separate child tables with a FK back to the
// parent; each child entry can itself carry a JSONFileConfig for deeper nesting.
// All other arrays are JSON-encoded as text (the default behaviour).
type JSONFileConfig struct {
	IDField  string
	Children map[string]*JSONFileConfig // field name → sub-config (nil = promote, no deeper nesting)
}

// Source reads all files from a local directory into the staging database.
// Format is detected by extension: .json and .csv are supported.
// Table name: <sourceName>_<filename_without_extension>.
//
// JSON: unwraps {"data": [...]} or bare array; nested objects are flattened
// with "_" separators; arrays are stored as JSON text.
// JSON with a matching JSONFileConfig: fields in Children become child tables (recursive).
// CSV: first row is treated as the header.
type Source struct {
	name        string
	dir         string
	delimiter   rune
	jsonOptions map[string]JSONFileConfig // keyed by lower-cased file base name (no ext)
	fileWriter  source.FileWriter
	log         zerolog.Logger
}

// SetFileWriter configures file-based staging output alongside the database.
func (s *Source) SetFileWriter(w source.FileWriter) {
	s.fileWriter = w
}

func New(name, dir string, delimiter rune, jsonOptions map[string]JSONFileConfig, log zerolog.Logger) *Source {
	if delimiter == 0 {
		delimiter = ','
	}
	return &Source{name: name, dir: dir, delimiter: delimiter, jsonOptions: jsonOptions, log: log}
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
// If a JSONFileConfig is configured for this file, fields in Children become
// separate child tables (recursively); otherwise the generic flattenMap strategy is used.
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

	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	if cfg, ok := s.jsonOptions[base]; ok {
		return NewLoader(s.name, s.fileWriter, s.log).Load(db, table, records, cfg, false)
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

	if s.fileWriter != nil {
		if err := s.fileWriter.WriteTableFull(table, cols, flat); err != nil {
			s.log.Warn().Err(err).Str("table", table).Msg("local: csv write failed")
		}
		if err := s.fileWriter.WriteJSONFull(table, records); err != nil {
			s.log.Warn().Err(err).Str("table", table).Msg("local: json write failed")
		}
	}
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
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if len(records) < 1 {
		s.log.Warn().Str("file", filepath.Base(path)).Msg("local: empty CSV")
		return nil
	}

	headers := sanitise(records[0])
	rawRows := records[1:]

	allRows := make([]map[string]interface{}, 0, len(rawRows))
	for _, rec := range rawRows {
		row := make(map[string]interface{}, len(headers))
		for i, h := range headers {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		allRows = append(allRows, row)
	}

	if err := recreate(db, table, headers); err != nil {
		return err
	}
	for _, row := range allRows {
		if err := insertRow(db, table, headers, row); err != nil {
			s.log.Error().Err(err).Str("table", table).Msg("local: insert failed")
		}
	}
	s.log.Info().Str("source", s.name).Str("type", "local").Str("table", table).Str("mode", "full").Int("rows", len(allRows)).Msg("source: loaded")

	if s.fileWriter != nil {
		if err := s.fileWriter.WriteTableFull(table, headers, allRows); err != nil {
			s.log.Warn().Err(err).Str("table", table).Msg("local: csv write failed")
		}
	}
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

// ParseJSONOptions parses the json_options config block into a map keyed by file base name.
func ParseJSONOptions(config map[string]interface{}) map[string]JSONFileConfig {
	rawOpts, ok := config["json_options"].(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string]JSONFileConfig, len(rawOpts))
	for fileName, v := range rawOpts {
		m, _ := v.(map[string]interface{})
		result[strings.ToLower(fileName)] = parseJSONFileConfig(m)
	}
	return result
}

func parseJSONFileConfig(m map[string]interface{}) JSONFileConfig {
	if m == nil {
		return JSONFileConfig{}
	}
	idField, _ := m["id_field"].(string)
	var children map[string]*JSONFileConfig
	if rawChildren, ok := m["children"].(map[string]interface{}); ok {
		children = make(map[string]*JSONFileConfig, len(rawChildren))
		for name, v := range rawChildren {
			if v == nil {
				children[name] = nil
			} else if cm, ok := v.(map[string]interface{}); ok {
				sub := parseJSONFileConfig(cm)
				children[name] = &sub
			} else {
				children[name] = nil
			}
		}
	}
	return JSONFileConfig{IDField: idField, Children: children}
}

// Constructor for registry-based source instantiation.
func constructor(name string, config map[string]interface{}, log zerolog.Logger) (source.Source, error) {
	dir, ok := config["dir"].(string)
	if !ok || dir == "" {
		return nil, fmt.Errorf("local source %q: missing or invalid 'dir'", name)
	}

	delimiter := ','
	if delim, ok := config["delimiter"].(string); ok && delim != "" {
		delimiter = rune(delim[0])
	}

	jsonOptions := ParseJSONOptions(config)
	return New(name, dir, delimiter, jsonOptions, log), nil
}

func init() {
	source.Register("local", constructor)
}
