package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
)

// RunTransforms executes the cleaned/DWH-layer SQL files against the staging DB, after loading
// and before FHIR conversion. There is no per-file config: transforms are discovered purely by
// directory convention, mirroring source queries.
//
//   - queries/<source>/dwh/*.sql   — per-source cleanup, run for every configured source.
//   - queries/dwh/cross/*.sql      — cross-source cleanup that may join several sources' staging
//     tables. Each file must declare the sources it needs via a "-- :requires <a>,<b>" annotation
//     (mirrors the "-- :pk <col>" convention used by sqldb staging queries). A required source
//     that is not configured is fatal — silently running with missing data would be worse.
//
// Files within a directory run in lexicographic order, so prefix with "01_", "02_", etc. when
// one transform depends on another.
func RunTransforms(db *sqlx.DB, cfg *Config, repoRoot string, log *zerolog.Logger) {
	for name := range cfg.Sources {
		dir := resolveTransformPath(repoRoot, filepath.Join("queries", name, "dwh"))
		runTransformDir(db, dir, nil, cfg, log)
	}
	runTransformDir(db, resolveTransformPath(repoRoot, filepath.Join("queries", "dwh", "cross")), parseRequires, cfg, log)
}

// runTransformDir executes every *.sql file in dir, in lexicographic order. If checkRequires is
// non-nil, it is used to extract a file's required source names, which are validated against
// cfg.Sources before the file runs (fatal on a missing source).
func runTransformDir(db *sqlx.DB, dir string, checkRequires func(query string) []string, cfg *Config, log *zerolog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // no transform layer here — most sources won't have one
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		relPath := filepath.Join(dir, e.Name())
		sqlBytes, err := os.ReadFile(relPath)
		if err != nil {
			log.Error().Err(err).Str("file", relPath).Msg("Failed to read transform file")
			continue
		}
		query := string(sqlBytes)
		if checkRequires != nil {
			for _, req := range checkRequires(query) {
				if _, ok := cfg.Sources[req]; !ok {
					log.Fatal().Str("file", relPath).Str("missing_source", req).
						Msg("transform requires a source that is not configured")
				}
			}
		}
		log.Info().Str("file", relPath).Msg("Running transform")
		for _, stmt := range splitStatements(query) {
			if _, err := db.Exec(stmt); err != nil {
				log.Error().Err(err).Str("file", relPath).Msg("Transform statement failed")
			}
		}
	}
}

// parseRequires extracts source names from a "-- :requires <a>,<b>" annotation.
func parseRequires(query string) []string {
	for _, line := range strings.Split(query, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-- :requires ") {
			raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "-- :requires "))
			var sources []string
			for _, s := range strings.Split(raw, ",") {
				if s = strings.TrimSpace(s); s != "" {
					sources = append(sources, s)
				}
			}
			return sources
		}
	}
	return nil
}

// resolveTransformPath resolves a relative path from repo root.
func resolveTransformPath(repoRoot, relativePath string) string {
	if filepath.IsAbs(relativePath) {
		return relativePath
	}
	return filepath.Join(repoRoot, relativePath)
}

// splitStatements splits a SQL string on ";" into individual non-empty statements.
// Lines that consist only of comments are skipped. Kept local to the config package to
// avoid importing the cmd/fenix converter (which would invert the dependency direction).
func splitStatements(sql string) []string {
	var statements []string
	for _, raw := range strings.Split(sql, ";") {
		var lines []string
		for _, line := range strings.Split(raw, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
				lines = append(lines, line)
			}
		}
		stmt := strings.TrimSpace(strings.Join(lines, "\n"))
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}
	return statements
}
