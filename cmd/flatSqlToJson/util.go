package main

import (
	"reflect"
	"strings"
)

func getStringValue(field reflect.Value) string {
	if field.Kind() == reflect.Ptr {
		if field.IsNil() {
			return ""
		}
		field = field.Elem()
	}
	sanitized := strings.ReplaceAll(field.String(), "\"", "") // Remove quotes
	sanitized = strings.ReplaceAll(sanitized, "\n", "")       // Remove newlines
	sanitized = strings.TrimSpace(sanitized)                  // Trim whitespace
	return sanitized
}
