package main

import (
	"flag"
	"os"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/SanteonNL/fenix/cmd/internal/config"
)

func main() {
	// Define a flag for the config file (relative to repo root)
	configPath := flag.String("config", "config/development.config.yaml", "Path to configuration file (relative to repo root)")
	flag.Parse()

	// Load configuration (will automatically find repo root)
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	// Setup logger based on environment
	setupLogger(cfg.Environment)

	log.Info().
		Str("root", cfg.RootDir).
		Str("config", cfg.GetAbsPath(*configPath)).
		Str("environment", cfg.Environment).
		Msg("Application initialized")

	log.Info().
		Int("count", len(cfg.Datasources)).
		Msg("Loaded datasources")

	// Example: show how paths work
	log.Debug().
		Str("input", cfg.GetAbsPath(cfg.Paths.Input)).
		Str("output", cfg.GetAbsPath(cfg.Paths.Output)).
		Str("logs", cfg.GetAbsPath(cfg.Paths.Logs)).
		Msg("Configured paths")

	// Test database connections
	for _, ds := range cfg.Datasources {
		logger := log.With().
			Str("datasource", ds.Name).
			Str("type", ds.Type).
			Logger()

		if ds.Type == "sql" {
			logger.Info().Str("driver", ds.Driver).Msg("Testing SQL connection")

			start := time.Now()
			if err := testSQLConnection(ds); err != nil {
				logger.Error().
					Err(err).
					Dur("duration_ms", time.Since(start)).
					Msg("Connection failed")
			} else {
				logger.Info().
					Dur("duration_ms", time.Since(start)).
					Msg("Connection successful")
			}
		}
	}
}

func setupLogger(environment string) {
	// Get hostname
	hostname, _ := os.Hostname()

	// Set global log level
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// For development, use pretty console output with colors
	if environment == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}).With().
			Caller().
			Logger()
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		// For production, use JSON output with caller info
		log.Logger = log.With().
			Caller().
			Str("hostname", hostname).
			Int("pid", os.Getpid()).
			Logger()
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	}
}

func testSQLConnection(ds config.Datasource) error {
	// Connect to database
	db, err := sqlx.Connect(ds.Driver, ds.ConnectionString)
	if err != nil {
		return err
	}
	defer db.Close()

	// Test with a simple query
	var count int
	query := "SELECT COUNT(*) FROM patient"
	if err := db.Get(&count, query); err != nil {
		return err
	}

	log.Info().
		Str("datasource", ds.Name).
		Int("patient_count", count).
		Msg("Query successful")

	return nil
}
