package structuredefinition

import (
	"fmt"
	"strings"
	"sync"

	"github.com/SanteonNL/fenix/models/fhir"
	"github.com/rs/zerolog"
)

// StructureDefinitionService manages structure definition operations and indexing
type StructureDefinitionService struct {
	repo            *StructureDefinitionRepository
	log             zerolog.Logger
	pathBindingsMap map[string]string // Maps paths to ValueSet URLs
	mu              sync.RWMutex
}

// NewStructureDefinitionService creates a new structure definition service
func NewStructureDefinitionService(repo *StructureDefinitionRepository, log zerolog.Logger) *StructureDefinitionService {
	return &StructureDefinitionService{
		repo:            repo,
		log:             log,
		pathBindingsMap: make(map[string]string),
	}
}

// BuildStructureDefinitionIndex builds the structure definition index for efficient lookups
func (svc *StructureDefinitionService) BuildStructureDefinitionIndex() error {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	// Clear existing index
	svc.pathBindingsMap = make(map[string]string)

	// Get all structure definitions from repository
	structDefs := svc.repo.GetAllStructureDefinitions()

	for _, sd := range structDefs {
		// Process each element for bindings
		if sd.Snapshot != nil {
			for _, element := range sd.Snapshot.Element {
				if element.Binding != nil && element.Binding.ValueSet != nil {
					path := element.Path
					valueSetUrl := *element.Binding.ValueSet

					svc.pathBindingsMap[path] = valueSetUrl

					// svc.log.Debug().
					// 	Str("path", path).
					// 	Str("valueSet", valueSetUrl).
					// 	Msg("Indexed path binding")
				}
			}
		}
	}

	svc.log.Info().
		Int("total_bindings", len(svc.pathBindingsMap)).
		Int("total_structdefs", len(structDefs)).
		Msg("Completed building structure definition index")

	return nil
}

// GetAllStructureDefinitions returns all structure definitions from repository
func (svc *StructureDefinitionService) GetAllStructureDefinitions() []*fhir.StructureDefinition {
	return svc.repo.GetAllStructureDefinitions()
}

// GetAllPathBindings returns all path bindings for a structure definition
func (svc *StructureDefinitionService) GetAllPathBindings(sd *fhir.StructureDefinition) map[string]string {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	result := make(map[string]string)
	prefix := sd.Type + "."

	// Only include paths that belong to this structure definition
	for path, valueSet := range svc.pathBindingsMap {
		if path == sd.Type || strings.HasPrefix(path, prefix) {
			result[path] = valueSet
		}
	}

	return result
}

// GetStructureDefinition retrieves a structure definition by URL or name
func (svc *StructureDefinitionService) GetStructureDefinition(identifier string) (*fhir.StructureDefinition, error) {
	return svc.repo.GetStructureDefinition(identifier)
}

// GetValueSetForPath returns the ValueSet URL for a given path
func (svc *StructureDefinitionService) GetBindingValueSet(path string) (string, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	if valueSet, exists := svc.pathBindingsMap[path]; exists {
		return valueSet, nil
	}
	return "", fmt.Errorf("no ValueSet binding found for path: %s", path)
}
