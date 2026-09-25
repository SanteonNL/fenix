package deident

import (
	"fmt"
	"time"
)

// dateOnlyLayout is the layout every fhir.Date value round-trips through
// (internal/models/fhir/date.go), and the layout first-of-month/clamp-age
// results are always formatted with, regardless of the input's granularity —
// both actions are birth-date-shaped operations that intentionally discard
// anything finer than a day.
const dateOnlyLayout = "2006-01-02"

// dateishLayouts are tried in order, most specific first, to parse a raw
// *string field that Walk classified as KindDateString — FHIR date/dateTime/
// instant values vary in granularity (year, year-month, date, or full
// datetime with an optional timezone).
var dateishLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05",
	dateOnlyLayout,
	"2006-01",
	"2006",
}

// ParseFHIRDateish parses s against the FHIR date/dateTime/instant layouts,
// returning the parsed time and the layout that matched, so the caller can
// format a transformed value back at the same granularity (used by shift,
// which must preserve whatever precision the source value had).
func ParseFHIRDateish(s string) (t time.Time, layout string, err error) {
	for _, l := range dateishLayouts {
		if t, err = time.Parse(l, s); err == nil {
			return t, l, nil
		}
	}
	return time.Time{}, "", fmt.Errorf("deident: %q does not match any known FHIR date/dateTime/instant layout", s)
}

// ShiftDate moves t by offsetDays, preserving its time-of-day and location —
// shift only changes the date, so intervals between a patient's events
// (recorded at whatever precision the source data used) are preserved.
func ShiftDate(t time.Time, offsetDays int) time.Time {
	return t.AddDate(0, 0, offsetDays)
}

// FirstOfMonth floors t to the first day of its month, removing day-level
// precision.
func FirstOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// ClampAge bounds birthDate so the whole-years age it implies, as of asOf,
// falls within [minAge, maxAge]. A birth date implying an age above maxAge
// is brought forward to the maxAge boundary; one implying an age below
// minAge is brought back to the minAge boundary; otherwise it is untouched.
func ClampAge(birthDate time.Time, minAge, maxAge int, asOf time.Time) time.Time {
	switch age := ageAt(birthDate, asOf); {
	case age > maxAge:
		return asOf.AddDate(-maxAge, 0, 0)
	case age < minAge:
		return asOf.AddDate(-minAge, 0, 0)
	default:
		return birthDate
	}
}

// ageAt computes the whole-years age of birthDate as of asOf.
func ageAt(birthDate, asOf time.Time) int {
	age := asOf.Year() - birthDate.Year()
	if asOf.Month() < birthDate.Month() || (asOf.Month() == birthDate.Month() && asOf.Day() < birthDate.Day()) {
		age--
	}
	return age
}
