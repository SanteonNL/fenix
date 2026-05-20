package source

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// SourceConstructor builds a Source from config parameters.
// name: source identifier from config (e.g., "luscii", "hix")
// config: raw config map for this source (type, base_url, dir, etc.)
// log: logger instance
type SourceConstructor func(name string, config map[string]interface{}, log zerolog.Logger) (Source, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]SourceConstructor)
)

// Register adds a source type to the registry.
// Call this from init() in each source package.
func Register(sourceType string, constructor SourceConstructor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[sourceType] = constructor
}

// Build creates a Source by looking up its type in the registry.
func Build(sourceType string, name string, config map[string]interface{}, log zerolog.Logger) (Source, error) {
	registryMu.RLock()
	constructor, ok := registry[sourceType]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown source type: %s", sourceType)
	}

	return constructor(name, config, log)
}
