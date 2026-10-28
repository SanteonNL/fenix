package converter

import (
	"strings"

	"github.com/SanteonNL/fenix/internal/findings"
)

// profiler counts the values of the data columns of one statement's rows (see
// findings.ProfileTally). The values are what the source delivered: they are
// counted as the rows are read, before concept mapping and validation.
type profiler struct {
	tally   *findings.ProfileTally
	cols    []string
	kinds   []int
	fpIdx   int
	profile *ProfileService
	paths   map[string][]string // fhir_path → element path of each column
}

func newProfiler(cols []string, kinds []int, profile *ProfileService) *profiler {
	p := &profiler{tally: findings.NewProfileTally(), cols: cols, kinds: kinds, fpIdx: -1, profile: profile, paths: map[string][]string{}}
	for i, k := range kinds {
		if k == colFHIRPath {
			p.fpIdx = i
		}
	}
	return p
}

// observe counts one scanned row. It does nothing on a nil profiler, so the
// caller does not need to check whether profiling is on.
func (p *profiler) observe(vals []interface{}) {
	if p == nil || p.fpIdx < 0 {
		return
	}
	group := toString(vals[p.fpIdx])
	if group == "" {
		return
	}
	p.tally.Row(group)
	elems, ok := p.paths[group]
	if !ok {
		elems = make([]string, len(p.cols))
		root, rest, _ := strings.Cut(group, ".")
		for i, c := range p.cols {
			if p.kinds[i] != colData {
				continue
			}
			rel := strings.TrimPrefix(rest+"."+c, ".") // "code.coding.code", or just the column for a root row
			if p.profile != nil {
				rel = p.profile.ChoicePath(root, rel)
			}
			elems[i] = root + "." + rel
		}
		p.paths[group] = elems
	}
	for i, k := range p.kinds {
		if k != colData {
			continue
		}
		v := vals[i]
		if v == nil {
			p.tally.Cell(group, elems[i], "", false)
			continue
		}
		p.tally.Cell(group, elems[i], toString(v), true)
	}
}
