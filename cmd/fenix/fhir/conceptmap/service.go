package conceptmap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

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

func stringPtr(s string) *string {
	return &s
}
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
		conceptMap, err := s.repo.GetConceptMap(url)
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

func (s *ConceptMapService) CreateConceptMap(id string, name string, sourceValueSet string, targetValueSet string) *fhir.ConceptMap {
	url := fmt.Sprintf("http://localhost/fhir/ConceptMap/%s", id)

	return &fhir.ConceptMap{
		Id:        &id,
		Url:       &url,
		Name:      &name,
		Status:    1,
		Date:      stringPtr(time.Now().Format(time.RFC3339)),
		SourceUri: &sourceValueSet,
		TargetUri: &targetValueSet,
		Group:     []fhir.ConceptMapGroup{},
	}
}

// Add this method to your existing conceptmap/service.go file

func (s *ConceptMapService) SaveConceptMap(outputPath string, cm *fhir.ConceptMap) error {
	data, err := json.MarshalIndent(cm, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ConceptMap: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		s.log.Error().Err(err).Str("path", outputPath).Msg("Failed to write ConceptMap to file")
		return fmt.Errorf("failed to write ConceptMap file: %w", err)
	}

	//s.log.Debug().Str("path", outputPath).Msg("Successfully saved ConceptMap")
	return nil
}

// getDisplayValue returns the display value if it is not nil, otherwise returns an empty string
func getDisplayValue(display *string) string {
	if display != nil {
		return *display
	}
	return ""
}

func (svc *ConceptMapService) GetConceptMapsByValuesetURL(valueSetURL string) ([]string, error) {
	return svc.repo.GetConceptMapsByValuesetURL(valueSetURL)
}

// Helper function to extract version from ConceptMap
func getVersionFromConceptMap(cm *fhir.ConceptMap) string {
	if cm.Version != nil {
		return *cm.Version
	}
	return ""
}

// GetRepository returns the ConceptMapRepository
func (s *ConceptMapService) GetRepository() *ConceptMapRepository {
	return s.repo
}
