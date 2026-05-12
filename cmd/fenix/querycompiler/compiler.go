package querycompiler

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// RenderedQuery is one rendered SQL string with its query name.
// A resource can produce multiple RenderedQueries (e.g. "vitals" and "labs").
type RenderedQuery struct {
	Name string
	SQL  string
}

// Compiler resolves a (source, groupID, resourceType, FHIR params) tuple to rendered SQL.
//
// All queries are configured per source (sources/<source>/source.yaml).
// Group overrides live inside each source (sources/<source>/groups/<group>.yaml):
//   - Where: AND fragment injected into every query for the resource.
//   - Per-named-query SQL replacement: replaces only the named query's SQL file.
//
// Substitutions are merged: global substitutions.yaml → source-specific substitutions.
type Compiler struct {
	sqlBaseDir  string
	globalSubs  map[string]string
	sources     map[string]SourceConfig
	searchIndex searchParamIndex
}

// New creates a Compiler.
//   - configDir  directory containing substitutions.yaml and sources/
//   - sqlBaseDir repo root; SQL paths are relative to this, and
//     terminology/searchparameter/search-parameter.json is loaded from here.
func New(configDir, sqlBaseDir string) (*Compiler, error) {
	globalSubs, err := loadGlobalSubs(configDir)
	if err != nil {
		return nil, err
	}
	sources, err := loadSources(configDir)
	if err != nil {
		return nil, err
	}
	searchIndex, err := loadSearchParams(
		filepath.Join(sqlBaseDir, "terminology", "searchparameter", "search-parameter.json"),
	)
	if err != nil {
		return nil, err
	}
	return &Compiler{
		sqlBaseDir:  sqlBaseDir,
		globalSubs:  globalSubs,
		sources:     sources,
		searchIndex: searchIndex,
	}, nil
}

// Resolve returns rendered SQL for every query defined for (source, groupID, resourceType).
// groupID may be empty for no group-level overrides.
func (c *Compiler) Resolve(source, groupID, resourceType string, fhirParams map[string]string) ([]RenderedQuery, error) {
	queries, groupRes := c.resolveQueries(source, groupID, resourceType)

	// Merge substitutions: global first, then source-specific on top.
	subs := make(map[string]string)
	for k, v := range c.globalSubs {
		subs[k] = v
	}
	if sc, ok := c.sources[source]; ok {
		for k, v := range sc.Substitutions {
			subs[k] = v
		}
	}

	var result []RenderedQuery
	for _, q := range queries {
		sqlFile, replaceRules := c.resolveQuerySQL(q, groupRes)
		raw, err := os.ReadFile(sqlFile)
		if err != nil {
			return nil, fmt.Errorf("reading %s (query %q): %w", sqlFile, q.Name, err)
		}

		// Apply literal text replacements before template rendering.
		sqlText := string(raw)
		for _, r := range replaceRules {
			sqlText = strings.ReplaceAll(sqlText, r.From, r.To)
		}

		vars := make(map[string]interface{})
		for k, v := range subs {
			vars[k] = v
		}
		for k, v := range buildTemplateVars(resourceType, fhirParams, q.Pushdown, c.searchIndex) {
			vars[k] = v
		}
		if groupRes.Where != "" {
			vars["extra_where"] = groupRes.Where
		}

		sql, err := render(sqlText, vars)
		if err != nil {
			return nil, fmt.Errorf("rendering query %q: %w", q.Name, err)
		}
		result = append(result, RenderedQuery{Name: q.Name, SQL: sql})
	}
	return result, nil
}

// resolveQueries returns the list of QueryConfigs to run and the group override for the resource.
func (c *Compiler) resolveQueries(source, groupID, resourceType string) ([]QueryConfig, GroupResourceConfig) {
	var groupRes GroupResourceConfig

	var queries []QueryConfig
	if sc, ok := c.sources[source]; ok {
		if res, ok := sc.Resources[resourceType]; ok {
			queries = res.Queries
		}
		// Load group override from this source's groups.
		if groupID != "" {
			if gc, ok := sc.Groups[groupID]; ok {
				if gr, ok := gc.Resources[resourceType]; ok {
					groupRes = gr
				}
			}
		}
	}

	return queries, groupRes
}

// resolveQuerySQL returns the SQL file path and any replace rules for query q.
// If the group overrides this named query with a new SQL file, that path is returned and Replace is ignored.
// If the group specifies Replace rules, the original SQL file is returned alongside those rules.
func (c *Compiler) resolveQuerySQL(q QueryConfig, groupRes GroupResourceConfig) (sqlFile string, rules []ReplaceRule) {
	for _, override := range groupRes.Queries {
		if override.Name != q.Name {
			continue
		}
		if override.SQL != "" {
			return filepath.Join(c.sqlBaseDir, override.SQL), nil
		}
		return filepath.Join(c.sqlBaseDir, q.SQL), override.Replace
	}
	return filepath.Join(c.sqlBaseDir, q.SQL), nil
}

func render(sqlTmpl string, vars map[string]interface{}) (string, error) {
	tmpl, err := template.New("query").Parse(sqlTmpl)
	if err != nil {
		return "", fmt.Errorf("parsing SQL template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("rendering SQL template: %w", err)
	}
	return buf.String(), nil
}
