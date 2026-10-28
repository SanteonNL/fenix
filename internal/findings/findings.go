// Package findings collects data-quality findings about the FHIR resources
// FENIX produces and writes them to their own file, apart from the
// application log, so dashboards can be built on them.
//
// A finding has a stable code (FX-DATA-nnn, the same FX-xxx-nnn family the
// query validator uses), is aggregated per rule, resource type and path, and
// carries the number of resources it affected plus a few example resource ids.
// Findings are only ever reported: collecting them never changes or drops a
// resource.
package findings

import (
	"encoding/json"
	"strings"
)

// Severity orders findings; the zero value is the least severe.
type Severity int

const (
	Info Severity = iota
	Warning
	Error
)

func (s Severity) String() string {
	switch s {
	case Error:
		return "ERROR"
	case Warning:
		return "WARN"
	}
	return "INFO"
}

func (s Severity) MarshalJSON() ([]byte, error) { return json.Marshal(strings.ToLower(s.String())) }

// ParseSeverity turns "error"/"warning"/"info" (any case) into a Severity.
func ParseSeverity(s string) (Severity, bool) {
	switch strings.ToLower(s) {
	case "error":
		return Error, true
	case "warn", "warning":
		return Warning, true
	case "info":
		return Info, true
	}
	return Info, false
}

// Rule codes. DATA = a converted resource against the FHIR base model and the
// loaded profiles.
const (
	CodeDataType       = "FX-DATA-001" // value has the wrong JSON type for the element
	CodeDataValue      = "FX-DATA-002" // value rejected by the typed model (invalid code, malformed date, ...)
	CodeDataRequired   = "FX-DATA-010" // profile: element with min >= 1 is missing
	CodeDataProhibited = "FX-DATA-011" // profile: element with max = 0 is present
	CodeDataCardinal   = "FX-DATA-012" // profile: several values for an element with max = 1
	CodeDataFixed      = "FX-DATA-013" // profile: value differs from the fixed value
	CodeDataChoiceType = "FX-DATA-014" // profile: value[x] type not allowed

	CodeDataUnmapped     = "FX-DATA-020" // source code has no mapping: the concept map's default replaces it
	CodeDataUnmappedNone = "FX-DATA-021" // source code has no mapping and there is no default: passed through unchanged

	CodeDataObsType   = "FX-DATA-030" // Observation value is of a type its ObservationDefinition does not permit
	CodeDataObsAnswer = "FX-DATA-031" // coded Observation value is not a valid answer for its observation code (ObservationDefinition)
)
