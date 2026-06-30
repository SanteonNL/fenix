package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"net/http"

	"github.com/SanteonNL/fenix/config"
	"github.com/SanteonNL/fenix/cmd/fenix/converter"
	"github.com/SanteonNL/fenix/cmd/fenix/fhirserver"
	"github.com/SanteonNL/fenix/cmd/fenix/output"
	"github.com/SanteonNL/fenix/cmd/fenix/querycompiler"
	"github.com/SanteonNL/fenix/source"
	_ "github.com/SanteonNL/fenix/source/local"
	_ "github.com/SanteonNL/fenix/source/luscii"
	_ "github.com/SanteonNL/fenix/source/sftp"
	_ "github.com/SanteonNL/fenix/source/sqldb"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

var version = "dev"

var (
	configPath     = flag.String("config", "config/config.yaml", "Path to configuration file")
	sqlFile        = flag.String("sql", "", "SQL file with conversion queries (overrides config)")
	csvFile        = flag.String("file", "", "Specific CSV file to load (optional, loads all if omitted)")
	command        = flag.String("cmd", "all", "Command: prepare, convert, all, serve, serve-all")
	help           = flag.Bool("help", false, "Show help message")
	servePort      = flag.String("port", "127.0.0.1:8080", "Address for the FHIR API server (used with -cmd serve)")
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
	case "prepare":
		loadSources(ctx, db, cfg, repoRoot, &log)
		runTransforms(db, cfg, repoRoot, &log)
	case "convert":
		runTransforms(db, cfg, repoRoot, &log)
		runSourceQueries(db, cfg, repoRoot, outputMgr, &log)
		convertToFHIR(db, cfg, repoRoot, outputMgr, &log)
	case "all":
		loadSources(ctx, db, cfg, repoRoot, &log)
		runTransforms(db, cfg, repoRoot, &log)
		runSourceQueries(db, cfg, repoRoot, outputMgr, &log)
		convertToFHIR(db, cfg, repoRoot, outputMgr, &log)
	case "serve-all":
		loadSources(ctx, db, cfg, repoRoot, &log)
		runTransforms(db, cfg, repoRoot, &log)
		fallthrough
	case "serve":
		startFHIRServer(db, cfg, repoRoot, &log)
	default:
		log.Fatal().Str("command", *command).Msg("Unknown command")
	}

	log.Info().Msg("Process completed successfully")
}

