// service.go
package valueset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
	"github.com/rs/zerolog"
)

func NewValueSetService(config Config, log zerolog.Logger) (*ValueSetService, error) {
	if config.LocalPath == "" {
		return nil, fmt.Errorf("local path is required")
	}

	// Set defaults if not provided
	if config.DefaultMaxAge == 0 {
		config.DefaultMaxAge = 24 * time.Hour // Default to 24 hours
	}
	if config.HTTPTimeout == 0 {
		config.HTTPTimeout = 30 * time.Second // Default to 30 seconds
	}

	// Ensure local directory exists
	if err := os.MkdirAll(config.LocalPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create local storage directory: %w", err)
	}

	service := &ValueSetService{
		cache:         make(map[string]*CachedValueSet),
		urlToPath:     make(map[string]URLMapping),
		localPath:     config.LocalPath,
		log:           log,
		defaultMaxAge: config.DefaultMaxAge,
		fhirClient:    &http.Client{Timeout: config.HTTPTimeout},
	}

	// Load URL mappings first
	if err := service.loadURLMappings(); err != nil {
		log.Error().Err(err).Msg("Failed to load URL mappings")
	}

	// Then load all ValueSets
	if err := service.loadAllFromDisk(); err != nil {
		log.Error().Err(err).Msg("Failed to load ValueSets from disk")
	}

	return service, nil
}

func (s *ValueSetService) GetValueSet(ctx context.Context, url string) (*fhir.ValueSet, error) {
	valueSetID, source := s.parseValueSetURL(url)

	s.log.Debug().
		Str("originalURL", url).
		Str("valueSetID", valueSetID).
		Str("source", source.String()).
		Msg("Resolving ValueSet source")

	// Try cache first
	s.mutex.RLock()
	cached, exists := s.cache[valueSetID]
	s.mutex.RUnlock()

	if exists {
		// Check if cache is still valid
		if !s.isCacheExpired(valueSetID, cached) {
			return cached.ValueSet, nil
		}
		s.log.Debug().Str("valueSetID", valueSetID).Msg("Cache expired, refreshing")
	}

	// Try local storage first
	valueSet, err := s.fetchFromLocal(valueSetID)
	if err == nil && valueSet != nil {
		// Check if local storage is still valid
		if !s.isLocalStorageExpired(valueSetID) {
			s.updateCache(valueSetID, valueSet)
			return valueSet, nil
		}
		s.log.Debug().Str("valueSetID", valueSetID).Msg("Local storage expired, trying remote")
	}

	// If local storage failed or expired, try remote for RemoteSource
	if source == RemoteSource {
		valueSet, err = s.fetchFromRemote(ctx, valueSetID)
		if err != nil {
			// If remote fails but we have expired cache/local, use that instead
			if exists {
				s.log.Warn().Err(err).Str("valueSetID", valueSetID).Msg("Remote fetch failed, using expired cache")
				return cached.ValueSet, nil
			}
			return nil, fmt.Errorf("failed to fetch ValueSet from remote: %w", err)
		}

		// Save successful remote fetch to disk and cache
		if err := s.saveToDisk(valueSetID, valueSet); err != nil {
			s.log.Error().Err(err).Str("valueSetID", valueSetID).Msg("Failed to save ValueSet to disk")
		}
		s.updateCache(valueSetID, valueSet)
		return valueSet, nil
	}

	return nil, fmt.Errorf("failed to fetch ValueSet: %s", valueSetID)
}

func (s *ValueSetService) isCacheExpired(valueSetID string, cached *CachedValueSet) bool {
	s.mutex.RLock()
	mapping, exists := s.urlToPath[valueSetID]
	s.mutex.RUnlock()

	maxAge := 24 * time.Hour // Default max age
	if exists && mapping.MaxAge > 0 {
		maxAge = mapping.MaxAge * time.Hour
	}

	return time.Since(cached.LastChecked) > maxAge
}

func (s *ValueSetService) isLocalStorageExpired(valueSetID string) bool {
	s.mutex.RLock()
	mapping, exists := s.urlToPath[valueSetID]
	s.mutex.RUnlock()

	if !exists {
		return true
	}

	filePath := filepath.Join(s.localPath, mapping.Path)
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return true
	}

	maxAge := 24 * time.Hour // Default max age
	if mapping.MaxAge > 0 {
		maxAge = mapping.MaxAge * time.Hour
	}

	return time.Since(fileInfo.ModTime()) > maxAge
}

func (s *ValueSetService) updateCache(valueSetID string, valueSet *fhir.ValueSet) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.cache[valueSetID] = &CachedValueSet{
		ValueSet:    valueSet,
		LastChecked: time.Now(),
	}
}

