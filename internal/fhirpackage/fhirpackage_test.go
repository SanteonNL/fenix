package fhirpackage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// makeTgz builds a package .tgz from name -> content.
func makeTgz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func igPackage(t *testing.T) []byte {
	return makeTgz(t, map[string]string{
		"package/package.json":               `{"name":"santeon.test","version":"0.1.0","dependencies":{"hl7.fhir.r4.core":"4.0.1","dep.one":"1.0.0"}}`,
		"package/ValueSet-a.json":            `{"resourceType":"ValueSet","id":"a","url":"https://ex.org/ValueSet/a"}`,
		"package/StructureDefinition-p.json": `{"resourceType":"StructureDefinition","id":"p","url":"https://ex.org/StructureDefinition/p"}`,
		"package/.index.json":                `{"index-version":1}`,
		"package/example/Patient-x.json":     `{"resourceType":"Patient","id":"x"}`,
	})
}

func TestLoadFromFileAndResources(t *testing.T) {
	dir := t.TempDir()
	tgz := filepath.Join(dir, "package.tgz")
	if err := os.WriteFile(tgz, igPackage(t), 0o644); err != nil {
		t.Fatal(err)
	}
	l := NewLoader(filepath.Join(dir, "cache"), zerolog.Nop())
	pkgs, err := l.Load(context.Background(), []Ref{{File: tgz, Use: []string{"valueSets"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "santeon.test" || pkgs[0].Version != "0.1.0" || pkgs[0].Use[0] != "valueSets" {
		t.Fatalf("got %+v", pkgs)
	}
	res, err := pkgs[0].Resources()
	if err != nil {
		t.Fatal(err)
	}
	if len(res["ValueSet"]) != 1 || len(res["StructureDefinition"]) != 1 || len(res["Patient"]) != 0 {
		t.Fatalf("examples and index files must be left out, got %v", res)
	}
	// loading again uses the cache entry for the same content
	if _, err := l.Load(context.Background(), []Ref{{File: tgz}}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFromRegistryWithDependencies(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/santeon.test/0.1.0":
			if r.Header.Get("Authorization") != "Bearer secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write(igPackage(t))
		case "/dep.one/1.0.0":
			_, _ = w.Write(makeTgz(t, map[string]string{
				"package/package.json":      `{"name":"dep.one","version":"1.0.0"}`,
				"package/CodeSystem-c.json": `{"resourceType":"CodeSystem","id":"c","url":"https://ex.org/cs"}`,
			}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	t.Setenv("REG_TOKEN", "secret")

	cache := t.TempDir()
	l := NewLoader(cache, zerolog.Nop())
	pkgs, err := l.Load(context.Background(), []Ref{{Name: "santeon.test", Version: "0.1.0", Registry: srv.URL, TokenEnv: "REG_TOKEN", Follow: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 || pkgs[1].Name != "dep.one" || !pkgs[1].Dependency {
		t.Fatalf("got %+v", pkgs)
	}
	for _, p := range paths {
		if strings.Contains(p, "hl7.fhir.r4.core") {
			t.Fatal("the core package is excluded by default")
		}
	}
	// second load is served from the cache: no new downloads
	n := len(paths)
	if _, err := l.Load(context.Background(), []Ref{{Name: "santeon.test", Version: "0.1.0", Registry: srv.URL, TokenEnv: "REG_TOKEN", Follow: true}}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != n {
		t.Fatalf("expected no downloads from cache, got %v", paths[n:])
	}
}

func TestRefusesUnsafePackages(t *testing.T) {
	l := NewLoader(t.TempDir(), zerolog.Nop())
	evil := makeTgz(t, map[string]string{"package/package.json": `{"name":"x","version":"1"}`, "../escape.json": "{}"})
	if err := extractInto(bytes.NewReader(evil), filepath.Join(l.CacheDir, "x")); err == nil {
		t.Fatal("an entry outside the package must be refused")
	}
	if _, err := l.Load(context.Background(), []Ref{{Name: "x", Version: "1.0.x"}}); err == nil {
		t.Fatal("wildcard versions must be refused")
	}
}
