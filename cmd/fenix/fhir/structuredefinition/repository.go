package structuredefinition

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// StructureDefinitionRepository handles loading and storing StructureDefinition resources.
type StructureDefinitionRepository struct {
	log                     zerolog.Logger
	structureDefinitionsMap map[string]*fhir.StructureDefinition
	mu                      sync.RWMutex
}

// NewStructureDefinitionRepository creates a new StructureDefinitionRepository.
func NewStructureDefinitionRepository(log zerolog.Logger) *StructureDefinitionRepository {
	return &StructureDefinitionRepository{
		log:                     log,
		structureDefinitionsMap: make(map[string]*fhir.StructureDefinition),
		mu:                      sync.RWMutex{},
	}
}

// LoadStructureDefinitions loads all StructureDefinitions from a directory
func (repo *StructureDefinitionRepository) LoadStructureDefinitions(dirPath string) error {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	var loadErrors []error
	loaded := 0

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(dirPath, file.Name())
		sd, err := repo.loadStructureDefinitionFile(filePath)
		if err != nil {
			loadErrors = append(loadErrors, fmt.Errorf("error loading %s: %w", file.Name(), err))
			repo.log.Error().Err(err).
				Str("file", file.Name()).
				Msg("Failed to load StructureDefinition")
			continue
		}

		if sd != nil {
			repo.mu.Lock()
			// Store by both URL and Name for flexible lookup
			repo.structureDefinitionsMap[sd.Url] = sd
			if sd.Name != "" {
				repo.structureDefinitionsMap[sd.Name] = sd
			}
			repo.mu.Unlock()

			loaded++
			repo.log.Debug().
				Str("file", file.Name()).
				Str("name", sd.Name).
				Str("url", sd.Url).
				Msg("Loaded StructureDefinition")
		}
	}

	// Log summary
	repo.log.Info().
		Int("total_files", len(files)).
		Int("loaded", loaded).
		Int("errors", len(loadErrors)).
		Str("directory", dirPath).
		Msg("Completed loading StructureDefinitions")

	if len(loadErrors) > 0 {
		return fmt.Errorf("encountered %d errors while loading StructureDefinitions", len(loadErrors))
	}

	return nil
}

// loadStructureDefinitionFile loads a single StructureDefinition file
func (repo *StructureDefinitionRepository) loadStructureDefinitionFile(filePath string) (*fhir.StructureDefinition, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	sd, err := fhir.UnmarshalStructureDefinition(data)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal: %w", err)
	}

	if sd.Url == "" {
		return nil, fmt.Errorf("invalid StructureDefinition: missing URL")
	}

	return &sd, nil
}

// GetStructureDefinition retrieves a StructureDefinition by URL or name
func (repo *StructureDefinitionRepository) GetStructureDefinition(identifier string) (*fhir.StructureDefinition, error) {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	sd, exists := repo.structureDefinitionsMap[identifier]
	if !exists {
		return nil, fmt.Errorf("StructureDefinition not found: %s", identifier)
	}
	return sd, nil
}

// GetAllStructureDefinitions returns all loaded StructureDefinitions
func (repo *StructureDefinitionRepository) GetAllStructureDefinitions() []*fhir.StructureDefinition {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	// Create a set to deduplicate (since we store by both URL and name)
	seen := make(map[string]bool)
	result := make([]*fhir.StructureDefinition, 0, len(repo.structureDefinitionsMap))

	for _, sd := range repo.structureDefinitionsMap {
		if !seen[sd.Url] {
			seen[sd.Url] = true
			result = append(result, sd)
		}
	}

	return result
}
