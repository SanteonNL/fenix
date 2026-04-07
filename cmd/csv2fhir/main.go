package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SanteonNL/fenix/cmd/csv2fhir/config"
	"github.com/SanteonNL/fenix/cmd/csv2fhir/converter"
	"github.com/SanteonNL/fenix/cmd/csv2fhir/loader"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

var (
	configPath = flag.String("config", "config/csv2fhir.yaml", "Path to configuration file")
	sqlFile    = flag.String("sql", "", "SQL file with conversion queries (overrides config)")
	csvFile    = flag.String("file", "", "Specific CSV file to load (optional, loads all if omitted)")
	command    = flag.String("cmd", "all", "Command: load, convert, all")
	help       = flag.Bool("help", false, "Show help message")
)

func main() {
	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// Find repository root
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find repository root: %v\n", err)
		os.Exit(1)
	}

	// Resolve config path
	resolvedConfigPath := resolveConfigPath(*configPath)
	cfg, err := config.LoadConfig(resolvedConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration from %s: %v\n", resolvedConfigPath, err)
		os.Exit(1)
	}

	// Initialize logger with conditional colored output based on config
	var log zerolog.Logger
	if cfg.Environment == "dev" {
		// Development mode: colored console output
		output := zerolog.ConsoleWriter{Out: os.Stderr}
		output.TimeFormat = "15:04:05"
		log = zerolog.New(output).With().Timestamp().Logger()
		log.Info().Msg("🔧 Development mode enabled (colored output)")
	} else {
		// Production mode: JSON output
		log = zerolog.New(os.Stderr).With().Timestamp().Logger()
		log.Info().Msg("Production mode (JSON output)")
	}

	log.Info().Str("repoRoot", repoRoot).Msg("Repository root found")
	log.Info().Str("config", resolvedConfigPath).Msg("Configuration loaded")

	db, err := initializeDatabase(cfg, repoRoot, &log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer db.Close()

	switch *command {
	case "load":
		loadCSVFiles(db, cfg, repoRoot, &log)
	case "convert":
		convertToFHIR(db, cfg, repoRoot, &log)
	case "all":
		loadCSVFiles(db, cfg, repoRoot, &log)
		convertToFHIR(db, cfg, repoRoot, &log)
	default:
		log.Fatal().Str("command", *command).Msg("Unknown command")
	}

	log.Info().Msg("Process completed successfully")
}

func initializeDatabase(cfg *config.Config, repoRoot string, log *zerolog.Logger) (*sqlx.DB, error) {
	switch cfg.Database.Type {
	case "sqlite":
		dbPath := resolvePath(repoRoot, cfg.Database.Path)
		dbDir := filepath.Dir(dbPath)
		if dbDir != "." && dbDir != "" {
			if err := os.MkdirAll(dbDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create database directory: %w", err)
			}
		}
		driver := cfg.Database.SQLiteDriver() // "sqlite" (pure Go) of "sqlite3" (CGO)
		db, err := sqlx.Connect(driver, dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to SQLite (driver=%s): %w", driver, err)
		}
		log.Info().Str("path", dbPath).Str("driver", driver).Msg("Connected to SQLite")
		return db, nil

	case "postgres":
		db, err := sqlx.Connect("postgres", cfg.Database.Connection)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		log.Info().Msg("Connected to PostgreSQL")
		return db, nil

	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Database.Type)
	}
}

func loadCSVFiles(db *sqlx.DB, cfg *config.Config, repoRoot string, log *zerolog.Logger) {
	if cfg.CSV.InputDir == "" {
		log.Warn().Msg("CSV input directory not configured")
		return
	}

	inputDir := resolvePath(repoRoot, cfg.CSV.InputDir)
	loaderConfig := loader.LoaderConfig{
		InputDir:  inputDir,
		Delimiter: rune(cfg.CSV.Delimiter[0]),
		HasHeader: cfg.CSV.HasHeader,
	}
	csvLoader := loader.NewCSVLoader(db, loaderConfig, *log)

	if *csvFile != "" {
		tableName := filepath.Base(*csvFile)
		tableName = tableName[:len(tableName)-len(filepath.Ext(tableName))]
		if err := csvLoader.LoadCSVFile(*csvFile, tableName); err != nil {
			log.Error().Err(err).Msg("Failed to load CSV file")
		}
	} else {
		if err := csvLoader.LoadAllCSVFiles(); err != nil {
			log.Error().Err(err).Msg("Failed to load CSV files")
		}
	}

	if tables, err := csvLoader.GetTables(); err == nil {
		log.Info().Strs("tables", tables).Msg("Loaded tables")
	}
}

