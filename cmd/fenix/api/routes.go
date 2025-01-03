package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/SanteonNL/fenix/cmd/fenix/datasource"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/bundle"
	"github.com/SanteonNL/fenix/cmd/fenix/fhir/searchparameter"
	"github.com/SanteonNL/fenix/cmd/fenix/processor"
	"github.com/SanteonNL/fenix/cmd/fenix/types"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Add bundleCache to FHIRRouter struct
type FHIRRouter struct {
	searchParamService *searchparameter.SearchParameterService
	processorService   *processor.ProcessorService
	bundleService      *bundle.BundleService
	dataSourceService  *datasource.DataSourceService
	bundleCache        *bundle.BundleCache // Add this
	log                zerolog.Logger
}

// Update NewFHIRRouter to include cache initialization
func NewFHIRRouter(
	searchParamService *searchparameter.SearchParameterService,
	processorService *processor.ProcessorService,
	dataSourceService *datasource.DataSourceService,
	log zerolog.Logger,
) *FHIRRouter {
	// Initialize cache with default config
	cacheConfig := bundle.DefaultCacheConfig()
	bundleCache := bundle.NewBundleCache(*cacheConfig, log)

	return &FHIRRouter{
		searchParamService: searchParamService,
		processorService:   processorService,
		bundleService:      bundle.NewBundleService(log, cacheConfig),
		dataSourceService:  dataSourceService,
		bundleCache:        bundleCache,
		log:                log,
	}
}

func (fr *FHIRRouter) SetupRoutes() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/r4", func(r chi.Router) {
		r.Route("/{resourceType}", func(r chi.Router) {
			r.Get("/", fr.handleSearch)
		})
	})

	return r
}

func (fr *FHIRRouter) handleSearch(w http.ResponseWriter, r *http.Request) {
	resourceType := chi.URLParam(r, "resourceType")
	queryParams := r.URL.Query()

	// Get pagination parameters
	pageSize, _ := strconv.Atoi(queryParams.Get("_count"))
	offset, _ := strconv.Atoi(queryParams.Get("_offset"))

	// Create search params string for cache key
	searchParams := getSearchParams(queryParams)

	// Try to get page from cache first
	if fr.bundleCache != nil {
		if cachedResult, found := fr.bundleCache.GetPageFromCache(resourceType, searchParams, offset, pageSize); found {
			fr.log.Debug().
				Str("resource_type", resourceType).
				Str("search_params", searchParams).
				Msg("Serving response from cache")

			// Create bundle from cached result
			fr.createAndRespondWithBundle(w, r, *cachedResult, http.StatusOK)
			return
		}
	}

	// If not in cache, proceed with full processing
	searchResult := bundle.SearchResult{}

	// Validate resource type
	if !isValidResourceType(resourceType) {
		searchResult.Issues = append(searchResult.Issues, bundle.NewNotFoundIssue(
			fmt.Sprintf("Resource type %s is not supported", resourceType)))
		fr.createAndRespondWithBundle(w, r, searchResult, http.StatusNotFound)
		return
	}

	// Validate search parameters
	validFilters, invalidFilters := fr.validateSearchParameters(resourceType, queryParams)

	// Add any parameter validation issues
	for _, invalidFilter := range invalidFilters {
		issue := fr.createIssueFromFilter(invalidFilter)
		searchResult.Issues = append(searchResult.Issues, issue)
	}

	// If there are only invalid parameters, return error response
	if len(validFilters) == 0 && len(invalidFilters) > 0 {
		fr.createAndRespondWithBundle(w, r, searchResult, http.StatusBadRequest)
		return
	}

	// Process the request
	if err := fr.processRequest(r.Context(), resourceType, &searchResult); err != nil {
		searchResult.Issues = append(searchResult.Issues, bundle.NewProcessingError(err.Error()))
		fr.createAndRespondWithBundle(w, r, searchResult, http.StatusInternalServerError)
		return
	}

	// Store the complete result in cache
	if fr.bundleCache != nil {
		fr.bundleCache.StoreResultSet(resourceType, searchParams, searchResult)
	}

	// Return successful response
	fr.createAndRespondWithBundle(w, r, searchResult, http.StatusOK)
}

// Helper method to process the request
func (fr *FHIRRouter) processRequest(ctx context.Context, resourceType string, searchResult *bundle.SearchResult) error {
	// Get query file path
	queryFiles, err := fr.dataSourceService.FindSQLFilesInDir("queries/hix/flat", resourceType)
	if err != nil {
		return fmt.Errorf("failed to find query file: %v", err)
	}

	// Load query file
	if err := fr.dataSourceService.LoadQueryFile(queryFiles[0]); err != nil {
		return fmt.Errorf("failed to load query file: %v", err)
	}

	// Execute query and get results
	_, err = fr.dataSourceService.ReadResources(resourceType, "")
	if err != nil {
		return fmt.Errorf("failed to read resources: %v", err)
	}

	// Process results
	resources, err := fr.processorService.ProcessResources(ctx, fr.dataSourceService, resourceType, "", nil)
	if err != nil {
		return fmt.Errorf("error processing resources: %v", err)
	}

	searchResult.Resources = resources
	searchResult.Total = len(resources)

	if len(resources) == 0 {
		searchResult.Issues = append(searchResult.Issues, bundle.NewNotFoundIssue(
			"No resources match the search criteria"))
	}

	return nil
}

