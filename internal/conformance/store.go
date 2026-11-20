// Package conformance holds the FHIR conformance resources FENIX knows about
// (StructureDefinitions, ValueSets, CodeSystems, ConceptMaps, ...), from the
// configured input folders and FHIR packages, so the FHIR API can serve them
// (search by url, read by id, ValueSet/$expand, $validate-code) and evaluate
// the :in / :not-in token search modifiers.
//
// The store is filled once at startup and read concurrently afterwards.
package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// canonicalTypes are the resource types the store keeps and the FHIR API serves.
var canonicalTypes = map[string]bool{
	"StructureDefinition":   true,
	"ValueSet":              true,
	"CodeSystem":            true,
	"ConceptMap":            true,
	"ObservationDefinition": true,
	"Questionnaire":         true,
	"SearchParameter":       true,
	"OperationDefinition":   true,
	"NamingSystem":          true,
	"ImplementationGuide":   true,
	"StructureMap":          true,
	"CapabilityStatement":   true,
}

// Serves reports whether resourceType is a conformance type the store keeps.
func Serves(resourceType string) bool { return canonicalTypes[resourceType] }

// Entry is one stored resource.
type Entry struct {
	ResourceType string
	ID           string
	URL          string
	Version      string
	Name         string
	Status       string
	Origin       string // where it was loaded from (file path), for logging
	Raw          json.RawMessage
}

// Store is an in-memory index of conformance resources.
type Store struct {
	mu      sync.RWMutex
	byID    map[string]map[string]*Entry   // type -> id -> entry
	byURL   map[string]map[string][]*Entry // type -> url (no version) -> entries (all versions)
	expands sync.Map                       // reference -> *cached expansion
	remotes []*RemoteServer                // terminology servers for what cannot be expanded locally (AddRemote)
	log     zerolog.Logger
}

// NewStore returns an empty store.
func NewStore(log zerolog.Logger) *Store {
	return &Store{byID: map[string]map[string]*Entry{}, byURL: map[string]map[string][]*Entry{}, log: log}
}

// Add stores one resource. Resources of a type the store does not keep are
// ignored (ok false, no error). A resource whose type and id are already
// stored is skipped: the first one loaded wins.
func (s *Store) Add(raw []byte, origin string) (ok bool, err error) {
	var head struct {
		ResourceType string          `json:"resourceType"`
		ID           string          `json:"id"`
		URL          string          `json:"url"`
		Version      string          `json:"version"`
		Name         string          `json:"name"`
		Status       string          `json:"status"`
		ValueSet     json.RawMessage `json:"valueSet"` // {"valueSet": {...}} wrapper used by some terminology folders
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return false, fmt.Errorf("%s: %w", origin, err)
	}
	if head.ResourceType == "" && len(head.ValueSet) > 0 {
		return s.Add(head.ValueSet, origin)
	}
	if !canonicalTypes[head.ResourceType] {
		return false, nil
	}
	if head.ID == "" && head.URL == "" {
		return false, fmt.Errorf("%s: %s has neither id nor url", origin, head.ResourceType)
	}
	e := &Entry{ResourceType: head.ResourceType, ID: head.ID, URL: head.URL, Version: head.Version,
		Name: head.Name, Status: head.Status, Origin: origin, Raw: append(json.RawMessage(nil), raw...)}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, prev := range s.byURL[e.ResourceType][e.URL] {
		if e.URL != "" && prev.Version == e.Version {
			return false, nil // the same canonical resource again, e.g. from a folder and a package
		}
	}
	if e.ID != "" {
		ids := s.byID[e.ResourceType]
		if ids == nil {
			ids = map[string]*Entry{}
			s.byID[e.ResourceType] = ids
		}
		if prev, dup := ids[e.ID]; dup {
			// Ids are only unique per server, not across packages: the first keeps the id
			// (read by id), but this one stays reachable by its canonical url.
			s.log.Debug().Str("resourceType", e.ResourceType).Str("id", e.ID).Str("kept", prev.Origin).
				Str("alsoByUrl", origin).Msg("Conformance resource id used twice; the first is read by id")
			if e.URL == "" {
				return false, nil
			}
		} else {
			ids[e.ID] = e
		}
	}
	if e.URL != "" {
		urls := s.byURL[e.ResourceType]
		if urls == nil {
			urls = map[string][]*Entry{}
			s.byURL[e.ResourceType] = urls
		}
		urls[e.URL] = append(urls[e.URL], e)
	}
	return true, nil
}

