package fhirserver

import (
	"reflect"
	"testing"
)

func obs(id, status, code, subjectID, effectiveDateTime string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType":      "Observation",
		"id":                id,
		"status":            status,
		"code":              map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": "http://x", "code": code}}},
		"subject":           map[string]interface{}{"reference": "Patient/" + subjectID},
		"effectiveDateTime": effectiveDateTime,
	}
}

// lookupFor returns a searchParamLookup backed by a fixed table, so
// filterUnwiredParams can be tested without a real
// terminology/searchparameter/search-parameter.json-backed compiler.
func lookupFor(table map[string][2]string) searchParamLookup {
	return func(_, code string) (string, string, bool) {
		v, ok := table[code]
		if !ok {
			return "", "", false
		}
		return v[0], v[1], true
	}
}

func TestFilterUnwiredParams(t *testing.T) {
	resources := []interface{}{
		obs("1", "final", "A", "123", "2023-01-10T10:00:00Z"),
		obs("2", "final", "B", "456", "2023-06-15T10:00:00Z"),
		obs("3", "preliminary", "A", "789", "2023-12-20T10:00:00Z"),
	}
	lookup := lookupFor(map[string][2]string{
		"status":  {"status", "token"},
		"code":    {"code", "token"},
		"patient": {"subject", "reference"},
		"date":    {"effective", "date"}, // choice-type: field "effective" strips the [x], JSON key is "effectiveDateTime"
	})

	t.Run("no params leaves resources untouched", func(t *testing.T) {
		got := filterUnwiredParams("Observation", map[string]string{}, nil, resources, lookup)
		if !reflect.DeepEqual(got, resources) {
			t.Errorf("got %v, want unchanged %v", got, resources)
		}
	})

	t.Run("pushed-down param is skipped, not re-filtered", func(t *testing.T) {
		pushed := map[string]bool{"status": true}
		got := filterUnwiredParams("Observation", map[string]string{"status": "final"}, pushed, resources, lookup)
		if len(got) != 3 {
			t.Fatalf("expected pushed-down param to be left alone (3 resources), got %d", len(got))
		}
	})

	t.Run("unwired token param filters down to the match", func(t *testing.T) {
		got := filterUnwiredParams("Observation", map[string]string{"code": "A"}, nil, resources, lookup)
		if len(got) != 2 {
			t.Fatalf("expected 2 matches for code=A, got %d: %v", len(got), got)
		}
	})

	t.Run("unwired reference param filters to the subject", func(t *testing.T) {
		got := filterUnwiredParams("Observation", map[string]string{"patient": "456"}, nil, resources, lookup)
		if len(got) != 1 {
			t.Fatalf("expected 1 match for patient=456, got %d: %v", len(got), got)
		}
	})

	t.Run("unwired date param resolves the choice-type JSON key", func(t *testing.T) {
		got := filterUnwiredParams("Observation", map[string]string{"date": "ge2023-06-01"}, nil, resources, lookup)
		if len(got) != 2 {
			t.Fatalf("expected 2 matches for date=ge2023-06-01, got %d: %v", len(got), got)
		}
	})

	t.Run("combined params AND together", func(t *testing.T) {
		got := filterUnwiredParams("Observation", map[string]string{"status": "final", "code": "A"}, nil, resources, lookup)
		if len(got) != 1 {
			t.Fatalf("expected 1 match for status=final&code=A, got %d: %v", len(got), got)
		}
	})

	t.Run("unknown param is left alone, same as before this filter existed", func(t *testing.T) {
		got := filterUnwiredParams("Observation", map[string]string{"not-a-real-param": "x"}, nil, resources, lookup)
		if len(got) != 3 {
			t.Fatalf("expected unknown param to be ignored (3 resources), got %d", len(got))
		}
	})
}

func TestResolveField(t *testing.T) {
	fields := map[string]interface{}{
		"status":            "final",
		"effectiveDateTime": "2023-06-01T00:00:00Z",
	}

	if got := resolveField(fields, "status"); got != "final" {
		t.Errorf("direct lookup: got %v, want final", got)
	}
	if got := resolveField(fields, "effective"); got != "2023-06-01T00:00:00Z" {
		t.Errorf("choice-type prefix lookup: got %v, want the effectiveDateTime value", got)
	}
	if got := resolveField(fields, "no-such-field"); got != nil {
		t.Errorf("missing field: got %v, want nil", got)
	}
}

func TestMatchToken(t *testing.T) {
	coding := map[string]interface{}{"coding": []interface{}{
		map[string]interface{}{"system": "http://sys-a", "code": "X"},
	}}

	tests := []struct {
		name  string
		value interface{}
		want  string
		match bool
	}{
		{"plain string exact", "final", "final", true},
		{"plain string mismatch", "final", "preliminary", false},
		{"coding bare code", coding, "X", true},
		{"coding system|code", coding, "http://sys-a|X", true},
		{"coding wrong system", coding, "http://sys-b|X", false},
		{"coding wrong code", coding, "Y", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchToken(tt.value, tt.want); got != tt.match {
				t.Errorf("matchToken(%v, %q) = %v, want %v", tt.value, tt.want, got, tt.match)
			}
		})
	}
}

func TestMatchReference(t *testing.T) {
	ref := map[string]interface{}{"reference": "Patient/123"}

	tests := []struct {
		name  string
		want  string
		match bool
	}{
		{"bare id", "123", true},
		{"typed reference", "Patient/123", true},
		{"wrong id", "456", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchReference(ref, tt.want); got != tt.match {
				t.Errorf("matchReference(%v, %q) = %v, want %v", ref, tt.want, got, tt.match)
			}
		})
	}
}

func TestMatchDate(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
		match bool
	}{
		{"exact", "2023-06-01", "2023-06-01", true},
		{"ge true", "2023-06-01", "ge2023-01-01", true},
		{"ge false", "2023-06-01", "ge2024-01-01", false},
		{"lt true", "2023-06-01", "lt2024-01-01", true},
		{"prefix match on dateTime", "2023-06-01T10:00:00Z", "2023-06-01", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchDate(tt.value, tt.want); got != tt.match {
				t.Errorf("matchDate(%q, %q) = %v, want %v", tt.value, tt.want, got, tt.match)
			}
		})
	}
}
