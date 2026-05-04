package sqlserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
type Source struct {
	name             string
	connectionString string
	stagingDir       string
	log              zerolog.Logger
}

func New(name, connectionString, stagingDir string, log zerolog.Logger) *Source {
	return &Source{
		name:             name,
		connectionString: connectionString,
		stagingDir:       stagingDir,
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

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		tableName := strings.TrimSuffix(e.Name(), ".sql")
		sqlPath := filepath.Join(s.stagingDir, e.Name())

		query, err := os.ReadFile(sqlPath)
		if err != nil {
			s.log.Error().Err(err).Str("file", e.Name()).Msg("sqlserver: read staging SQL failed")
			continue
		}

		if err := s.loadQuery(ctx, extDB, stagingDB, string(query), tableName); err != nil {
			s.log.Error().Err(err).Str("table", tableName).Msg("sqlserver: load failed")
		}
	}
	return nil
}

func (s *Source) loadQuery(ctx context.Context, extDB, stagingDB *sqlx.DB, query, tableName string) error {
	rows, err := extDB.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("get columns: %w", err)
	}

	if err := recreate(stagingDB, tableName, cols); err != nil {
		return fmt.Errorf("create staging table: %w", err)
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
		if err := insertRow(stagingDB, tableName, cols, row); err != nil {
			s.log.Error().Err(err).Str("table", tableName).Msg("sqlserver: insert failed")
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows error: %w", err)
	}

	s.log.Info().Str("table", tableName).Int("rows", count).Msg("sqlserver: loaded")
	return nil
}

func recreate(db *sqlx.DB, table string, cols []string) error {
	_, _ = db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", quoted(table)))
	defs := make([]string, len(cols))
	for i, c := range cols {
		defs[i] = c + " TEXT"
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
	_, err := db.Exec(
		fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
			quoted(table),
			strings.Join(cols, ", "),
			strings.Join(placeholders, ", ")),
		vals...)
	return err
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
	return New(name, connStr, stagingDir, log), nil
}

func init() {
	source.Register("sqlserver", constructor)
}
