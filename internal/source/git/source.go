// Package git implements the "git" repository type: a (possibly private) git
// repository holding non-tabular inputs such as SQL queries or FHIR
// terminology. It has no staging data and is not a data source: it is
// configured under repositories: and used through config.ResolveDir.
package git

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

// Source clones a repository on first use and fast-forwards it afterwards.
type Source struct {
	name     string
	url      string
	ref      string // branch or tag; empty = remote default branch
	tokenEnv string // env var holding an access token for private repos
	cacheDir string
	log      zerolog.Logger
}

func New(name, url, ref, tokenEnv, cacheDir string, log zerolog.Logger) *Source {
	return &Source{name: name, url: url, ref: ref, tokenEnv: tokenEnv, cacheDir: cacheDir, log: log}
}

// Dir syncs the repository into the cache directory and returns it. When the
// remote cannot be reached but a previous checkout exists, the stale checkout
// is used (with a warning) so an offline run still works.
func (s *Source) Dir(ctx context.Context) (string, error) {
	if _, err := os.Stat(filepath.Join(s.cacheDir, ".git")); err != nil {
		if err := os.MkdirAll(filepath.Dir(s.cacheDir), 0o755); err != nil {
			return "", fmt.Errorf("git source %q: %w", s.name, err)
		}
		args := []string{"clone", "--depth", "1"}
		if s.ref != "" {
			args = append(args, "--branch", s.ref)
		}
		args = append(args, s.url, s.cacheDir)
		if err := s.git(ctx, "", args...); err != nil {
			return "", fmt.Errorf("git source %q: clone: %w", s.name, err)
		}
		s.log.Info().Str("source", s.name).Str("dir", s.cacheDir).Msg("git: cloned")
		return s.cacheDir, nil
	}

	ref := s.ref
	if ref == "" {
		ref = "HEAD"
	}
	if err := s.git(ctx, s.cacheDir, "fetch", "--depth", "1", "origin", ref); err != nil {
		s.log.Warn().Err(err).Str("source", s.name).Msg("git: fetch failed, using existing checkout")
		return s.cacheDir, nil
	}
	if err := s.git(ctx, s.cacheDir, "reset", "--hard", "FETCH_HEAD"); err != nil {
		return "", fmt.Errorf("git source %q: reset: %w", s.name, err)
	}
	s.log.Info().Str("source", s.name).Str("dir", s.cacheDir).Msg("git: updated")
	return s.cacheDir, nil
}

// git runs one git command. The token is passed through GIT_CONFIG_* env vars
// rather than argv so it never shows up in a process listing or in error text.
func (s *Source) git(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if s.tokenEnv != "" {
		token := os.Getenv(s.tokenEnv)
		if token == "" {
			return fmt.Errorf("env var %s (token_env) is not set", s.tokenEnv)
		}
		basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		cmd.Env = append(cmd.Env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic "+basic,
		)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
