package client

import (
	"encoding/json"
	"fmt"
	"net/http"

	lusciimodels "github.com/SanteonNL/fenix/internal/models/luscii"
)

// Client calls the Luscii Vitals API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{baseURL: baseURL, apiKey: apiKey, httpClient: &http.Client{}}
}

type patientsResponse struct {
	Data []lusciimodels.PatientTransformer `json:"data"`
}

func (c *Client) GetPatients() ([]lusciimodels.PatientTransformer, error) {
	var resp patientsResponse
	if err := c.get("/v1/patients", &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *Client) get(path string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: status %d", path, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
