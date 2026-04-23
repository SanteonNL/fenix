package luscii

import (
	"context"
	"fmt"

	"github.com/SanteonNL/fenix/internal/loader"
	"github.com/SanteonNL/fenix/internal/models/luscii/client"
	"github.com/SanteonNL/fenix/internal/source"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// Source loads patient data from the Luscii Vitals REST API into the staging database.
type Source struct {
	baseURL string
	apiKey  string
	log     zerolog.Logger
}

func New(baseURL, apiKey string, log zerolog.Logger) *Source {
	return &Source{baseURL: baseURL, apiKey: apiKey, log: log}
}

func (s *Source) Load(ctx context.Context, db *sqlx.DB) error {
	c := client.New(s.baseURL, s.apiKey)

	patients, err := c.GetPatients()
	if err != nil {
		return err
	}

	records := make([]interface{}, len(patients))
	for i, p := range patients {
		records[i] = p
	}

	return loader.New(db, s.log).Load("luscii_patients", &loader.Flattener{}, records)
}

// Constructor for registry-based source instantiation.
func constructor(name string, config map[string]interface{}, log zerolog.Logger) (source.Source, error) {
	baseURL, ok := config["base_url"].(string)
	if !ok || baseURL == "" {
		return nil, fmt.Errorf("luscii source %q: missing or invalid 'base_url'", name)
	}

	apiKey, ok := config["api_key"].(string)
	if !ok {
		apiKey = ""
	}

	return New(baseURL, apiKey, log), nil
}

func init() {
	source.Register("api", constructor)
}
