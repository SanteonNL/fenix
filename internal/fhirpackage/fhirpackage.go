// Package fhirpackage loads FHIR NPM packages (package.tgz, as built by the
// HL7 IG Publisher and served by package registries such as
// packages.fhir.org and packages.simplifier.net) into a local cache, and
// lists the resources they contain.
//
// A package is taken from a registry (name + version) or from a local file:
// a .tgz or an already extracted folder. Registry packages are cached per
// name#version and never downloaded again; a local .tgz is cached per content
// hash, so a rebuilt package is picked up automatically. Dependencies listed
// in package.json can be followed.
package fhirpackage

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// DefaultRegistry is used when a package reference names no registry.
const DefaultRegistry = "https://packages.fhir.org"

// DefaultExclude are dependencies not followed unless a reference lists its own
// exclude: the FHIR core package is large and its definitions are built in.
var DefaultExclude = []string{"hl7.fhir.r4.core"}

// Ref says where one package comes from.
type Ref struct {
	Name     string   // package name, e.g. santeon.sim-on-fhir (registry packages)
	Version  string   // exact version, e.g. 0.1.0 (registry packages)
	Registry string   // registry base URL; DefaultRegistry when empty
	TokenEnv string   // optional env var with a bearer token for the registry
	File     string   // local .tgz or extracted folder; when set, Name/Version are read from its package.json
	Follow   bool     // also load the dependencies in its package.json, recursively
	Exclude  []string // dependency names not followed; nil means DefaultExclude
	Use      []string // what the caller feeds the resources into; carried through unchanged
}

// Package is one loaded package.
type Package struct {
	Name, Version string
	Dir           string   // folder holding the resource .json files (the package/ folder)
	Dependency    bool     // loaded because another package depends on it
	Use           []string // the Ref's Use; empty for dependencies
	Dependencies  map[string]string
}

// Loader loads packages into a cache folder.
type Loader struct {
	CacheDir string
	Client   *http.Client
	Log      zerolog.Logger
}

// NewLoader returns a loader that caches into cacheDir.
func NewLoader(cacheDir string, log zerolog.Logger) *Loader {
	return &Loader{CacheDir: cacheDir, Client: &http.Client{Timeout: 5 * time.Minute}, Log: log}
}

// Load loads every reference and, where asked, their dependencies. A package
// that is reached twice is loaded once. A reference that fails is an error; a
// dependency that fails is logged and skipped.
func (l *Loader) Load(ctx context.Context, refs []Ref) ([]Package, error) {
	var out []Package
	seen := map[string]bool{} // package name
	var errs []string
	for _, ref := range refs {
		p, err := l.one(ctx, ref)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if seen[p.Name] {
			l.Log.Warn().Str("package", p.Name+"#"+p.Version).Msg("Package listed twice, loading it once")
			continue
		}
		seen[p.Name] = true
		p.Use = ref.Use
		out = append(out, p)
		if ref.Follow {
			out = append(out, l.dependencies(ctx, p, ref, seen)...)
		}
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return out, nil
}

// dependencies loads the dependencies of p breadth first, skipping excluded and
// already loaded names. Breadth first means the version a package asks for
// itself wins over the version a dependency of it asks for.
func (l *Loader) dependencies(ctx context.Context, p Package, root Ref, seen map[string]bool) []Package {
	exclude := root.Exclude
	if exclude == nil {
		exclude = DefaultExclude
	}
	var out []Package
	queue := []Package{p}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		names := make([]string, 0, len(cur.Dependencies))
		for n := range cur.Dependencies {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			if seen[name] || contains(exclude, name) {
				continue
			}
			seen[name] = true // also when it fails: the first version asked for is the one tried
			version := cur.Dependencies[name]
			dep, err := l.one(ctx, Ref{Name: name, Version: version, Registry: root.Registry, TokenEnv: root.TokenEnv})
			if err != nil {
				l.Log.Warn().Err(err).Str("package", cur.Name).Str("dependency", name+"#"+version).Msg("Skipping package dependency")
				continue
			}
			dep.Dependency = true
			out = append(out, dep)
			queue = append(queue, dep)
		}
	}
	return out
}

func (l *Loader) one(ctx context.Context, ref Ref) (Package, error) {
	if ref.File != "" {
		return l.fromFile(ref.File)
	}
	if ref.Name == "" || ref.Version == "" {
		return Package{}, fmt.Errorf("package needs name and version, or file")
	}
	return l.fromRegistry(ctx, ref)
}

