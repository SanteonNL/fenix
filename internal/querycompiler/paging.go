package querycompiler

import (
	"fmt"
	"strings"

	"github.com/SanteonNL/fenix/internal/converter"
)

// maxInListIDs caps how many ids go into one IN (...) list. SQL Server accepts
// far more, but a statement this size already compiles slowly there; callers
// batch/page well below it.
const maxInListIDs = 10000

// InList renders ids as the body of a SQL IN (...) list: 'a','b','c'. With no
// ids it renders NULL, which matches nothing, so the result is always valid
// inside IN (...). It is also usable as the membersSQL of ResolveInGroup,
// which wraps it in IN (...) itself.
func InList(ids []string) string {
	if len(ids) == 0 {
		return "NULL"
	}
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = "'" + strings.ReplaceAll(id, "'", "''") + "'"
	}
	return strings.Join(quoted, ",")
}

// RootIDsSQL returns SQL listing the resource ids the query's root statement
// (its first) selects: the resources the query would return.
func RootIDsSQL(q RenderedQuery) (string, bool) {
	root, ok := rootStatement(q.SQL)
	if !ok {
		return "", false
	}
	return fmt.Sprintf("SELECT DISTINCT r.resource_id FROM (\n%s\n) r", root), true
}

// RestrictToResources limits every statement of every query to rows of the
// given resource ids (all statements of a query share resource_id with its
// root). It is how a page of a search is fetched once the page's ids are known.
func RestrictToResources(queries []RenderedQuery, ids []string) ([]RenderedQuery, error) {
	if len(ids) > maxInListIDs {
		return nil, fmt.Errorf("cannot restrict a query to %d resources at once (max %d)", len(ids), maxInListIDs)
	}
	list := InList(ids)
	out := make([]RenderedQuery, len(queries))
	for i, q := range queries {
		out[i] = q
		stmts := converter.SplitStatements(q.SQL)
		wrapped := make([]string, len(stmts))
		for j, stmt := range stmts {
			wrapped[j] = fmt.Sprintf("SELECT s.* FROM (\n%s\n) s\nWHERE s.resource_id IN (%s)", stmt, list)
		}
		out[i].SQL = strings.Join(wrapped, ";\n")
	}
	return out, nil
}
