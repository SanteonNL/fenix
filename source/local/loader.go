package local

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SanteonNL/fenix/source"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// Loader flattens JSON records and writes them to the staging database.
// Supports full (table recreated each run) and incremental (upsert by IDField) modes.
// Fields in JSONFileConfig.Children become separate child tables, processed recursively.
type Loader struct {
	name       string
	fileWriter source.FileWriter
	log        zerolog.Logger
}

// NewLoader creates a Loader for the given source name. fileWriter may be nil.
func NewLoader(name string, fileWriter source.FileWriter, log zerolog.Logger) *Loader {
	return &Loader{name: name, fileWriter: fileWriter, log: log}
}

// Load writes records into db using cfg to control flattening.
// incremental=true requires cfg.IDField; rows are upserted and the table is kept
// across runs. incremental=false drops and recreates the table on every run.
// Fields in cfg.Children become child tables named <table>_<field>, processed recursively.
func (l *Loader) Load(db *sqlx.DB, table string, records []map[string]interface{}, cfg JSONFileConfig, incremental bool) error {
	return l.load(db, table, records, records, cfg, incremental, "", nil)
}

// load is the recursive implementation.
// rawRecords holds the original unflattened records for hierarchical JSON file output.
// parentFKCol and parentUpdatedIDs are set by the parent when it is in incremental mode,
// so stale child rows can be deleted before insertion.
func (l *Loader) load(
	db *sqlx.DB,
	table string,
	records []map[string]interface{},
	rawRecords []map[string]interface{},
	cfg JSONFileConfig,
	incremental bool,
	parentFKCol string,
	parentUpdatedIDs map[string]bool,
) error {
	// Delete stale rows injected by the incremental parent before inserting.
	if parentFKCol != "" && len(parentUpdatedIDs) > 0 {
		del := db.Rebind(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", quoted(table), quoted(parentFKCol)))
		for pid := range parentUpdatedIDs {
			if _, err := db.Exec(del, pid); err != nil {
				l.log.Warn().Err(err).Str("table", table).Msg("loader: delete stale rows failed")
			}
		}
	}

	l.validateConfig(table, records, cfg)

	fkCol := table + "_id"
	flatRecords := make([]map[string]interface{}, 0, len(records))
	type childBatch struct {
		cfg  JSONFileConfig
		rows []map[string]interface{}
	}
	childBatches := map[string]*childBatch{}
	updatedIDs := map[string]bool{} // IDs at this level, for child-table cleanup

	for _, rec := range records {
		flat := make(map[string]interface{})
		parentID := fmt.Sprintf("%v", rec[cfg.IDField])
		if cfg.IDField != "" {
			updatedIDs[parentID] = true
		}

		for k, v := range rec {
			childCfg, isChild := cfg.Children[k]
			if isChild {
				if arr, ok := asObjectArray(v); ok {
					childTable := table + "_" + k
					if _, exists := childBatches[childTable]; !exists {
						var sub JSONFileConfig
						if childCfg != nil {
							sub = *childCfg
						}
						childBatches[childTable] = &childBatch{cfg: sub}
					}
					for _, item := range arr {
						row := make(map[string]interface{}, len(item)+1)
						for ik, iv := range item {
							row[ik] = iv
						}
						row[fkCol] = parentID
						childBatches[childTable].rows = append(childBatches[childTable].rows, row)
					}
					continue
				}
				// Listed in Children but not actually an array of objects — JSON text fallback.
				b, _ := json.Marshal(v)
				flat[k] = string(b)
			} else {
				switch val := v.(type) {
				case map[string]interface{}:
					flattenMap(k, val, flat)
				case []interface{}:
					b, _ := json.Marshal(val)
					flat[k] = string(b)
				default:
					flat[k] = val
				}
			}
		}
		flatRecords = append(flatRecords, flat)
	}

	cols := columnSet(flatRecords)
	mode := "full"

	switch {
	case incremental:
		mode = "incremental"
		if err := loaderEnsure(db, table, cols, cfg.IDField); err != nil {
			return fmt.Errorf("ensure table %s: %w", table, err)
		}
		for _, row := range flatRecords {
			if err := loaderUpsert(db, table, cols, cfg.IDField, row); err != nil {
				l.log.Error().Err(err).Str("table", table).Msg("loader: upsert failed")
			}
		}
	case parentFKCol != "":
		// Child table in incremental-parent mode: stale rows already removed, just insert.
		if err := loaderEnsure(db, table, cols, ""); err != nil {
			return fmt.Errorf("ensure child table %s: %w", table, err)
		}
		for _, row := range flatRecords {
			if err := loaderWrite(db, table, cols, row); err != nil {
				l.log.Error().Err(err).Str("table", table).Msg("loader: insert failed")
			}
		}
	default:
		if err := loaderRecreate(db, table, cols, cfg.IDField); err != nil {
			return fmt.Errorf("recreate table %s: %w", table, err)
		}
		for _, row := range flatRecords {
			if err := loaderWrite(db, table, cols, row); err != nil {
				l.log.Error().Err(err).Str("table", table).Msg("loader: insert failed")
			}
		}
	}
	l.log.Info().Str("source", l.name).Str("table", table).Str("mode", mode).Int("rows", len(flatRecords)).Msg("loader: loaded")

	// File output.
	if l.fileWriter != nil && len(flatRecords) > 0 {
		if parentFKCol == "" {
			// Top-level table: flat CSV + hierarchical JSON.
			var csvErr error
			if incremental {
				csvErr = l.fileWriter.WriteTableAppend(table, cols, flatRecords)
			} else {
				csvErr = l.fileWriter.WriteTableFull(table, cols, flatRecords)
			}
			if csvErr != nil {
				l.log.Warn().Err(csvErr).Str("table", table).Msg("loader: csv write failed")
			}
			var jsonErr error
			if incremental {
				jsonErr = l.fileWriter.UpsertJSON(table, cfg.IDField, rawRecords)
			} else {
				jsonErr = l.fileWriter.WriteJSONFull(table, rawRecords)
			}
			if jsonErr != nil {
				l.log.Warn().Err(jsonErr).Str("table", table).Msg("loader: json write failed")
			}
		} else if !incremental && parentFKCol != "" {
			// Child table on full parent load: flat CSV only.
			if err := l.fileWriter.WriteTableFull(table, cols, flatRecords); err != nil {
				l.log.Warn().Err(err).Str("table", table).Msg("loader: child csv write failed")
			}
		}
		// Child tables on incremental parent: skip — hierarchical JSON covers child data.
	}

	// Recurse into children. Pass fkCol + updatedIDs so grandchild tables can clean up
	// stale rows when the parent (this level) was in any incremental context.
	inIncrementalCtx := incremental || parentFKCol != ""
	for childTable, batch := range childBatches {
		var childFKCol string
		var childUpdatedIDs map[string]bool
		if inIncrementalCtx && len(updatedIDs) > 0 {
			childFKCol = fkCol
			childUpdatedIDs = updatedIDs
		}
		if err := l.load(db, childTable, batch.rows, batch.rows, batch.cfg, false, childFKCol, childUpdatedIDs); err != nil {
			l.log.Error().Err(err).Str("table", childTable).Msg("loader: child table failed")
		}
	}

	return nil
}

// validateConfig checks that configured field names appear in at least one record.
// Mismatches are logged as warnings so users catch typos without a hard failure.
func (l *Loader) validateConfig(table string, records []map[string]interface{}, cfg JSONFileConfig) {
	if len(records) == 0 {
		return
	}

	// Build a set of all field names across the first few records (sample up to 10).
	sample := records
	if len(sample) > 10 {
		sample = sample[:10]
	}
	present := make(map[string]bool)
	for _, rec := range sample {
		for k := range rec {
			present[k] = true
		}
	}

	if cfg.IDField != "" && !present[cfg.IDField] {
		l.log.Warn().Str("table", table).Str("id_field", cfg.IDField).
			Msg("loader: id_field not found in records — check config spelling")
	}

	for field := range cfg.Children {
		if !present[field] {
			l.log.Warn().Str("table", table).Str("child_field", field).
				Msg("loader: child field not found in records — check config spelling")
		}
	}
}

// ── DB helpers ─────────────────────────────────────────────────────────────────

func pkTextType(db *sqlx.DB) string {
	if db.DriverName() == "sqlserver" {
		return "NVARCHAR(450)"
	}
	return "TEXT"
}

func loaderRecreate(db *sqlx.DB, table string, cols []string, pk string) error {
	_, _ = db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoted(table)))
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (%s)", quoted(table), loaderColDefs(db, cols, pk)))
	return err
}

