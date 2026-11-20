package querycompiler

import "strings"

// buildTemplateVars resolves pushdown param codes to Go template variables.
//
// For each listed param code that appears in fhirParams:
//  1. Look up the SearchParameter for (resourceType, code) → get expression + type.
//  2. Derive the FHIR field name from the expression (e.g. "Observation.effective" → "effective").
//  3. For date-type params: split the value on its prefix (ge/gt/le/lt/eq) into
//     <field>_from and <field>_to template vars.
//  4. For all other types: create a single <field> template var with the raw value.
//
// The SQL template author uses the derived var name (e.g. {{.effective_from}}) in the WHERE
// clause alongside the actual DB column name (e.g. effective_date):
//
//	{{if .effective_from}} AND effective_date >= '{{.effective_from}}'{{end}}
//
// Token params (type "token", e.g. Observation.code) additionally get
// <field>_in: a ready-made SQL list for "col IN ({{.code_in}})". The FHIR
// search code is a TARGET code, so when translate is set and a concept map
// covers the element, <field>_in holds the SOURCE codes that map to it
// (reverse concept mapping). Otherwise it holds the code itself, and
// <field>_system carries the "system|" part of the search value; after a
// reverse mapping the system is dropped, as it names the target system, not
// the source column's.
func buildTemplateVars(resourceType string, fhirParams map[string]string, pushdownCodes []string, idx searchParamIndex, translate ParamTranslator) map[string]interface{} {
	vars := make(map[string]interface{})

	for _, code := range pushdownCodes {
		value, ok := fhirParams[code]
		if !ok {
			continue
		}
		info, ok := lookupSearchParam(idx, resourceType, code)
		if !ok {
			continue // param not defined for this resource type — skip
		}

		field := fieldName(resourceType, info.expression)
		if field == "" {
			field = code // fallback: use param code as var name
		}

		if info.paramType == "date" {
			from, to := splitDateValue(value)
			if from != "" {
				vars[field+"_from"] = from
			}
			if to != "" {
				vars[field+"_to"] = to
			}
		} else if info.paramType == "token" && len(tokenAlternatives(value)) > 1 {
			// A comma is an OR: every alternative goes into <field>_in, and no
			// single-value vars are set. InexactParams reports the param, so the
			// in-Go filter checks the exact system|code pairs afterwards.
			var codes []string
			for _, alt := range tokenAlternatives(value) {
				code := alt[1]
				if translate != nil {
					if sources, exact, mapped := translate(resourceType, field, code); mapped {
						if !exact {
							codes = nil // one alternative cannot be listed: no SQL filter at all
							break
						}
						codes = append(codes, sources...)
						continue
					}
				}
				codes = append(codes, code)
			}
			if codes != nil {
				vars[field+"_in"] = sqlList(codes)
			}
		} else {
			vars[field] = value
			if info.paramType == "token" {
				system, code, hasSystem := strings.Cut(value, "|")
				if !hasSystem {
					system, code = "", value
				}
				if translate != nil {
					if sources, exact, mapped := translate(resourceType, field, code); mapped {
						if exact {
							vars[field+"_in"] = sqlList(sources)
						} // inexact: no SQL filter, the in-Go filter handles it
						continue
					}
				}
				vars[field+"_in"] = sqlList([]string{code})
				if system != "" {
					vars[field+"_system"] = system
				}
			}
		}
	}
	return vars
}

// tokenAlternatives splits a token search value on its unescaped commas into
// [system, code] pairs (system "" when absent), removing the FHIR escapes
// (\, \| \$ \\). "a|1,b|2" -> [[a 1] [b 2]].
func tokenAlternatives(value string) [][2]string {
	var out [][2]string
	for _, alt := range splitEscaped(value, ',') {
		parts := splitEscaped(alt, '|')
		if len(parts) == 1 {
			out = append(out, [2]string{"", unescapeToken(parts[0])})
			continue
		}
		out = append(out, [2]string{unescapeToken(parts[0]), unescapeToken(strings.Join(parts[1:], `\|`))})
	}
	return out
}

func splitEscaped(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case sep:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

func unescapeToken(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// splitDateValue parses a FHIR date value with optional comparison prefix.
//
//	ge2023-01-01 → from=2023-01-01, to=""
//	le2023-12-31 → from="",         to=2023-12-31
//	gt2023-01-01 → from=2023-01-01, to=""
//	lt2023-12-31 → from="",         to=2023-12-31
//	eq2023-06-01 → from=2023-06-01, to=2023-06-01
//	2023-06-01   → from=2023-06-01, to=2023-06-01  (exact, no prefix)
func splitDateValue(value string) (from, to string) {
	if len(value) < 3 {
		return value, value
	}
	prefix, date := value[:2], value[2:]
	switch strings.ToLower(prefix) {
	case "ge", "gt":
		return date, ""
	case "le", "lt":
		return "", date
	case "eq":
		return date, date
	}
	// No recognised prefix — treat as exact match
	return value, value
}

// ParamTranslator reverse-maps a FHIR search code of resourceType.field to
// the source codes stored in the database (see converter.SourceCodes).
type ParamTranslator func(resourceType, field, code string) (sourceCodes []string, exact, mapped bool)

// sqlList renders values as a quoted, comma-separated SQL list ('a', 'b').
// An empty list renders NULL, so "IN (NULL)" matches nothing instead of
// being a syntax error.
func sqlList(values []string) string {
	if len(values) == 0 {
		return "NULL"
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return strings.Join(quoted, ", ")
}
