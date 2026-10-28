package findings

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	maxExamples = 3
	maxValues   = 50 // distinct offending values kept per finding
)

// ValueCount is one offending value and how many resources carried it.
type ValueCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Sample is the full resource of one example, as the converter built it (after
// concept mapping, with the offending values still in place).
type Sample struct {
	ID       string          `json:"id"`
	Resource json.RawMessage `json:"resource"`
}

// Record is one line of the findings file. Kind "finding" is an aggregated
// rule violation; kind "summary" says how many resources of a type were seen,
// which is the denominator a dashboard needs to turn counts into rates.
type Record struct {
	Kind     string    `json:"kind"`
	RunID    string    `json:"run_id"`
	Time     time.Time `json:"time"`
	Source   string    `json:"source,omitempty"`
	Query    string    `json:"query,omitempty"` // what produced the resources: the SQL file, or the group of an $export; runs with the same source and query are comparable
	Resource string    `json:"resource_type,omitempty"`
	Profile  string    `json:"profile,omitempty"` // StructureDefinition the rule came from

	// kind = finding
	Severity    Severity     `json:"severity"`
	Code        string       `json:"code,omitempty"`
	Path        string       `json:"path,omitempty"`
	Message     string       `json:"message,omitempty"`
	Count       int          `json:"count,omitempty"`        // resources affected
	Examples    []string     `json:"examples,omitempty"`     // ids of a few affected resources
	Samples     []Sample     `json:"samples,omitempty"`      // their full resources, only when the collector has IncludeResources set
	Values      []ValueCount `json:"values,omitempty"`       // offending values, most frequent first (capped, see maxValues)
	OtherValues int          `json:"other_values,omitempty"` // resources whose value did not fit under the cap

	// kind = profile: what the source delivered for one element (Path), counted while
	// the converter reads its rows, so before any concept map has changed a value.
	// Values holds the values with their counts when the element is a code-like field.
	Rows     int    `json:"rows,omitempty"`     // rows of the element's fhir_path (resources, for a root element)
	Empty    int    `json:"empty,omitempty"`    // of those, rows where the element is NULL or empty
	Distinct int    `json:"distinct,omitempty"` // distinct non-empty values
	Min      string `json:"min,omitempty"`      // numbers that are not listed value by value
	Max      string `json:"max,omitempty"`
	Note     string `json:"note,omitempty"` // why the values are not listed

	// kind = summary
	Seen    int `json:"seen,omitempty"`    // resources of this type that were built
	Skipped int `json:"skipped,omitempty"` // of those, resources dropped because they failed validation
}

type valueTally struct {
	by    map[string]int
	other int
}

type key struct {
	code, resource, profile, path, message string
}

type typeStats struct{ seen, skipped int }

// Collector aggregates findings. It is safe for concurrent use: the
// converter's build workers all report into the same one.
type Collector struct {
	// IncludeResources keeps the full resource of each example in the findings.
	// Those are real data: leave it off unless the findings file is as protected
	// as the data itself. Set it before the collector is shared.
	IncludeResources bool

	// Profile counts the values of every element while the converter reads its rows
	// (see ProfileTally). ProfileMaxDistinct and ProfileSkip decide which elements
	// are listed value by value. Set before the collector is shared.
	Profile            bool
	ProfileMaxDistinct int
	ProfileSkip        []string

	mu    sync.Mutex
	by    map[key]*Record
	vals  map[key]*valueTally
	stats map[string]*typeStats
	prof  *ProfileTally
}

func NewCollector() *Collector {
	return &Collector{by: map[key]*Record{}, vals: map[key]*valueTally{}, stats: map[string]*typeStats{}}
}

// Add records that one resource violated a rule. example is the resource id;
// value is the offending value ("man" for an invalid gender code), "" when the
// rule has none (a missing element). A rule that fires several times within
// one resource should be added once per resource, so Count means "resources
// affected" and the value counts add up to it.
func (c *Collector) Add(sev Severity, code, resourceType, profile, path, message, example, value string) {
	c.AddSample(sev, code, resourceType, profile, path, message, example, value, nil)
}

// NeedsSample reports whether a finding would still keep the resource of one
// more example, so a caller can skip serialising a resource nobody will keep.
func (c *Collector) NeedsSample(code, resourceType, profile, path, message string) bool {
	if !c.IncludeResources {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.by[key{code, resourceType, profile, path, message}]
	return r == nil || len(r.Samples) < maxExamples
}

// AddSample is Add plus the example's full resource (JSON), kept when the
// collector has IncludeResources set and the finding has room for it.
func (c *Collector) AddSample(sev Severity, code, resourceType, profile, path, message, example, value string, resource []byte) {
	k := key{code, resourceType, profile, path, message}
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.by[k]
	if !ok {
		r = &Record{Kind: "finding", Severity: sev, Code: code, Resource: resourceType, Profile: profile, Path: path, Message: message}
		c.by[k] = r
	}
	r.Count++
	if example != "" && len(r.Examples) < maxExamples {
		r.Examples = append(r.Examples, example)
		if c.IncludeResources && resource != nil {
			r.Samples = append(r.Samples, Sample{ID: example, Resource: json.RawMessage(resource)})
		}
	}
	if value == "" {
		return
	}
	t := c.vals[k]
	if t == nil {
		t = &valueTally{by: map[string]int{}}
		c.vals[k] = t
	}
	if _, known := t.by[value]; known || len(t.by) < maxValues {
		t.by[value]++
	} else {
		t.other++
	}
}

// ResourceDone counts one built resource of resourceType; skipped says it was
// dropped because validation failed.
func (c *Collector) ResourceDone(resourceType string, skipped bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.stats[resourceType]
	if s == nil {
		s = &typeStats{}
		c.stats[resourceType] = s
	}
	s.seen++
	if skipped {
		s.skipped++
	}
}

// Empty reports whether nothing was collected.
func (c *Collector) Empty() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.by) == 0 && len(c.stats) == 0 && c.prof == nil
}

