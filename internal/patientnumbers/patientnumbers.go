// Package patientnumbers implements the DSG patient-id strategy (HipsETL
// goSim/PatientID.go) for FENIX: a patient can have had several patient
// numbers over time (merges, renumbering). Every old number points at the
// number that replaced it ("replaced_by"); following that chain always ends at
// one active number.
//
// A cohort is extended with all numbers of its patients, so the queries find
// the data recorded under old numbers too (test patients are left out, as in
// the DSG); the resulting resources are then
// brought back to the single active number (UseActiveNumber).
package patientnumbers

import (
	"sort"
	"strings"

	"github.com/SanteonNL/fenix/internal/converter"
	"github.com/SanteonNL/fenix/internal/deident"
	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// Registry knows, for every patient number, the active number it leads to and
// whether it belongs to a test patient. A nil *Registry means "no patient-number
// strategy": every method is then a no-op.
type Registry struct {
	replacedBy map[string]string   // old number -> number that replaced it (direct)
	numbers    map[string][]string // active number -> every number of that patient, active one included
	test       map[string]bool     // numbers flagged as test patient
}

// NewRegistry builds a Registry from the rows of the patient-number query:
// number, replaced_by (empty = the number is active) and optionally
// test_patient ("true"/"1"/"yes"). A cycle (bad source data) is broken by
// treating the number where the cycle closes as the active one.
func NewRegistry(rows [][]string) *Registry {
	m := &Registry{replacedBy: make(map[string]string, len(rows)), numbers: map[string][]string{}, test: map[string]bool{}}
	known := map[string]struct{}{}
	for _, row := range rows {
		col := func(i int) string {
			if i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		id, repl := col(0), col(1)
		if id == "" {
			continue
		}
		known[id] = struct{}{}
		if repl != "" && repl != id {
			m.replacedBy[id] = repl
			known[repl] = struct{}{}
		}
		switch strings.ToLower(col(2)) {
		case "true", "1", "t", "yes", "y":
			m.test[id] = true
		}
	}
	for id := range known {
		a := m.ActiveNumber(id)
		m.numbers[a] = append(m.numbers[a], id)
	}
	for a := range m.numbers {
		sort.Strings(m.numbers[a])
	}
	return m
}

// Excluded reports whether id is left out entirely, as in the DSG
// (CreatePatientIDList): the number is flagged as test patient, or the active
// number it leads to is.
func (m *Registry) Excluded(id string) bool {
	if m == nil || len(m.test) == 0 {
		return false
	}
	return m.test[id] || m.test[m.ActiveNumber(id)]
}

// ActiveNumber follows the replaced_by chain from id to the active number. A number
// the map doesn't know is its own active number.
func (m *Registry) ActiveNumber(id string) string {
	if m == nil {
		return id
	}
	seen := map[string]struct{}{id: {}}
	for {
		next, ok := m.replacedBy[id]
		if !ok {
			return id
		}
		if _, loop := seen[next]; loop {
			return id
		}
		seen[next] = struct{}{}
		id = next
	}
}

// WithOldNumbers returns the sorted, de-duplicated numbers of every patient in ids:
// each id is brought to its active number, and all numbers (old and active)
// of that patient are returned. This is what the queries run on.
func (m *Registry) WithOldNumbers(ids []string) []string {
	if m == nil {
		return ids
	}
	set := map[string]struct{}{}
	for _, id := range ids {
		if m.Excluded(id) {
			continue
		}
		a := m.ActiveNumber(id)
		if all, ok := m.numbers[a]; ok {
			for _, n := range all {
				if m.Excluded(n) {
					continue
				}
				set[n] = struct{}{}
			}
		} else {
			set[a] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ActiveNumbers returns the sorted, de-duplicated active numbers of ids.
func (m *Registry) ActiveNumbers(ids []string) []string {
	if m == nil {
		return ids
	}
	set := map[string]struct{}{}
	for _, id := range ids {
		if !m.Excluded(id) {
			set[m.ActiveNumber(id)] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// UseActiveNumber brings resources to the active patient number: a Patient under an old
// number is dropped (the active number's own Patient stays), Patient.id and
// every "Patient/<number>" reference are rewritten to the active number, and the
// active Patient lists its old numbers as identifiers (use "old", OldNumberSystem).
// Resources about excluded (test) patients are dropped as well.
// Resources are changed in place; the returned slice omits the dropped ones.
func (m *Registry) UseActiveNumber(resources []interface{}) []interface{} {
	if m == nil || (len(m.replacedBy) == 0 && len(m.test) == 0) {
		return resources
	}
	out := resources[:0]
	for _, r := range resources {
		rt, val, typed := converter.Unwrap(r)
		if !typed {
			out = append(out, r)
			continue
		}
		fields := deident.Walk(rt, val)
		if rt == "Patient" && (m.isOld(patientResourceID(fields)) || m.Excluded(patientResourceID(fields))) {
			continue
		}
		if m.refersToExcluded(fields) {
			continue
		}
		for _, f := range fields {
			cur, ok := f.Get()
			if !ok {
				continue
			}
			switch {
			case rt == "Patient" && f.Path == "Patient.id":
				f.Set(m.ActiveNumber(cur))
			case f.Kind == deident.KindReference && strings.HasPrefix(cur, "Patient/"):
				rest := strings.TrimPrefix(cur, "Patient/")
				id, tail, _ := strings.Cut(rest, "/")
				if a := m.ActiveNumber(id); a != id {
					if tail != "" {
						a += "/" + tail
					}
					f.Set("Patient/" + a)
				}
			}
		}
		if p, ok := val.(*fhir.Patient); ok {
			m.addOldNumbers(p)
		}
		out = append(out, r)
	}
	return out
}

func (m *Registry) isOld(id string) bool {
	_, ok := m.replacedBy[id]
	return ok
}

func patientResourceID(fields []deident.Field) string {
	for _, f := range fields {
		if f.Path == "Patient.id" {
			v, _ := f.Get()
			return v
		}
	}
	return ""
}

// OldNumberSystem is the identifier system of the old patient numbers listed
// on the active Patient. The values are hashed by the de-identification rule
// for Patient.identifier like any other identifier.
const OldNumberSystem = "https://santeon.nl/fhir/NamingSystem/patientnumber-old"

// addOldNumbers lists every old number of p's patient as an identifier with
// use "old", so the mapping stays in the output. Already listed numbers are skipped.
func (m *Registry) addOldNumbers(p *fhir.Patient) {
	if p.Id == nil {
		return
	}
	have := map[string]struct{}{}
	for _, id := range p.Identifier {
		if id.System != nil && *id.System == OldNumberSystem && id.Value != nil {
			have[*id.Value] = struct{}{}
		}
	}
	for _, n := range m.numbers[*p.Id] {
		if _, dup := have[n]; dup || n == *p.Id {
			continue
		}
		use, system, value := fhir.IdentifierUseOld, OldNumberSystem, n
		p.Identifier = append(p.Identifier, fhir.Identifier{Use: &use, System: &system, Value: &value})
	}
}

// refersToExcluded reports whether a resource points at an excluded (test) patient.
func (m *Registry) refersToExcluded(fields []deident.Field) bool {
	for _, f := range fields {
		if f.Kind != deident.KindReference {
			continue
		}
		if cur, ok := f.Get(); ok && strings.HasPrefix(cur, "Patient/") {
			id, _, _ := strings.Cut(strings.TrimPrefix(cur, "Patient/"), "/")
			if m.Excluded(id) {
				return true
			}
		}
	}
	return false
}
