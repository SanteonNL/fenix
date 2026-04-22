package searchparameter

import (
	"fmt"
	"strings"

	"github.com/SanteonNL/fenix/cmd/fenix/types"
	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// NewSearchParameterService creates a new search parameter service
func NewSearchParameterService(repo *SearchParameterRepository, log zerolog.Logger) *SearchParameterService {
	return &SearchParameterService{
		repo:        repo,
		log:         log,
		pathCodeMap: make(map[string]map[string]string), // Patient.gender -> map[code]type e.g	gender -> token
	}
}

func (svc *SearchParameterService) BuildSearchParameterIndex() error {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	// Clear existing index
	svc.pathCodeMap = make(map[string]map[string]string)

	// Get all search parameters from repository
	searchParams := svc.repo.GetAllSearchParameters()

	svc.log.Info().Int("total_params", len(searchParams)).Msg("Starting search parameter indexing")

	for _, sp := range searchParams {
		if sp.Expression == nil {
			continue
		}

		//svc.log.Debug().Str("expression", *sp.Expression).Msg("Processing Expresion")

		// Split the expression into individual paths
		paths := strings.Split(*sp.Expression, "|")
		for _, pathRaw := range paths {
			path := strings.TrimSpace(pathRaw)
			if path == "" {
				continue
			}

			// Extract resource and field
			parts := strings.Split(path, ".")
			if len(parts) < 2 {
				svc.log.Debug().
					Str("path", path).
					Msg("Skipping invalid path format")
				continue
			}

			// Create standardized path
			standardPath := parts[0] + "." + parts[1]

			// Initialize map for this path if needed
			if _, exists := svc.pathCodeMap[standardPath]; !exists {
				svc.pathCodeMap[standardPath] = make(map[string]string)
			}

			// Add this search parameter's code and type
			svc.pathCodeMap[standardPath][sp.Code] = sp.Type.String()

		}
	}

	return nil
}

// GetSearchTypeByPathAndCode gets the search type for a path and code combination
func (svc *SearchParameterService) GetSearchTypeByPathAndCode(path string, code string) (string, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	if codeMap, exists := svc.pathCodeMap[path]; exists {
		if searchType, exists := codeMap[code]; exists {
			return searchType, nil
		}
	}
	return "", fmt.Errorf("no search parameter found for path %s and code %s", path, code)
}

// GetAllSearchTypesForPath returns all search types for a path
func (svc *SearchParameterService) GetAllSearchTypesForPath(path string) map[string]string {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	if codeMap, exists := svc.pathCodeMap[path]; exists {
		// Create a copy to avoid exposing internal map
		result := make(map[string]string)
		for code, searchType := range codeMap {
			result[code] = searchType
		}
		return result
	}
	return nil
}

// GetAllPathSearchTypes returns all path-to-searchtype mappings
func (svc *SearchParameterService) GetAllPathSearchTypes() map[string]map[string]string {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	// Create a deep copy to avoid exposing internal maps
	result := make(map[string]map[string]string)
	for path, codeMap := range svc.pathCodeMap {
		result[path] = make(map[string]string)
		for code, searchType := range codeMap {
			result[path][code] = searchType
		}
	}
	return result
}

// ValidateSearchParameter validates both the search parameter and its modifier
func (svc *SearchParameterService) ValidateSearchParameter(resourceType string, paramCode string, modifier string) (*types.Filter, error) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()
	// First check if the search parameter exists and get its type
	isValid, searchType := svc.IsValidResourceSearchParameter(resourceType, paramCode)
	if !isValid {
		return &types.Filter{
			Code:      paramCode,
			Modifier:  modifier,
			IsValid:   false,
			ErrorType: "unknown-parameter",
		}, fmt.Errorf("search parameter not found: %s", paramCode)
	}

	// If there's no modifier, the parameter is valid
	if modifier == "" {
		return &types.Filter{
			Code:     paramCode,
			Modifier: modifier,
			IsValid:  true,
		}, nil
	}

	// Validate the modifier
	if err := svc.ValidateModifier(searchType, modifier); err != nil {
		return &types.Filter{
			Code:      paramCode,
			Modifier:  modifier,
			IsValid:   false,
			ErrorType: "invalid-modifier",
		}, err
	}

	// Parameter and modifier are valid
	return &types.Filter{
		Code:     paramCode,
		Modifier: modifier,
		IsValid:  true,
	}, nil
}

