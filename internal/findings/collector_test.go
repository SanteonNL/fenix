package findings

import (
	"strings"
	"testing"
	"time"
)

func TestSamplesAreOffByDefault(t *testing.T) {
	c := NewCollector()
	if c.NeedsSample("FX-DATA-002", "Patient", "", "Patient.gender", "m") {
		t.Fatal("NeedsSample must be false unless IncludeResources is set")
	}
	c.AddSample(Error, "FX-DATA-002", "Patient", "", "Patient.gender", "m", "p1", "man", []byte(`{"id":"p1"}`))
	recs := c.Drain("r", "s", "q", time.Time{})
	if len(recs) != 1 || len(recs[0].Samples) != 0 || len(recs[0].Examples) != 1 {
		t.Fatalf("unexpected: %+v", recs)
	}
}

func TestSamplesKeepTheFirstFewResources(t *testing.T) {
	c := NewCollector()
	c.IncludeResources = true
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if c.NeedsSample("FX-DATA-002", "Patient", "", "Patient.gender", "m") {
			c.AddSample(Error, "FX-DATA-002", "Patient", "", "Patient.gender", "m", id, "man", []byte(`{"id":"`+id+`"}`))
		} else {
			c.Add(Error, "FX-DATA-002", "Patient", "", "Patient.gender", "m", id, "man")
		}
	}
	r := c.Drain("r", "s", "q", time.Time{})[0]
	if r.Count != 5 || len(r.Examples) != 3 || len(r.Samples) != 3 {
		t.Fatalf("count %d, examples %v, samples %d", r.Count, r.Examples, len(r.Samples))
	}
	if r.Samples[1].ID != "b" || string(r.Samples[1].Resource) != `{"id":"b"}` {
		t.Errorf("sample = %+v", r.Samples[1])
	}
}

func TestCleanKeepsSamplesOfGoodResources(t *testing.T) {
	c := NewCollector()
	c.IncludeResources = true
	calls := 0
	for _, id := range []string{"p1", "p2", "p3", "p4", "p5"} {
		c.ResourceDone("Patient", false)
		c.Clean("Patient", id, func() []byte { calls++; return []byte(`{"id":"` + id + `"}`) })
	}
	c.ResourceDone("Patient", true)

	var sum *Record
	for _, r := range c.Drain("run", "src", "q", time.Now()) {
		if r.Kind == "summary" && r.Resource == "Patient" {
			r := r
			sum = &r
		}
	}
	if sum == nil {
		t.Fatal("no Patient summary")
	}
	if sum.Seen != 6 || sum.Skipped != 1 || sum.Clean == nil || *sum.Clean != 5 {
		t.Fatalf("seen/skipped/clean = %d/%d/%v, want 6/1/5", sum.Seen, sum.Skipped, sum.Clean)
	}
	if len(sum.Examples) != maxExamples || len(sum.Samples) != maxExamples || sum.Samples[0].ID != "p1" {
		t.Fatalf("examples %v, samples %d", sum.Examples, len(sum.Samples))
	}
	if calls != maxExamples {
		t.Fatalf("resource serialised %d times, want %d", calls, maxExamples)
	}
}

// A check after conversion can take back a resource the converter counted clean.
func TestNotCleanTakesBackACleanResource(t *testing.T) {
	c := NewCollector()
	c.IncludeResources = true
	for _, id := range []string{"V1", "V2", "V3"} {
		c.Clean("Procedure", id, func() []byte { return []byte(`{}`) })
	}
	c.NotClean("Procedure", "V2")
	c.NotClean("Procedure", "V2")      // twice: still taken back once
	c.NotClean("Procedure", "unknown") // never counted clean: nothing changes
	c.NotClean("Observation", "V1")    // other type: nothing changes

	recs := c.Drain("run", "src", "q", time.Time{})
	if len(recs) != 1 || recs[0].Kind != "summary" {
		t.Fatalf("records = %+v, want one summary", recs)
	}
	s := recs[0]
	if *s.Clean != 2 {
		t.Errorf("clean = %d, want 2", *s.Clean)
	}
	if strings.Join(s.Examples, ",") != "V1,V3" || len(s.Samples) != 2 || s.Samples[1].ID != "V3" {
		t.Errorf("examples %v, samples %+v; want V1 and V3 only", s.Examples, s.Samples)
	}
}