// Helper to create issue from invalid filter
func (fr *FHIRRouter) createIssueFromFilter(filter *types.Filter) bundle.SearchIssue {
	switch filter.ErrorType {
	case "unknown-parameter":
		return bundle.NewInvalidParameterIssue(
			fmt.Sprintf("Unknown search parameter '%s'", filter.Code))
	case "unsupported-modifier":
		return bundle.NewInvalidParameterIssue(
			fmt.Sprintf("Search modifier '%s' is not supported for parameter '%s'",
				filter.Modifier, filter.Code))
	default:
		return bundle.NewInvalidParameterIssue(
			fmt.Sprintf("Invalid parameter '%s'", filter.Code))
	}
}
func (fr *FHIRRouter) createAndRespondWithBundle(w http.ResponseWriter, r *http.Request, result bundle.SearchResult, status int) {
	// Extract pagination parameters
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("_count"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("_offset"))

	// Create pagination params
	paginationParams := &bundle.PaginationParams{
		PageSize:     pageSize,
		PageOffset:   offset,
		BaseURL:      getBaseURL(r),
		ResourceType: chi.URLParam(r, "resourceType"),
		SearchParams: getSearchParams(r.URL.Query()),
	}

	// Create bundle with pagination
	bundle, err := fr.bundleService.CreateSearchBundle(result, paginationParams)
	if err != nil {
		fr.log.Error().Err(err).Msg("Failed to create search bundle")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	respondWithJSON(w, status, bundle)
}

// Helper functions

func getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/r4", scheme, r.Host)
}

func getSearchParams(params map[string][]string) string {
	// Filter out pagination parameters
	var queryParts []string
	for key, values := range params {
		if key != "_count" && key != "_offset" {
			for _, value := range values {
				queryParts = append(queryParts, fmt.Sprintf("%s=%s", key, value))
			}
		}
	}
	return strings.Join(queryParts, "&")
}

func splitParameter(param string) (string, string) {
	parts := strings.Split(param, ":")
	if len(parts) > 1 {
		return parts[0], parts[1]
	}
	return param, ""
}

func isValidResourceType(resourceType string) bool {
	_, exists := processor.ResourceFactoryMap[resourceType]
	return exists
}

func (fr *FHIRRouter) validateSearchParameters(resourceType string, params map[string][]string) ([]*types.Filter, []*types.Filter) {
	var validFilters, invalidFilters []*types.Filter

	for paramName, values := range params {
		// Skip pagination parameters
		if paramName == "_count" || paramName == "_offset" {
			continue
		}

		baseParam, modifier := splitParameter(paramName)
		filter, err := fr.searchParamService.ValidateSearchParameter(resourceType, baseParam, modifier)

		if err != nil || !filter.IsValid {
			filter.Value = values[0]
			invalidFilters = append(invalidFilters, filter)
			continue
		}

		filter.Value = values[0]
		validFilters = append(validFilters, filter)
	}

	return validFilters, invalidFilters
}
func respondWithJSON(w http.ResponseWriter, status int, data interface{}) {
	// Set headers
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(status)

	// Use a bytes.Buffer to encode the JSON without escaping
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	// Encode the data
	if err := encoder.Encode(data); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
	log.Debug().Msgf("Bundle JSON: %s", strings.TrimSpace(buf.String()))
	// Convert the JSON bytes to string and manually decode Unicode escape sequences
	decodedJSON := decodeUnicodeEscapes(buf.String())

	// Write the JSON to the response writer
	w.Write([]byte(decodedJSON))
}

// decodeUnicodeEscapes decodes the Unicode escape sequences (e.g., \u0026) into their proper characters
// like '&', and ensures that no other special character is wrongly escaped.
func decodeUnicodeEscapes(jsonString string) string {
	var result string
	i := 0
	for i < len(jsonString) {
		// Check if we encounter a Unicode escape sequence (e.g., \u0026)
		if jsonString[i] == '\\' && i+5 < len(jsonString) && jsonString[i+1] == 'u' {
			// Capture the Unicode sequence and decode
			hexValue := jsonString[i+2 : i+6]
			codepoint, err := hexToRune(hexValue)
			if err != nil {
				// If we cannot decode the Unicode sequence, add the original characters
				result += jsonString[i : i+6]
				i += 6
				continue
			}

			// Add the decoded character
			result += string(codepoint)
			i += 6
		} else {
			// If no escape sequence, just add the current character
			result += string(jsonString[i])
			i++
		}
	}

	return result
}

// hexToRune converts a hex string (e.g., "0026") to a rune (character).
func hexToRune(hexStr string) (rune, error) {
	var codepoint int
	_, err := fmt.Sscanf(hexStr, "%x", &codepoint)
	if err != nil {
		return 0, err
	}
	return rune(codepoint), nil
}
