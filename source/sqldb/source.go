package sqlserver

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/SanteonNL/fenix/internal/source"
	"github.com/jmoiron/sqlx"
	_ "github.com/microsoft/go-mssqldb"
	"github.com/rs/zerolog"
)

// Source connects to an external SQL Server, executes staging SQL files against it,
// and loads the results into the fenix staging database.
//
// Staging SQL files live in a dedicated subdirectory (stagingDir).
// Each file is named after the staging table it produces — e.g. patients.sql
// creates a "patients" table in the fenix staging DB.
//
// Incremental loading: if a .watermark.json file exists at watermarkPath, tables
// annotated with "-- :pk <col>" are loaded incrementally using INSERT OR REPLACE,
// filtering source rows by "updated_at > <watermark timestamp>".
// After a successful load the watermark is updated to the current time.
type Source struct {
	name             string
	connectionString string
	stagingDir       string
	watermarkPath    string
	watermark        map[string]string // table name → RFC3339 timestamp
	log              zerolog.Logger
}

func New(name, connectionString, stagingDir, watermarkPath string, log zerolog.Logger) *Source {
	return &Source{
		name:             name,
		connectionString: connectionString,
		stagingDir:       stagingDir,
		watermarkPath:    watermarkPath,
		watermark:        source.ReadWatermark(watermarkPath, log),
		log:              log,
	}
}

func (s *Source) Load(ctx context.Context, stagingDB *sqlx.DB) error {
	extDB, err := sqlx.ConnectContext(ctx, "sqlserver", s.connectionString)
	if err != nil {
		return fmt.Errorf("sqlserver source %q: connect: %w", s.name, err)
	}
	defer extDB.Close()

	entries, err := os.ReadDir(s.stagingDir)
	if err != nil {
		return fmt.Errorf("sqlserver source %q: read staging dir %q: %w", s.name, s.stagingDir, err)
	}

	// Track which tables have a :pk annotation so we can update their watermark.
	incrementalTables := map[string]bool{}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		tableName := strings.TrimSuffix(e.Name(), ".sql")
		sqlPath := filepath.Join(s.stagingDir, e.Name())

		rawQuery, err := os.ReadFile(sqlPath)
		if err != nil {
			s.log.Error().Err(err).Str("file", e.Name()).Msg("sqlserver: read staging SQL failed")
			continue
		}

		if parsePK(string(rawQuery)) != "" {
			incrementalTables[tableName] = true
		}

		if err := s.loadQuery(ctx, extDB, stagingDB, string(rawQuery), tableName); err != nil {
			s.log.Error().Err(err).Str("table", tableName).Msg("sqlserver: load failed")
		}
	}

	if s.watermarkPath != "" && len(incrementalTables) > 0 {
		now := time.Now().UTC().Format(time.RFC3339)
		updated := make(map[string]string, len(s.watermark))
		for k, v := range s.watermark {
			updated[k] = v
		}
		for t := range incrementalTables {
			updated[t] = now
		}
		if err := source.WriteWatermark(s.watermarkPath, updated); err != nil {
			s.log.Warn().Err(err).Msg("sqlserver: failed to save watermark")
		} else {
			s.log.Info().Str("path", s.watermarkPath).Msg("sqlserver: watermark updated")
		}
	}

	return nil
}

func (s *Source) loadQuery(ctx context.Context, extDB, stagingDB *sqlx.DB, rawQuery, tableName string) error {
	pk := parsePK(rawQuery)
	since := s.watermark[tableName]
	incremental := since != "" && pk != ""

	query, err := renderTemplate(rawQuery, map[string]interface{}{"since": since})
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}
	query = stripAnnotations(query)

	rows, err := extDB.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("get columns: %w", err)
	}

	if incremental {
		s.log.Info().Str("table", tableName).Str("since", since).Msg("sqlserver: incremental load")
		if err := ensureTable(stagingDB, tableName, cols, pk); err != nil {
			return fmt.Errorf("ensure staging table: %w", err)
		}
	} else {
		if err := recreate(stagingDB, tableName, cols, pk); err != nil {
			return fmt.Errorf("create staging table: %w", err)
		}
	}

	count := 0
	vals := make([]interface{}, len(cols))
	ptrs := make([]interface{}, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			s.log.Error().Err(err).Str("table", tableName).Msg("sqlserver: scan failed")
			continue
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			if vals[i] != nil {
				row[col] = fmt.Sprintf("%v", vals[i])
			}
		}
		if incremental {
			if err := upsertRow(stagingDB, tableName, cols, pk, row); err != nil {
				s.log.Error().Err(err).Str("table", tableName).Msg("sqlserver: upsert failed")
			}
		} else {
			if err := insertRow(stagingDB, tableName, cols, row); err != nil {
				s.log.Error().Err(err).Str("table", tableName).Msg("sqlserver: insert failed")
			}
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	mode := "full"
	if incremental {
		mode = "incremental"
	}
	s.log.Info().Str("table", tableName).Str("mode", mode).Int("rows", count).Msg("sqlserver: loaded")
	return nil
}