// Drain returns everything collected so far as records stamped with the run
// and source, and clears the collector. Findings come most severe first, then
// the per-type summaries.
func (c *Collector) Drain(runID, source, query string, now time.Time) []Record {
	c.mu.Lock()
	by, vals, stats, prof := c.by, c.vals, c.stats, c.prof
	c.by, c.vals, c.stats, c.prof = map[key]*Record{}, map[key]*valueTally{}, map[string]*typeStats{}, nil
	c.mu.Unlock()

	out := make([]Record, 0, len(by)+len(stats))
	for k, r := range by {
		r.RunID, r.Time, r.Source, r.Query = runID, now, source, query
		if t := vals[k]; t != nil {
			for v, n := range t.by {
				r.Values = append(r.Values, ValueCount{v, n})
			}
			sort.Slice(r.Values, func(i, j int) bool {
				if r.Values[i].Count != r.Values[j].Count {
					return r.Values[i].Count > r.Values[j].Count
				}
				return r.Values[i].Value < r.Values[j].Value
			})
			r.OtherValues = t.other
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		if a.Resource != b.Resource {
			return a.Resource < b.Resource
		}
		return fmt.Sprint(a.Code, a.Path, a.Message) < fmt.Sprint(b.Code, b.Path, b.Message)
	})
	types := make([]string, 0, len(stats))
	for t := range stats {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		out = append(out, Record{Kind: "summary", RunID: runID, Time: now, Source: source, Query: query, Resource: t, Seen: stats[t].seen, Skipped: stats[t].skipped})
	}
	for _, r := range profileRecords(prof, c.ProfileMaxDistinct, c.ProfileSkip) {
		r.RunID, r.Time, r.Source, r.Query = runID, now, source, query
		out = append(out, r)
	}
	return out
}