// ValidateModifier checks if a modifier is valid for a given search type
func (svc *SearchParameterService) ValidateModifier(searchType string, modifier string) error {
	// Convert search type to lowercase for consistency
	searchType = strings.ToLower(searchType)
	modifier = strings.ToLower(modifier)

	// Check if we have modifier definitions for this search type
	validMods, exists := ValidModifiers[searchType]
	if !exists {
		return fmt.Errorf("unknown search type: %s", searchType)
	}

	// Check if the modifier is valid for this search type
	if !validMods[modifier] {
		return fmt.Errorf("modifier '%s' is not valid for search type '%s'", modifier, searchType)
	}

	return nil
}

// IsValidResourceSearchParameter checks if a search parameter is valid for a resource type
// and returns its search type
func (svc *SearchParameterService) IsValidResourceSearchParameter(resourceType, paramCode string) (bool, string) {
	// Iterate over the pathCodeMap
	for path, codeMap := range svc.pathCodeMap {
		// Split the key to get the resource type
		parts := strings.Split(path, ".")
		if len(parts) < 2 {
			continue
		}

		// Check if the first part of the key matches the resourceType
		if parts[0] == resourceType {
			// Check if we have a search type for this code
			if searchType, hasType := codeMap[paramCode]; hasType {
				return true, searchType
			}
		}
	}

	return false, ""
}

// GetAllSearchParameters returns all search parameters from repository
func (svc *SearchParameterService) GetAllSearchParameters() []*fhir.SearchParameter {
	return svc.repo.GetAllSearchParameters()
}

// GetSearchParameterByCode retrieves a search parameter by code and resource type
func (svc *SearchParameterService) GetSearchParameterByCode(code string, resourceType string) (*fhir.SearchParameter, error) {
	return svc.repo.GetSearchParameterByCode(code, resourceType)
}

// GetSearchParametersForResource returns all search parameters for a resource type
func (svc *SearchParameterService) GetSearchParametersForResource(resourceType string) []*fhir.SearchParameter {
	return svc.repo.GetSearchParametersForResource(resourceType)
}

// ListSearchParametersForResource returns all valid search parameters for a resource type
func (svc *SearchParameterService) ListSearchParametersForResource(resourceType string) map[string][]string {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	// Map to store unique codes and their search types
	// code -> []searchTypes (a code might have different types in different paths)
	parameters := make(map[string][]string)

	for path, codeMap := range svc.pathCodeMap {
		if strings.HasPrefix(path, resourceType+".") {
			for code, searchType := range codeMap {
				// Check if we already have this code
				exists := false
				if types, ok := parameters[code]; ok {
					for _, t := range types {
						if t == searchType {
							exists = true
							break
						}
					}
					if !exists {
						parameters[code] = append(parameters[code], searchType)
					}
				} else {
					parameters[code] = []string{searchType}
				}
			}
		}
	}

	return parameters
}

// Debug function to help visualize available search parameters
func (svc *SearchParameterService) DebugResourceSearchParameters(resourceType string) {
	params := svc.ListSearchParametersForResource(resourceType)

	svc.log.Info().
		Str("resourceType", resourceType).
		Msgf("Available search parameters:")

	for code, types := range params {
		svc.log.Info().
			Str("code", code).
			Strs("types", types).
			Msg("Search parameter")
	}
}

// Helper function to debug a specific path
func (svc *SearchParameterService) DebugPath(path string) {
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	fmt.Printf("\n=== Debug info for path: %s ===\n", path)

	// Show what's in our index
	if codes, exists := svc.pathCodeMap[path]; exists {
		fmt.Printf("In index:\n")
		for code, type_ := range codes {
			fmt.Printf("  - %s: %s\n", code, type_)
		}
	} else {
		fmt.Printf("Path not found in index\n")
	}

	// Check all search parameters for this path
	searchParams := svc.repo.GetAllSearchParameters()
	fmt.Printf("\nChecking all search parameters:\n")
	for _, sp := range searchParams {
		if sp.Expression == nil {
			continue
		}
		if strings.Contains(*sp.Expression, path) {
			fmt.Printf("\nFound in search parameter:\n")
			fmt.Printf("  Code: %s\n", sp.Code)
			fmt.Printf("  Type: %s\n", sp.Type)
			fmt.Printf("  URL: %s\n", sp.Url)
			fmt.Printf("  Expression: %s\n", *sp.Expression)
		}
	}
}
