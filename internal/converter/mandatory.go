package converter

import (
	"reflect"
	"strings"
	"sync"
)

var mandatoryCache sync.Map // resource type -> map[string]bool

// MandatoryElements returns the top-level elements of a resource type that
// FHIR requires (min 1), read from the FHIR model: a field whose JSON tag has
// no omitempty, e.g. Observation.status and Observation.code. Unknown types
// have none.
func MandatoryElements(resourceType string) map[string]bool {
	if v, ok := mandatoryCache.Load(resourceType); ok {
		return v.(map[string]bool)
	}
	out := map[string]bool{}
	if t, ok := cachedFHIRType(resourceType); ok && t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			parts := strings.Split(t.Field(i).Tag.Get("json"), ",")
			name := parts[0]
			if name == "" || name == "-" || name == "resourceType" {
				continue
			}
			omit := false
			for _, p := range parts[1:] {
				if p == "omitempty" {
					omit = true
				}
			}
			if !omit {
				out[name] = true
			}
		}
	}
	mandatoryCache.Store(resourceType, out)
	return out
}
