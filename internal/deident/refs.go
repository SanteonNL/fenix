package deident

import "strings"

// refID extracts the id portion of a relative FHIR reference string like
// "Patient/abc123" (or "Patient/abc123/_history/1"). Returns "" if ref
// doesn't look like a relative reference.
func refID(ref string) string {
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// refType extracts the resource-type portion of a relative reference
// string like "Patient/abc123", e.g. "Patient".
func refType(ref string) string {
	parts := strings.Split(ref, "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}
