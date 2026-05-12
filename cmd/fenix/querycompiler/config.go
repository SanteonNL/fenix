package querycompiler

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// QueryConfig is one named SQL query for a resource type.
// A resource can have multiple named queries (e.g. "vitals" and "labs" for Observation).
type QueryConfig struct {
	Name     string   `yaml:"name"`
	SQL      string   `yaml:"sql"`
	Pushdown []string `yaml:"pushdown"`
}

// ResourceConfig defines the set of queries for one resource type.
type ResourceConfig struct {
	Queries []QueryConfig `yaml:"queries"`
}

// ReplaceRule is a literal string substitution applied to the raw SQL before template rendering.
// Use this to inject fragments (e.g. a JOIN) without replacing the entire SQL file.
type ReplaceRule struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// GroupQueryOverride targets one named query within a group.
//   - SQL replaces the entire SQL file.
//   - Replace applies literal string substitutions to the raw SQL (applied before template rendering).
// SQL and Replace are mutually exclusive: if SQL is set, Replace is ignored.
type GroupQueryOverride struct {
	Name    string        `yaml:"name"`
	SQL     string        `yaml:"sql,omitempty"`
	Replace []ReplaceRule `yaml:"replace,omitempty"`
}

// GroupResourceConfig defines what a group overrides for one resource type.
//   - Where is an AND fragment injected into every query for that resource.
//   - Queries lists per-named-query SQL replacements; unmentioned queries keep their source SQL.
type GroupResourceConfig struct {
	Where   string               `yaml:"where,omitempty"`
	Queries []GroupQueryOverride `yaml:"queries,omitempty"`
}

// GroupConfig holds all resource overrides for one export group within a source.
type GroupConfig struct {
	Group     string                         `yaml:"group"`
	Resources map[string]GroupResourceConfig `yaml:"resources"`
}

// SourceConfig holds source-specific resource queries, substitutions, and group overrides.
type SourceConfig struct {
	Source        string                    `yaml:"source"`
	Substitutions map[string]string         `yaml:"substitutions"`
	Resources     map[string]ResourceConfig `yaml:"resources"`
	Groups        map[string]GroupConfig    // loaded from groups/ subdirectory
}

// loadGlobalSubs reads substitutions.yaml from configDir.
// Returns an empty map (not an error) when the file is absent.
func loadGlobalSubs(dir string) (map[string]string, error) {
	subs := make(map[string]string)
	data, err := os.ReadFile(filepath.Join(dir, "substitutions.yaml"))
	if os.IsNotExist(err) {
		return subs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading substitutions.yaml: %w", err)
	}
	if err := yaml.Unmarshal(data, &subs); err != nil {
		return nil, fmt.Errorf("parsing substitutions.yaml: %w", err)
	}
	return subs, nil
}

// loadSources reads every subdirectory of configDir/sources/ as a SourceConfig.
// Each source directory must contain source.yaml and may contain a groups/ subdirectory.
func loadSources(configDir string) (map[string]SourceConfig, error) {
	sources := make(map[string]SourceConfig)
	sourcesDir := filepath.Join(configDir, "sources")

	entries, err := os.ReadDir(sourcesDir)
	if os.IsNotExist(err) {
		return sources, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading sources dir: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sc, err := loadSource(filepath.Join(sourcesDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("loading source %s: %w", entry.Name(), err)
		}
		sources[sc.Source] = sc
	}
	return sources, nil
}

func loadSource(dir string) (SourceConfig, error) {
	var sc SourceConfig
	// File name matches the directory name: sources/hix/hix.yaml
	sourceName := filepath.Base(dir)
	data, err := os.ReadFile(filepath.Join(dir, sourceName+".yaml"))
	if err != nil {
		return sc, fmt.Errorf("reading %s.yaml: %w", sourceName, err)
	}
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return sc, fmt.Errorf("parsing source.yaml: %w", err)
	}

	sc.Groups, err = loadGroups(dir)
	if err != nil {
		return sc, err
	}
	return sc, nil
}

func loadGroups(dir string) (map[string]GroupConfig, error) {
	groups := make(map[string]GroupConfig)
	groupsDir := filepath.Join(dir, "groups")

	entries, err := os.ReadDir(groupsDir)
	if os.IsNotExist(err) {
		return groups, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading groups dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(groupsDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var gc GroupConfig
		if err := yaml.Unmarshal(data, &gc); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		groups[gc.Group] = gc
	}
	return groups, nil
}
