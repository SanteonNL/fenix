// types.go
package valueset

import (
	"net/http"
	"sync"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

type URLMapping struct {
	Path   string        `json:"path"`
	MaxAge time.Duration `json:"maxAge"` // Duration in hours
}

// ValueSetService handles the loading, caching, and validation of FHIR ValueSets
type ValueSetService struct {
	cache         map[string]*CachedValueSet
	urlToPath     map[string]URLMapping
	mutex         sync.RWMutex
	localPath     string
	defaultMaxAge time.Duration
	fhirClient    *http.Client
	log           zerolog.Logger
}
type ValueSetMetadata struct {
	OriginalURL string         `json:"originalUrl"`
	LastUpdated time.Time      `json:"lastUpdated"`
	ValueSet    *fhir.ValueSet `json:"valueSet"`
}

type CachedValueSet struct {
	ValueSet    *fhir.ValueSet
	LastChecked time.Time
	MaxAge      time.Duration
}

type Config struct {
	LocalPath     string
	DefaultMaxAge time.Duration // in hours
	HTTPTimeout   time.Duration // in seconds
}
type ValidationResult struct {
	Valid        bool
	MatchedIn    string
	ErrorMessage string
}

type ValueSetSource int

const (
	LocalSource ValueSetSource = iota
	RemoteSource
)

func (v ValueSetSource) String() string {
	switch v {
	case LocalSource:
		return "LocalSource"
	case RemoteSource:
		return "RemoteSource"
	default:
		return "UnknownSource"
	}
}
