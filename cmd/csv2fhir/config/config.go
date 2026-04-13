package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Environment string         `yaml:"environment"` // "dev" or "prod"
	Database    DatabaseConfig `yaml:"database"`
	CSV         CSVConfig      `yaml:"csv"`
	FHIR        FHIRConfig     `yaml:"fhir"`
	Output      OutputConfig   `yaml:"output"`
}

type DatabaseConfig struct {
	Type       string `yaml:"type"`       // sqlite, postgres, mysql
	Driver     string `yaml:"driver"`     // Go driver name: "sqlite" (modernc, pure Go) or "sqlite3" (mattn, CGO)
	Connection string `yaml:"connection"` // Connection string (postgres/mysql)
	Path       string `yaml:"path"`       // File path (sqlite)
}

// SQLiteDriver returns the configured driver name, defaulting to "sqlite" (pure Go, no CGO).
func (dc *DatabaseConfig) SQLiteDriver() string {
	if dc.Driver != "" {
		return dc.Driver
	}
	return "sqlite"
}

type CSVConfig struct {
	InputDir  string `yaml:"inputDir"`
	Delimiter string `yaml:"delimiter"`
	HasHeader bool   `yaml:"hasHeader"`
}

type FHIRConfig struct {
	SQLFile        string `yaml:"sqlFile"`        // Path to multi-statement SQL conversion file
	ProfilesDir    string `yaml:"profilesDir"`    // Directory with FHIR StructureDefinition .json files
	ConceptMapsDir string `yaml:"conceptMapsDir"` // Directory with flat CSV concept map files
}

type OutputConfig struct {
	Dir    string `yaml:"dir"`
	Format string `yaml:"format"` // json, xml, ndjson
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{
		Database: DatabaseConfig{
			Type: "sqlite",
			Path: "data/csv2fhir.db",
		},
		CSV: CSVConfig{
			Delimiter: ",",
			HasHeader: true,
		},
		Output: OutputConfig{
			Format: "json",
			Dir:    "output",
		},
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// GetDSN returns the database connection string
func (dc *DatabaseConfig) GetDSN() string {
	switch dc.Type {
	case "sqlite":
		return "sqlite:///" + dc.Path
	case "postgres":
		return dc.Connection
	case "mysql":
		return dc.Connection
	default:
		return dc.Connection
	}
}
