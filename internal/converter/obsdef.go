package converter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// ObservationDefinitions checks an Observation's value against the
// ObservationDefinition for its code: that the kind of value (Quantity,
// CodeableConcept, ...) is one the definition permits, and that a coded answer
// is one of the valid answers of that observation code (validCodedValueSet).
// The same answer can be right for one observation code and wrong for another,
// which is why the check needs the definition and not just a valueset binding.
type ObservationDefinitions struct {
	mu     sync.RWMutex
	defs   []*obsDef
	maps   *ConceptMapService // the valuesets of the answers, and the concept maps that map answers onto them
	logger zerolog.Logger
}

type obsDef struct {
	id       string
	file     string
	codes    []codeRef // the observation codes the definition is for
	types    []string  // permittedDataType, e.g. "Quantity", "CodeableConcept"
	answers  string    // validCodedValueSet reference
	label    string    // preferredReportName, for display
	contents map[string]bool
}

type codeRef struct{ System, Code string }

// NewObservationDefinitions creates an empty set. maps supplies the answer
// valuesets (ConceptMapService.LoadValueSetsDir) and the concept maps that
// translate an answer onto them.
func NewObservationDefinitions(maps *ConceptMapService, logger zerolog.Logger) *ObservationDefinitions {
	return &ObservationDefinitions{maps: maps, logger: logger}
}

// LoadDir (re)loads every ObservationDefinition .json file in dir.
func (o *ObservationDefinitions) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	var defs []*obsDef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			o.logger.Warn().Err(err).Str("file", e.Name()).Msg("Skipping observation definition")
			continue
		}
		od, err := fhir.UnmarshalObservationDefinition(data)
		if err != nil {
			o.logger.Warn().Err(err).Str("file", e.Name()).Msg("Skipping observation definition")
			continue
		}
		d := &obsDef{file: e.Name(), label: stringOrEmpty(od.PreferredReportName)}
		if od.Id != nil {
			d.id = *od.Id
		}
		for _, c := range od.Code.Coding {
			if c.Code != nil {
				d.codes = append(d.codes, codeRef{stringOrEmpty(c.System), *c.Code})
			}
		}
		for _, t := range od.PermittedDataType {
			d.types = append(d.types, t.Code())
		}
		if od.ValidCodedValueSet != nil && od.ValidCodedValueSet.Reference != nil {
			d.answers = *od.ValidCodedValueSet.Reference
		}
		if len(d.codes) == 0 {
			o.logger.Warn().Str("file", e.Name()).Msg("Observation definition has no code, skipping")
			continue
		}
		defs = append(defs, d)
	}
	o.mu.Lock()
	o.defs = defs
	o.mu.Unlock()
	o.logger.Info().Str("dir", dir).Int("definitions", len(defs)).Msg("Loaded observation definitions")
	return nil
}

// find returns the first definition for one of the observation's codings. A
// system is compared only when both sides have one.
func (o *ObservationDefinitions) find(codings []codeRef) (*obsDef, codeRef) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	for _, d := range o.defs {
		for _, dc := range d.codes {
			for _, c := range codings {
				if c.Code == dc.Code && (c.System == "" || dc.System == "" || c.System == dc.System) {
					return d, dc
				}
			}
		}
	}
	return nil, codeRef{}
}

// ObsViolation is one way a value does not fit its observation definition.
type ObsViolation struct {
	Kind    string // ObsViolationType or ObsViolationAnswer
	ObsCode string // the observation code the definition is for
	Detail  string // the value type, or the answer code
	Allowed string // what the definition allows, for the message
	DefID   string
}

const (
	ObsViolationType   = "type"   // the value's type is not in permittedDataType
	ObsViolationAnswer = "answer" // the coded answer is not in validCodedValueSet
)

// Check compares a value with the definition for the observation's codings.
// valueType is the kind of value ("Quantity", "CodeableConcept", ...), answers
// the codes of a coded value. It returns nothing when there is no definition
// for the observation code.
func (o *ObservationDefinitions) Check(obsCodings []codeRef, valueType string, answers []string) []ObsViolation {
	d, dc := o.find(obsCodings)
	if d == nil || valueType == "" {
		return nil
	}
	var out []ObsViolation
	if len(d.types) > 0 && !containsFold(d.types, valueType) {
		out = append(out, ObsViolation{Kind: ObsViolationType, ObsCode: dc.Code, Detail: valueType, Allowed: strings.Join(d.types, ", "), DefID: d.id})
	}
	if strings.EqualFold(valueType, "CodeableConcept") && d.answers != "" {
		if members, ok := o.maps.ValueSetMembers(d.answers); ok {
			seen := map[string]bool{}
			for _, a := range answers {
				if a != "" && !members[a] && !seen[a] {
					seen[a] = true
					out = append(out, ObsViolation{Kind: ObsViolationAnswer, ObsCode: dc.Code, Detail: a, Allowed: strings.Join(sortedKeys(members, 8), ", "), DefID: d.id})
				}
			}
		}
	}
	return out
}

// DefinitionInfo describes the definition for an observation code, for the dashboard.
type DefinitionInfo struct {
	Code     string   `json:"code"`
	Found    bool     `json:"found"`
	ID       string   `json:"id,omitempty"`
	File     string   `json:"file,omitempty"`
	Label    string   `json:"label,omitempty"`
	Types    []string `json:"types,omitempty"`
	Answers  []string `json:"answers,omitempty"` // valid answers, if the answer valueset is known
	ValueSet string   `json:"valueset,omitempty"`
}

// Describe returns what the definition for an observation code allows. The
// code is matched without its system, as the dashboard only has the code.
func (o *ObservationDefinitions) Describe(code string) DefinitionInfo {
	info := DefinitionInfo{Code: code}
	d, _ := o.find([]codeRef{{Code: code}})
	if d == nil {
		return info
	}
	info.Found, info.ID, info.File, info.Label, info.Types, info.ValueSet = true, d.id, d.file, d.label, d.types, d.answers
	if d.answers != "" {
		if members, ok := o.maps.ValueSetMembers(d.answers); ok {
			info.Answers = sortedKeys(members, 0)
		}
	}
	return info
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// sortedKeys returns the keys in order; max > 0 caps how many are returned.
func sortedKeys(m map[string]bool, max int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if max > 0 && len(keys) > max {
		keys = keys[:max]
	}
	return keys
}

// AnswerSet returns the valueset of valid answers for an observation code: the
// definition's id, the valueset's canonical URL (what a concept map for the
// answers targets) and its member codes. ok is false when the code has no
// definition, or its definition names no answer valueset that is loaded.
func (o *ObservationDefinitions) AnswerSet(obsCode string) (defID, valueSetURL string, answers []string, ok bool) {
	d, _ := o.find([]codeRef{{Code: obsCode}})
	if d == nil || d.answers == "" {
		return "", "", nil, false
	}
	members, known := o.maps.ValueSetMembers(d.answers)
	url := o.maps.ValueSetURL(d.answers)
	if !known || url == "" {
		return d.id, "", nil, false
	}
	return d.id, url, sortedKeys(members, 0), true
}
