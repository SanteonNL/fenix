package queryvalidator

import (
	"regexp"
	"strings"
)

// selectItem is one expression of a statement's SELECT list.
type selectItem struct {
	Alias string // output column name
	Expr  string // the expression as written
}

// literal returns the value when the expression is a plain string literal
// ('final'), and false for anything else (columns, functions, concatenation).
func (s selectItem) literal() (string, bool) {
	e := strings.TrimSpace(s.Expr)
	if len(e) >= 2 && e[0] == '\'' && e[len(e)-1] == '\'' {
		inner := e[1 : len(e)-1]
		if !strings.Contains(strings.ReplaceAll(inner, "''", ""), "'") {
			return strings.ReplaceAll(inner, "''", "'"), true
		}
	}
	return "", false
}

func (s selectItem) isNull() bool { return strings.EqualFold(strings.TrimSpace(s.Expr), "null") }

var (
	reLineComment  = regexp.MustCompile(`--[^\n]*`)
	reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reSelectStar   = regexp.MustCompile(`(?i)^(distinct\s+)?(top\s+\d+\s+)?([\w"\[\]]+\.)?\*$`)
	reTrailingAs   = regexp.MustCompile(`(?is)^(.*\S)\s+as\s+("[^"]+"|\[[^\]]+\]|` + "`[^`]+`" + `|\w+)$`)
	reBareAlias    = regexp.MustCompile(`(?s)^(.*[\w'")\]])\s+("[^"]+"|\w+)$`)
	reColumnRef    = regexp.MustCompile(`^(?:[\w"\[\]]+\.)?("[^"]+"|\[[^\]]+\]|\w+)$`)
)

// stripComments removes -- and /* */ comments, leaving string literals alone.
func stripComments(sql string) string {
	var b strings.Builder
	inStr := false
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case inStr:
			b.WriteByte(c)
			if c == '\'' {
				inStr = false
			}
		case c == '\'':
			inStr = true
			b.WriteByte(c)
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			b.WriteByte('\n')
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return b.String()
			}
			i += end + 3
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// selectList extracts the SELECT list of the statement's main (first
// top-level) SELECT: for "WITH ... SELECT" the one after the CTEs, for a
// UNION the first branch. ok is false when no SELECT could be found.
func selectList(stmt string) (items []selectItem, star bool, ok bool) {
	s := stripComments(stmt)
	depth, inStr := 0, false
	start := -1
	lower := strings.ToLower(s)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\'' {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'':
			inStr = true
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && start < 0 && strings.HasPrefix(lower[i:], "select") &&
			(i == 0 || !isWord(lower[i-1])) && (i+6 >= len(s) || !isWord(lower[i+6])) {
			start = i + 6
			break
		}
	}
	if start < 0 {
		return nil, false, false
	}

	// Walk to the top-level FROM (or end), splitting on top-level commas.
	depth, inStr = 0, false
	var parts []string
	last := start
	end := len(s)
loop:
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == '\'' {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'':
			inStr = true
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[last:i])
				last = i + 1
			}
		default:
			if depth == 0 && (c == 'f' || c == 'F') && strings.HasPrefix(lower[i:], "from") &&
				!isWord(lower[i-1]) && (i+4 >= len(s) || !isWord(lower[i+4])) {
				end = i
				break loop
			}
		}
	}
	parts = append(parts, s[last:end])

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if reSelectStar.MatchString(p) {
			star = true
			continue
		}
		items = append(items, parseItem(p))
	}
	return items, star, true
}

func isWord(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func unquoteIdent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		switch {
		case s[0] == '"' && s[len(s)-1] == '"', s[0] == '[' && s[len(s)-1] == ']', s[0] == '`' && s[len(s)-1] == '`':
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseItem(p string) selectItem {
	if m := reTrailingAs.FindStringSubmatch(p); m != nil {
		return selectItem{Alias: unquoteIdent(m[2]), Expr: strings.TrimSpace(m[1])}
	}
	if m := reColumnRef.FindStringSubmatch(p); m != nil {
		return selectItem{Alias: unquoteIdent(m[1]), Expr: p}
	}
	if m := reBareAlias.FindStringSubmatch(p); m != nil && !strings.EqualFold(m[2], "end") {
		return selectItem{Alias: unquoteIdent(m[2]), Expr: strings.TrimSpace(m[1])}
	}
	return selectItem{Alias: "", Expr: p} // unnamed expression
}
