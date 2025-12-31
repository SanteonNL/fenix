package main

import (
	"flag"
	"fmt"
	"os"

	_ "github.com/denisenkom/go-mssqldb"
	"github.com/jmoiron/sqlx"

	"github.com/SanteonNL/fenix/cmd/internal/config"
)

func main() {
	// Define a flag for the config file (relative to repo root)
	configPath := flag.String("config", "config/development.config.yaml", "Path to configuration file (relative to repo root)")
	flag.Parse()

	// Load configuration (will automatically find repo root)
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Repository root: %s\n", cfg.RootDir)
	fmt.Printf("Using config file: %s\n", cfg.GetAbsPath(*configPath))
	fmt.Printf("Loaded %d datasource(s)\n", len(cfg.Datasources))

	// Example: show how paths work
	fmt.Printf("\nPaths (all relative to repo root):\n")
	fmt.Printf("  Input:  %s\n", cfg.GetAbsPath(cfg.Paths.Input))
	fmt.Printf("  Output: %s\n", cfg.GetAbsPath(cfg.Paths.Output))
	fmt.Printf("  Logs:   %s\n", cfg.GetAbsPath(cfg.Paths.Logs))

	// Test database connections
	for _, ds := range cfg.Datasources {
		fmt.Printf("\nTesting connection to datasource: %s\n", ds.Name)
		fmt.Printf("  Type: %s\n", ds.Type)
		fmt.Printf("  Driver: %s\n", ds.Driver)

		if ds.Type == "sql" {
			if err := testSQLConnection(ds); err != nil {
				fmt.Printf("  ❌ Connection failed: %v\n", err)
			} else {
				fmt.Printf("  ✅ Connection successful!\n")
			}
		}
	}
}

func testSQLConnection(ds config.Datasource) error {
	// Connect to database
	db, err := sqlx.Connect(ds.Driver, ds.ConnectionString)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer db.Close()

	// Test with a simple query
	var count int
	query := "SELECT COUNT(*) FROM patient"
	if err := db.Get(&count, query); err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	fmt.Printf("  📊 Found %d patient(s) in database\n", count)
	return nil
}
