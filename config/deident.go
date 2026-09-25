package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/SanteonNL/fenix/internal/deident"
)

// LoadDeidentRuleset reads and validates a resolved DeidentificationRuleset
// JSON file (the output of deident.Resolve, or the embedded santeon-default
// used as-is) from path.
func LoadDeidentRuleset(path string) (deident.Ruleset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return deident.Ruleset{}, fmt.Errorf("read deident ruleset file %s: %w", path, err)
	}
	var rs deident.Ruleset
	if err := json.Unmarshal(data, &rs); err != nil {
		return deident.Ruleset{}, fmt.Errorf("parse deident ruleset file %s: %w", path, err)
	}
	if err := rs.Validate(); err != nil {
		return deident.Ruleset{}, fmt.Errorf("deident ruleset file %s is invalid: %w", path, err)
	}
	return rs, nil
}

// DeidentKey reads and base64-decodes the HMAC key from the env var named by
// c.Deident.EffectiveKeyEnv(). The key must never appear in YAML, be logged,
// or be written to any generated artefact — see docs/fenix_architecture.md,
// "Pseudonymisation and the per-export seed".
func (c *Config) DeidentKey() ([]byte, error) {
	envVar := c.Deident.EffectiveKeyEnv()
	raw := os.Getenv(envVar)
	if raw == "" {
		return nil, fmt.Errorf("deident: %s is not set", envVar)
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("deident: %s is not valid base64: %w", envVar, err)
	}
	return key, nil
}
