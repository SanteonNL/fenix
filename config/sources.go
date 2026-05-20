package config

import (
	"context"
	"path/filepath"

	"github.com/SanteonNL/fenix/source"
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
	}

	return source.Build(sc.Type, name, configMap, log)
}

// LoadAllSources builds and loads every source defined in cfg into db.
// Sources that fail to build or load are logged and skipped; the function
// always returns nil so the caller can continue with whatever data did load.
func LoadAllSources(ctx context.Context, db *sqlx.DB, cfg *Config, repoRoot string, watermarkPath string, log zerolog.Logger) error {
	for name, sc := range cfg.Sources {
		src, err := BuildSource(name, sc, repoRoot, watermarkPath, log)
		if err != nil {
			log.Error().Err(err).Str("source", name).Msg("failed to build source")
			continue
		}
		if err := src.Load(ctx, db); err != nil {
			log.Error().Err(err).Str("source", name).Msg("failed to load source")
		}
	}
	return nil
}

// WatermarkPath returns the path for the incremental watermark file.
// For SQLite it sits next to the DB file; for other databases it sits in the
// output directory. Returns "" for in-memory SQLite (watermarks disabled).
func WatermarkPath(cfg *Config, repoRoot string) string {
	switch cfg.Staging.Database {
	case "", "sqlite":
		dbPath := cfg.Staging.StagingPath()
		if dbPath == ":memory:" {
			return ""
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