func (s *ValueSetService) parseValueSetURL(url string) (string, ValueSetSource) {
	url = strings.TrimPrefix(url, "ValueSet/")
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url, RemoteSource
	}
	return url, LocalSource
}

func (s *ValueSetService) fetchFromRemote(ctx context.Context, url string) (*fhir.ValueSet, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Accept", "application/json")

	resp, err := s.fhirClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var valueSet fhir.ValueSet
	if err := json.Unmarshal(bodyBytes, &valueSet); err != nil {
		return nil, fmt.Errorf("failed to decode ValueSet: %w", err)
	}

	return &valueSet, nil
}

// Fetch ValueSet from local storage
func (s *ValueSetService) fetchFromLocal(valueSetID string) (*fhir.ValueSet, error) {
	s.mutex.RLock()
	URLMapping, exists := s.urlToPath[valueSetID]
	s.mutex.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no local file mapping found for ValueSet: %s", valueSetID)
	}

	filePath := filepath.Join(s.localPath, URLMapping.Path)
	return s.loadValueSetFromDisk(filePath)
}

func (s *ValueSetService) loadValueSetFromDisk(filePath string) (*fhir.ValueSet, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var metadata ValueSetMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		// Try loading as plain ValueSet for backwards compatibility
		var valueSet fhir.ValueSet
		if err := json.Unmarshal(data, &valueSet); err != nil {
			return nil, fmt.Errorf("failed to parse ValueSet: %w", err)
		}
		return &valueSet, nil
	}

	return metadata.ValueSet, nil
}

func (s *ValueSetService) ValidateCode(ctx context.Context, valueSetURL string, coding *fhir.Coding) (*ValidationResult, error) {
	processedURLs := sync.Map{}
	return s.validateCodeRecursive(ctx, valueSetURL, coding, &processedURLs)
}

func (s *ValueSetService) validateCodeRecursive(ctx context.Context, valueSetURL string, coding *fhir.Coding, processedURLs *sync.Map) (*ValidationResult, error) {
	if _, exists := processedURLs.Load(valueSetURL); exists {
		return nil, fmt.Errorf("circular reference detected in ValueSet: %s", valueSetURL)
	}
	processedURLs.Store(valueSetURL, true)

	// Get the ValueSet
	valueSet, err := s.GetValueSet(ctx, valueSetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ValueSet %s: %w", valueSetURL, err)
	}

	// First check the direct concepts in this ValueSet
	result := s.validateDirectConcepts(valueSet, coding)
	if result.Valid {
		return result, nil
	}

	// If no direct match found and compose section exists, check included ValueSets
	if valueSet.Compose != nil {
		// Create a channel for results from included ValueSets
		type includeResult struct {
			result *ValidationResult
			err    error
		}
		results := make(chan includeResult)

		// Process each include in parallel
		var wg sync.WaitGroup
		for _, include := range valueSet.Compose.Include {
			if include.ValueSet == nil || len(include.ValueSet) == 0 {
				continue
			}

			for _, includeValueSetURL := range include.ValueSet {
				wg.Add(1)
				go func(url string) {
					defer wg.Done()
					res, err := s.validateCodeRecursive(ctx, url, coding, processedURLs)
					results <- includeResult{result: res, err: err}
				}(includeValueSetURL)
			}
		}

		// Close the results channel when all goroutines are done
		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results
		for res := range results {
			if res.err != nil {
				s.log.Warn().Err(res.err).Msg("Error validating code in included ValueSet")
				continue
			}
			if res.result.Valid {
				return res.result, nil
			}
		}
	}

	return &ValidationResult{
		Valid:        false,
		ErrorMessage: fmt.Sprintf("Code not found in ValueSet %s", valueSetURL),
	}, nil
}

func (s *ValueSetService) validateDirectConcepts(valueSet *fhir.ValueSet, coding *fhir.Coding) *ValidationResult {
	var codingSystem, codingCode string
	if coding.System != nil {
		codingSystem = *coding.System
	}
	if coding.Code != nil {
		codingCode = *coding.Code
	}

	for _, include := range valueSet.Compose.Include {
		if include.System != nil && *include.System != codingSystem {
			continue
		}

		for _, concept := range include.Concept {
			if concept.Code == codingCode {
				return &ValidationResult{
					Valid:     true,
					MatchedIn: *valueSet.Url,
				}
			}
		}
	}

	return &ValidationResult{
		Valid: false,
	}
}