// parsePK extracts the primary-key column name from a "-- :pk <col>" annotation.
func parsePK(query string) string {
	for _, line := range strings.Split(query, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-- :pk ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "-- :pk "))
		}
	}
	return ""
}

// stripAnnotations removes "-- :<key> <value>" directive lines before sending SQL to the server.
func stripAnnotations(query string) string {
	var lines []string
	for _, line := range strings.Split(query, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "-- :") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// renderTemplate processes Go template directives (e.g. {{if .since}}) in the SQL query.
func renderTemplate(query string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("sql").Parse(query)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// textType returns the appropriate text column type for the target database.
func textType(db *sqlx.DB) string {
	if db.DriverName() == "sqlserver" {
		return "NVARCHAR(MAX)"
	}
	return "TEXT"
}

// colDefs builds column definition strings for CREATE TABLE, using the correct text type for the driver.
func colDefs(db *sqlx.DB, cols []string, pk string) []string {
	t := textType(db)
	defs := make([]string, len(cols))
	for i, c := range cols {
		if c == pk {
			defs[i] = c + " " + t + " PRIMARY KEY"
		} else {
			defs[i] = c + " " + t
		}
	}
	return defs
}

func recreate(db *sqlx.DB, table string, cols []string, pk string) error {
	_, _ = db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoted(table)))
	defs := colDefs(db, cols, pk)
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE %s (%s)", quoted(table), strings.Join(defs, ", ")))
	return err
}

func ensureTable(db *sqlx.DB, table string, cols []string, pk string) error {
	defs := colDefs(db, cols, pk)
	createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", quoted(table), strings.Join(defs, ", "))
	if db.DriverName() == "sqlserver" {
		// SQL Server does not support CREATE TABLE IF NOT EXISTS
		_, err := db.Exec(fmt.Sprintf(
			`IF OBJECT_ID('%s', 'U') IS NULL %s`,
			strings.ReplaceAll(table, "'", ""), createSQL))
		return err
	}
	_, err := db.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", quoted(table), strings.Join(defs, ", ")))
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

// upsertRow deletes any existing row with the same pk then inserts, making it
// database-independent (avoids SQLite-specific INSERT OR REPLACE).
func upsertRow(db *sqlx.DB, table string, cols []string, pk string, row map[string]interface{}) error {
	if pk != "" {
		if pkVal := row[pk]; pkVal != nil {
			del := db.Rebind(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", quoted(table), pk))
			if _, err := db.Exec(del, fmt.Sprintf("%v", pkVal)); err != nil {
				return fmt.Errorf("delete before upsert: %w", err)
			}
		}
	}
	return insertRow(db, table, cols, row)
}

func quoted(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, ``) + `"`
}


func constructor(name string, config map[string]interface{}, log zerolog.Logger) (source.Source, error) {
	connStr, _ := config["connection_string"].(string)
	if connStr == "" {
		return nil, fmt.Errorf("sqlserver source %q: missing 'connection_string'", name)
	}
	stagingDir, _ := config["staging_dir"].(string)
	if stagingDir == "" {
		return nil, fmt.Errorf("sqlserver source %q: missing 'staging_dir'", name)
	}
	watermarkPath, _ := config["watermark_path"].(string)
	return New(name, connStr, stagingDir, watermarkPath, log), nil
}

func init() {
	source.Register("sqlserver", constructor)
}
