// Tests for StagingText: every source value becomes the exact text that staging stores.
package source

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStagingText(t *testing.T) {
	testCases := []struct {
		name     string
		value    any
		expected string
	}{
		{"JSON number keeps digits", json.Number("111"), "111"},
		{"JSON number above float precision", json.Number("9007199254740993"), "9007199254740993"},
		{"JSON decimal", json.Number("12.50"), "12.50"},
		{"float without .0", float64(111), "111"},
		{"float without exponent", float64(1234567), "1234567"},
		{"float decimal", 12.5, "12.5"},
		{"integer", int64(42), "42"},
		{"boolean", true, "true"},
		{"string", "abc", "abc"},
		{"time as RFC 3339", time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), "2025-03-01T00:00:00Z"},
		{"bytes as text", []byte("123.45"), "123.45"},
		{"list as JSON, numbers exact", []any{json.Number("1234567"), "x"}, `[1234567,"x"]`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			text, isPresent := StagingText(testCase.value)
			if !isPresent || text != testCase.expected {
				t.Errorf("StagingText(%#v) = %q, %v; want %q, true", testCase.value, text, isPresent, testCase.expected)
			}
		})
	}
}

func TestStagingTextNull(t *testing.T) {
	if _, isPresent := StagingText(nil); isPresent {
		t.Error("nil should be NULL")
	}
	if StagingValue(nil) != nil {
		t.Error("StagingValue(nil) should be nil")
	}
}
