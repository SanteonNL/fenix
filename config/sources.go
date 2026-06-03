package config

import (
	"context"
	"path/filepath"

	"github.com/SanteonNL/fenix/source"
	"github.com/SanteonNL/fenix/source/stagingfiles"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// BuildSource constructs the Source for the given config entry using the source registry.
// The caller must blank-import the source type packages they need so their init() runs:
//
//	_ "github.com/SanteonNL/fenix/source/luscii"
//	_ "github.com/SanteonNL/fenix/source/sqldb"
//	_ "github.com/SanteonNL/fenix/source/local"
//	_ "github.com/SanteonNL/fenix/source/sftp"
func BuildSource(name string, sc SourceConfig, repoRoot string, watermarkPath string, log zerolog.Logger) (source.Source, error) {
	stagingDir := sc.StagingDir
	if stagingDir == "" {
		stagingDir = filepath.Join("queries", name, "staging")
	}

	endpointsRaw := make([]interface{}, len(sc.Endpoints))
	for i, ep := range sc.Endpoints {
		endpointsRaw[i] = map[string]interface{}{
			"path":        ep.Path,
			"table":       ep.Table,
			"since_param": ep.SinceParam,
			"end_param":   ep.EndParam,
			"id_field":    ep.IDField,
		}
	}

	configMap := map[string]interface{}{
		"type":              sc.Type,
		"base_url":          sc.BaseURL,
		"api_key":           sc.APIKey,
		"dir":               resolvePath(repoRoot, sc.Dir),
		"delimiter":         sc.Delimiter,
		"connection_string": sc.ConnectionString,
		"staging_dir":       resolvePath(repoRoot, stagingDir),
		"watermark_path":    watermarkPath,
		"host":              sc.Host,
		"port":              sc.Port,
		"username":          sc.Username,
		"key_file":          resolvePath(repoRoot, sc.KeyFile),
		"remote_dir":        sc.RemoteDir,
		"endpoints":         endpointsRaw,
		"json_options":      sc.JSONOptions,
	}

	return source.Build(sc.Type, name, configMap, log)
}

// NewFileWriter creates a staging file writer from cfg, or returns nil when
// staging.files is not configured. The caller should log any returned error
// and continue — file output is best-effort alongside the staging database.
func NewFileWriter(cfg *Config, repoRoot string) (source.FileWriter, error) {
	if cfg.Staging.Files == nil || cfg.Staging.Files.Dir == "" {
		return nil, nil
	}
	dir := resolvePath(repoRoot, cfg.Staging.Files.Dir)
	rawDir := ""
	if cfg.Staging.Files.RawDir != "" {
		rawDir = resolvePath(repoRoot, cfg.Staging.Files.RawDir)
	}
	return stagingfiles.New(dir, rawDir)
}

// LoadAllSources builds and loads every source defined in cfg into db.
// If staging.files is configured, sources that implement source.FileWritable
// also write to files (flat CSV + hierarchical JSON).
// Sources that fail to build or load are logged and skipped; the function
// always returns nil so the caller can continue with whatever data did load.
func LoadAllSources(ctx context.Context, db *sqlx.DB, cfg *Config, repoRoot string, watermarkPath string, log zerolog.Logger) error {
	fw, err := NewFileWriter(cfg, repoRoot)
	if err != nil {
		log.Error().Err(err).Msg("staging files: failed to create file writer — file output disabled")
	}

	for name, sc := range cfg.Sources {
		src, err := BuildSource(name, sc, repoRoot, watermarkPath, log)
		if err != nil {
			log.Error().Err(err).Str("source", name).Msg("failed to build source")
			continue
		}
		if fw != nil {
			if fa, ok := src.(source.FileWritable); ok {
				fa.SetFileWriter(fw)
			}
		}
		if err := src.Load(ctx, db); err != nil {
			log.Error().Err(err).Str("source", name).Msg("failed to load source")
		}
	}
	return nil
}

// WatermarkPath returns the path for the incremental watermark file.
// Priority: files.dir (if configured) > SQLite DB dir > output dir.
// Returns "" only for pure in-memory SQLite with no files configured.
func WatermarkPath(cfg *Config, repoRoot string) string {
	// File-based staging always enables watermarks, even with in-memory DB.
	if cfg.Staging.Files != nil && cfg.Staging.Files.Dir != "" {
		return filepath.Join(resolvePath(repoRoot, cfg.Staging.Files.Dir), ".watermark.json")
	}
	switch cfg.Staging.Database {
	case "", "sqlite":
		dbPath := cfg.Staging.StagingPath()
		if dbPath == ":memory:" {
			return "" // no persistence — watermarks disabled
		}
		return filepath.Join(filepath.Dir(resolvePath(repoRoot, dbPath)), ".watermark.json")
	default:
		return filepath.Join(resolvePath(repoRoot, cfg.Output.Local.Dir), ".watermark.json")
	}
}

// resolvePath resolves a relative path against repoRoot; absolute paths pass through.
func resolvePath(repoRoot, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repoRoot, path)
}