// getValueSetFilename generates a unique filename based on name and URL hash
func (s *ValueSetService) getValueSetFilename(valueSet *fhir.ValueSet) string {
	// Get base name from ValueSet
	var baseName string
	if valueSet.Title != nil && *valueSet.Title != "" {
		baseName = *valueSet.Title
	} else if valueSet.Id != nil && *valueSet.Id != "" {
		baseName = *valueSet.Id
	} else {
		baseName = "unnamed_valueset"
	}

	// Clean the base name
	baseName = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == ' ' || r == '-' || r == '_':
			return '_'
		default:
			return -1
		}
	}, baseName)

	// Generate URL hash
	hasher := sha256.New()
	hasher.Write([]byte(*valueSet.Url))
	urlHash := hex.EncodeToString(hasher.Sum(nil))[:8] // Use first 8 characters of hash

	// Combine name and hash
	return fmt.Sprintf("%s_%s.json", baseName, urlHash)
}

// WriteNewValueSet writes a newly downloaded ValueSet to disk and updates mappings
func (s *ValueSetService) WriteNewValueSet(ctx context.Context, valueSet *fhir.ValueSet) error {
	if valueSet.Url == nil {
		return fmt.Errorf("valueset URL is required")
	}

	// Generate unique filename
	filename := s.getValueSetFilename(valueSet)

	// Create metadata wrapper
	metadata := ValueSetMetadata{
		OriginalURL: *valueSet.Url,
		LastUpdated: time.Now(),
		ValueSet:    valueSet,
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ValueSet: %w", err)
	}

	// Write ValueSet to disk
	filePath := filepath.Join(s.localPath, filename)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write ValueSet file: %w", err)
	}

	// Update URL mappings
	s.mutex.Lock()
	s.urlToPath[*valueSet.Url] = URLMapping{
		Path:   filename,
		MaxAge: s.defaultMaxAge / time.Hour, // Convert duration to hours
	}
	s.mutex.Unlock()

	// Save updated URL mappings
	if err := s.saveURLMappings(); err != nil {
		return fmt.Errorf("failed to save URL mappings: %w", err)
	}

	s.log.Info().
		Str("url", *valueSet.Url).
		Str("filename", filename).
		Msg("Saved ValueSet to disk")

	return nil
}

func (s *ValueSetService) loadURLMappings() error {
	mappingPath := filepath.Join(s.localPath, "url-mappings.json")

	// Check if file exists
	if _, err := os.Stat(mappingPath); os.IsNotExist(err) {
		s.log.Info().Msg("No url-mappings.json found, starting with empty mapping")
		return nil
	}

	data, err := os.ReadFile(mappingPath)
	if err != nil {
		return fmt.Errorf("failed to read URL mappings: %w", err)
	}

	var mappings map[string]URLMapping
	if err := json.Unmarshal(data, &mappings); err != nil {
		return fmt.Errorf("failed to parse URL mappings: %w", err)
	}

	// Convert maxAge to duration
	for url, mapping := range mappings {
		if mapping.MaxAge == 0 {
			mapping.MaxAge = s.defaultMaxAge / time.Hour
		}
		mappings[url] = mapping
	}

	s.mutex.Lock()
	s.urlToPath = mappings
	s.mutex.Unlock()

	s.log.Info().
		Int("mappingsCount", len(mappings)).
		Msg("Loaded URL mappings")

	return nil
}

func (s *ValueSetService) saveURLMappings() error {
	s.mutex.RLock()
	mappings := make(map[string]URLMapping, len(s.urlToPath))
	for k, v := range s.urlToPath {
		mappings[k] = v
	}
	s.mutex.RUnlock()

	data, err := json.MarshalIndent(mappings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal URL mappings: %w", err)
	}

	mappingPath := filepath.Join(s.localPath, "url-mappings.json")
	if err := os.WriteFile(mappingPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write URL mappings: %w", err)
	}

	return nil
}

// The saveToDisk method should be updated to use the new URL mapping approach
func (s *ValueSetService) saveToDisk(valueSetID string, valueSet *fhir.ValueSet) error {
	// Always use getValueSetFilename for consistent file naming
	filename := s.getValueSetFilename(valueSet)

	metadata := ValueSetMetadata{
		OriginalURL: valueSetID,
		LastUpdated: time.Now(),
		ValueSet:    valueSet,
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ValueSet: %w", err)
	}

	filePath := filepath.Join(s.localPath, filename)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	s.mutex.Lock()
	s.urlToPath[valueSetID] = URLMapping{
		Path:   filename,
		MaxAge: s.defaultMaxAge / time.Hour,
	}
	s.mutex.Unlock()

	// Save the updated mappings
	if err := s.saveURLMappings(); err != nil {
		s.log.Error().Err(err).Msg("Failed to save URL mappings")
	}

	s.log.Info().
		Str("url", valueSetID).
		Str("filename", filename).
		Msg("Saved ValueSet to disk")

	return nil
}