func loaderEnsure(db *sqlx.DB, table string, cols []string, pk string) error {
	defs := loaderColDefs(db, cols, pk)
	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", quoted(table), defs)
	if db.DriverName() == "sqlserver" {
		_, err := db.Exec(fmt.Sprintf(`IF OBJECT_ID('%s', 'U') IS NULL %s`,
			strings.ReplaceAll(table, "'", ""), createSQL))
		return err
	}
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", quoted(table), defs))
	return err
}

func loaderColDefs(db *sqlx.DB, cols []string, pk string) string {
	t := textType(db)
	defs := make([]string, len(cols))
	for i, c := range cols {
		if c == pk && pk != "" {
			defs[i] = quoted(c) + " " + pkTextType(db) + " PRIMARY KEY"
		} else {
			defs[i] = quoted(c) + " " + t
		}
	}
	return strings.Join(defs, ", ")
}

func loaderUpsert(db *sqlx.DB, table string, cols []string, pk string, rec map[string]interface{}) error {
	if pk != "" {
		if pkVal := rec[pk]; pkVal != nil {
			del := db.Rebind(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", quoted(table), quoted(pk)))
			if _, err := db.Exec(del, fmt.Sprintf("%v", pkVal)); err != nil {
				return fmt.Errorf("delete before upsert: %w", err)
			}
		}
	}
	return loaderWrite(db, table, cols, rec)
}

func loaderWrite(db *sqlx.DB, table string, cols []string, rec map[string]interface{}) error {
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
	sql := db.Rebind(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoted(table),
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", ")))
	_, err := db.Exec(sql, vals...)
	return err
}
