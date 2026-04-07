package loader

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

type CSVLoader struct {
	db        *sqlx.DB
	inputDir  string
	delimiter rune
	hasHeader bool
	logger    zerolog.Logger
}

type LoaderConfig struct {
	InputDir  string
	Delimiter rune
	HasHeader bool
}

// NewCSVLoader creates a new CSV loader
func NewCSVLoader(db *sqlx.DB, config LoaderConfig, logger zerolog.Logger) *CSVLoader {
	return &CSVLoader{
		db:        db,
		inputDir:  config.InputDir,
		delimiter: config.Delimiter,
		hasHeader: config.HasHeader,
		logger:    logger,
	}
}

// LoadCSVFile loads a single CSV file into the database
func (cl *CSVLoader) LoadCSVFile(filePath string, tableName string) error {
	cl.logger.Info().Str("file", filePath).Str("table", tableName).Msg("Loading CSV file")

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = cl.delimiter

	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV file: %w", err)
	}

	if len(records) == 0 {
		return fmt.Errorf("CSV file is empty")
	}

	// Get headers
	var headers []string
	startIdx := 0

	if cl.hasHeader {
		headers = records[0]
		startIdx = 1
	} else {
		for i := range records[0] {
			headers = append(headers, fmt.Sprintf("col_%d", i))
		}
	}

	// Create table
	if err := cl.createTable(tableName, headers); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Insert records
	if err := cl.insertRecords(tableName, headers, records[startIdx:]); err != nil {
		return fmt.Errorf("failed to insert records: %w", err)
	}

	cl.logger.Info().Str("table", tableName).Int("rows", len(records)-startIdx).Msg("CSV file loaded successfully")
	return nil
}

// LoadAllCSVFiles loads all CSV files from the input directory
func (cl *CSVLoader) LoadAllCSVFiles() error {
	entries, err := os.ReadDir(cl.inputDir)
	if err != nil {
		return fmt.Errorf("failed to read input directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".csv") {
			continue
		}

		filePath := filepath.Join(cl.inputDir, entry.Name())
		tableName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))

		if err := cl.LoadCSVFile(filePath, tableName); err != nil {
			cl.logger.Error().Err(err).Str("file", entry.Name()).Msg("Failed to load CSV file")
		}
	}

	return nil
}

func (cl *CSVLoader) createTable(tableName string, headers []string) error {
	// Drop table if exists
	_, _ = cl.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", sanitizeTableName(tableName)))

	// Build CREATE TABLE statement
	columns := make([]string, len(headers))
	for i, header := range headers {
		columns[i] = fmt.Sprintf("%s TEXT", sanitizeColumnName(header))
	}

	createSQL := fmt.Sprintf("CREATE TABLE %s (id INTEGER PRIMARY KEY, %s)",
		sanitizeTableName(tableName),
		strings.Join(columns, ", "))

	_, err := cl.db.Exec(createSQL)
	return err
}

func (cl *CSVLoader) insertRecords(tableName string, headers []string, records [][]string) error {
	if len(records) == 0 {
		return nil
	}

	// Build INSERT statement
	placeholders := make([]string, len(headers))
	for i := range headers {
		placeholders[i] = "?"
	}

	sanitizedHeaders := make([]string, len(headers))
	for i, h := range headers {
		sanitizedHeaders[i] = sanitizeColumnName(h)
	}

	insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		sanitizeTableName(tableName),
		strings.Join(sanitizedHeaders, ", "),
		strings.Join(placeholders, ", "))

	stmt, err := cl.db.Prepare(insertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	for _, record := range records {
		values := make([]interface{}, len(record))
		for i, v := range record {
			values[i] = v
		}

		if _, err := stmt.Exec(values...); err != nil {
			cl.logger.Error().Err(err).Msg("Failed to insert record")
		}
	}

	return nil
}

func sanitizeTableName(name string) string {
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(name, "\"", ""))
}

func sanitizeColumnName(name string) string {
	// Strip spaces and quotes; keep the name unquoted so SQL queries can
	// reference columns without quotes (e.g. SELECT patient_id FROM patients).
	clean := strings.ReplaceAll(name, "\"", "")
	clean = strings.TrimSpace(clean)
	clean = strings.ReplaceAll(clean, " ", "_")
	return clean
}

// GetTables returns all tables in the database
func (cl *CSVLoader) GetTables() ([]string, error) {
	var tables []string
	query := "SELECT name FROM sqlite_master WHERE type='table'"
	err := cl.db.Select(&tables, query)
	return tables, err
}

// GetTableData returns all data from a table
func (cl *CSVLoader) GetTableData(tableName string) ([]map[string]interface{}, error) {
	query := fmt.Sprintf("SELECT * FROM %s", sanitizeTableName(tableName))
	rows, err := cl.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query table: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range cols {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		entry := make(map[string]interface{})
		for i, col := range cols {
			entry[col] = values[i]
		}
		results = append(results, entry)
	}

	return results, nil
}
