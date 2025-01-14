package conceptmap

import (
	"fmt"

	"github.com/SanteonNL/fenix/models/fhir"
	"github.com/rs/zerolog"
)

// ConceptMapService provides functionality to interact with ConceptMap resources.
type ConceptMapService struct {
	repo *ConceptMapRepository
	log  zerolog.Logger
}

// NewConceptMapService creates a new ConceptMapService.
func NewConceptMapService(repo *ConceptMapRepository, log zerolog.Logger) *ConceptMapService {
	return &ConceptMapService{
		repo: repo,
		log:  log,
	}
}

// GetConceptMapForValueSet retrieves the ConceptMap for a given ValueSet URL.
// TODO not sure if this function should be here
func stringPtr(s string) *string {
	return &s
}

// TODO think about a better name for this method, as not only codes are translated
// It is also not clear if it is about code/coding/quantity, but also not because a coding contains a system code and display
func (s *ConceptMapService) TranslateCode(conceptMapURLs []string, sourceCode string, typeIsCode bool) (*TranslationResult, error) {
	if len(conceptMapURLs) == 0 || sourceCode == "" {
		return nil, fmt.Errorf("at least one conceptMap URL and sourceCode are required")
	}

	s.log.Debug().
		Str("sourceCode", sourceCode).
		Bool("typeIsCode", typeIsCode).
		Msg("Starting code translation")

	// Try each concept map
	for _, url := range conceptMapURLs {
		conceptMap, err := s.repo.GetOrLoadConceptMap(url)
		if err != nil {
			s.log.Debug().Err(err).Str("url", url).Msg("Failed to get concept map, trying next")
			continue
		}

		// Try normal mapping first
		result := s.findDirectMapping(conceptMap, sourceCode)
		if result != nil {
			return result, nil
		}

		// Try default mapping for code types
		if typeIsCode {
			result := s.findDefaultMapping(conceptMap)
			if result != nil {
				return result, nil
			}
		}
	}

	// No valid translation found in any concept map
	return nil, nil
}

// TODO: Change name to something more descriptive
func (s *ConceptMapService) findDirectMapping(conceptMap *fhir.ConceptMap, sourceCode string) *TranslationResult {
	for _, group := range conceptMap.Group {
		for _, element := range group.Element {
			if element.Code != nil && *element.Code == sourceCode {
				for _, target := range element.Target {
					if target.Code != nil {
						return &TranslationResult{
							TargetCode:    *target.Code,
							TargetDisplay: getDisplayValue(target.Display),
						}
					}
				}
			}
		}
	}
	return nil
}

// TODO: change name, it is not a default mapping but a wildcard mapping
func (s *ConceptMapService) findDefaultMapping(conceptMap *fhir.ConceptMap) *TranslationResult {
	for _, group := range conceptMap.Group {
		for _, element := range group.Element {
			if element.Code != nil && *element.Code == "*" {
				for _, target := range element.Target {
					if target.Code != nil {
						return &TranslationResult{
							TargetCode:    *target.Code,
							TargetDisplay: getDisplayValue(target.Display),
						}
					}
				}
			}
		}
	}
	return nil
}

// getDisplayValue returns the display value if it is not nil, otherwise returns an empty string
func getDisplayValue(display *string) string {
	if display != nil {
		return *display
	}
	return ""
}

func (svc *ConceptMapService) GetConceptMapURLsByValuesetURL(valueSetURL string) ([]string, error) {
	return svc.repo.GetConceptMapURLsByValuesetURL(valueSetURL)
}
