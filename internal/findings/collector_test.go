package findings

import (
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
