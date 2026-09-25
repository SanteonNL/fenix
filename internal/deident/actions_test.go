package deident

import (
	"testing"
	"time"
)

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(dateOnlyLayout, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

func TestShiftDatePreservesGranularity(t *testing.T) {
	base := time.Date(2024, 3, 22, 14, 30, 0, 0, time.UTC)
	got := ShiftDate(base, -13)
	want := time.Date(2024, 3, 9, 14, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("ShiftDate() = %v, want %v", got, want)
	}
}

func TestShiftDateAcrossMonthAndYearBoundary(t *testing.T) {
	base := mustParseDate(t, "2024-01-03")
	got := ShiftDate(base, -5)
	want := mustParseDate(t, "2023-12-29")
	if !got.Equal(want) {
		t.Fatalf("ShiftDate() across year boundary = %v, want %v", got, want)
	}
}

func TestFirstOfMonth(t *testing.T) {
	got := FirstOfMonth(mustParseDate(t, "2024-03-22"))
	want := mustParseDate(t, "2024-03-01")
	if !got.Equal(want) {
		t.Fatalf("FirstOfMonth() = %v, want %v", got, want)
	}
}

func TestFirstOfMonthLeapDay(t *testing.T) {
	got := FirstOfMonth(mustParseDate(t, "2024-02-29"))
	want := mustParseDate(t, "2024-02-01")
	if !got.Equal(want) {
		t.Fatalf("FirstOfMonth() on leap day = %v, want %v", got, want)
	}
}

func TestClampAgeWithinRangeUnchanged(t *testing.T) {
	asOf := mustParseDate(t, "2024-01-01")
	birth := mustParseDate(t, "1990-06-15") // age 33
	got := ClampAge(birth, 18, 85, asOf)
	if !got.Equal(birth) {
		t.Fatalf("ClampAge() in-range = %v, want unchanged %v", got, birth)
	}
}

func TestClampAgeAboveMaxBoundsToBoundary(t *testing.T) {
	asOf := mustParseDate(t, "2024-01-01")
	birth := mustParseDate(t, "1920-06-15") // age 103
	got := ClampAge(birth, 18, 85, asOf)
	if age := ageAt(got, asOf); age != 85 {
		t.Fatalf("expected clamped age 85, got %d (%v)", age, got)
	}
}

func TestClampAgeBelowMinBoundsToBoundary(t *testing.T) {
	asOf := mustParseDate(t, "2024-01-01")
	birth := mustParseDate(t, "2020-06-15") // age 3
	got := ClampAge(birth, 18, 85, asOf)
	if age := ageAt(got, asOf); age != 18 {
		t.Fatalf("expected clamped age 18, got %d (%v)", age, got)
	}
}

func TestParseFHIRDateishLayouts(t *testing.T) {
	cases := []string{
		"2024-03-22T14:30:00Z",
		"2024-03-22T14:30:00+02:00",
		"2024-03-22",
		"2024-03",
		"2024",
	}
	for _, s := range cases {
		if _, layout, err := ParseFHIRDateish(s); err != nil {
			t.Errorf("ParseFHIRDateish(%q) error = %v", s, err)
		} else if _, err := time.Parse(layout, s); err != nil {
			t.Errorf("layout %q returned for %q doesn't itself reparse: %v", layout, s, err)
		}
	}
}

func TestParseFHIRDateishRejectsGarbage(t *testing.T) {
	if _, _, err := ParseFHIRDateish("not-a-date"); err == nil {
		t.Fatal("expected an error for an unparseable value")
	}
}
