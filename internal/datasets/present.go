package datasets

import "strings"

// Present reports whether a resource (its JSON as a map) holds a value for an
// element of a data requirement: "code", a choice element "value[x]" (any of
// valueQuantity, valueCodeableConcept, ...), or a dotted path "code.coding"
// (present when any repetition holds it). null, "", [] and {} count as absent.
func Present(resource map[string]interface{}, element string) bool {
	return present(resource, strings.Split(element, "."))
}

func present(v interface{}, path []string) bool {
	if len(path) == 0 {
		return hasValue(v)
	}
	switch x := v.(type) {
	case []interface{}:
		for _, item := range x {
			if present(item, path) {
				return true
			}
		}
		return false
	case map[string]interface{}:
		name := path[0]
		if base, ok := strings.CutSuffix(name, "[x]"); ok {
			for k, child := range x {
				if rest, ok := strings.CutPrefix(k, base); ok && rest != "" && rest[0] >= 'A' && rest[0] <= 'Z' && present(child, path[1:]) {
					return true
				}
			}
			return false
		}
		child, ok := x[name]
		return ok && present(child, path[1:])
	}
	return false
}

func hasValue(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case []interface{}:
		for _, item := range x {
			if hasValue(item) {
				return true
			}
		}
		return false
	case map[string]interface{}:
		return len(x) > 0
	}
	return true
}
