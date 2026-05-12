package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"net/http"

	"github.com/SanteonNL/fenix/cmd/csv2fhir/config"
	"github.com/SanteonNL/fenix/cmd/csv2fhir/converter"
	"github.com/SanteonNL/fenix/cmd/csv2fhir/fhirserver"
	"github.com/SanteonNL/fenix/cmd/csv2fhir/output"
	"github.com/SanteonNL/fenix/cmd/csv2fhir/querycompiler"
	"github.com/SanteonNL/fenix/internal/source"
	_ "github.com/SanteonNL/fenix/internal/source/local"
	_ "github.com/SanteonNL/fenix/internal/source/luscii"
	_ "github.com/SanteonNL/fenix/internal/source/sqlserver"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

var (
	configPath     = flag.String("config", "config/config.yaml", "Path to configuration file")
	sqlFile        = flag.String("sql", "", "SQL file with conversion queries (overrides config)")
	csvFile        = flag.String("file", "", "Specific CSV file to load (optional, loads all if omitted)")
	command        = flag.String("cmd", "all", "Command: load, convert, all, serve, serve-all")
	help           = flag.Bool("help", false, "Show help message")
	servePort      = flag.String("port", "8080", "HTTP port for the FHIR API server (used with -cmd serve)")
	queryConfigDir = flag.String("query-config", "config/queries", "Query compiler config directory (used with -cmd serve)")
	sourceName     = flag.String("source", "hix", "Query compiler source name (used with -cmd serve)")
	groupID        = flag.String("group", "", "Query compiler group ID override (used with -cmd serve)")
	dataSource     = flag.String("data-source", "", "Source name from config to connect to directly, bypassing the staging DB (used with -cmd serve)")
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
	} else {
		// Production mode: JSON output
		log = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}

	// Set global log level (dev defaults to debug, prod to info)
	level, err := zerolog.ParseLevel(cfg.EffectiveLogLevel())
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	log.Info().Str("environment", cfg.Environment).Str("logLevel", level.String()).Msg("Logger initialized")

	log.Info().Str("repoRoot", repoRoot).Msg("Repository root found")
	log.Info().Str("config", resolvedConfigPath).Msg("Configuration loaded")

	db, err := initializeStagingDatabase(cfg, repoRoot, &log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database")
	}
	defer db.Close()

	// Initialize output manager
	outputDir := resolvePath(repoRoot, cfg.Output.Local.Dir)
	var outputMgr *output.Manager
	if cfg.Output.Local.UseTimestamp {
		outputMgr, err = output.NewManager(outputDir, cfg.Output.Local.ArchiveCount, &log)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to initialize output manager")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	switch *command {
	case "load":
		loadSources(ctx, db, cfg, repoRoot, outputMgr, &log)
	case "convert":
		convertToFHIR(db, cfg, repoRoot, outputMgr, &log)
	case "all":
		loadSources(ctx, db, cfg, repoRoot, outputMgr, &log)
		convertToFHIR(db, cfg, repoRoot, outputMgr, &log)
	case "serve-all":
		loadSources(ctx, db, cfg, repoRoot, outputMgr, &log)
		fallthrough
	case "serve":
		startFHIRServer(db, cfg, repoRoot, &log)
	default:
		log.Fatal().Str("command", *command).Msg("Unknown command")
	}

	log.Info().Msg("Process completed successfully")
}

func initializeStagingDatabase(cfg *config.Config, repoRoot string, log *zerolog.Logger) (*sqlx.DB, error) {
	switch cfg.Staging.Database {
	case "sqlite", "":
		dbPath := cfg.Staging.StagingPath()
		if dbPath != ":memory:" {
			dbPath = resolvePath(repoRoot, dbPath)
			if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return nil, fmt.Errorf("failed to create database directory: %w", err)
				}
			}
		}
		driver := cfg.Staging.SQLiteDriver()
		db, err := sqlx.Connect(driver, dbPath)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to SQLite (driver=%s): %w", driver, err)
		}
		log.Info().Str("path", dbPath).Str("driver", driver).Msg("Connected to staging database")
		return db, nil

	case "postgres":
		db, err := sqlx.Connect("postgres", cfg.Staging.Connection)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		log.Info().Msg("Connected to PostgreSQL")
		return db, nil

	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Staging.Database)
	}
}

