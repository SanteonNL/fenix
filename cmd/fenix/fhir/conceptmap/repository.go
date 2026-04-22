package conceptmap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

// ConceptMapRepository handles loading and storing ConceptMap resources.
type ConceptMapRepository struct {
	log         zerolog.Logger
	localPath   string
	cache       sync.Map
	conceptMaps map[string]fhir.ConceptMap
}

// NewConceptMapRepository creates a new ConceptMapRepository.
func NewConceptMapRepository(localPath string, log zerolog.Logger) *ConceptMapRepository {
	return &ConceptMapRepository{
		log:         log,
		localPath:   localPath,
		conceptMaps: make(map[string]fhir.ConceptMap),
	}
}

// LoadConceptMaps loads all ConceptMaps into the repository.
func (repo *ConceptMapRepository) LoadConceptMaps() error {
	files, err := os.ReadDir(repo.localPath)
	if err != nil {
		repo.log.Error().Err(err).Msg("Failed to read directory")
		return fmt.Errorf("failed to read directory: %w", err)
	}

	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			filePath := filepath.Join(repo.localPath, file.Name())
			repo.log.Debug().Str("filePath", filePath).Msg("Loading ConceptMap file")

			conceptMap, err := repo.loadConceptMapFile(filePath)
			if err != nil {
				repo.log.Error().
					Err(err).
					Str("file", file.Name()).
					Msg("Failed to load ConceptMap file")
				continue
			}

			if conceptMap.Url != nil {
				repo.cache.Store(*conceptMap.Url, conceptMap)
				repo.log.Debug().
					Str("id", *conceptMap.Url).
					Msg("Loaded ConceptMap into cache by Url")
			} else {
				repo.log.Warn().
					Str("file", file.Name()).
					Msg("ConceptMap has no Url")
			}

			if conceptMap.TargetUri != nil {
				repo.cache.Store(*conceptMap.TargetUri, conceptMap)
				repo.log.Debug().
					Str("targetUri", *conceptMap.TargetUri).
					Msg("Loaded ConceptMap into cache by TargetUri")
			} else {
				repo.log.Warn().
					Str("file", file.Name()).
					Msg("ConceptMap has no TargetUri")
			}
		}
	}

	repo.log.Info().Msg("Finished loading ConceptMaps from disk")
	return nil
}

// loadConceptMapFile loads a ConceptMap from a file.
func (repo *ConceptMapRepository) loadConceptMapFile(filePath string) (*fhir.ConceptMap, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		repo.log.Fatal().Err(err).Str("path", filePath).Msg("Failed to read ConceptMap file")
		return nil, fmt.Errorf("failed to read ConceptMap file: %w", err)
	}

	var conceptMap fhir.ConceptMap
	if err := json.Unmarshal(data, &conceptMap); err != nil {
		return nil, fmt.Errorf("failed to parse ConceptMap: %w", err)
	}

	return &conceptMap, nil
}

// GetConceptMap retrieves a ConceptMap by ID or URL.
func (repo *ConceptMapRepository) GetConceptMap(url string) (*fhir.ConceptMap, error) {

	// Try cache first
	if cached, ok := repo.cache.Load(url); ok {
		return cached.(*fhir.ConceptMap), nil
	}

	fileName, err := repo.GetConceptMapFileNameByURL(url)
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(repo.localPath, fileName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		repo.log.Error().Err(err).Str("path", filePath).Msg("Failed to read ConceptMap file")
		return nil, fmt.Errorf("failed to read ConceptMap file: %w", err)
	}

	var conceptMap fhir.ConceptMap
	if err := json.Unmarshal(data, &conceptMap); err != nil {
		return nil, fmt.Errorf("failed to parse ConceptMap: %w", err)
	}
	repo.cache.Store(url, &conceptMap)
	return &conceptMap, nil
}

// GetConceptMapsByValuesetURL retrieves all ConceptMaps with a target URI matching the input URL.
func (repo *ConceptMapRepository) GetConceptMapsByValuesetURL(valueSetURL string) ([]string, error) {
	var matchingConceptMapURLs []string

	repo.cache.Range(func(key, value interface{}) bool {
		conceptMap := value.(*fhir.ConceptMap)
		if conceptMap.TargetUri != nil && *conceptMap.TargetUri == valueSetURL {
			matchingConceptMapURLs = append(matchingConceptMapURLs, *conceptMap.Url)
		}
		return true
	})

	if len(matchingConceptMapURLs) == 0 {
		repo.log.Warn().Str("valueSetURL", valueSetURL).Msg("No ConceptMaps found for ValueSet URL")
		return nil, fmt.Errorf("no ConceptMaps found for ValueSet URL: %s", valueSetURL)
	}

	return matchingConceptMapURLs, nil
}

// Helper function to get the file name for a ConceptMap by ID or URL.
func (repo *ConceptMapRepository) getFileName(key string) string {
	return fmt.Sprintf("%s.json", key)
}

// GetConceptMapFileNameByURL returns the filename of a ConceptMap based on its URL
func (repo *ConceptMapRepository) GetConceptMapFileNameByURL(url string) (string, error) {
	var matchingFileName string
	err := filepath.Walk(repo.localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-JSON files
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}

		conceptMap, err := repo.loadConceptMapFile(path)
		if err != nil {
			repo.log.Warn().
				Err(err).
				Str("file", info.Name()).
				Str("path", path).
				Msg("Failed to load ConceptMap file while searching")
			return nil // Continue walking even if one file fails
		}

		// Check both URL and TargetUri
		if (conceptMap.Url != nil && *conceptMap.Url == url) ||
			(conceptMap.TargetUri != nil && *conceptMap.TargetUri == url) {
			repo.log.Debug().
				Str("url", url).
				Str("filename", info.Name()).
				Msg("Found matching ConceptMap file")
			matchingFileName = info.Name()
			return filepath.SkipAll // Stop walking once we find a match
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("error walking directory: %w", err)
	}

	if matchingFileName == "" {
		return "", fmt.Errorf("no ConceptMap file found for URL: %s", url)
	}

	return matchingFileName, nil
}
