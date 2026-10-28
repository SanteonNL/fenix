// Package queryvalidator checks FENIX SQL conversion queries against the FHIR
// model and the loaded profiles: not just "does it run" but "will the
// resources it produces be valid, and does it cover what the profile asks for".
//
// Every finding has a stable code (FX-xxx-nnn) so a team can discuss, suppress
// or grep for a specific rule. Findings are aggregated: one finding per rule,
// path and column, with the number of affected rows and a few example values.
package queryvalidator

import (
	"fmt"
	"sort"

	"github.com/SanteonNL/fenix/internal/findings"
)

// Severity orders findings; the zero value is the least severe. It is shared
// with the data findings of internal/findings so both report on one scale.
type Severity = findings.Severity

const (
	Info    = findings.Info
	Warning = findings.Warning
	Error   = findings.Error
)

// ParseSeverity turns "error"/"warning"/"info" (any case) into a Severity.
func ParseSeverity(s string) (Severity, bool) { return findings.ParseSeverity(s) }

// Rule codes. SQL = query shape, PATH = fhir_path tree, COL = column vs. the
// FHIR model, LIT = literal values, PROF = profile coverage, SEM = semantic
// sanity of element groups.
const (
	CodeSQLNoSelect     = "FX-SQL-001"
	CodeSQLMeta         = "FX-SQL-002" // resource_id / id / parent_id / fhir_path missing
	CodeSQLDupCol       = "FX-SQL-003"
	CodeSQLSelectStar   = "FX-SQL-004"
	CodeSQLUnnamed      = "FX-SQL-005" // expression without AS alias
	CodeSQLDynamicPath  = "FX-SQL-006" // fhir_path is not a string literal
	CodePathNoRoot      = "FX-PATH-001"
	CodePathMixedRoots  = "FX-PATH-002"
	CodePathUnknownType = "FX-PATH-003"
	CodePathUntyped     = "FX-PATH-004"
	CodePathUnknown     = "FX-PATH-005"
	CodePathNoRootStmt  = "FX-PATH-006" // child path but no statement for its resource
	CodeColUnknown      = "FX-COL-001"
	CodeColCase         = "FX-COL-002"
	CodeColComplex      = "FX-COL-003"
	CodeColProhibited   = "FX-COL-004"
	CodeColChoiceType   = "FX-COL-005"
	CodeColChoiceMulti  = "FX-COL-006"
	CodeColPrimitive    = "FX-COL-007" // sub-element of a primitive
	CodeLitValue        = "FX-LIT-001" // literal not valid for the element type
	CodeLitCode         = "FX-LIT-002" // literal not in the code list
	CodeLitFixed        = "FX-LIT-003" // literal differs from the profile's fixed value
	CodeLitNull         = "FX-LIT-004" // NULL for a required element
	CodeLitReference    = "FX-LIT-005" // reference prefix invalid / not allowed by profile
	CodeLitCast         = "FX-LIT-006" // CAST target does not fit the element type
	CodeProfRequired    = "FX-PROF-001"
	CodeProfMustSupport = "FX-PROF-002"
	CodeProfNone        = "FX-PROF-003"
	CodeProfObligation  = "FX-PROF-004" // SHALL/SHOULD populate for the exposing system
	CodeSemCoding       = "FX-SEM-001"
	CodeSemIdentifier   = "FX-SEM-002"
	CodeSemQuantity     = "FX-SEM-003"
	CodeSemReference    = "FX-SEM-004"
)

// Finding is one aggregated validation result.
type Finding struct {
	Severity  Severity `json:"severity"`
	Code      string   `json:"code"`
	Statement int      `json:"statement,omitempty"` // 1-based; 0 = whole file
	Path      string   `json:"path,omitempty"`
	Column    string   `json:"column,omitempty"`
	Message   string   `json:"message"`
	Count     int      `json:"count,omitempty"`
	Examples  []string `json:"examples,omitempty"`
}

// FileReport holds the findings for one SQL file.
type FileReport struct {
	File       string    `json:"file"`
	Statements int       `json:"statements"`
	Rows       int       `json:"rows"`
	Findings   []Finding `json:"findings"`
}

// Count returns how many findings have exactly severity s.
func (r *FileReport) Count(s Severity) int {
	n := 0
	for _, f := range r.Findings {
		if f.Severity == s {
			n++
		}
	}
	return n
}

// Max returns the highest severity present, and false when there are none.
func (r *FileReport) Max() (Severity, bool) {
	if len(r.Findings) == 0 {
		return Info, false
	}
	m := Info
	for _, f := range r.Findings {
		if f.Severity > m {
			m = f.Severity
		}
	}
	return m, true
}

// collector aggregates repeated findings (same rule, place and message) into one.
type collector struct {
	order []string
	by    map[string]*Finding
}

const maxExamples = 3

func newCollector() *collector { return &collector{by: map[string]*Finding{}} }

// add records a finding. count is the number of rows affected (0 for
// structural findings); example, when non-empty, is kept for the first few.
func (c *collector) add(sev Severity, code string, stmt int, path, column, msg string, count int, example string) {
	key := fmt.Sprintf("%s|%d|%s|%s|%s", code, stmt, path, column, msg)
	f, ok := c.by[key]
	if !ok {
		f = &Finding{Severity: sev, Code: code, Statement: stmt, Path: path, Column: column, Message: msg}
		c.by[key] = f
		c.order = append(c.order, key)
	}
	f.Count += count
	if example != "" && len(f.Examples) < maxExamples {
		for _, e := range f.Examples {
			if e == example {
				return
			}
		}
		f.Examples = append(f.Examples, example)
	}
}

// findings returns the aggregated findings, most severe first.
func (c *collector) findings() []Finding {
	out := make([]Finding, 0, len(c.order))
	for _, k := range c.order {
		out = append(out, *c.by[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Severity > out[j].Severity })
	return out
}