func convertToFHIR(db *sqlx.DB, cfg *config.Config, repoRoot string, outputMgr *output.Manager, log *zerolog.Logger) {
	sqlPath := *sqlFile
	if sqlPath == "" {
		sqlPath = cfg.FHIR.SQLFile
	}
	if sqlPath == "" {
		log.Debug().Msg("No fhir.sqlFile configured, skipping generic FHIR conversion")
		return
	}
	runFHIRConversion(db, cfg, resolvePath(repoRoot, sqlPath), repoRoot, outputMgr, log)
}

// loadSources iterates all configured sources, loads each into the staging
// database, then runs FHIR conversion for every SQL file found in
// queries/<sourceName>/.
func loadSources(ctx context.Context, db *sqlx.DB, cfg *config.Config, repoRoot string, outputMgr *output.Manager, log *zerolog.Logger) {
	for name, sc := range cfg.Sources {
		src := buildSource(name, sc, repoRoot, *log)
		if err := src.Load(ctx, db); err != nil {
			log.Error().Err(err).Str("source", name).Msg("Failed to load source")
			continue
		}

		queriesDir := resolvePath(repoRoot, "queries/"+name)
		entries, err := os.ReadDir(queriesDir)
		if err != nil {
			log.Warn().Str("dir", queriesDir).Msg("No query directory found, skipping FHIR conversion")
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
				continue
			}
			runFHIRConversion(db, cfg, filepath.Join(queriesDir, e.Name()), repoRoot, outputMgr, log)
		}
	}
}

// buildSource constructs the Source implementation for the given config entry using the registry.
// Converts the SourceConfig struct to a map and uses the registry to instantiate the source by type.
func buildSource(name string, sc config.SourceConfig, repoRoot string, log zerolog.Logger) source.Source {
	stagingDir := sc.StagingDir
	if stagingDir == "" {
		stagingDir = "queries/" + name + "/staging"
	}

	// Convert SourceConfig to map for generic registry use
	configMap := map[string]interface{}{
		"type":              sc.Type,
		"base_url":          sc.BaseURL,
		"api_key":           sc.APIKey,
		"dir":               resolvePath(repoRoot, sc.Dir),
		"delimiter":         sc.Delimiter,
		"connection_string": sc.ConnectionString,
		"staging_dir":       resolvePath(repoRoot, stagingDir),
	}

	src, err := source.Build(sc.Type, name, configMap, log)
	if err != nil {
		log.Fatal().Err(err).Str("source", name).Msg("Failed to build source")
	}
	return src
}

// runFHIRConversion executes one SQL file against the database and writes FHIR output.
func runFHIRConversion(db *sqlx.DB, cfg *config.Config, sqlPath string, repoRoot string, outputMgr *output.Manager, log *zerolog.Logger) {
	log.Info().Str("sql", sqlPath).Msg("Starting FHIR conversion")

	query, err := os.ReadFile(sqlPath)
	if err != nil {
		log.Error().Err(err).Str("file", sqlPath).Msg("Failed to read SQL file")
		return
	}

	profileSvc := converter.NewProfileService(*log)
	if cfg.FHIR.ProfilesDir != "" {
		if err := profileSvc.LoadDir(resolvePath(repoRoot, cfg.FHIR.ProfilesDir)); err != nil {
			log.Warn().Err(err).Msg("Failed to load profiles")
		}
	}

	conceptMapSvc := converter.NewConceptMapService(*log)
	if cfg.FHIR.ConceptMapsDir != "" {
		if err := conceptMapSvc.LoadDir(resolvePath(repoRoot, cfg.FHIR.ConceptMapsDir)); err != nil {
			log.Warn().Err(err).Msg("Failed to load concept maps")
		}
	}

	resources, err := converter.NewFHIRConverter(db, *log, profileSvc, conceptMapSvc).ConvertSQL(string(query))
	if err != nil {
		log.Error().Err(err).Msg("Conversion failed")
		return
	}

	baseName := strings.TrimSuffix(filepath.Base(sqlPath), ".sql")
	ext := cfg.Output.Format
	if ext == "pretty" {
		ext = "json"
	}

	var data []byte
	switch cfg.Output.Format {
	case "ndjson":
		data, err = converter.ExportToNDJSON(resources)
	case "pretty":
		data, err = converter.ExportToPretty(resources)
	default:
		data, err = converter.ExportToJSON(resources)
	}
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal output")
		return
	}

	// Write output file using manager if available, otherwise write directly
	var outputFile string
	if outputMgr != nil {
		// Use timestamped output manager
		outputFile, err = outputMgr.WriteFile(baseName+"."+ext, data)
		if err != nil {
			log.Error().Err(err).Msg("Failed to write output file")
			return
		}
	} else {
		// Write directly to output directory
		outputDir := resolvePath(repoRoot, cfg.Output.Local.Dir)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatal().Err(err).Msg("Failed to create output directory")
		}
		outputFile = filepath.Join(outputDir, baseName+"."+ext)
		if err := os.WriteFile(outputFile, data, 0644); err != nil {
			log.Error().Err(err).Str("file", outputFile).Msg("Failed to write output file")
			return
		}
	}

	log.Info().Str("file", outputFile).Int("resources", len(resources)).Msg("FHIR resources exported")
}

