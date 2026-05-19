package querycompiler

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

type searchParamInfo struct {
	paramType  string // "date", "token", "string", "reference", …
	expression string // full FHIRPath expression (may cover multiple resource types)
}

// searchParamIndex maps resourceType → paramCode → info.
type searchParamIndex map[string]map[string]searchParamInfo

var stripSuffix = regexp.MustCompile(`(\[x\]|\s+\(as\s+\w+\)).*`)

func loadSearchParams(filePath string) (searchParamIndex, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", filePath, err)
	}

	var bundle struct {
		Entry []struct {
			Resource struct {
				Code       string   `json:"code"`
				Base       []string `json:"base"`
				Type       string   `json:"type"`
				Expression string   `json:"expression"`
			} `json:"resource"`
		} `json:"entry"`
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("parsing search-parameter.json: %w", err)
	}

	idx := make(searchParamIndex)
	for _, e := range bundle.Entry {
		r := e.Resource
		if r.Code == "" || r.Expression == "" {
			continue
		}
		for _, base := range r.Base {
			if idx[base] == nil {
				idx[base] = make(map[string]searchParamInfo)
			}
			idx[base][r.Code] = searchParamInfo{
				paramType:  r.Type,
				expression: r.Expression,
			}
		}
	}
	return idx, nil
}

// fieldName extracts the immediate FHIR field name for resourceType from a
// potentially multi-resource expression, then strips polymorphic markers.
//
//	"Observation.status"                         → "status"
//	"... | Observation.effective | ..."          → "effective"
//	"Observation.effective[x]"                   → "effective"
//	"(RiskAssessment.occurrence as dateTime)"    → "occurrence"
func fieldName(resourceType, expression string) string {
	prefix := resourceType + "."
	for _, part := range strings.Split(expression, "|") {
		part = strings.TrimSpace(part)
		// strip leading ( if present
		part = strings.TrimPrefix(part, "(")
		if !strings.HasPrefix(part, prefix) {
			continue
		}
		field := strings.TrimPrefix(part, prefix)
		field = stripSuffix.ReplaceAllString(field, "")
		// take only the first path segment (no sub-paths like "subject.where(…)")
		if dot := strings.IndexByte(field, '.'); dot != -1 {
			field = field[:dot]
		}
		return strings.TrimSpace(field)
	}
	return "" // not found for this resource type
}
