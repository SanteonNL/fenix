package config

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestResolveDir(t *testing.T) {
	root := t.TempDir()
	cfg := &Config{Sources: SourcesConfig{
		"terminology": {Type: "local", Dir: "term"},
		"luscii":      {Type: "luscii"},
	}}
	log := zerolog.Nop()
	ctx := context.Background()

	cases := []struct {
		name string
		ref  DirRef
		want string
	}{
		{"nothing configured", DirRef{}, ""},
		{"builtin repo", DirRef{Source: "repo", Path: "queries"}, filepath.Join(root, "queries")},
		{"builtin repo root", DirRef{Source: "repo"}, root},
		{"declared local source", DirRef{Source: "terminology", Path: "conceptmaps"}, filepath.Join(root, "term", "conceptmaps")},
	}
	for _, c := range cases {
		got, err := ResolveDir(ctx, cfg, c.ref, root, log)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v want %q", c.name, got, err, c.want)
		}
	}

	for _, ref := range []DirRef{{Source: "nope"}, {Source: "luscii"}} {
		if _, err := ResolveDir(ctx, cfg, ref, root, log); err == nil {
			t.Errorf("%+v: expected error", ref)
		}
	}
}

func TestLoadsData(t *testing.T) {
	no, yes := false, true
	for _, c := range []struct {
		sc   SourceConfig
		want bool
	}{
		{SourceConfig{Type: "local"}, true},
		{SourceConfig{Type: "sqldb"}, true},
		{SourceConfig{Type: "git"}, false},
		{SourceConfig{Type: "local", Load: &no}, false},
		{SourceConfig{Type: "git", Load: &yes}, true},
	} {
		if got := c.sc.LoadsData(); got != c.want {
			t.Errorf("%+v: got %v want %v", c.sc, got, c.want)
		}
	}
}

func TestQueriesRootFallsBackToRepoRoot(t *testing.T) {
	root := t.TempDir()
	cfg := &Config{}
	if got := cfg.QueriesRoot(context.Background(), root, zerolog.Nop()); got != root {
		t.Fatalf("unset: %q", got)
	}
	cfg.Inputs.Queries = DirRef{Source: "missing"}
	if got := cfg.QueriesRoot(context.Background(), root, zerolog.Nop()); got != root {
		t.Fatalf("broken source should fall back, got %q", got)
	}
}

func TestResolveDirGitSource(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	origin := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "--allow-empty", "-m", "c"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = origin
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	root := t.TempDir()
	cfg := &Config{Sources: SourcesConfig{"q": {Type: "git", URL: origin}}}
	got, err := ResolveDir(context.Background(), cfg, DirRef{Source: "q", Path: "queries"}, root, zerolog.Nop())
	want := filepath.Join(root, ".cache", "sources", "q", "queries")
	if err != nil || got != want {
		t.Fatalf("got %q, %v want %q", got, err, want)
	}
	if cfg.Sources["q"].LoadsData() {
		t.Fatal("git source must not load into staging")
	}
}
