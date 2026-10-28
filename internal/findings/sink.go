package findings

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteJSONL appends records to dir/findings-<runID>.jsonl, one JSON object
// per line, and returns the file's path. Appending lets one run flush more
// than once (the API server flushes after each export). JSON lines load
// straight into DuckDB, Power BI, Grafana/Loki or pandas.
func WriteJSONL(dir, runID string, records []Record) (string, error) {
	if len(records) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create findings dir: %w", err)
	}
	path := filepath.Join(dir, "findings-"+runID+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("open findings file: %w", err)
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			f.Close()
			return "", fmt.Errorf("write finding: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return "", err
	}
	return path, f.Close()
}
