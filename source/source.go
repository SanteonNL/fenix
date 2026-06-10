package source

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Source loads data from an external system into the staging database.
// Each source corresponds to one entry under `sources:` in the config.
type Source interface {
	Load(ctx context.Context, db *sqlx.DB) error
}

// FileWriter writes staging table data and raw JSON to files.
// Implementations must be safe for sequential use within a single source load.
type FileWriter interface {
	// WriteTableFull overwrites the table's CSV with all given records.
	WriteTableFull(table string, cols []string, rows []map[string]interface{}) error
	// WriteTableAppend appends rows to an existing CSV; writes header only on first write.
	WriteTableAppend(table string, cols []string, rows []map[string]interface{}) error
	// WriteJSONFull overwrites the JSON file with all records (full hierarchy).
	WriteJSONFull(name string, records []map[string]interface{}) error
	// UpsertJSON merges records into existing JSON by idField (replace matching, append new).
	// Falls back to WriteJSONFull when idField is empty.
	UpsertJSON(name string, idField string, records []map[string]interface{}) error
}

// FileWritable is an optional interface sources can implement to receive a FileWriter.
// When staging.files is configured, the loader calls SetFileWriter before Load.
type FileWritable interface {
	SetFileWriter(w FileWriter)
}
