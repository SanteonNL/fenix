package bundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/models/fhir"
	"github.com/SanteonNL/fenix/util"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// BundleService handles creation and management of FHIR bundles
// BundleService handles creation and management of FHIR bundles
type BundleService struct {
	log             zerolog.Logger
	defaultPageSize int
	cache           *BundleCache
}

// SearchResult represents a search operation result including any issues
type SearchResult struct {
	Resources []interface{}
	Issues    []SearchIssue
	Total     int
}

// SearchIssue represents a validation or processing issue
type SearchIssue struct {
	Severity fhir.IssueSeverity
	Code     fhir.IssueType
	Details  string
}

// PaginationParams contains information needed for pagination
type PaginationParams struct {
	PageSize     int
	PageOffset   int    // Offset for the current page
	BaseURL      string // Base URL for generating links
	ResourceType string
	SearchParams string // Original search parameters
}

func NewBundleService(log zerolog.Logger, cacheConfig *CacheConfig) *BundleService {
	service := &BundleService{
		log:             log,
		defaultPageSize: 2,
	}

	if cacheConfig != nil && cacheConfig.Enabled {
		service.cache = NewBundleCache(*cacheConfig, log)
	}

	return service
}

func (s *BundleService) CreateSearchBundle(result SearchResult, params *PaginationParams) (*fhir.Bundle, error) {

	s.log.Debug().Interface("result", result).Interface("params", params).Msg("CreateSearchBundle called")

	bundle := &fhir.Bundle{
		Id:        util.StringPtr(fmt.Sprintf("bundle-%s", time.Now().Format("20060102150405"))),
		Type:      fhir.BundleTypeSearchset,
		Total:     &result.Total,
		Timestamp: util.StringPtr(time.Now().Format(time.RFC3339)),
	}

	// Add pagination links if params are provided
	if params != nil {
		bundle.Link = s.createPaginationLinks(params, result.Total)
	}

	log.Debug().Interface("bundle", bundle.Link).Msg("Created bundle links")

	// Initialize entries slice
	totalEntries := len(result.Resources) + len(result.Issues)
	bundle.Entry = make([]fhir.BundleEntry, 0, totalEntries)

	// Add issues as OperationOutcome entries
	for _, issue := range result.Issues {
		outcome := &fhir.OperationOutcome{
			Issue: []fhir.OperationOutcomeIssue{
				{
					Severity: issue.Severity,
					Code:     issue.Code,
					Details: &fhir.CodeableConcept{
						Text: ptr(issue.Details),
					},
				},
			},
		}

		// Use buffer to prevent HTML escaping
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(outcome); err != nil {
			return nil, fmt.Errorf("failed to marshal operation outcome: %w", err)
		}

		entry := fhir.BundleEntry{
			Resource: json.RawMessage(buf.Bytes()),
		}
		bundle.Entry = append(bundle.Entry, entry)
	}

	// Handle pagination
	start := 0
	end := len(result.Resources)
	if params != nil {
		start = params.PageOffset
		if start < 0 {
			start = 0
		}

		pageSize := params.PageSize
		if pageSize <= 0 {
			pageSize = s.defaultPageSize
		}

		end = start + pageSize

		// Ensure `start` and `end` are within bounds
		if start > len(result.Resources) {
			start = len(result.Resources) // Start cannot exceed the number of resources
		}
		if end > len(result.Resources) {
			end = len(result.Resources) // End cannot exceed the number of resources
		}
	}

	// Add resources with proper JSON encoding
	for _, resource := range result.Resources[start:end] {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(resource); err != nil {
			return nil, fmt.Errorf("failed to marshal resource: %w", err)
		}

		entry := fhir.BundleEntry{
			Resource: json.RawMessage(bytes.TrimSpace(buf.Bytes())),
		}
		bundle.Entry = append(bundle.Entry, entry)
	}

	return bundle, nil
}

