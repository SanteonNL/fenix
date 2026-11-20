package config

import (
	"fmt"
	"path/filepath"
	"time"
)

// PackageRef is one FHIR package (package.tgz) under inputs.packages. It comes
// either from a package registry (name + version) or from a local file (a .tgz
// or an extracted folder, as a path inside a source):
//
//	packages:
//	  - name: santeon.sim-on-fhir
//	    version: 0.1.0
//	    registry: https://packages.simplifier.net   # default https://packages.fhir.org
//	    dependencies: true                          # also load the packages it depends on
//	    use: [valueSets, observationDefinitions]
//	  - file: {source: repo, path: ../sim-on-fhir/output/package.tgz}
//
// Every conformance resource of every loaded package (also of dependencies) is
// served by the FHIR API (search by url, read, ValueSet/$expand,
// $validate-code) and is available to the :in / :not-in search modifiers.
// "use" additionally feeds the package's own resources (not its dependencies')
// into conversion, exactly like the matching folder inputs do:
//
//	profiles                code bindings used for concept mapping and profile checks
//	valueSets               members that need no concept mapping
//	conceptMaps             code translations
//	observationDefinitions  checks of Observation values
//	datasets                the sim-on-fhir dataset and data requirement definitions (Binary):
//	                        a Group $export of a dataset's cohort warns about missing required elements
//
// use is empty by default: the package is then only served, conversion is unchanged.
type PackageRef struct {
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version"`
	Registry     string   `yaml:"registry"`     // registry base URL; default https://packages.fhir.org
	TokenEnv     string   `yaml:"token_env"`    // optional env var holding a bearer token for a private registry
	File         *DirRef  `yaml:"file"`         // local .tgz or extracted package folder instead of a registry
	Dependencies bool     `yaml:"dependencies"` // follow package.json dependencies (served only, never fed into conversion)
	Exclude      []string `yaml:"exclude"`      // dependency names not followed; default [hl7.fhir.r4.core]
	Use          []string `yaml:"use"`          // profiles | valueSets | conceptMaps | observationDefinitions | datasets
}

// TerminologyServer is a FHIR terminology server under inputs.terminologyServers.
// FENIX expands a ValueSet itself when it is loaded (folder or package) and only
// lists codes; it goes to the first server whose rules match when the ValueSet
// is not loaded and its url starts with one of valueSets, or when it needs one
// of systems (a filter such as SNOMED ECL, all of a code system that is not
// loaded, or an implicit ValueSet like http://snomed.info/sct?fhir_vs=ecl/...):
//
//	terminologyServers:
//	  - name: nts
//	    url: https://terminologieserver.nl/fhir
//	    systems: [http://snomed.info/sct, http://loinc.org]
//	    valueSets: [https://terminologieserver.nl/]
//	    auth:
//	      type: password
//	      tokenUrl: https://terminologieserver.nl/authorisation/auth/realms/nictiz/protocol/openid-connect/token
//	      clientId: cli_client
//	      username_env: NTS_USERNAME
//	      password_env: NTS_PASSWORD
type TerminologyServer struct {
	Name        string          `yaml:"name"`
	URL         string          `yaml:"url"`         // FHIR base url
	ValueSets   []string        `yaml:"valueSets"`   // ValueSet canonical url prefixes it answers
	Systems     []string        `yaml:"systems"`     // code systems it answers
	CacheTTL    string          `yaml:"cacheTTL"`    // how long an expansion is reused, e.g. 24h (default)
	MaxConcepts int             `yaml:"maxConcepts"` // largest expansion accepted; default 100000
	Auth        TerminologyAuth `yaml:"auth"`
}

// TerminologyAuth is how FENIX signs in to a terminology server. Secrets are only read from env vars.
type TerminologyAuth struct {
	Type        string `yaml:"type"`         // none (default) | bearer | password (OpenID password grant)
	TokenEnv    string `yaml:"token_env"`    // bearer
	TokenURL    string `yaml:"tokenUrl"`     // password
	ClientID    string `yaml:"clientId"`     // password
	UsernameEnv string `yaml:"username_env"` // password
	PasswordEnv string `yaml:"password_env"` // password
}

// TTL parses CacheTTL; 0 means the default.
func (t TerminologyServer) TTL() (time.Duration, error) {
	if t.CacheTTL == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(t.CacheTTL)
	if err != nil {
		return 0, fmt.Errorf("terminology server %s: cacheTTL: %w", t.Name, err)
	}
	return d, nil
}

// Validate checks one terminology server.
func (t TerminologyServer) Validate() error {
	if t.Name == "" || t.URL == "" {
		return fmt.Errorf("terminology server needs name and url")
	}
	if len(t.ValueSets) == 0 && len(t.Systems) == 0 {
		return fmt.Errorf("terminology server %s: give valueSets and/or systems, otherwise nothing is sent to it", t.Name)
	}
	switch t.Auth.Type {
	case "", "none":
	case "bearer":
		if t.Auth.TokenEnv == "" {
			return fmt.Errorf("terminology server %s: auth bearer needs token_env", t.Name)
		}
	case "password":
		if t.Auth.TokenURL == "" || t.Auth.ClientID == "" || t.Auth.UsernameEnv == "" || t.Auth.PasswordEnv == "" {
			return fmt.Errorf("terminology server %s: auth password needs tokenUrl, clientId, username_env and password_env", t.Name)
		}
	default:
		return fmt.Errorf("terminology server %s: unknown auth type %q (none, bearer, password)", t.Name, t.Auth.Type)
	}
	_, err := t.TTL()
	return err
}

// PackageUses are the valid values of PackageRef.Use.
var PackageUses = map[string]bool{"profiles": true, "valueSets": true, "conceptMaps": true, "observationDefinitions": true, "datasets": true}

// Validate checks one package reference.
func (p PackageRef) Validate() error {
	label := p.Name
	if p.File != nil {
		if p.Name != "" || p.Version != "" {
			return fmt.Errorf("package %s: give either file or name+version, not both", p.File.Path)
		}
		if p.File.Source == "" || p.File.Path == "" {
			return fmt.Errorf("package file needs source and path")
		}
		label = p.File.Path
	} else if p.Name == "" || p.Version == "" {
		return fmt.Errorf("package %q: needs name and version (or file)", p.Name)
	}
	for _, u := range p.Use {
		if !PackageUses[u] {
			return fmt.Errorf("package %s: unknown use %q (profiles, valueSets, conceptMaps, observationDefinitions, datasets)", label, u)
		}
	}
	return nil
}

// PackageCacheDir returns the package cache folder, resolved against repoRoot.
func (i InputsConfig) PackageCacheDir(repoRoot string) string {
	dir := i.PackageCache
	if dir == "" {
		dir = filepath.Join(".cache", "packages")
	}
	return resolvePath(repoRoot, dir)
}
