package conformance

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Concept is one member of a ValueSet expansion.
type Concept struct {
	System  string `json:"system,omitempty"`
	Version string `json:"version,omitempty"`
	Code    string `json:"code"`
	Display string `json:"display,omitempty"`
}

func (c Concept) key() string { return c.System + "|" + c.Code }

// Expansion is an expanded ValueSet.
type Expansion struct {
	URL, Version string
	Entry        *Entry    // the loaded ValueSet; nil when only a terminology server knows it
	Concepts     []Concept // the members
	Source       string    // "local", or the name of the terminology server that expanded it
}

// Members is the membership of an expanded ValueSet, for token matching.
type Members struct {
	Pairs map[string]bool // "system|code"
	Codes map[string]bool // code alone, for plain code fields that carry no system
}

// Contains reports whether a coding is a member. Without a system only the code is compared.
func (m Members) Contains(system, code string) bool {
	if system == "" {
		return m.Codes[code]
	}
	return m.Pairs[system+"|"+code]
}

// needsServerError marks an expansion that only a terminology server can do:
// a compose filter (e.g. SNOMED ECL, is-a) or a whole code system that is not loaded.
type needsServerError struct {
	system string
	msg    string
}

func (e *needsServerError) Error() string { return e.msg }

type cached struct {
	x       *Expansion
	err     error
	expires time.Time // zero: never
}

// Expand returns the members of the ValueSet a reference points at ("url",
// "url|version" or "ValueSet/<id>"). Where it is expanded:
//
//  1. Loaded locally (input folder or package) and expandable here: compose
//     include/exclude with listed concepts, whole code systems loaded as a
//     complete CodeSystem, nested valueSets, or a ValueSet that only carries an
//     expansion.
//  2. Otherwise the first terminology server whose rules match (see
//     RemoteServer): the ValueSet url starts with one of its valueSets
//     prefixes, or it needs one of its systems (a filter on, or all of, a code
//     system that is not loaded here; also an implicit ValueSet such as
//     http://snomed.info/sct?fhir_vs=ecl/...).
//  3. Otherwise an error that says why.
//
// Local expansions are cached for the lifetime of the store, terminology
// server expansions for the server's CacheTTL.
func (s *Store) Expand(ref string) (*Expansion, error) {
	return s.expandRef(ref, map[string]bool{})
}

// Members returns the membership of a ValueSet for :in / :not-in matching.
func (s *Store) Members(ref string) (Members, error) {
	x, err := s.Expand(ref)
	if err != nil {
		return Members{}, err
	}
	m := Members{Pairs: map[string]bool{}, Codes: map[string]bool{}}
	for _, c := range x.Concepts {
		m.Pairs[c.key()] = true
		m.Codes[c.Code] = true
	}
	return m, nil
}

// ValidateCode reports whether system|code is in the ValueSet, with the display the ValueSet gives it.
func (s *Store) ValidateCode(ref, system, code string) (found bool, display string, err error) {
	x, err := s.Expand(ref)
	if err != nil {
		return false, "", err
	}
	for _, c := range x.Concepts {
		if c.Code == code && (system == "" || c.System == system) {
			return true, c.Display, nil
		}
	}
	return false, "", nil
}

func (s *Store) expandRef(ref string, visiting map[string]bool) (*Expansion, error) {
	if v, ok := s.expands.Load(ref); ok {
		c := v.(*cached)
		if c.expires.IsZero() || time.Now().Before(c.expires) {
			return c.x, c.err
		}
	}
	if visiting[ref] {
		return nil, fmt.Errorf("ValueSet %s includes itself", ref)
	}
	visiting[ref] = true
	defer delete(visiting, ref)

	url, version, _ := strings.Cut(ref, "|")
	if e, ok := s.Resolve("ValueSet", ref); ok {
		concepts, err := s.expandLocal(e, visiting)
		if err == nil {
			x := &Expansion{URL: e.URL, Version: e.Version, Entry: e, Concepts: concepts, Source: "local"}
			s.expands.Store(ref, &cached{x: x})
			return x, nil
		}
		var ns *needsServerError
		if errors.As(err, &ns) {
			if r := s.remoteFor(e.URL, ns.system); r != nil {
				return s.expandRemote(r, ref, e.URL, e.Version, e)
			}
			return nil, fmt.Errorf("%s (no terminology server is configured for it)", err)
		}
		s.expands.Store(ref, &cached{err: err})
		return nil, err
	}
	if r := s.remoteFor(url, implicitSystem(url)); r != nil {
		return s.expandRemote(r, ref, url, version, nil)
	}
	return nil, fmt.Errorf("ValueSet %s is not loaded (add it to a folder or package input) and no terminology server is configured for it", ref)
}

func (s *Store) expandRemote(r *RemoteServer, ref, url, version string, e *Entry) (*Expansion, error) {
	concepts, err := r.expand(url, version)
	if err != nil {
		return nil, err // not cached: may be temporary
	}
	x := &Expansion{URL: url, Version: version, Entry: e, Concepts: concepts, Source: r.Name}
	s.expands.Store(ref, &cached{x: x, expires: time.Now().Add(r.ttl())})
	s.log.Info().Str("valueSet", ref).Str("server", r.Name).Int("concepts", len(concepts)).Msg("Expanded ValueSet on terminology server")
	return x, nil
}

// implicitSystem returns the code system of an implicit ValueSet url
// ("http://snomed.info/sct?fhir_vs=..." -> "http://snomed.info/sct").
func implicitSystem(url string) string {
	if system, _, ok := strings.Cut(url, "?fhir_vs"); ok {
		return system
	}
	return ""
}

