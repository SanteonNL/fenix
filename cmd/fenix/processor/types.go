// types.go
package processor

import (
	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// Filter represents the basic filter input
type Filter struct {
	Code      string // e.g., "code"
	Value     string // e.g., "8480-6"
	Modifier  string // e.g., "in"
	IsValid   bool
	ErrorType string
}

// FilterResult represents the outcome of a filter check
type FilterResult struct {
	Passed  bool
	Message string
}

// ResourceFactoryMap maps resource types to their factory functions
var ResourceFactoryMap = map[string]func() interface{}{
	"Patient":      func() interface{} { return &fhir.Patient{} },
	"Observation":  func() interface{} { return &fhir.Observation{} },
	"Encounter":    func() interface{} { return &fhir.Encounter{} },
	"Condition":    func() interface{} { return &fhir.Condition{} },
	"Procedure":    func() interface{} { return &fhir.Procedure{} },
	"Immunization": func() interface{} { return &fhir.Immunization{} },
}
