package findings

import (
	"fmt"
	"testing"
	"time"
)

func profileOf(t *testing.T, c *Collector) map[string]Record {
	t.Helper()
	out := map[string]Record{}
	for _, r := range c.Drain("run", "src", "q", time.Time{}) {
		if r.Kind == "profile" {
			out[r.Path] = r
		}
	}
	return out
}

func TestProfileListsCodesAndHidesMeasurements(t *testing.T) {
	c := NewCollector()
	a := NewProfileTally()
	for i := 0; i < 30; i++ {
		a.Row("Observation")
		status := []string{"final", "final", "preliminary", ""}[i%4]
		a.Cell("Observation", "Observation.status", status, status != "")
		a.Cell("Observation", "Observation.valueQuantity.value", fmt.Sprintf("%d.5", i), true)
		a.Cell("Observation", "Observation.id", fmt.Sprintf("id-%d", i), true)
	}
	// a second statement (or batch) adds to the same element
	b := NewProfileTally()
	b.Row("Observation")
	b.Cell("Observation", "Observation.status", "amended", true)
	for i := 0; i < 5; i++ {
		b.Cell("Observation", "Observation.id", fmt.Sprintf("extra-%d", i), true) // 35 distinct ids: over the listing limit of 30
	}
	c.MergeProfile(a)
	c.MergeProfile(b)

	p := profileOf(t, c)
	st := p["Observation.status"]
	if st.Rows != 31 || st.Empty != 7 || st.Distinct != 3 || len(st.Values) != 3 || st.Values[0].Value != "final" || st.Values[0].Count != 16 {
		t.Errorf("status: %+v", st)
	}
	v := p["Observation.valueQuantity.value"]
	if len(v.Values) != 0 || v.Min != "0.5" || v.Max != "29.5" || v.Note == "" {
		t.Errorf("a measurement is not listed but has a range: %+v", v)
	}
	if id := p["Observation.id"]; len(id.Values) != 0 || id.Distinct != 35 || id.Note == "" {
		t.Errorf("ids are too many to list: %+v", id)
	}
}

func TestProfileSkipAndCap(t *testing.T) {
	c := NewCollector()
	c.ProfileSkip = []string{"birthDate", "Patient.name.family"}
	a := NewProfileTally()
	for i := 0; i < profileCap+50; i++ {
		a.Row("Patient")
		a.Cell("Patient", "Patient.birthDate", "1980-01-01", true)
		a.Cell("Patient", "Patient.name.family", "x", true)
		a.Cell("Patient", "Patient.identifier.value", fmt.Sprint(i), true)
	}
	c.MergeProfile(a)
	p := profileOf(t, c)
	if r := p["Patient.birthDate"]; r.Note != "excluded by config" || len(r.Values) != 0 {
		t.Errorf("birthDate: %+v", r)
	}
	if r := p["Patient.name.family"]; r.Note != "excluded by config" {
		t.Errorf("family: %+v", r)
	}
	if r := p["Patient.identifier.value"]; r.Distinct != profileCap || r.Note == "" || len(r.Values) != 0 {
		t.Errorf("identifier beyond the cap: %+v", r)
	}
}

func TestProfileSurvivesFlushOnlyOnce(t *testing.T) {
	c := NewCollector()
	p := NewProfileTally()
	p.Row("Patient")
	p.Cell("Patient", "Patient.gender", "M", true)
	c.MergeProfile(p)
	if c.Empty() {
		t.Fatal("a collector holding only a profile is not empty")
	}
	if got := profileOf(t, c); len(got) != 1 {
		t.Fatalf("first drain: %v", got)
	}
	if !c.Empty() {
		t.Error("drain clears the profile")
	}
}

func TestMeasurementWithJunkIsStillAMeasurementButNumericCodesAreNot(t *testing.T) {
	c := NewCollector()
	a := NewProfileTally()
	// 16 distinct results, a quarter of them junk: still a measurement
	for i := 0; i < 12; i++ {
		a.Row("Observation")
		a.Cell("Observation", "Observation.value[x].value", fmt.Sprintf("%d.5", i), true)
	}
	for _, junk := range []string{"abc", "abc", "n/a", "1,5"} {
		a.Row("Observation")
		a.Cell("Observation", "Observation.value[x].value", junk, true)
	}
	// a score from 1 to 12 over many rows: a code list, listed
	for i := 0; i < 240; i++ {
		a.Row("Observation")
		a.Cell("Observation", "Observation.score", fmt.Sprint(i%12+1), true)
	}
	c.MergeProfile(a)
	p := profileOf(t, c)
	if r := p["Observation.value[x].value"]; len(r.Values) != 0 || r.Min == "" || r.Note == "" {
		t.Errorf("a dirty measurement is hidden with its range: %+v", r)
	}
	if r := p["Observation.score"]; len(r.Values) != 12 || r.Note != "" {
		t.Errorf("a numeric code list is listed: %+v", r)
	}
}
