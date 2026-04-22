package searchparameter

import (
	"sync"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// ValidModifiers defines allowed modifiers for each search type according to FHIR spec
var ValidModifiers = map[string]map[string]bool{
	"number": {
		"eq": true,
		"ne": true,
		"gt": true,
		"lt": true,
		"ge": true,
		"le": true,
		"sa": true,
		"eb": true,
		"ap": true,
	},
	"date": {
		"eq": true,
		"ne": true,
		"gt": true,
		"lt": true,
		"ge": true,
		"le": true,
		"sa": true,
		"eb": true,
	},
	"string": {
		"contains": true,
		"exact":    true,
		"missing":  true,
	},
	"token": {
		"text":    true,
		"not":     true,
		"above":   true,
		"below":   true,
		"in":      true,
		"not-in":  true,
		"of-type": true,
	},
	"reference": {
		"above": true,
		"below": true,
	},
	"composite": {}, // Composite type doesn't support modifiers
	"quantity": {
		"eq": true,
		"ne": true,
		"gt": true,
		"lt": true,
		"ge": true,
		"le": true,
		"sa": true,
		"eb": true,
		"ap": true,
	},
	"uri": {
		"below": true,
		"above": true,
	},
	"special": {}, // Special type typically doesn't support modifiers
}

type SearchParameterRepository struct {
	searchParametersMap map[string]*fhir.SearchParameter // URL -> SearchParameter
	mu                  sync.RWMutex
	log                 zerolog.Logger
}

type SearchParamInfo struct {
	Type string
	Code string
	Base []string
}

// SearchParameterService manages search parameter operations and indexing
type SearchParameterService struct {
	repo        *SearchParameterRepository
	log         zerolog.Logger
	pathCodeMap map[string]map[string]string
	mu          sync.RWMutex
}
