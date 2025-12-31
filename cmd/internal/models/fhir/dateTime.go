package fhir

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DateTime represents a FHIR dateTime
type DateTime struct {
	time.Time
	Precision string // "YYYY", "YYYY-MM", "YYYY-MM-DD", or "FULL"
}

// NewDateTime creates a new DateTime from a time.Time
func NewDateTime(t time.Time) DateTime {
	return DateTime{
		Time:      t,
		Precision: "FULL",
	}
}

// String returns the datetime in FHIR format based on precision
func (d DateTime) String() string {
	if d.Time.IsZero() {
		return ""
	}

	switch d.Precision {
	case "YYYY":
		return d.Time.Format("2006")
	case "YYYY-MM":
		return d.Time.Format("2006-01")
	case "YYYY-MM-DD":
		return d.Time.Format("2006-01-02")
	default:
		t := d.Time
		baseFormat := "2006-01-02T15:04:05.000" // Always include milliseconds

		if t.Location() == time.UTC {
			return t.Format(baseFormat + "Z")
		}

		_, offset := t.Zone()
		hours := offset / 3600
		minutes := (offset % 3600) / 60

		if hours == 0 && minutes == 0 {
			return t.Format(baseFormat + "Z")
		}

		return fmt.Sprintf("%s%+03d:%02d", t.Format(baseFormat), hours, minutes)
	}
}

// MarshalJSON implements the json.Marshaler interface
func (d DateTime) MarshalJSON() ([]byte, error) {
	if d.Time.IsZero() {
		return json.Marshal("")
	}
	return json.Marshal(d.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (d *DateTime) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "" {
		d.Time = time.Time{}
		d.Precision = ""
		return nil
	}

	// Parse partial dates
	switch len(s) {
	case 4: // YYYY
		if t, err := time.Parse("2006", s); err == nil {
			d.Time = t
			d.Precision = "YYYY"
			return nil
		}
	case 7: // YYYY-MM
		if t, err := time.Parse("2006-01", s); err == nil {
			d.Time = t
			d.Precision = "YYYY-MM"
			return nil
		}
	case 10: // YYYY-MM-DD
		if t, err := time.Parse("2006-01-02", s); err == nil {
			d.Time = t
			d.Precision = "YYYY-MM-DD"
			return nil
		}
	}

	// Try parsing full datetime formats
	formats := []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05.000-07:00",
		"2006-01-02T15:04:05.000+07:00",
		"2006-01-02T15:04:05Z",      // Support reading without milliseconds
		"2006-01-02T15:04:05-07:00", // Support reading without milliseconds
		"2006-01-02T15:04:05+07:00", // Support reading without milliseconds
	}

	var lastErr error
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			d.Time = t
			d.Precision = "FULL"
			return nil
		} else {
			lastErr = err
		}
	}

	return fmt.Errorf("invalid datetime format: %s (last error: %v)", s, lastErr)
}
