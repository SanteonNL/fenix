package searchparameter

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/SanteonNL/fenix/models/fhir"
	"github.com/rs/zerolog"
)

func NewSearchParameterRepository(log zerolog.Logger) *SearchParameterRepository {
	return &SearchParameterRepository{
		searchParametersMap: make(map[string]*fhir.SearchParameter),
		log:                 log,
	}
}

// LoadSearchParametersFromFile loads search parameters from a file path
func (repo *SearchParameterRepository) LoadSearchParametersFromFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Try to load as a bundle first
	if err := repo.loadFromBundle(data); err == nil {
		repo.log.Info().
			Str("file", filePath).
			Int("count", len(repo.searchParametersMap)).
			Msg("Loaded search parameters from bundle")
		return nil
	}

	// If not a bundle, try loading as a single SearchParameter
	var searchParam fhir.SearchParameter
	if err := json.Unmarshal(data, &searchParam); err != nil {
		return fmt.Errorf("failed to unmarshal file as bundle or search parameter: %w", err)
	}

	if searchParam.Url == "" {
		return fmt.Errorf("invalid SearchParameter: missing URL")
	}

	repo.mu.Lock()
	repo.searchParametersMap[searchParam.Url] = &searchParam
	repo.mu.Unlock()

	repo.log.Debug().
		Str("file", filePath).
		Str("url", searchParam.Url).
		Msg("Loaded single search parameter")

	return nil
}

// loadFromBundle attempts to load search parameters from a FHIR bundle
func (repo *SearchParameterRepository) loadFromBundle(data []byte) error {
	var bundle fhir.Bundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return fmt.Errorf("failed to unmarshal bundle: %w", err)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()

	for _, entry := range bundle.Entry {
		if entry.Resource == nil {
			continue
		}

		// Convert the raw resource to a SearchParameter
		resourceData, err := json.Marshal(entry.Resource)
		if err != nil {
			repo.log.Error().Err(err).Msg("Failed to marshal resource")
			continue
		}

		var searchParam fhir.SearchParameter
		if err := json.Unmarshal(resourceData, &searchParam); err != nil {
			repo.log.Error().Err(err).Msg("Failed to unmarshal to SearchParameter")
			continue
		}

		if searchParam.Url == "" {
			repo.log.Warn().Msg("Skipping SearchParameter with missing URL")
			continue
		}

		// repo.log.Debug().
		// 	Str("url", searchParam.Url).
		// 	Str("code", searchParam.Code).
		// 	Str("type", searchParam.Type.String()).
		// 	Interface("base", searchParam.Base).
		// 	Msg("Loaded search parameter")

		repo.searchParametersMap[searchParam.Url] = &searchParam
	}

	return nil
}

// GetSearchParameter retrieves a search parameter by URL
func (repo *SearchParameterRepository) GetSearchParameter(url string) (*fhir.SearchParameter, error) {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	sp, exists := repo.searchParametersMap[url]
	if !exists {
		return nil, fmt.Errorf("search parameter not found: %s", url)
	}

	return sp, nil
}

// GetSearchParameterByCode retrieves a search parameter by its code and base resource
func (repo *SearchParameterRepository) GetSearchParameterByCode(code string, resourceType string) (*fhir.SearchParameter, error) {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	for _, sp := range repo.searchParametersMap {
		if sp.Code == code {
			// Check if this search parameter applies to the given resource type
			for _, base := range sp.Base {
				base := base.String()
				if base == resourceType {
					return sp, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no search parameter found for code %s and resource %s", code, resourceType)
}

// GetAllSearchParameters returns all loaded search parameters
func (repo *SearchParameterRepository) GetAllSearchParameters() []*fhir.SearchParameter {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	result := make([]*fhir.SearchParameter, 0, len(repo.searchParametersMap))
	for _, sp := range repo.searchParametersMap {
		result = append(result, sp)
	}
	return result
}

// GetSearchParametersForResource returns all search parameters applicable to a resource type
func (repo *SearchParameterRepository) GetSearchParametersForResource(resourceType string) []*fhir.SearchParameter {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	var result []*fhir.SearchParameter
	for _, sp := range repo.searchParametersMap {
		for _, base := range sp.Base {
			base := base.String()
			if base == resourceType {
				result = append(result, sp)
				break
			}
		}
	}
	return result
}

func (repo *SearchParameterRepository) DumpAllParameters() {
	repo.mu.RLock()
	defer repo.mu.RUnlock()

	fmt.Printf("\n=== All Loaded Search Parameters ===\n")
	for url, sp := range repo.searchParametersMap {
		fmt.Printf("\nSearch Parameter:\n")
		fmt.Printf("  URL: %s\n", url)
		fmt.Printf("  Code: %s\n", sp.Code)
		if sp.Expression != nil {
			fmt.Printf("  Expression: %s\n", *sp.Expression)
		}
		fmt.Printf("  Type: %s\n", sp.Type)
		fmt.Printf("  Base resources: %v\n", sp.Base)
	}
}
