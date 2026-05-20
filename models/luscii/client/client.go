package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Client calls the Luscii Vitals API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 300 * time.Second},
	}
}

// pageResponse is the standard Luscii paginated response envelope.
type pageResponse struct {
	Meta struct {
		NextPageURL string `json:"nextPageUrl"`
	} `json:"meta"`
	Data []map[string]interface{} `json:"data"`
}

// FetchParams carries optional date-range query parameters for one endpoint fetch.
type FetchParams struct {
	SinceParam string // query parameter name for start date, e.g. "startDate"
	EndParam   string // query parameter name for end date, e.g. "endDate"
	Since      string // ISO-8601 date value, e.g. "2025-01-01"
}

// FetchAll fetches every page from path, appending date params when provided,
// and returns the combined records across all pages.
func (c *Client) FetchAll(path string, params FetchParams) ([]map[string]interface{}, error) {
	firstURL := c.buildURL(path, params)
	var all []map[string]interface{}
	nextURL := firstURL

	for nextURL != "" {
		page, err := c.fetchPage(nextURL)
		if err != nil {
			return all, err
		}
		all = append(all, page.Data...)
		nextURL = page.Meta.NextPageURL
		if nextURL != "" {
			// Preserve date params on paginated URLs that may omit them.
			nextURL = ensureDateParams(nextURL, params)
			time.Sleep(100 * time.Millisecond) // light rate-limiting
		}
	}
	return all, nil
}

func (c *Client) buildURL(path string, p FetchParams) string {
	base := c.baseURL
	if len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	if len(path) > 0 && path[0] != '/' {
		path = "/" + path
	}
	full := base + path

	if p.SinceParam == "" || p.Since == "" {
		return full
	}
	today := time.Now().Format("2006-01-02")
	q := url.Values{}
	q.Set(p.SinceParam, toDateOnly(p.Since))
	if p.EndParam != "" {
		q.Set(p.EndParam, today)
	}
	return full + "?" + q.Encode()
}

func (c *Client) fetchPage(rawURL string) (*pageResponse, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s: %w", rawURL, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", rawURL, resp.StatusCode)
	}
	var page pageResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode %s: %w", rawURL, err)
	}
	return &page, nil
}

// toDateOnly truncates an RFC3339 timestamp or any YYYY-MM-DD... string to date-only format.
// The Luscii API accepts only YYYY-MM-DD for startDate/endDate parameters.
func toDateOnly(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// ensureDateParams re-adds date parameters to pagination URLs that may drop them.
func ensureDateParams(rawURL string, p FetchParams) string {
	if p.SinceParam == "" || p.Since == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	if q.Get(p.SinceParam) == "" {
		q.Set(p.SinceParam, toDateOnly(p.Since))
	}
	if p.EndParam != "" && q.Get(p.EndParam) == "" {
		q.Set(p.EndParam, time.Now().Format("2006-01-02"))
	}
	u.RawQuery = q.Encode()
	return u.String()
}
