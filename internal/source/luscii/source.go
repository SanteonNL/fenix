package luscii

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SanteonNL/fenix/internal/loader"
	lusciiclient "github.com/SanteonNL/fenix/internal/models/luscii/client"
	lusciimodels "github.com/SanteonNL/fenix/internal/models/luscii"
	"github.com/SanteonNL/fenix/internal/source"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// Source loads Luscii patient data into the staging database.
// Both modes produce identical table structure via the typed Flattener:
//   - local: reads JSON files from dir directly (no HTTP)
//   - api:   calls the live Luscii REST API
type Source struct {
	dir     string // non-empty → local mode
	baseURL string // non-empty → api mode
	apiKey  string
	log     zerolog.Logger
}

func (s *Source) Load(ctx context.Context, db *sqlx.DB) error {
	var patients []lusciimodels.PatientTransformer
	var err error

	if s.dir != "" {
		patients, err = s.readLocal()
	} else {
		patients, err = lusciiclient.New(s.baseURL, s.apiKey).GetPatients()
	}
	if err != nil {
		return err
	}

	records := make([]interface{}, len(patients))
	for i, p := range patients {
		records[i] = p
	}
	return loader.New(db, s.log).Load("luscii_patients", &loader.Flattener{}, records)
}

// readLocal reads patients.json from dir and deserialises into typed structs,
// producing the same table structure as the live API path.
func (s *Source) readLocal() ([]lusciimodels.PatientTransformer, error) {
	path := filepath.Join(s.dir, "patients.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("luscii local: %w", err)
	}
	var wrapper struct {
		Data []lusciimodels.PatientTransformer `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("luscii local: parse patients.json: %w", err)
	}
	return wrapper.Data, nil
}

func constructor(name string, cfg map[string]interface{}, log zerolog.Logger) (source.Source, error) {
	dir, _ := cfg["dir"].(string)
	baseURL, _ := cfg["base_url"].(string)

	if dir == "" && baseURL == "" {
		return nil, fmt.Errorf("luscii source %q: set either 'dir' (local) or 'base_url' (api)", name)
	}

	apiKey, _ := cfg["api_key"].(string)
	return &Source{dir: dir, baseURL: baseURL, apiKey: apiKey, log: log}, nil
}

func init() {
	source.Register("luscii", constructor)
}
