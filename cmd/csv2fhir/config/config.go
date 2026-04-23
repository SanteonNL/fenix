package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Environment string         `yaml:"environment"` // "dev" (colored output) or "prod" (JSON output)
	LogLevel    string         `yaml:"logLevel"`    // trace, debug, info, warn, error — defaults to debug in dev, info in prod
	Staging     StagingConfig  `yaml:"staging"`
	FHIR        FHIRConfig     `yaml:"fhir"`
	Output      OutputConfig   `yaml:"output"`
	Sources     SourcesConfig  `yaml:"sources"`
}

// SourcesConfig maps source name (e.g. "luscii") to its configuration.
type SourcesConfig map[string]SourceConfig

// SourceConfig configures one external data source.
// type: "api" calls the live REST API; "local" reads files from a local directory.
type SourceConfig struct {
	Type      string `yaml:"type"`      // "api" | "local"
	BaseURL   string `yaml:"base_url"`  // api: REST base URL
	APIKey    string `yaml:"api_key"`   // api: Bearer token
	Dir       string `yaml:"dir"`       // local: directory containing data files (.json or .csv)
	Delimiter string `yaml:"delimiter"` // local/csv: field delimiter, default ","
}

// EffectiveLogLevel returns the log level to use, applying the smart default:
// dev → debug, prod → info (unless explicitly overridden).
func (c *Config) EffectiveLogLevel() string {
	if c.LogLevel != "" {
		return c.LogLevel
	}
	if c.Environment == "dev" {
		return "debug"
	}
	return "info"
}

type StagingConfig struct {
	Database   string `yaml:"database"`   // sqlite (default) | postgres
	Driver     string `yaml:"driver"`     // sqlite driver: "sqlite" (modernc, pure Go) or "sqlite3" (mattn, CGO)
	Path       string `yaml:"path"`       // sqlite: file path; omit or "" for in-memory (default)
	Connection string `yaml:"connection"` // postgres: full connection string
}

// StagingPath returns the SQLite path to use.
// Empty string means in-memory (":memory:"), which is the default.
func (sc *StagingConfig) StagingPath() string {
	if sc.Path == "" {
		return ":memory:"
	}
	return sc.Path
}

// SQLiteDriver returns the configured driver name, defaulting to "sqlite" (pure Go, no CGO).
func (sc *StagingConfig) SQLiteDriver() string {
	if sc.Driver != "" {
		return sc.Driver
	}
	return "sqlite"
}


type FHIRConfig struct {
	SQLFile        string `yaml:"sqlFile"`        // Path to multi-statement SQL conversion file
	Profile        string `yaml:"profile"`        // FHIR profile name or repo, e.g. "sim-on-fhir"
	ProfilesDir    string `yaml:"profilesDir"`    // Directory with FHIR StructureDefinition .json files
	ConceptMapsDir string `yaml:"conceptMapsDir"` // Directory with flat CSV concept map files
}

// OutputConfig selects the output destination via Type ("local" or "datalake").
type OutputConfig struct {
	Format   string            `yaml:"format"`             // json, ndjson, pretty
	Type     string            `yaml:"type"`               // "local" (default) or "datalake"
	Local    LocalOutputConfig `yaml:"local"`
	DataLake *DataLakeConfig   `yaml:"datalake,omitempty"`
}

type LocalOutputConfig struct {
	Dir string `yaml:"dir"`
}

// DataLakeConfig configures a data gateway endpoint (e.g. dls-t.hips.santeon.nl).
// Authentication uses a client certificate (mTLS or certificate-based OAuth2).
type DataLakeConfig struct {
	URL            string `yaml:"url"`            // data gateway base URL, e.g. dls-t.hips.santeon.nl
	Path           string `yaml:"path"`           // target path inside the storage account
	ClientID       string `yaml:"clientId"`       // client identifier for the endpoint
	Certificate    string `yaml:"certificate"`    // path to certificate file (.pem or combined PEM)
	CertificateKey string `yaml:"certificateKey"` // path to private key file (.pem), if separate
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{
		Staging: StagingConfig{
			Database: "sqlite",
			// Path defaults to "" → in-memory SQLite
		},

		Output: OutputConfig{
			Format: "json",
			Type:   "local",
			Local:  LocalOutputConfig{Dir: "output"},
		},
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

