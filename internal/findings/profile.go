package findings

import (
	"path"
	"sort"
	"strconv"
	"strings"
)

// What is tallied per element, and when it is listed value by value.
const (
	profileCap         = 200 // distinct values tracked per element; beyond that only counted
	DefaultMaxDistinct = 30  // list values only for elements with at most this many distinct values
	measurementMin     = 10  // a mostly numeric element with more distinct values than this may be a measurement
	measurementRepeat  = 3.0 // ... unless each value occurs this often or more on average (then it is a code list)
)

type colTally struct {
	group  string // fhir_path whose rows are this element's denominator
	empty  int
	vals   map[string]int
	capped bool // more distinct values than profileCap were seen
}

// ProfileTally counts the values of the elements in the rows of one statement.
// It is not safe for concurrent use: every statement has its own, merged into
// the Collector (MergeProfile) when the statement is done.
type ProfileTally struct {
	rows map[string]int
	cols map[string]*colTally
}

func NewProfileTally() *ProfileTally {
	return &ProfileTally{rows: map[string]int{}, cols: map[string]*colTally{}}
}

// Row counts one row of the fhir_path group.
func (p *ProfileTally) Row(group string) { p.rows[group]++ }

// Cell counts the value of the element at elemPath in a row of group. A NULL
// (present = false) or empty value counts as empty.
func (p *ProfileTally) Cell(group, elemPath, value string, present bool) {
	ct := p.cols[elemPath]
	if ct == nil {
		ct = &colTally{group: group, vals: map[string]int{}}
		p.cols[elemPath] = ct
	}
	if !present || value == "" {
		ct.empty++
		return
	}
	ct.add(value, 1)
}

func (ct *colTally) add(value string, n int) {
	if _, known := ct.vals[value]; known || len(ct.vals) < profileCap {
		ct.vals[value] += n
		return
	}
	ct.capped = true
}

// MergeProfile adds a statement's tally to the collector.
func (c *Collector) MergeProfile(p *ProfileTally) {
	if p == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.prof == nil {
		c.prof = NewProfileTally()
	}
	for g, n := range p.rows {
		c.prof.rows[g] += n
	}
	for elem, ct := range p.cols {
		dst := c.prof.cols[elem]
		if dst == nil {
			dst = &colTally{group: ct.group, vals: map[string]int{}}
			c.prof.cols[elem] = dst
		}
		dst.empty += ct.empty
		dst.capped = dst.capped || ct.capped
		for v, n := range ct.vals {
			dst.add(v, n)
		}
	}
}

// profileRecords turns the collected tallies into profile records, ordered by
// path. skip are patterns (path or last path segment, * and ? wildcards,
// case-insensitive) for elements that are never listed value by value.
func profileRecords(p *ProfileTally, maxDistinct int, skip []string) []Record {
	if p == nil {
		return nil
	}
	if maxDistinct <= 0 {
		maxDistinct = DefaultMaxDistinct
	}
	paths := make([]string, 0, len(p.cols))
	for elem := range p.cols {
		paths = append(paths, elem)
	}
	sort.Strings(paths)
	out := make([]Record, 0, len(paths))
	for _, elem := range paths {
		ct := p.cols[elem]
		rec := Record{Kind: "profile", Resource: strings.SplitN(elem, ".", 2)[0], Path: elem,
			Rows: p.rows[ct.group], Empty: ct.empty, Distinct: len(ct.vals)}
		vs := make([]ValueCount, 0, len(ct.vals))
		for v, n := range ct.vals {
			vs = append(vs, ValueCount{v, n})
		}
		sort.Slice(vs, func(i, j int) bool {
			if vs[i].Count != vs[j].Count {
				return vs[i].Count > vs[j].Count
			}
			return vs[i].Value < vs[j].Value
		})
		switch lo, hi, measured := measurement(vs); {
		case matchesAny(skip, elem):
			rec.Note = "excluded by config"
		case len(vs) == 0:
		case ct.capped:
			rec.Note = "more than " + strconv.Itoa(profileCap) + " distinct values: not listed"
		case measured:
			rec.Note, rec.Min, rec.Max = "measurement: the values differ per observation, so they are not listed", lo, hi
		case len(vs) > maxDistinct:
			rec.Note = "more than " + strconv.Itoa(maxDistinct) + " distinct values: not listed"
		default:
			rec.Values = vs
		}
		out = append(out, rec)
	}
	return out
}

// measurement reports whether the values are numbers that differ per row, and
// their range: many distinct values, at least half of them numeric, and each
// occurring only a few times on average. A code list that happens to be numeric
// (a score of 1 to 12) repeats a lot, so it is not taken for a measurement; a
// measurement column with some junk in it ("abc", "n/a") still is.
func measurement(vs []ValueCount) (lo, hi string, ok bool) {
	if len(vs) <= measurementMin {
		return "", "", false
	}
	var numeric, total int
	var min, max float64
	first := true
	for _, v := range vs {
		total += v.Count
		f, err := strconv.ParseFloat(strings.TrimSpace(v.Value), 64)
		if err != nil {
			continue
		}
		numeric += v.Count
		if first || f < min {
			min = f
		}
		if first || f > max {
			max = f
		}
		first = false
	}
	if total == 0 || float64(numeric)/float64(total) < 0.5 || float64(total)/float64(len(vs)) >= measurementRepeat {
		return "", "", false
	}
	return strconv.FormatFloat(min, 'g', -1, 64), strconv.FormatFloat(max, 'g', -1, 64), true
}

func matchesAny(patterns []string, elem string) bool {
	last := elem[strings.LastIndex(elem, ".")+1:]
	for _, p := range patterns {
		p = strings.ToLower(p)
		for _, name := range []string{strings.ToLower(elem), strings.ToLower(last)} {
			if ok, _ := path.Match(p, name); ok {
				return true
			}
		}
	}
	return false
}
