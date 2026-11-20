package conformance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RemoteServer is a FHIR terminology server (e.g. the Nationale
// Terminologieserver) that expands the ValueSets FENIX cannot expand itself.
// A ValueSet goes to the first server whose rules match, see Store.Expand.
type RemoteServer struct {
	Name      string
	BaseURL   string        // FHIR base, e.g. https://terminologieserver.nl/fhir
	ValueSets []string      // ValueSet canonical url prefixes this server answers
	Systems   []string      // code systems this server answers (filters, whole systems, implicit ValueSets)
	CacheTTL  time.Duration // how long an expansion is reused; default 24h
	MaxSize   int           // largest expansion accepted; default 100000 concepts
	Auth      Auth
	Client    *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

// Auth is how FENIX signs in to a terminology server.
type Auth struct {
	Type        string // none (default) | bearer | password
	TokenEnv    string // bearer: env var holding the token
	TokenURL    string // password: OpenID token endpoint
	ClientID    string // password: client id
	UsernameEnv string // password: env var holding the user name
	PasswordEnv string // password: env var holding the password
}

// AddRemote registers a terminology server; servers are tried in the order added.
func (s *Store) AddRemote(r *RemoteServer) {
	if r.Client == nil {
		r.Client = &http.Client{Timeout: 2 * time.Minute}
	}
	s.mu.Lock()
	s.remotes = append(s.remotes, r)
	s.mu.Unlock()
}

// remoteFor returns the first server that answers a ValueSet url, or one that needs system.
func (s *Store) remoteFor(vsURL, system string) *RemoteServer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.remotes {
		for _, p := range r.ValueSets {
			if p != "" && strings.HasPrefix(vsURL, p) {
				return r
			}
		}
		if system != "" {
			for _, sys := range r.Systems {
				if sys == system {
					return r
				}
			}
		}
	}
	return nil
}

func (r *RemoteServer) ttl() time.Duration {
	if r.CacheTTL > 0 {
		return r.CacheTTL
	}
	return 24 * time.Hour
}

const pageSize = 1000

// expand pages through ValueSet/$expand on the server.
func (r *RemoteServer) expand(vsURL, version string) ([]Concept, error) {
	max := r.MaxSize
	if max <= 0 {
		max = 100000
	}
	var out []Concept
	for offset := 0; ; {
		q := url.Values{"url": {vsURL}, "count": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(offset)}}
		if version != "" {
			q.Set("valueSetVersion", version)
		}
		var vs struct {
			ResourceType string `json:"resourceType"`
			Expansion    struct {
				Total    *int           `json:"total"`
				Contains []containsItem `json:"contains"`
			} `json:"expansion"`
		}
		if err := r.get("/ValueSet/$expand?"+q.Encode(), &vs); err != nil {
			return nil, fmt.Errorf("terminology server %s: expand %s: %w", r.Name, vsURL, err)
		}
		if vs.ResourceType != "ValueSet" {
			return nil, fmt.Errorf("terminology server %s: expand %s: answered %s, not a ValueSet", r.Name, vsURL, vs.ResourceType)
		}
		var page []Concept
		flattenContains(vs.Expansion.Contains, &page)
		out = append(out, page...)
		if len(out) > max {
			return nil, fmt.Errorf("terminology server %s: ValueSet %s has more than %d concepts (raise maxConcepts)", r.Name, vsURL, max)
		}
		offset += len(vs.Expansion.Contains)
		if len(vs.Expansion.Contains) == 0 || vs.Expansion.Total == nil || offset >= *vs.Expansion.Total {
			return out, nil
		}
	}
}

func (r *RemoteServer) get(path string, into any) error {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(r.BaseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/fhir+json")
	req.Header.Set("User-Agent", "fenix")
	if err := r.authorize(req); err != nil {
		return err
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		r.mu.Lock()
		r.token = "" // sign in again next time
		r.mu.Unlock()
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

func (r *RemoteServer) authorize(req *http.Request) error {
	switch r.Auth.Type {
	case "", "none":
		return nil
	case "bearer":
		token := os.Getenv(r.Auth.TokenEnv)
		if token == "" {
			return fmt.Errorf("env var %s (token) is empty", r.Auth.TokenEnv)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	case "password":
		token, err := r.passwordToken()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
	return fmt.Errorf("unknown auth type %q (none, bearer, password)", r.Auth.Type)
}

// passwordToken signs in with the OpenID password grant and reuses the token until it expires.
func (r *RemoteServer) passwordToken() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.token != "" && time.Now().Before(r.expires) {
		return r.token, nil
	}
	user, pass := os.Getenv(r.Auth.UsernameEnv), os.Getenv(r.Auth.PasswordEnv)
	if user == "" || pass == "" {
		return "", fmt.Errorf("env vars %s and %s (sign-in) must be set", r.Auth.UsernameEnv, r.Auth.PasswordEnv)
	}
	form := url.Values{"grant_type": {"password"}, "client_id": {r.Auth.ClientID}, "username": {user}, "password": {pass}}
	resp, err := r.Client.PostForm(r.Auth.TokenURL, form)
	if err != nil {
		return "", fmt.Errorf("sign-in: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sign-in: HTTP %d", resp.StatusCode)
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil || t.AccessToken == "" {
		return "", fmt.Errorf("sign-in: no access token in the answer")
	}
	life := time.Duration(t.ExpiresIn) * time.Second
	if life <= 30*time.Second {
		life = 5 * time.Minute
	}
	r.token, r.expires = t.AccessToken, time.Now().Add(life-30*time.Second)
	return r.token, nil
}