// createPaginationLinks creates the FHIR bundle links for pagination with proper URL handling
func (s *BundleService) createPaginationLinks(params *PaginationParams, total int) []fhir.BundleLink {
	var links []fhir.BundleLink
	pageSize := params.PageSize
	if pageSize <= 0 {
		pageSize = s.defaultPageSize
	}

	// Calculate pages
	currentPage := (params.PageOffset / pageSize) + 1
	totalPages := (total + pageSize - 1) / pageSize

	// Parse and normalize the base URL
	baseURL := fmt.Sprintf("%s/%s", strings.TrimRight(params.BaseURL, "/"), params.ResourceType)

	// Create URL values for query parameters
	query := url.Values{}
	if params.SearchParams != "" {
		existingParams, err := url.ParseQuery(params.SearchParams)
		if err != nil {
			s.log.Warn().Err(err).Msg("Failed to parse search parameters")
		} else {
			for k, v := range existingParams {
				if strings.TrimSpace(k) != "" && len(v) > 0 {
					query[k] = v
				} else {
					s.log.Warn().Str("parameter", k).Msg("Ignoring malformed search parameter")
				}
			}
		}
	}

	// Helper function to create links with proper encoding
	createLink := func(offset int) string {
		queryParams := url.Values{}
		for k, v := range query {
			queryParams[k] = v
		}
		queryParams.Set("_count", fmt.Sprintf("%d", pageSize))
		queryParams.Set("_offset", fmt.Sprintf("%d", offset))
		url := fmt.Sprintf("%s?%s", baseURL, queryParams.Encode())
		return url
	}

	// Self link
	links = append(links, fhir.BundleLink{
		Relation: "self",
		Url:      createLink(params.PageOffset),
	})

	// First page link
	if params.PageOffset > 0 {
		links = append(links, fhir.BundleLink{
			Relation: "first",
			Url:      createLink(0),
		})
	}

	// Previous page link
	if params.PageOffset > 0 {
		prevOffset := params.PageOffset - pageSize
		if prevOffset < 0 {
			prevOffset = 0
		}
		links = append(links, fhir.BundleLink{
			Relation: "previous",
			Url:      createLink(prevOffset),
		})
	}

	// Next page link
	if currentPage < totalPages {
		nextOffset := params.PageOffset + pageSize
		links = append(links, fhir.BundleLink{
			Relation: "next",
			Url:      createLink(nextOffset),
		})
	}

	// Last page link
	if total > 0 {
		lastOffset := ((total - 1) / pageSize) * pageSize
		links = append(links, fhir.BundleLink{
			Relation: "last",
			Url:      createLink(lastOffset),
		})
	}

	return links
}

// Rest of the helper functions remain the same...

// Processing failure
func NewProcessingError(details string) SearchIssue {
	return SearchIssue{
		Severity: fhir.IssueSeverityError,
		Code:     fhir.IssueTypeProcessing,
		Details:  details,
	}
}

// Not found
func NewNotFoundIssue(details string) SearchIssue {
	return SearchIssue{
		Severity: fhir.IssueSeverityWarning,
		Code:     fhir.IssueTypeNotFound,
		Details:  details,
	}
}

// Invalid parameter
func NewInvalidParameterIssue(details string) SearchIssue {
	return SearchIssue{
		Severity: fhir.IssueSeverityError,
		Code:     fhir.IssueTypeInvalid,
		Details:  details,
	}
}

// Business rule violation
func NewBusinessRuleIssue(details string) SearchIssue {
	return SearchIssue{
		Severity: fhir.IssueSeverityError,
		Code:     fhir.IssueTypeBusinessRule,
		Details:  details,
	}
}

// Security issue
func NewSecurityIssue(details string) SearchIssue {
	return SearchIssue{
		Severity: fhir.IssueSeverityError,
		Code:     fhir.IssueTypeSecurity,
		Details:  details,
	}
}

// Informational note
func NewInformationalIssue(details string) SearchIssue {
	return SearchIssue{
		Severity: fhir.IssueSeverityInformation,
		Code:     fhir.IssueTypeInformational,
		Details:  details,
	}
}

// Custom issue with specific severity and code
func NewIssue(severity fhir.IssueSeverity, code fhir.IssueType, details string) SearchIssue {
	return SearchIssue{
		Severity: severity,
		Code:     code,
		Details:  details,
	}
}

// Helper function to create string pointers
func ptr(s string) *string {
	return &s
}
