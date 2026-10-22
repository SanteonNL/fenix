package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func commit(t *testing.T, repo, file, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "add", ".")
	run(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "c")
}

func TestDirClonesThenUpdates(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	origin := t.TempDir()
	run(t, origin, "init")
	commit(t, origin, "a.sql", "v1")

	cache := filepath.Join(t.TempDir(), "cache")
	s := New("q", origin, "", "", cache, zerolog.Nop())

	dir, err := s.Dir(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.sql")); string(b) != "v1" {
		t.Fatalf("after clone got %q", b)
	}

	commit(t, origin, "a.sql", "v2")
	if _, err := s.Dir(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.sql")); string(b) != "v2" {
		t.Fatalf("after update got %q", b)
	}
}

func TestDirFallsBackToStaleCheckoutWhenOffline(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	origin := t.TempDir()
	run(t, origin, "init")
	commit(t, origin, "a.sql", "v1")

	cache := filepath.Join(t.TempDir(), "cache")
	s := New("q", origin, "", "", cache, zerolog.Nop())
	if _, err := s.Dir(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(origin); err != nil {
		t.Fatal(err)
	}
	dir, err := s.Dir(context.Background())
	if err != nil {
		t.Fatalf("expected stale fallback, got %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.sql")); string(b) != "v1" {
		t.Fatalf("got %q", b)
	}
}

func TestMissingTokenEnvIsAnError(t *testing.T) {
	s := New("q", "https://example.invalid/x.git", "", "FENIX_TEST_UNSET_TOKEN", filepath.Join(t.TempDir(), "c"), zerolog.Nop())
	if _, err := s.Dir(context.Background()); err == nil {
		t.Fatal("expected error for unset token env")
	}
}
