package querycompiler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/SanteonNL/fenix/internal/converter"
)

// groupCol is the single column of a group query: the patient number.
const groupCol = "patient_id"

// GroupFilter is one Group member-filter: a FHIR search ("Observation",
// {"status": "final"}) whose matching patients belong to the group.
//
// When some param isn't pushed down into SQL (see PushedDownByAll), the caller
// resolves the filter itself — the normal search with its in-Go fallback
// filtering — and passes the matching patient numbers as PatientIDs with
// Static set; the group query then joins that fixed list instead of SQL.
type GroupFilter struct {
	ResourceType string
	Params       map[string]string
	Static       bool
	PatientIDs   []string
}

// PushedDownByAll reports whether every param is pushed down into SQL by
// every named query of resourceType in source — i.e. whether the filter can
// be evaluated entirely inside the group query.
func (c *Compiler) PushedDownByAll(source, resourceType string, params map[string]string) bool {
	if len(c.InexactParams(resourceType, params)) > 0 {
		return false
	}
	_, bad := c.unpushed(source, resourceType, params)
	return bad == ""
}

// unpushed returns the first (query name, param) pair that is not pushed down.
func (c *Compiler) unpushed(source, resourceType string, params map[string]string) (query, param string) {
	for _, q := range c.sources[source].Resources[resourceType].Queries {
		pushed := make(map[string]bool)
		for _, p := range q.memberFilterPushdown() {
			pushed[p] = true
		}
		for code := range params {
			if !pushed[code] {
				return q.Name, code
			}
		}
	}
	return "", ""
}

// ResolveGroupMembersSQL builds ONE SQL query returning the patient numbers (column
// patient_id) that satisfy every filter. Each filter contributes its
// resource's rendered query — params already pushed down into its WHERE —
// and the filters are INNER JOINed on patient number, so the intersection
// happens in the database instead of in Go afterwards.
//
// Convention (already required for FHIR conversion): the first statement of
// each query is the root statement, exposing resource_id and either id
// (Patient) or "subject.reference" = 'Patient/<number>' (everything else).
//
// Because nothing is filtered afterwards, every param must be pushed down for
// its resource type; an unwired param is an error rather than silently
// ignored.
func (c *Compiler) ResolveGroupMembersSQL(source, groupID string, filters []GroupFilter) (string, error) {
	if len(filters) == 0 {
		return "", fmt.Errorf("group needs at least one filter")
	}

	parts := make([]string, len(filters))
	for i, f := range filters {
		if f.Static {
			parts[i] = fmt.Sprintf("-- %s: %d patient(s) found by a normal search, because a param is not pushed down into SQL\n%s",
				filterLabel(f), len(f.PatientIDs), StaticGroupMembers(f.PatientIDs))
			continue
		}

		// A resource can have several named queries (e.g. Observation
		// "vitals" and "labs"); a param must be pushed down by EVERY one of
		// them, or that query would match patients the filter excludes.
		if q, code := c.unpushed(source, f.ResourceType, f.Params); code != "" {
			return "", fmt.Errorf("%s?%s: param %q is not pushed down into SQL by query %q of %s in source %q (resolve it as a Static filter instead)",
				f.ResourceType, code, code, q, f.ResourceType, source)
		}

		rendered, err := c.resolve(source, groupID, f.ResourceType, f.Params, "", true)
		if err != nil {
			return "", err
		}
		if len(rendered) == 0 {
			return "", fmt.Errorf("resource type %q is not configured for source %q", f.ResourceType, source)
		}

		selects := make([]string, len(rendered))
		for j, rq := range rendered {
			root, ok := rootStatement(rq.SQL)
			if !ok {
				return "", fmt.Errorf("query %q for %s has no statements", rq.Name, f.ResourceType)
			}
			label := ""
			if len(rendered) > 1 {
				label = fmt.Sprintf("-- query %q\n", rq.Name)
			}
			selects[j] = fmt.Sprintf("%sSELECT %s AS %s FROM (\n%s\n)", label, rq.PatientExpr, groupCol, root)
		}
		parts[i] = "-- " + filterLabel(f) + "\n" + strings.Join(selects, "\nUNION\n")
	}

	var b strings.Builder
	b.WriteString("-- Patients that match ALL of the filters below (each filter is joined on patient_id)\n")
	fmt.Fprintf(&b, "SELECT DISTINCT c0.%s AS %s FROM (\n%s\n) c0", groupCol, groupCol, parts[0])
	for i := 1; i < len(parts); i++ {
		fmt.Fprintf(&b, "\nJOIN (\n%s\n) c%d ON c%d.%s = c0.%s", parts[i], i, i, groupCol, groupCol)
	}
	return b.String(), nil
}

// filterLabel renders a filter as the FHIR search it came from, e.g.
// "Observation?code=MCD_B&status=final", for the comments in the group query.
// Characters that would break SQL comment splitting are replaced.
func filterLabel(f GroupFilter) string {
	keys := make([]string, 0, len(f.Params))
	for k := range f.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = k + "=" + f.Params[k]
	}
	label := f.ResourceType
	if len(pairs) > 0 {
		label += "?" + strings.Join(pairs, "&")
	}
	return strings.NewReplacer(";", ",", "\n", " ", "\r", " ").Replace(label)
}

// StaticGroupMembers builds a group query for an explicit list of patient numbers
// (a Group without member-filters), usable anywhere ResolveGroupMembersSQL's result is.
func StaticGroupMembers(patientIDs []string) string {
	if len(patientIDs) == 0 {
		return fmt.Sprintf("SELECT '' AS %s FROM (SELECT 1) WHERE 1=0", groupCol)
	}
	selects := make([]string, len(patientIDs))
	for i, id := range patientIDs {
		selects[i] = fmt.Sprintf("SELECT '%s' AS %s", strings.ReplaceAll(id, "'", "''"), groupCol)
	}
	return strings.Join(selects, "\nUNION\n")
}

// restrictToGroup wraps every statement of one rendered query so it only
// returns rows of resources whose patient is in the group. Child statements
// (Observation.code.coding, ...) share resource_id with the query's root
// statement, so the patient lookup always goes through the root.
func restrictToGroup(patientExpr, sql, membersSQL string) string {
	stmts := converter.SplitStatements(sql)
	if len(stmts) == 0 {
		return sql
	}
	root := stmts[0]

	wrapped := make([]string, len(stmts))
	for i, stmt := range stmts {
		wrapped[i] = fmt.Sprintf(
			"SELECT s.* FROM (\n%s\n) s\nWHERE s.resource_id IN (\n"+
				"SELECT resource_id FROM (\n%s\n)\nWHERE %s IN (\n%s\n))",
			stmt, root, patientExpr, membersSQL)
	}
	return strings.Join(wrapped, ";\n")
}

func rootStatement(sql string) (string, bool) {
	stmts := converter.SplitStatements(sql)
	if len(stmts) == 0 {
		return "", false
	}
	return stmts[0], true
}
