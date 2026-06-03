package luscii

import (
	"context"
	"fmt"
	"time"

	lusciiclient "github.com/SanteonNL/fenix/models/luscii/client"
	"github.com/SanteonNL/fenix/source"
	"github.com/SanteonNL/fenix/source/local"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// EndpointConfig configures one Luscii API endpoint.
// IDField is retained for backward compatibility with callers that use New() directly
// (e.g. HipsETL). For new YAML-based configs, put id_field under json_options instead.
type EndpointConfig struct {
	Path       string // URL path, e.g. /v1/patients
	Table      string // target staging table name
	SinceParam string // query param for incremental start date
	EndParam   string // query param for end date
	IDField    string // deprecated: use json_options.<table>.id_field instead
}

type endpointConfig struct {
	path       string
	table      string
	sinceParam string
	endParam   string
	idField    string // fallback when jsonOptions has no entry for this table
}

// Source loads Luscii API data into the staging database via the shared Loader.
// JSON flattening and child-table splitting are configured via json_options,
// keyed by table name (e.g. "luscii_patients").
type Source struct {
	name          string
	baseURL       string
	apiKey        string
	endpoints     []endpointConfig
	watermarkPath string
	watermark     map[string]string
	jsonOptions   map[string]local.JSONFileConfig
	fileWriter    source.FileWriter
	log           zerolog.Logger
}

// SetFileWriter configures file-based staging output alongside the database.
func (s *Source) SetFileWriter(w source.FileWriter) {
	s.fileWriter = w
}

// New creates a Luscii source for direct (non-YAML) instantiation.
// IDField in endpoints is accepted for backward compatibility and auto-migrated
// into jsonOptions so the shared Loader can use it for incremental upsert.
func New(name, baseURL, apiKey, watermarkPath string, endpoints []EndpointConfig, log zerolog.Logger) source.Source {
	eps := make([]endpointConfig, len(endpoints))
	jsonOpts := make(map[string]local.JSONFileConfig, len(endpoints))
	for i, e := range endpoints {
		eps[i] = endpointConfig{path: e.Path, table: e.Table, sinceParam: e.SinceParam, endParam: e.EndParam, idField: e.IDField}
		if e.IDField != "" {
			existing := jsonOpts[e.Table]
			if existing.IDField == "" {
				existing.IDField = e.IDField
				jsonOpts[e.Table] = existing
			}
		}
	}
	return &Source{
		name:          name,
		baseURL:       baseURL,
		apiKey:        apiKey,
		endpoints:     eps,
		watermarkPath: watermarkPath,
		watermark:     source.ReadWatermark(watermarkPath, log),
		jsonOptions:   jsonOpts,
		log:           log,
	}
}

func (s *Source) Load(ctx context.Context, db *sqlx.DB) error {
	cli := lusciiclient.New(s.baseURL, s.apiKey)
	loader := local.NewLoader(s.name, s.fileWriter, s.log)

	for _, ep := range s.endpoints {
		cfg := s.jsonOptions[ep.table]
		// Backward compat: if no IDField in jsonOptions, fall back to endpoint idField.
		if cfg.IDField == "" && ep.idField != "" {
			cfg.IDField = ep.idField
		}

		since := ""
		if ep.sinceParam != "" {
			since = s.watermark[ep.table]
		}

		params := lusciiclient.FetchParams{
			SinceParam: ep.sinceParam,
			EndParam:   ep.endParam,
			Since:      since,
		}

		incremental := since != "" && cfg.IDField != ""
		s.log.Info().Str("source", s.name).Str("type", "luscii").Str("table", ep.table).
			Str("since", since).Str("id_field", cfg.IDField).Bool("incremental", incremental).
			Msg("source: loading")

		records, err := cli.FetchAll(ep.path, params)
		if err != nil {
			s.log.Error().Err(err).Str("path", ep.path).Msg("luscii: fetch failed")
			continue
		}
		if len(records) == 0 {
			s.log.Info().Str("table", ep.table).Msg("luscii: no records returned")
			continue
		}

		if err := loader.Load(db, ep.table, records, cfg, incremental); err != nil {
			s.log.Error().Err(err).Str("table", ep.table).Msg("luscii: load failed")
		}
	}

	if s.watermarkPath != "" {
		now := time.Now().UTC().Format(time.RFC3339)
		updated := make(map[string]string, len(s.watermark))
		for k, v := range s.watermark {
			updated[k] = v
		}
		for _, ep := range s.endpoints {
			if ep.sinceParam != "" {
				updated[ep.table] = now
			}
		}
		s.log.Info().Str("path", s.watermarkPath).Msg("luscii: saving watermark")
		if err := source.WriteWatermark(s.watermarkPath, updated); err != nil {
			s.log.Error().Err(err).Str("path", s.watermarkPath).Msg("luscii: failed to save watermark")
		}
	} else {
		s.log.Warn().Msg("luscii: no watermark_path configured — incremental loading disabled")
	}

	return nil
}

func parseEndpoints(cfg map[string]interface{}) []endpointConfig {
	raw, _ := cfg["endpoints"].([]interface{})
	result := make([]endpointConfig, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		result = append(result, endpointConfig{
			path:       strVal(m, "path"),
			table:      strVal(m, "table"),
			sinceParam: strVal(m, "since_param"),
			endParam:   strVal(m, "end_param"),
			idField:    strVal(m, "id_field"), // accepted but deprecated; prefer json_options
		})
	}
	return result
}

func strVal(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func constructor(name string, cfg map[string]interface{}, log zerolog.Logger) (source.Source, error) {
	baseURL, _ := cfg["base_url"].(string)
	apiKey, _ := cfg["api_key"].(string)
	if baseURL == "" {
		return nil, fmt.Errorf("luscii source %q: missing 'base_url'", name)
	}
	if apiKey == "" {
		return nil, fmt.Errorf("luscii source %q: missing 'api_key'", name)
	}
	endpoints := parseEndpoints(cfg)
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("luscii source %q: no endpoints configured", name)
	}
	watermarkPath, _ := cfg["watermark_path"].(string)
	jsonOptions := local.ParseJSONOptions(cfg)
	for table, jcfg := range jsonOptions {
		log.Info().Str("source", name).Str("table", table).Str("id_field", jcfg.IDField).Int("children", len(jcfg.Children)).Msg("luscii: json_options parsed")
	}
	if len(jsonOptions) == 0 {
		log.Warn().Str("source", name).Msg("luscii: no json_options found — incremental loading requires id_field in json_options")
	}
	return &Source{
		name:          name,
		baseURL:       baseURL,
		apiKey:        apiKey,
		endpoints:     endpoints,
		watermarkPath: watermarkPath,
		watermark:     source.ReadWatermark(watermarkPath, log),
		jsonOptions:   jsonOptions,
		log:           log,
	}, nil
}

func init() {
	source.Register("luscii", constructor)
}