// fromFile loads a local .tgz (extracted into the cache by content hash) or an extracted folder.
func (l *Loader) fromFile(path string) (Package, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Package{}, fmt.Errorf("package file: %w", err)
	}
	if info.IsDir() {
		dir := path
		if st, err := os.Stat(filepath.Join(path, "package")); err == nil && st.IsDir() {
			dir = filepath.Join(path, "package")
		}
		return readPackage(dir)
	}
	sum, err := fileHash(path)
	if err != nil {
		return Package{}, err
	}
	dest := filepath.Join(l.CacheDir, "file-"+sum[:16])
	if _, err := os.Stat(filepath.Join(dest, "package", "package.json")); err != nil {
		f, err := os.Open(path)
		if err != nil {
			return Package{}, err
		}
		defer f.Close()
		if err := extractInto(f, dest); err != nil {
			return Package{}, fmt.Errorf("extract %s: %w", path, err)
		}
	}
	return readPackage(filepath.Join(dest, "package"))
}

// fromRegistry downloads name#version from the registry unless it is cached.
func (l *Loader) fromRegistry(ctx context.Context, ref Ref) (Package, error) {
	if strings.ContainsAny(ref.Version, "x*^~ ") || ref.Version == "current" || ref.Version == "dev" {
		return Package{}, fmt.Errorf("%s#%s: only exact versions can be loaded", ref.Name, ref.Version)
	}
	dest := filepath.Join(l.CacheDir, ref.Name+"#"+ref.Version)
	if _, err := os.Stat(filepath.Join(dest, "package", "package.json")); err == nil {
		return readPackage(filepath.Join(dest, "package"))
	}
	registry := strings.TrimRight(ref.Registry, "/")
	if registry == "" {
		registry = DefaultRegistry
	}
	url := registry + "/" + ref.Name + "/" + ref.Version
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Package{}, err
	}
	req.Header.Set("Accept", "application/tar+gzip, application/gzip, application/octet-stream")
	if ref.TokenEnv != "" {
		token := os.Getenv(ref.TokenEnv)
		if token == "" {
			return Package{}, fmt.Errorf("%s#%s: env var %s (registry token) is empty", ref.Name, ref.Version, ref.TokenEnv)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	l.Log.Info().Str("package", ref.Name+"#"+ref.Version).Str("url", url).Msg("Downloading FHIR package")
	resp, err := l.Client.Do(req)
	if err != nil {
		return Package{}, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Package{}, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	if err := extractInto(resp.Body, dest); err != nil {
		return Package{}, fmt.Errorf("extract %s: %w", url, err)
	}
	return readPackage(filepath.Join(dest, "package"))
}

// extractInto extracts a package .tgz into dest, through a temporary folder so
// an interrupted extraction never leaves a half-filled cache entry behind.
func extractInto(r io.Reader, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dest), ".extract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := untar(r, tmp); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(tmp, "package", "package.json")); err != nil {
		return fmt.Errorf("not a FHIR package: no package/package.json")
	}
	_ = os.RemoveAll(dest)
	return os.Rename(tmp, dest)
}

func untar(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	root := filepath.Clean(dest) + string(os.PathSeparator)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg {
			continue // folders are created as needed; links and others are not followed
		}
		target := filepath.Join(dest, filepath.FromSlash(h.Name))
		if !strings.HasPrefix(target, root) {
			return fmt.Errorf("entry %q points outside the package", h.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
}

func readPackage(dir string) (Package, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return Package{}, fmt.Errorf("%s: no package.json: %w", dir, err)
	}
	var pj struct {
		Name         string            `json:"name"`
		Version      string            `json:"version"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &pj); err != nil {
		return Package{}, fmt.Errorf("%s/package.json: %w", dir, err)
	}
	return Package{Name: pj.Name, Version: pj.Version, Dir: dir, Dependencies: pj.Dependencies}, nil
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Resources lists the resource files of a package by resource type. Only the
// top folder is read (examples and other sub folders are left out), and
// package.json / .index.json are skipped.
func (p Package) Resources() (map[string][]string, error) {
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(name), ".json") || name == "package.json" || strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(p.Dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var head struct {
			ResourceType string `json:"resourceType"`
		}
		if json.Unmarshal(data, &head) != nil || head.ResourceType == "" {
			continue
		}
		out[head.ResourceType] = append(out[head.ResourceType], path)
	}
	for t := range out {
		sort.Strings(out[t])
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