// AddFile stores the resource in one JSON file.
func (s *Store) AddFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return s.Add(data, path)
}

// AddFiles stores every file and returns how many were kept. Unreadable files are logged and skipped.
func (s *Store) AddFiles(paths []string) int {
	n := 0
	for _, p := range paths {
		ok, err := s.AddFile(p)
		if err != nil {
			s.log.Warn().Err(err).Str("file", p).Msg("Skipping conformance resource")
			continue
		}
		if ok {
			n++
		}
	}
	return n
}

// AddDir stores every .json file directly inside dir (not recursive).
func (s *Store) AddDir(dir string) (int, error) {
	files, err := JSONFiles(dir)
	if err != nil {
		return 0, err
	}
	return s.AddFiles(files), nil
}

// JSONFiles lists the .json files directly inside dir, sorted.
func JSONFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

// Count returns how many resources of each type are stored.
func (s *Store) Count() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]int{}
	for t, ids := range s.byID {
		out[t] += len(ids)
	}
	for t, urls := range s.byURL {
		for _, list := range urls {
			for _, e := range list {
				if e.ID == "" {
					out[t]++
				}
			}
		}
	}
	return out
}

// Read returns the resource of a type with an id.
func (s *Store) Read(resourceType, id string) (*Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.byID[resourceType][id]
	return e, ok
}

// Resolve returns the resource of a type for a canonical reference: "url",
// "url|version", or "<Type>/<id>". Without a version the highest version is taken.
func (s *Store) Resolve(resourceType, ref string) (*Entry, bool) {
	if id, ok := strings.CutPrefix(ref, resourceType+"/"); ok {
		return s.Read(resourceType, id)
	}
	url, version, _ := strings.Cut(ref, "|")
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := s.byURL[resourceType][url]
	if version != "" {
		for _, e := range list {
			if e.Version == version {
				return e, true
			}
		}
		return nil, false
	}
	if len(list) == 0 {
		return nil, false
	}
	best := list[0]
	for _, e := range list[1:] {
		if compareVersions(e.Version, best.Version) > 0 {
			best = e
		}
	}
	return best, true
}

// Search returns the resources of a type matching every given param. Supported
// params: _id, url (also "url|version"), version, name, status. Comma-separated
// values are OR-ed. Unsupported params return an error.
func (s *Store) Search(resourceType string, params map[string]string) ([]*Entry, error) {
	for p := range params {
		switch p {
		case "_id", "url", "version", "name", "status":
		default:
			return nil, fmt.Errorf("search parameter %q is not supported for %s (supported: _id, url, version, name, status)", p, resourceType)
		}
	}
	s.mu.RLock()
	var all []*Entry
	seen := map[*Entry]bool{}
	for _, e := range s.byID[resourceType] {
		all, seen[e] = append(all, e), true
	}
	for _, list := range s.byURL[resourceType] {
		for _, e := range list {
			if !seen[e] {
				all = append(all, e)
			}
		}
	}
	s.mu.RUnlock()

	var out []*Entry
	for _, e := range all {
		if matchesAll(e, params) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URL != out[j].URL {
			return out[i].URL < out[j].URL
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func matchesAll(e *Entry, params map[string]string) bool {
	for p, v := range params {
		if !anyOf(v, func(want string) bool {
			switch p {
			case "_id":
				return e.ID == want
			case "url":
				url, version, _ := strings.Cut(want, "|")
				return e.URL == url && (version == "" || e.Version == version)
			case "version":
				return e.Version == want
			case "name":
				return strings.HasPrefix(strings.ToLower(e.Name), strings.ToLower(want))
			case "status":
				return e.Status == want
			}
			return false
		}) {
			return false
		}
	}
	return true
}

func anyOf(value string, f func(string) bool) bool {
	for _, v := range strings.Split(value, ",") {
		if f(strings.TrimSpace(v)) {
			return true
		}
	}
	return false
}

// compareVersions compares dotted versions numerically where both parts are
// numbers, and as text otherwise.
func compareVersions(a, b string) int {
	pa, pb := strings.FieldsFunc(a, isSep), strings.FieldsFunc(b, isSep)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		if i >= len(pa) {
			return -1
		}
		if i >= len(pb) {
			return 1
		}
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		switch {
		case ea == nil && eb == nil && na != nb:
			if na < nb {
				return -1
			}
			return 1
		case (ea != nil || eb != nil) && pa[i] != pb[i]:
			return strings.Compare(pa[i], pb[i])
		}
	}
	return 0
}

func isSep(r rune) bool { return r == '.' || r == '-' || r == '+' }
