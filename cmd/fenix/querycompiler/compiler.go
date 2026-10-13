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
	Name        string
	SQL         string
	PlainSQL    string // SQL before any group restriction (what the query is on its own)
	PatientExpr string // SQL over the root statement yielding the patient number (see MemberFilterConfig)
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
	translate   ParamTranslator // reverse concept mapping for token params, may be nil
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

// SetParamTranslator sets the reverse concept mapping applied to pushed-down
// token params (e.g. Observation.code), see buildTemplateVars.
func (c *Compiler) SetParamTranslator(t ParamTranslator) { c.translate = t }

// InexactParams returns the token params whose reverse concept mapping can't
// be listed (the code is the concept map's catch-all "unmapped" target).
// They are not applied in SQL at all, so callers must filter them in Go
// even though they are listed in pushdown.
func (c *Compiler) InexactParams(resourceType string, fhirParams map[string]string) map[string]bool {
	out := map[string]bool{}
	if c.translate == nil {
		return out
	}
	for code, value := range fhirParams {
		info, ok := lookupSearchParam(c.searchIndex, resourceType, code)
		if !ok || info.paramType != "token" {
			continue
		}
		field := fieldName(resourceType, info.expression)
		if field == "" {
			field = code
		}
		_, bare, hasSystem := strings.Cut(value, "|")
		if !hasSystem {
			bare = value
		}
		if _, exact, mapped := c.translate(resourceType, field, bare); mapped && !exact {
			out[code] = true
		}
	}
	return out
}

// Resolve returns rendered SQL for every query defined for (source, groupID, resourceType).
// groupID may be empty for no group-level overrides.
func (c *Compiler) Resolve(source, groupID, resourceType string, fhirParams map[string]string) ([]RenderedQuery, error) {
	return c.resolve(source, groupID, resourceType, fhirParams, "", false)
}

// ResolveInGroup is Resolve restricted to the patients selected by
// membersSQL (as built by ResolveGroupMembersSQL): every statement of every query is
// wrapped so it only returns rows of resources whose patient is in the
// group. The restriction happens in SQL, so callers need no post-filtering.
func (c *Compiler) ResolveInGroup(source, groupID, resourceType string, fhirParams map[string]string, membersSQL string) ([]RenderedQuery, error) {
	return c.resolve(source, groupID, resourceType, fhirParams, membersSQL, false)
}

// resolve renders the queries. asGroupFilter selects each query's group
// pushdown list (MemberFilterConfig.Pushdown) instead of its standalone one.
func (c *Compiler) resolve(source, groupID, resourceType string, fhirParams map[string]string, membersSQL string, asGroupFilter bool) ([]RenderedQuery, error) {
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
		pushdown := q.Pushdown
		if asGroupFilter {
			pushdown = q.memberFilterPushdown()
		}
		for k, v := range buildTemplateVars(resourceType, fhirParams, pushdown, c.searchIndex, c.translate) {
			vars[k] = v
		}
		if groupRes.Where != "" {
			vars["extra_where"] = groupRes.Where
		}

		sql, err := RenderSQL(sqlText, vars)
		if err != nil {
			return nil, fmt.Errorf("rendering query %q: %w", q.Name, err)
		}
		plain := sql
		if membersSQL != "" {
			sql = restrictToGroup(q.patientExpr(resourceType), sql, membersSQL)
		}
		result = append(result, RenderedQuery{Name: q.Name, SQL: sql, PlainSQL: plain, PatientExpr: q.patientExpr(resourceType)})
	}
	return result, nil
}

// SearchParamField resolves (resourceType, code) to the FHIR field name and
// SearchParameter type FENIX would use for SQL pushdown (see
// buildTemplateVars), derived from the same
// terminology/searchparameter/search-parameter.json index. Callers that
// can't push a param down into SQL — the live server's struct-based
// fallback filter (fhirserver.filterUnwiredParams) — use this to know which
// field to inspect on the converted resource and how to compare it.
func (c *Compiler) SearchParamField(resourceType, code string) (field, paramType string, ok bool) {
	info, ok := lookupSearchParam(c.searchIndex, resourceType, code)
	if !ok {
		return "", "", false
	}
	field = fieldName(resourceType, info.expression)
	if field == "" {
		field = code
	}
	return field, info.paramType, true
}

// PushdownCodes returns the set of FHIR param codes already pushed down
// into SQL by every query configured for (source, resourceType) — group
// overrides don't carry their own pushdown list, so groupID doesn't factor
// in here. The live server's struct-based fallback filter skips these,
// since the SQL already scoped them.
func (c *Compiler) PushdownCodes(source, resourceType string) map[string]bool {
	codes := make(map[string]bool)
	sc, ok := c.sources[source]
	if !ok {
		return codes
	}
	res, ok := sc.Resources[resourceType]
	if !ok {
		return codes
	}
	for _, q := range res.Queries {
		for _, p := range q.Pushdown {
			codes[p] = true
		}
	}
	return codes
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

// RenderSQL renders a SQL file's Go template directives ({{if .status}}, etc.)
// against vars. Exported so the CLI batch pipeline (main.go runFHIRConversion)
// can render the same queries/<source>/fhir/*.sql files Resolve does — those
// files are executed as raw SQL there with no vars at all, so any {{if}}
// conditional simply evaluates false/absent, same as "no filter" (a full
// batch export, as opposed to a live per-request search).
func RenderSQL(sqlTmpl string, vars map[string]interface{}) (string, error) {
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
