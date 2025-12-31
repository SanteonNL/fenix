// internal/config/config.go
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	UseSQLMockServer bool         `yaml:"use_sql_mock_server"`
	Datasources      []Datasource `yaml:"datasources"`
	Paths            Paths        `yaml:"paths,omitempty"`
	RootDir          string       `yaml:"-"` // Not loaded from YAML, computed at runtime
}

type Datasource struct {
	Name             string            `yaml:"name"`
	Type             string            `yaml:"type"` // sql, nosql, api, flat_file, cloud_storage, message_queue, cache, other

	// SQL datasource fields
	Driver           string `yaml:"driver,omitempty"`           // sqlserver, postgres, mysql, etc.
	ConnectionString string `yaml:"connection_string,omitempty"`

	// NoSQL datasource fields
	Engine string `yaml:"engine,omitempty"` // mongodb, redis, cassandra, etc.
	URI    string `yaml:"uri,omitempty"`

	// API datasource fields
	BaseURL string            `yaml:"base_url,omitempty"`
	APIKey  string            `yaml:"api_key,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`

	// Flat file datasource fields
	Format string `yaml:"format,omitempty"` // json, csv, xml, etc.
	Path   string `yaml:"path,omitempty"`

	// Cloud storage fields
	Provider    string            `yaml:"provider,omitempty"`    // aws, azure, gcp
	Bucket      string            `yaml:"bucket,omitempty"`      // S3 bucket or Azure container
	Container   string            `yaml:"container,omitempty"`   // Azure container
	Region      string            `yaml:"region,omitempty"`      // AWS region
	AccountName string            `yaml:"account_name,omitempty"` // Azure account name
	AccountKey  string            `yaml:"account_key,omitempty"`  // Azure account key
	Credentials map[string]string `yaml:"credentials,omitempty"` // AWS credentials

	// Message queue fields
	QueueURL string `yaml:"queue_url,omitempty"`
	Topic    string `yaml:"topic,omitempty"`

	// Cache fields
	Host     string `yaml:"host,omitempty"`
	Port     int    `yaml:"port,omitempty"`
	Password string `yaml:"password,omitempty"`
	DB       int    `yaml:"db,omitempty"`
}

type Paths struct {
	BaseDir        string `yaml:"base_dir"`
	Input          string `yaml:"input"`
	Output         string `yaml:"output"`
	Logs           string `yaml:"logs"`
	ValueSetLocal  string `yaml:"valueset_local"`
	ValueSetCustom string `yaml:"valueset_custom"`
	ConceptMaps    string `yaml:"conceptmaps"`
}

func DefaultPaths() Paths {
	return Paths{
		BaseDir:        ".",
		Input:          "input",
		Output:         "output",
		Logs:           "logs",
		ValueSetLocal:  "terminology/valuesets/local",
		ValueSetCustom: "terminology/valuesets/custom",
		ConceptMaps:    "terminology/conceptmaps",
	}
}

// FindRepoRoot finds the repository root by looking for go.mod or .git
func FindRepoRoot() (string, error) {
	// First try git
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(output)), nil
	}

	// Fallback: walk up looking for go.mod
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find repository root (no go.mod or .git found)")
		}
		dir = parent
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(configPath string) (*Config, error) {
	// Find repository root
	rootDir, err := FindRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to find repository root: %w", err)
	}

	// Make configPath relative to root if it's not absolute
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(rootDir, configPath)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set repository root
	config.RootDir = rootDir

	// Set default paths if not specified
	if config.Paths.BaseDir == "" {
		config.Paths = DefaultPaths()
	}

	return &config, nil
}

// GetAbsPath converts a relative path (from config) to an absolute path from repository root
func (c *Config) GetAbsPath(relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	return filepath.Join(c.RootDir, relativePath)
}
