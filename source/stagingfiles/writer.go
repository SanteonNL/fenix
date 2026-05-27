package stagingfiles

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Writer writes staging data to local files.
// Flat records go to <dir>/<table>.csv; hierarchical JSON goes to <rawDir>/<table>.json.
// When RawDir is empty, JSON files are written to Dir.
type Writer struct {
	dir    string
	rawDir string
}

// New creates a Writer and ensures the target directories exist.
func New(dir, rawDir string) (*Writer, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create staging files dir %q: %w", dir, err)
	}
	rd := rawDir
	if rd == "" {
		rd = dir
	} else if err := os.MkdirAll(rd, 0755); err != nil {
		return nil, fmt.Errorf("create staging raw dir %q: %w", rd, err)
	}
	return &Writer{dir: dir, rawDir: rd}, nil
}

// WriteTableFull overwrites <dir>/<table>.csv with all records.
func (w *Writer) WriteTableFull(table string, cols []string, rows []map[string]interface{}) error {
	f, err := os.Create(filepath.Join(w.dir, table+".csv"))
	if err != nil {
		return fmt.Errorf("create csv %s: %w", table, err)
	}
	defer f.Close()
	cw := csv.NewWriter(f)
	if err := cw.Write(cols); err != nil {
		return err
	}
	writeRows(cw, cols, rows)
	cw.Flush()
	return cw.Error()
}

// WriteTableAppend appends rows to <dir>/<table>.csv.
// The header row is written only when the file does not yet exist.
func (w *Writer) WriteTableAppend(table string, cols []string, rows []map[string]interface{}) error {
	path := filepath.Join(w.dir, table+".csv")
	_, statErr := os.Stat(path)
	isNew := os.IsNotExist(statErr)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open csv %s: %w", table, err)
	}
	defer f.Close()

	cw := csv.NewWriter(f)
	if isNew {
		if err := cw.Write(cols); err != nil {
			return err
		}
	}
	writeRows(cw, cols, rows)
	cw.Flush()
	return cw.Error()
}

// WriteJSONFull overwrites <rawDir>/<name>.json with all records as a JSON array.
func (w *Writer) WriteJSONFull(name string, records []map[string]interface{}) error {
	return writeJSONFile(filepath.Join(w.rawDir, name+".json"), records)
}

// UpsertJSON merges incoming records into <rawDir>/<name>.json by idField:
// records with a matching ID replace the existing entry; new IDs are appended.
// Falls back to WriteJSONFull when idField is empty or the file does not exist.
func (w *Writer) UpsertJSON(name string, idField string, records []map[string]interface{}) error {
	if idField == "" {
		return w.WriteJSONFull(name, records)
	}
	path := filepath.Join(w.rawDir, name+".json")
	existing, err := readJSONFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return writeJSONFile(path, records)
		}
		return fmt.Errorf("read existing json %s: %w", name, err)
	}
	return writeJSONFile(path, mergeByID(existing, records, idField))
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeRows(cw *csv.Writer, cols []string, rows []map[string]interface{}) {
	row := make([]string, len(cols))
	for _, rec := range rows {
		for i, col := range cols {
			v := rec[col]
			if v == nil {
				row[i] = ""
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		_ = cw.Write(row)
	}
}

func writeJSONFile(path string, records []map[string]interface{}) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func readJSONFile(path string) ([]map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []map[string]interface{}
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return records, nil
}

// mergeByID replaces existing records whose idField matches an incoming record,
// then appends incoming records that have no match.
func mergeByID(existing, incoming []map[string]interface{}, idField string) []map[string]interface{} {
	result := make([]map[string]interface{}, len(existing))
	copy(result, existing)

	index := make(map[string]int, len(existing))
	for i, r := range existing {
		if id, ok := r[idField]; ok {
			index[fmt.Sprintf("%v", id)] = i
		}
	}
	for _, r := range incoming {
		key := fmt.Sprintf("%v", r[idField])
		if i, ok := index[key]; ok {
			result[i] = r
		} else {
			index[key] = len(result)
			result = append(result, r)
		}
	}
	return result
}
