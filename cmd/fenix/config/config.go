package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Environment string        `yaml:"environment"` // "dev" (colored output) or "prod" (JSON output)
	LogLevel    string        `yaml:"logLevel"`    // trace, debug, info, warn, error — defaults to debug in dev, info in prod
	Staging     StagingConfig `yaml:"staging"`
	FHIR        FHIRConfig    `yaml:"fhir"`
	Output      OutputConfig  `yaml:"output"`
	Sources     SourcesConfig `yaml:"sources"`
}

// SourcesConfig maps source name (e.g. "luscii") to its configuration.
type SourcesConfig map[string]SourceConfig

// SourceConfig configures one external data source.
// type: "api" calls the live REST API; "local" reads files from a local directory;
// "sqlserver" queries an external SQL Server and loads results into staging.
type SourceConfig struct {
	Type             string `yaml:"type"`              // "api" | "local" | "sqlserver"
	BaseURL          string `yaml:"base_url"`          // api: REST base URL
	APIKey           string `yaml:"api_key"`           // api: Bearer token
	Dir              string `yaml:"dir"`               // local: directory containing data files (.json or .csv)
	Delimiter        string `yaml:"delimiter"`         // local/csv: field delimiter, default ","
	ConnectionString string `yaml:"connection_string"` // sqlserver: connection string for SQL Server
	StagingDir       string `yaml:"staging_dir"`       // sqlserver: directory containing staging SQL queries
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
	Format   string            `yaml:"format"` // json, ndjson, pretty
	Type     string            `yaml:"type"`   // "local" (default) or "datalake"
	Local    LocalOutputConfig `yaml:"local"`
	DataLake *DataLakeConfig   `yaml:"datalake,omitempty"`
}

type LocalOutputConfig struct {
	Dir          string `yaml:"dir"`          // Output directory
	UseTimestamp bool   `yaml:"useTimestamp"` // Create timestamped subdirectories (default: true)
	ArchiveCount int    `yaml:"archiveCount"` // Number of previous runs to keep in archive (0 = keep all, default: 5)
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

// loadDotEnv loads the .env file co-located with the config file.
func loadDotEnv(configFilePath string) {
	_ = godotenv.Load(filepath.Join(filepath.Dir(configFilePath), ".env"))
}

// resolveEnvVars replaces ${ENV_VAR_NAME} patterns with environment variable values.
// Supports all string fields in the configuration structure.
func resolveEnvVars(data []byte) ([]byte, error) {

	// Pattern to match ${VAR_NAME}
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

	result := re.ReplaceAllStringFunc(string(data), func(match string) string {
		// Extract variable name from ${VAR_NAME}
		varName := match[2 : len(match)-1]

		// Get the environment variable
		value, exists := os.LookupEnv(varName)
		if !exists {
			// Return the original placeholder if env var not found
			// This allows optional vars and will show the placeholder in error messages
			return match
		}
		return value
	})

	return []byte(result), nil
}

// validateEnvVars checks that all ${ENV_VAR_NAME} placeholders in the config
// have corresponding environment variables set. This helps catch missing secrets early.
func validateEnvVars(data []byte) error {
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	matches := re.FindAllStringSubmatch(string(data), -1)

	var missing []string
	for _, match := range matches {
		varName := match[1]
		if _, exists := os.LookupEnv(varName); !exists {
			missing = append(missing, varName)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}

	return nil
}

// LoadConfig loads configuration from a YAML file, resolving environment variables.
// Syntax: Use ${ENV_VAR_NAME} in the YAML to reference environment variables.
// Environment variables are loaded from a .env file if it exists.
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	loadDotEnv(filePath)

	if err := validateEnvVars(data); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// Resolve ${ENV_VAR_NAME} placeholders with actual values
	resolvedData, err := resolveEnvVars(data)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve environment variables: %w", err)
	}

	config := &Config{
		Staging: StagingConfig{
			Database: "sqlite",
			// Path defaults to "" → in-memory SQLite
		},

		Output: OutputConfig{
			Format: "json",
			Type:   "local",
			Local: LocalOutputConfig{
				Dir:          "output",
				UseTimestamp: true, // Default: create timestamped subdirectories
				ArchiveCount: 5,    // Default: keep last 5 runs in archive
			},
		},
	}

	if err := yaml.Unmarshal(resolvedData, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}
