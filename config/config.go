package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
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

// EndpointConfig configures one API endpoint for sources that support a list of endpoints (e.g. luscii).
type EndpointConfig struct {
	Path       string `yaml:"path"`        // URL path, e.g. /v1/export/patients
	Table      string `yaml:"table"`       // target staging table name
	SinceParam string `yaml:"since_param"` // query param for start date (enables incremental loading)
	EndParam   string `yaml:"end_param"`   // query param for end date (appended alongside since_param)
	IDField    string `yaml:"id_field"`    // response field used as PK for upsert deduplication
}

// SourceConfig configures one external data source.
// type: "luscii" calls the Luscii REST API; "local" reads files from a local directory;
// "sqldb" queries an external SQL database and loads results into staging;
// "sftp" downloads CSV/JSON files from a remote SFTP server.
type SourceConfig struct {
	Type             string           `yaml:"type"`              // "luscii" | "local" | "sqldb" | "sftp"
	BaseURL          string           `yaml:"base_url"`          // luscii: REST base URL
	APIKey           string           `yaml:"api_key"`           // luscii: Bearer token
	Dir              string           `yaml:"dir"`               // local: directory containing data files (.json or .csv)
	Delimiter        string           `yaml:"delimiter"`         // local/csv/sftp: field delimiter, default ","
	ConnectionString string           `yaml:"connection_string"` // sqldb: connection string for the external SQL database
	StagingDir       string           `yaml:"staging_dir"`       // sqldb: directory containing staging SQL queries
	Host             string           `yaml:"host"`              // sftp: hostname or IP
	Port             int              `yaml:"port"`              // sftp: port, default 22
	Username         string           `yaml:"username"`          // sftp: login username
	KeyFile          string           `yaml:"key_file"`          // sftp: path to SSH private key file
	RemoteDir        string           `yaml:"remote_dir"`        // sftp: remote directory to download files from
	Endpoints        []EndpointConfig `yaml:"endpoints"`         // luscii: list of API endpoints to fetch
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
	Database   string `yaml:"database"`   // sqlite (default) | postgres | sqlserver
	Driver     string `yaml:"driver"`     // sqlite driver: "sqlite" (modernc, pure Go) or "sqlite3" (mattn, CGO)
	Path       string `yaml:"path"`       // sqlite: file path; omit or "" for in-memory (default)
	Connection string `yaml:"connection"` // postgres/sqlserver: full connection string
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
type DataLakeConfig struct {
	URL            string `yaml:"url"`
	Path           string `yaml:"path"`
	ClientID       string `yaml:"clientId"`
	Certificate    string `yaml:"certificate"`
	CertificateKey string `yaml:"certificateKey"`
}

// LoadConfig loads configuration from a YAML file, resolving environment variables.
// Use ${ENV_VAR_NAME} in the YAML to reference environment variables.
// A .env file co-located with the config file is loaded automatically.
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	_ = godotenv.Load(filepath.Join(filepath.Dir(filePath), ".env"))

	if err := validateEnvVars(data); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	resolvedData, err := resolveEnvVars(data)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve environment variables: %w", err)
	}

	cfg := &Config{
		Staging: StagingConfig{Database: "sqlite"},
		Output: OutputConfig{
			Format: "json",
			Type:   "local",
			Local:  LocalOutputConfig{Dir: "output", UseTimestamp: true, ArchiveCount: 5},
		},
	}

	if err := yaml.Unmarshal(resolvedData, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return cfg, nil
}

// NewStagingDB opens a staging database connection from the given config.
// The caller is responsible for blank-importing the required driver:
//
//	SQLite:     _ "modernc.org/sqlite"
//	PostgreSQL: _ "github.com/lib/pq"
//	SQL Server: _ "github.com/microsoft/go-mssqldb"
func NewStagingDB(cfg *Config) (*sqlx.DB, error) {
	switch cfg.Staging.Database {
	case "", "sqlite":
		dbPath := cfg.Staging.StagingPath()
		if dbPath != ":memory:" {
			if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return nil, fmt.Errorf("create staging dir: %w", err)
				}
			}
		}
		return sqlx.Connect(cfg.Staging.SQLiteDriver(), dbPath)
	case "postgres":
		return sqlx.Connect("postgres", cfg.Staging.Connection)
	case "sqlserver":
		return sqlx.Connect("sqlserver", cfg.Staging.Connection)
	default:
		return nil, fmt.Errorf("unsupported staging database: %s", cfg.Staging.Database)
	}
}

func resolveEnvVars(data []byte) ([]byte, error) {
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	result := re.ReplaceAllStringFunc(string(data), func(match string) string {
		varName := match[2 : len(match)-1]
		if value, exists := os.LookupEnv(varName); exists {
			return value
		}
		return match
	})
	return []byte(result), nil
}

func validateEnvVars(data []byte) error {
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	var missing []string
	for _, match := range re.FindAllStringSubmatch(string(data), -1) {
		if _, exists := os.LookupEnv(match[1]); !exists {
			missing = append(missing, match[1])
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}
