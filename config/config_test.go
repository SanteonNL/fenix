// Tests for strict config decoding: valid config files load, unknown fields are rejected.
package config

import (
	"os"
	"strings"
	"testing"
)

func TestDecodeStrictAcceptsExampleConfig(t *testing.T) {
	content, err := os.ReadFile("config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var configuration Config
	if err := decodeStrict(content, &configuration); err != nil {
		t.Fatalf("config.example.yaml should decode without errors: %v", err)
	}
	if configuration.Sources["hix-test"].Type != "sqldb" {
		t.Errorf("hix-test type = %q, want sqldb", configuration.Sources["hix-test"].Type)
	}
}

func TestDecodeStrictRejectsUnknownField(t *testing.T) {
	content := []byte("sources:\n  pja:\n    type: local\n    dirr: data/pja\n")
	var configuration Config
	err := decodeStrict(content, &configuration)
	if err == nil || !strings.Contains(err.Error(), "dirr") {
		t.Fatalf("want error naming the unknown field \"dirr\", got %v", err)
	}
}

func TestDecodeStrictAcceptsEmptyFile(t *testing.T) {
	var configuration Config
	if err := decodeStrict(nil, &configuration); err != nil {
		t.Fatalf("empty config should decode without errors: %v", err)
	}
}
