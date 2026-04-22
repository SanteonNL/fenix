// internal/fhir/conceptmap/types.go
package conceptmap

import (
	"strings"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// ValidationResult represents the result of code validation against a ValueSet
type ValidationResult struct {
	Valid        bool
	MatchedIn    string // Which ValueSet contained the match
	ErrorMessage string
}

// TranslationResult represents the result of code translation
type TranslationResult struct {
	TargetCode    string
	TargetDisplay string
}

// ConceptMapMetadata contains metadata about a stored ConceptMap
// ConceptMapMetadata contains metadata about a stored ConceptMap
type ConceptMapMetadata struct {
	ID          string
	Version     string
	LastUpdated time.Time
	SourceURI   string
	TargetURI   string
	ConceptMap  *fhir.ConceptMap
}

// CSVMapping represents a row in a concept mapping CSV file
type CSVMapping struct {
	SourceSystem  string
	SourceCode    string
	SourceDisplay string
	TargetSystem  string
	TargetCode    string
	TargetDisplay string
	IsValid       bool
	ValueSetURI   string
}

// columnIndices helps track CSV column positions
type columnIndices struct {
	sourceSystem  int
	sourceCode    int
	sourceDisplay int
	targetSystem  int
	targetCode    int
	targetDisplay int
	valueSetURI   int
}

// getColumnIndices finds the indices of required columns in CSV headers
func getColumnIndices(headers []string) columnIndices {
	return columnIndices{
		sourceSystem:  findColumn(headers, "system_source"),
		sourceCode:    findColumn(headers, "code_source"),
		sourceDisplay: findColumn(headers, "display_source"),
		targetSystem:  findColumn(headers, "system_target"),
		targetCode:    findColumn(headers, "code_target"),
		targetDisplay: findColumn(headers, "display_target"),
		valueSetURI:   findColumn(headers, "target_valueset_uri"),
	}
}

// findColumn finds the index of a column by name
func findColumn(headers []string, name string) int {
	for i, h := range headers {
		if strings.EqualFold(strings.TrimSpace(h), name) {
			return i
		}
	}
	return -1
}

// areValid checks if required column indices were found
func (ci columnIndices) areValid() bool {
	return ci.sourceSystem != -1 &&
		ci.sourceCode != -1 &&
		ci.targetSystem != -1 &&
		ci.targetCode != -1
}

// maxIndex returns the highest column index
func (ci columnIndices) maxIndex() int {
	max := ci.sourceSystem
	indices := []int{
		ci.sourceCode,
		ci.sourceDisplay,
		ci.targetSystem,
		ci.targetCode,
		ci.targetDisplay,
		ci.valueSetURI,
	}

	for _, idx := range indices {
		if idx > max {
			max = idx
		}
	}
	return max
}
