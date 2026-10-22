package loader

import (
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// rowID extracts the "id" field from a flattened row as a string.
// Named string types (e.g. UuidSchema) don't satisfy .(string), so we
// use fmt.Sprint to cover any underlying-string type.
func rowID(row map[string]interface{}) string {
	v := row["id"]
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return fmt.Sprint(v)
}

// Loader writes flattened structs into relational SQL tables.
//
// Each struct becomes one main-table row. Slice fields become rows in a child
// table named "<table>_<field>" with a FK column "<singular(table)>_id".
//
// Example — Load("patients", lusciiFlattener, records):
//
//	patients               → scalar + inlined-struct columns
//	patients_identifiers   → one row per identifier, patient_id FK
type Loader struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

func New(db *sqlx.DB, logger zerolog.Logger) *Loader {
	return &Loader{db: db, logger: logger}
}

// Load flattens each record using f and inserts into tableName and child tables.
// Tables are recreated on every call (DROP + CREATE).
func (l *Loader) Load(tableName string, f *Flattener, records []interface{}) error {
	if len(records) == 0 {
		l.logger.Warn().Str("table", tableName).Msg("no records to load")
		return nil
	}

	// Flatten all records; collect full column sets for main + every child table.
	flattened := make([]FlatResult, 0, len(records))
	mainCols := make(map[string]struct{})
	childCols := make(map[string]map[string]struct{}) // childTable → colSet

	for _, r := range records {
		fr := f.Flatten(r)
		flattened = append(flattened, fr)

		for k := range fr.Row {
			mainCols[k] = struct{}{}
		}
		for field, rows := range fr.Children {
			ct := childTable(tableName, field)
			if childCols[ct] == nil {
				childCols[ct] = make(map[string]struct{})
			}
			childCols[ct][fkCol(tableName)] = struct{}{}
			for _, row := range rows {
				for k := range row {
					childCols[ct][k] = struct{}{}
				}
			}
		}
	}

	cols := sorted(mainCols)
	if err := l.recreate(tableName, cols); err != nil {
		return fmt.Errorf("create %s: %w", tableName, err)
	}
	for ct, colSet := range childCols {
		if err := l.recreate(ct, sorted(colSet)); err != nil {
			return fmt.Errorf("create %s: %w", ct, err)
		}
	}

	fk := fkCol(tableName)
	for _, fr := range flattened {
		parentID := rowID(fr.Row)

		if err := l.insert(tableName, cols, fr.Row); err != nil {
			l.logger.Error().Err(err).Str("table", tableName).Msg("insert failed")
		}
		for field, childRows := range fr.Children {
			ct := childTable(tableName, field)
			ctCols := sorted(childCols[ct])
			for _, childRow := range childRows {
				childRow[fk] = parentID
				if err := l.insert(ct, ctCols, childRow); err != nil {
					l.logger.Error().Err(err).Str("table", ct).Msg("child insert failed")
				}
			}
		}
	}

	l.logger.Info().Str("table", tableName).Int("rows", len(flattened)).Msg("loaded")
	return nil
}

func (l *Loader) recreate(tableName string, cols []string) error {
	_, _ = l.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoted(tableName)))
	t := textType(l.db)
	defs := make([]string, len(cols))
	for i, c := range cols {
		defs[i] = clean(c) + " " + t
	}
	_, err := l.db.Exec(fmt.Sprintf("CREATE TABLE %s (%s)", quoted(tableName), strings.Join(defs, ", ")))
	return err
}

func (l *Loader) insert(tableName string, cols []string, row map[string]interface{}) error {
	sanCols := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, c := range cols {
		sanCols[i] = clean(c)
		placeholders[i] = "?"
	}
	sql := l.db.Rebind(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		quoted(tableName),
		strings.Join(sanCols, ", "),
		strings.Join(placeholders, ", ")))

	vals := make([]interface{}, len(cols))
	for i, c := range cols {
		vals[i] = row[c]
	}
	_, err := l.db.Exec(sql, vals...)
	return err
}

// textType returns the appropriate text column type for the target database.
func textType(db *sqlx.DB) string {
	if db.DriverName() == "sqlserver" {
		return "NVARCHAR(MAX)"
	}
	return "TEXT"
}

func childTable(parent, field string) string { return parent + "_" + field }
func fkCol(parent string) string             { return singular(parent) + "_id" }

func sorted(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func quoted(name string) string {
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(name, `"`, ``))
}

func clean(name string) string {
	name = strings.ReplaceAll(name, `"`, ``)
	return strings.ReplaceAll(strings.TrimSpace(name), " ", "_")
}