func convertToFHIR(db *sqlx.DB, cfg *config.Config, repoRoot string, log *zerolog.Logger) {
	// Determine which SQL file to use: flag overrides config
	sqlPath := *sqlFile
	if sqlPath == "" {
		sqlPath = cfg.FHIR.SQLFile
	}
	if sqlPath == "" {
		log.Fatal().Msg("No SQL file configured. Set fhir.sqlFile in config or pass -sql flag.")
	}

	resolvedSQLPath := resolvePath(repoRoot, sqlPath)
	log.Info().Str("sql", resolvedSQLPath).Msg("Starting FHIR conversion")

	query, err := os.ReadFile(resolvedSQLPath)
	if err != nil {
		log.Fatal().Err(err).Str("file", resolvedSQLPath).Msg("Failed to read SQL file")
	}

	fhirConverter := converter.NewFHIRConverter(db, *log)

	resources, err := fhirConverter.ConvertSQL(string(query))
	if err != nil {
		log.Error().Err(err).Msg("Conversion failed")
		return
	}

	outputDir := resolvePath(repoRoot, cfg.Output.Dir)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatal().Err(err).Msg("Failed to create output directory")
	}

	// Output file name derived from SQL file name
	baseName := filepath.Base(resolvedSQLPath)
	baseName = baseName[:len(baseName)-len(filepath.Ext(baseName))]
	outputFile := filepath.Join(outputDir, baseName+"."+cfg.Output.Format)

	var data []byte
	if cfg.Output.Format == "ndjson" {
		data, err = converter.ExportToNDJSON(resources)
	} else {
		data, err = converter.ExportToJSON(resources)
	}
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal output")
		return
	}

	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		log.Error().Err(err).Str("file", outputFile).Msg("Failed to write output file")
		return
	}

	log.Info().Str("file", outputFile).Int("resources", len(resources)).Msg("FHIR resources exported")
}

func printHelp() {
	fmt.Println(`CSV to FHIR Converter

Usage: csv2fhir [options]

Options:
  -config string   Path to configuration file (default "config/csv2fhir.yaml")
  -sql    string   SQL file with multi-statement conversion queries
  -file   string   Specific CSV file to load (optional)
  -cmd    string   load | convert | all  (default "all")
  -help            Show this help message

Environment:
  Configure environment mode in config file:
    environment: dev   # Colored console output (useful for development)
    environment: prod  # JSON output (useful for production logging)

SQL file format:
  Multiple SELECT statements separated by ";".
  Required columns per row:
    resource_id  – root resource identifier
    id           – this row's identifier
    parent_id    – parent row's id (empty string for root rows)
    fhir_path    – e.g. "Patient", "Patient.name", "Patient.name.coding"
    <fields>     – any other column is a leaf field at this level
                   Use dot-notation for scalar nesting: "subject.reference"
                   Multiple rows with same fhir_path + parent_id → FHIR array

Example (patient_1.sql):
  -- Root patient
  SELECT patient_id AS resource_id, patient_id AS id, '' AS parent_id,
         'Patient' AS fhir_path, gender, birth_date AS birthDate
  FROM patients;

  -- Names (multiple rows per patient → Patient.name array)
  SELECT patient_id AS resource_id, name_id AS id, patient_id AS parent_id,
         'Patient.name' AS fhir_path, name_use AS "use", family, given
  FROM patient_names;

Configuration (csv2fhir.yaml):
  database:
    type: sqlite
    path: data/csv2fhir.db
  csv:
    inputDir: data/csv
    delimiter: ","
    hasHeader: true
  fhir:
    sqlFile: queries/hix/flat/patient_1.sql
  output:
    dir: output
    format: ndjson
`)
}

// findRepoRoot finds the fenix repository root by looking for go.mod
func findRepoRoot() (string, error) {
	// Try current working directory
	cwd, err := os.Getwd()
	if err == nil {
		current := cwd
		for {
			if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
				return current, nil
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}

	// Try relative to executable location
	ex, err := os.Executable()
	if err == nil {
		// Start from executable directory
		current := filepath.Dir(ex)
		for i := 0; i < 5; i++ { // Search up 5 levels
			if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
				return current, nil
			}
			current = filepath.Dir(current)
		}
	}

	return "", fmt.Errorf("could not find fenix repository root (go.mod)")
}

// resolveConfigPath resolves the config path relative to project root
func resolveConfigPath(configPath string) string {
	// If absolute path, use as-is
	if filepath.IsAbs(configPath) {
		return configPath
	}

	// Try to find repo root
	repoRoot, err := findRepoRoot()
	if err == nil {
		candidate := filepath.Join(repoRoot, configPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	// Fallback: try current working directory
	if _, err := os.Stat(configPath); err == nil {
		return configPath
	}

	// Return original path and let error handling deal with it
	return configPath
}

// resolvePath resolves a relative path from repo root
func resolvePath(repoRoot, relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	return filepath.Join(repoRoot, relativePath)
}