// connectSourceDirectly opens a database connection directly to the named source in cfg.Sources,
// bypassing the SQLite staging DB. Only "sqlserver" sources are supported.
func connectSourceDirectly(name string, cfg *config.Config, log *zerolog.Logger) *sqlx.DB {
	sc, ok := cfg.Sources[name]
	if !ok {
		log.Fatal().Str("source", name).Msg("Source not found in config")
	}
	if sc.Type != "sqlserver" {
		log.Fatal().Str("source", name).Str("type", sc.Type).Msg("Direct connection only supported for sqlserver sources")
	}
	db, err := sqlx.Connect("sqlserver", sc.ConnectionString)
	if err != nil {
		log.Fatal().Err(err).Str("source", name).Msg("Failed to connect directly to SQL Server")
	}
	log.Info().Str("source", name).Msg("Connected directly to SQL Server")
	return db
}

// startFHIRServer starts the FHIR API server. It blocks until the server exits.
func startFHIRServer(stagingDB *sqlx.DB, cfg *config.Config, repoRoot string, log *zerolog.Logger) {
	db := stagingDB
	if *dataSource != "" {
		db = connectSourceDirectly(*dataSource, cfg, log)
		defer db.Close()
	}

	configDir := resolvePath(repoRoot, *queryConfigDir)
	compiler, err := querycompiler.New(configDir, repoRoot)
	if err != nil {
		log.Fatal().Err(err).Str("configDir", configDir).Msg("Failed to initialise query compiler")
	}

	profileSvc := converter.NewProfileService(*log)
	if cfg.FHIR.ProfilesDir != "" {
		if err := profileSvc.LoadDir(resolvePath(repoRoot, cfg.FHIR.ProfilesDir)); err != nil {
			log.Warn().Err(err).Msg("Failed to load profiles")
		}
	}

	conceptMapSvc := converter.NewConceptMapService(*log)
	if cfg.FHIR.ConceptMapsDir != "" {
		if err := conceptMapSvc.LoadDir(resolvePath(repoRoot, cfg.FHIR.ConceptMapsDir)); err != nil {
			log.Warn().Err(err).Msg("Failed to load concept maps")
		}
	}

	conv := converter.NewFHIRConverter(db, *log, profileSvc, conceptMapSvc)
	outputDir := resolvePath(repoRoot, cfg.Output.Local.Dir)
	srv := fhirserver.New(compiler, conv, *sourceName, *groupID, outputDir, *log)

	addr := ":" + *servePort
	log.Info().Str("addr", addr).Str("source", *sourceName).Msg("Starting FHIR API server")
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal().Err(err).Msg("FHIR API server stopped")
	}
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
    format: ndjson`)
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
		for range 5 { // Search up 5 levels
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