type valueSetDoc struct {
	Compose *struct {
		Include []composeRule `json:"include"`
		Exclude []composeRule `json:"exclude"`
	} `json:"compose"`
	Expansion *struct {
		Contains []containsItem `json:"contains"`
	} `json:"expansion"`
}

type composeRule struct {
	System  string `json:"system"`
	Version string `json:"version"`
	Concept []struct {
		Code    string `json:"code"`
		Display string `json:"display"`
	} `json:"concept"`
	Filter   []json.RawMessage `json:"filter"`
	ValueSet []string          `json:"valueSet"`
}

type containsItem struct {
	System   string         `json:"system"`
	Version  string         `json:"version"`
	Code     string         `json:"code"`
	Display  string         `json:"display"`
	Abstract bool           `json:"abstract"`
	Contains []containsItem `json:"contains"`
}

func (s *Store) expandLocal(e *Entry, visiting map[string]bool) ([]Concept, error) {
	var vs valueSetDoc
	if err := json.Unmarshal(e.Raw, &vs); err != nil {
		return nil, fmt.Errorf("ValueSet %s: %w", e.URL, err)
	}
	if vs.Compose == nil {
		if vs.Expansion == nil {
			return nil, fmt.Errorf("ValueSet %s has neither compose nor expansion", e.URL)
		}
		var out []Concept
		flattenContains(vs.Expansion.Contains, &out)
		return out, nil
	}

	included := map[string]Concept{}
	var order []string
	for _, rule := range vs.Compose.Include {
		cs, err := s.applyRule(e, rule, visiting)
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			if _, dup := included[c.key()]; !dup {
				order = append(order, c.key())
			}
			included[c.key()] = c
		}
	}
	for _, rule := range vs.Compose.Exclude {
		cs, err := s.applyRule(e, rule, visiting)
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			delete(included, c.key())
		}
	}
	out := make([]Concept, 0, len(included))
	for _, k := range order {
		if c, ok := included[k]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// applyRule returns the concepts one include/exclude rule selects: the
// intersection of its system part and every valueSet it names.
func (s *Store) applyRule(owner *Entry, rule composeRule, visiting map[string]bool) ([]Concept, error) {
	if len(rule.Filter) > 0 {
		return nil, &needsServerError{system: rule.System,
			msg: fmt.Sprintf("ValueSet %s uses a compose filter on %s, which needs a terminology server", owner.URL, rule.System)}
	}
	var sets [][]Concept
	if rule.System != "" {
		if len(rule.Concept) > 0 {
			var cs []Concept
			for _, c := range rule.Concept {
				cs = append(cs, Concept{System: rule.System, Version: rule.Version, Code: c.Code, Display: c.Display})
			}
			sets = append(sets, cs)
		} else {
			cs, err := s.wholeCodeSystem(owner, rule.System, rule.Version)
			if err != nil {
				return nil, err
			}
			sets = append(sets, cs)
		}
	}
	for _, ref := range rule.ValueSet {
		x, err := s.expandRef(ref, visiting) // may itself go to a terminology server
		if err != nil {
			return nil, fmt.Errorf("ValueSet %s includes %s: %w", owner.URL, ref, err)
		}
		sets = append(sets, x.Concepts)
	}
	if len(sets) == 0 {
		return nil, nil
	}
	out := sets[0]
	for _, other := range sets[1:] {
		keep := map[string]bool{}
		for _, c := range other {
			keep[c.key()] = true
		}
		var both []Concept
		for _, c := range out {
			if keep[c.key()] {
				both = append(both, c)
			}
		}
		out = both
	}
	return out, nil
}

type codeSystemConcept struct {
	Code    string              `json:"code"`
	Display string              `json:"display"`
	Concept []codeSystemConcept `json:"concept"`
}

// wholeCodeSystem returns every concept of a code system, which must be loaded with content "complete".
func (s *Store) wholeCodeSystem(owner *Entry, system, version string) ([]Concept, error) {
	ref := system
	if version != "" {
		ref += "|" + version
	}
	cs, ok := s.Resolve("CodeSystem", ref)
	if !ok {
		return nil, &needsServerError{system: system,
			msg: fmt.Sprintf("ValueSet %s includes all of code system %s, which is not loaded as a CodeSystem", owner.URL, system)}
	}
	var doc struct {
		Content string              `json:"content"`
		Concept []codeSystemConcept `json:"concept"`
	}
	if err := json.Unmarshal(cs.Raw, &doc); err != nil {
		return nil, fmt.Errorf("CodeSystem %s: %w", system, err)
	}
	if doc.Content != "complete" {
		return nil, &needsServerError{system: system,
			msg: fmt.Sprintf("ValueSet %s includes all of code system %s, but its loaded CodeSystem has content %q, not complete", owner.URL, system, doc.Content)}
	}
	var out []Concept
	var walk func([]codeSystemConcept)
	walk = func(list []codeSystemConcept) {
		for _, c := range list {
			out = append(out, Concept{System: system, Version: cs.Version, Code: c.Code, Display: c.Display})
			walk(c.Concept)
		}
	}
	walk(doc.Concept)
	return out, nil
}

func flattenContains(items []containsItem, out *[]Concept) {
	for _, c := range items {
		if c.Code != "" && !c.Abstract {
			*out = append(*out, Concept{System: c.System, Version: c.Version, Code: c.Code, Display: c.Display})
		}
		flattenContains(c.Contains, out)
	}
}
