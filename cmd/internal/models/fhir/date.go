package fhir

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Date represents a FHIR date value
type Date struct {
	time time.Time
}

// MarshalJSON implements the json.Marshaler interface
func (d Date) MarshalJSON() ([]byte, error) {
	if d.time.IsZero() {
		return []byte("null"), nil
	}
	return []byte(fmt.Sprintf(`"%s"`, d.time.Format("2006-01-02"))), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (d *Date) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "null" || s == "" {
		d.time = time.Time{}
		return nil
	}

	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return fmt.Errorf("failed to parse date %s: %v", s, err)
	}

	d.time = t
	return nil
}

// String provides a string representation
func (d Date) String() string {
	return d.time.Format("2006-01-02")
}

// Time returns the underlying time.Time
func (d Date) Time() time.Time {
	return d.time
}

// IsZero reports whether the date represents the zero time instant
func (d Date) IsZero() bool {
	return d.time.IsZero()
}

// Equal compares two dates for equality (ignoring time components)
func (d Date) Equal(other Date) bool {
	return d.String() == other.String()
}

// Before checks if this date is before another (ignoring time components)
func (d Date) Before(other Date) bool {
	return d.String() < other.String()
}

// After checks if this date is after another (ignoring time components)
func (d Date) After(other Date) bool {
	return d.String() > other.String()
}

// NewDate creates a new Date from a time.Time
func NewDate(t time.Time) Date {
	// Strip time component by parsing only the date part
	dateStr := t.Format("2006-01-02")
	t, _ = time.Parse("2006-01-02", dateStr)
	return Date{time: t}
}

// ParseDate parses a date string in YYYY-MM-DD format
func ParseDate(s string) (Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return Date{}, fmt.Errorf("invalid date format: %v", err)
	}
	return Date{time: t}, nil
}

// Make sure we implement the necessary interfaces
var (
	_ json.Unmarshaler = (*Date)(nil)
	_ json.Marshaler   = (*Date)(nil)
)

// Add these helper methods to your Date implementation
// IsDate checks if an interface is of type Date
func IsDate(i interface{}) bool {
	_, ok := i.(Date)
	return ok
}

// IsDatePtr checks if an interface is of type *Date
func IsDatePtr(i interface{}) bool {
	_, ok := i.(*Date)
	return ok
}

// IsDateType checks if a reflect.Type represents Date or *Date
func IsDateType(t reflect.Type) bool {
	if t == nil {
		return false
	}

	// Check for pointer to Date
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Check if it's our Date type
	return t.Name() == "Date" && t.PkgPath() == "your/package/path/fhir"
}