func initializeStagingDatabase(cfg *config.Config, repoRoot string, log *zerolog.Logger) (*sqlx.DB, error) {
	// Resolve a relative SQLite path against the repo root before handing off.
	if (cfg.Staging.Database == "" || cfg.Staging.Database == "sqlite") && cfg.Staging.Path != "" {
		cfg.Staging.Path = resolvePath(repoRoot, cfg.Staging.Path)
	}
	db, err := config.NewStagingDB(cfg)
	if err != nil {
		return nil, err
	}
	log.Info().Str("database", cfg.Staging.Database).Msg("Connected to staging database")
	return db, nil
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

// loadSources iterates all configured sources and loads each into the staging database.
func loadSources(ctx context.Context, db *sqlx.DB, cfg *config.Config, repoRoot string, log *zerolog.Logger) {
	watermarkPath := config.WatermarkPath(cfg, repoRoot)

	fw, err := config.NewFileWriter(cfg, repoRoot)
	if err != nil {
		log.Error().Err(err).Msg("staging files: failed to create file writer — file output disabled")
	}

	for name, sc := range cfg.Sources {
		src, err := config.BuildSource(name, sc, repoRoot, watermarkPath, *log)
		if err != nil {
			log.Error().Err(err).Str("source", name).Msg("Failed to build source")
			continue
		}
		if fw != nil {
			if fa, ok := src.(source.FileWritable); ok {
				fa.SetFileWriter(fw)
			}
		}
		if err := src.Load(ctx, db); err != nil {
			log.Error().Err(err).Str("source", name).Msg("Failed to load source")
		}
	}
}

// runTransforms executes the cleaned/DWH-layer SQL files against the staging DB, after loading
// and before FHIR conversion. There is no per-file config: transforms are discovered purely by
// directory convention, mirroring runSourceQueries.
//
//   - queries/<source>/dwh/*.sql   — per-source cleanup, run for every configured source.
//   - queries/dwh/cross/*.sql      — cross-source cleanup that may join several sources' staging
//     tables. Each file must declare the sources it needs via a "-- :requires <a>,<b>" annotation
//     (mirrors the "-- :pk <col>" convention used by sqldb staging queries). A required source
//     that is not configured is fatal — silently running with missing data would be worse.
//
// Files within a directory run in lexicographic order, so prefix with "01_", "02_", etc. when
// one transform depends on another.
func runTransforms(db *sqlx.DB, cfg *config.Config, repoRoot string, log *zerolog.Logger) {
	for name := range cfg.Sources {
		dir := resolvePath(repoRoot, "queries/"+name+"/dwh")
		runTransformDir(db, dir, nil, cfg, log)
	}
	runTransformDir(db, resolvePath(repoRoot, "queries/dwh/cross"), parseRequires, cfg, log)
}

// runTransformDir executes every *.sql file in dir, in lexicographic order. If checkRequires is
// non-nil, it is used to extract a file's required source names, which are validated against
// cfg.Sources before the file runs (fatal on a missing source).
func runTransformDir(db *sqlx.DB, dir string, checkRequires func(query string) []string, cfg *config.Config, log *zerolog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // no transform layer here — most sources won't have one
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		relPath := filepath.Join(dir, e.Name())
		sqlBytes, err := os.ReadFile(relPath)
		if err != nil {
			log.Error().Err(err).Str("file", relPath).Msg("Failed to read transform file")
			continue
		}
		query := string(sqlBytes)
		if checkRequires != nil {
			for _, req := range checkRequires(query) {
				if _, ok := cfg.Sources[req]; !ok {
					log.Fatal().Str("file", relPath).Str("missing_source", req).
						Msg("transform requires a source that is not configured")
				}
			}
		}
		log.Info().Str("file", relPath).Msg("Running transform")
		for _, stmt := range converter.SplitStatements(query) {
			if _, err := db.Exec(stmt); err != nil {
				log.Error().Err(err).Str("file", relPath).Msg("Transform statement failed")
			}
		}
	}
}

// parseRequires extracts source names from a "-- :requires <a>,<b>" annotation.
func parseRequires(query string) []string {
	for _, line := range strings.Split(query, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-- :requires ") {
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "-- :requires "))
			var sources []string
			for _, s := range strings.Split(raw, ",") {
				if s = strings.TrimSpace(s); s != "" {
					sources = append(sources, s)
				}
			}
			return sources
		}
	}
	return nil
}

// runSourceQueries runs FHIR conversion for every SQL file found in queries/<sourceName>/fhir/.
func runSourceQueries(db *sqlx.DB, cfg *config.Config, repoRoot string, outputMgr *output.Manager, log *zerolog.Logger) {
	for name := range cfg.Sources {
		queriesDir := resolvePath(repoRoot, "queries/"+name+"/fhir")
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
	if sc.Type != "sqldb" {
		log.Fatal().Str("source", name).Str("type", sc.Type).Msg("Direct connection only supported for sqldb sources")
	}
	var db *sqlx.DB
	var err error
	for attempt := 1; attempt <= 10; attempt++ {
		db, err = sqlx.Connect("sqlserver", sc.ConnectionString)
		if err == nil {
			break
		}
		log.Warn().Err(err).Int("attempt", attempt).Str("source", name).Msg("SQL Server not ready, retrying in 5s...")
		time.Sleep(5 * time.Second)
	}
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

	addr := *servePort
	log.Info().Str("addr", addr).Str("source", *sourceName).Msg("Starting FHIR API server")
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal().Err(err).Msg("FHIR API server stopped")
	}
}

func printHelp() {
	fmt.Println(`CSV to FHIR Converter

Usage: fenix [options]

Options:
  -config string   Path to configuration file (default "config/config.yaml")
  -sql    string   SQL file with multi-statement conversion queries
  -file   string   Specific CSV file to load (optional)
  -cmd    string   prepare | convert | all  (default "all")
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

Configuration (config.yaml):
  database:
    type: sqlite
    path: data/fenix.db
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
