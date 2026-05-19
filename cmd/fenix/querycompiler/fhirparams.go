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
func buildTemplateVars(resourceType string, fhirParams map[string]string, pushdownCodes []string, idx searchParamIndex) map[string]interface{} {
	vars := make(map[string]interface{})
	resourceIdx := idx[resourceType]

	for _, code := range pushdownCodes {
		value, ok := fhirParams[code]
		if !ok {
			continue
		}
		info, ok := resourceIdx[code]
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
		} else {
			vars[field] = value
		}
	}
	return vars
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
