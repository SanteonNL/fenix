package deident

import (
	"fmt"
	"strings"
	"time"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// Resource pairs a FHIR resourceType with its concrete Go value — a pointer
// to a fhir.* struct (e.g. *fhir.Patient), or a map[string]interface{} for a
// resource type the converter has no typed model for (Deidentify refuses to
// run against these — see below).
type Resource struct {
	Type  string
	Value interface{}
}

// walked is one resource together with its walked fields and the original
// id of the patient it is about, captured before any mutation so that
// hashing Patient.id doesn't corrupt the shift-offset linkID computed from
// it (see resolvePatientAnchor).
type walked struct {
	res           Resource
	fields        []Field
	patientAnchor string // original Patient.id this resource is about; "" if none found
}

// Deidentify de-identifies resources in place against rs.
//
// elements, keyed by resource type, is the caller's declared _elements list
// (e.g. parsed from a request's _elements query param); a missing entry for
// a resource type means "use whatever fields are actually present" (the
// struct-walk fallback). related supplies any RelatedPerson resources in
// scope, used to resolve shared shift offsets for linked patients (LinkID).
//
// Returns the coverage report for every resource considered. If any field is
// uncovered, no resource is mutated and the returned error names every
// uncovered path — a request/batch with an uncovered element must not be
// served or written.
func Deidentify(resources []Resource, rs Ruleset, elements map[string][]string, ctx RunContext, related []fhir.RelatedPerson) ([]Coverage, error) {
	all := make([]walked, 0, len(resources))
	var allCoverage []Coverage
	uncoveredSet := map[string]bool{}

	for _, res := range resources {
		if _, isMap := res.Value.(map[string]interface{}); isMap {
			return nil, fmt.Errorf("deident: resource type %q has no typed FHIR model; cannot verify de-identification coverage", res.Type)
		}

		fields := Walk(res.Type, res.Value)
		if declared, ok := elements[res.Type]; ok {
			fields = restrictToDeclared(res.Type, fields, declared)
		}

		cov, uncovered := CheckCoverage(rs, fields)
		allCoverage = append(allCoverage, cov...)
		for _, u := range uncovered {
			uncoveredSet[u] = true
		}

		all = append(all, walked{
			res:           res,
			fields:        fields,
			patientAnchor: resolvePatientAnchor(res.Type, fields),
		})
	}

	if len(uncoveredSet) > 0 {
		paths := make([]string, 0, len(uncoveredSet))
		for p := range uncoveredSet {
			paths = append(paths, p)
		}
		return allCoverage, fmt.Errorf("deident: %d element(s) not covered by any rule: %s", len(paths), strings.Join(paths, ", "))
	}

	covByPath := make(map[string]Coverage, len(allCoverage))
	for _, c := range allCoverage {
		covByPath[c.Path] = c
	}

	// Pass 1: hash every resource's own id field (Patient.id is the standard
	// ruleset's only such rule, but any resource type's id can carry one).
	// Must run, and complete, before pass 2 so date-shift linkIDs (captured
	// in patientAnchor above) still reflect the original id.
	for _, w := range all {
		idPath := w.res.Type + ".id"
		cov, ok := covByPath[idPath]
		if !ok {
			continue
		}
		for _, f := range w.fields {
			if f.Path != idPath {
				continue
			}
			for _, r := range cov.Rules {
				if r.Action == ActionHash {
					if old, ok := f.Get(); ok {
						f.Set(ctx.Hash(old))
					}
				}
			}
		}
	}

	// Pass 2: everything else — hash (e.g. identifier), shift, first-of-month,
	// clamp-age, and any reference to a resource type whose id is hashed.
	// none is a no-op (the value is already correct).
	//
	// A reference is hashed by its own id, directly — not by matching it
	// against a sibling resource hashed in pass 1. ctx.Hash is a pure
	// function of (key, runID, value), so hashing "123" here always equals
	// hashing "123" as Patient.id, whether or not that Patient resource is
	// present in this same batch/request (it usually isn't: the CLI
	// de-identifies one SQL file — one resource type — at a time, and the
	// live server one resource type per request). This is also why the
	// caller must keep RunID stable across everything it considers "the
	// same delivery" — see RunContext.
	for _, w := range all {
		idPath := w.res.Type + ".id"
		linkID := linkIDFor(w, related)
		for _, f := range w.fields {
			if f.Path == idPath {
				continue
			}
			cov, ok := covByPath[f.Path]
			if !ok {
				continue
			}
			if f.Kind == KindReference {
				applyReferenceHash(f, cov.Rules, ctx)
				continue
			}
			applyField(f, cov.Rules, ctx, linkID)
		}
	}

	return allCoverage, nil
}

// applyReferenceHash hashes the id portion of a reference field covered by a
// hash rule on the resource type it points at, preserving the resource-type
// prefix (e.g. "Patient/123" -> "Patient/<hash>"). Every reference to a
// resource type whose id is hashed is rewritten the same way, automatically
// — cov.Rules (from rulesHashingResourceType) already only contains rules
// whose resource type matches this reference's own target type, so a
// polymorphic reference field pointing at some other, non-hashed type is
// simply uncovered rather than mismatched here.
func applyReferenceHash(f Field, rules []Rule, ctx RunContext) {
	v, ok := f.Get()
	if !ok {
		return
	}
	rt, id := refType(v), refID(v)
	if rt == "" || id == "" {
		return
	}
	for _, r := range rules {
		if r.Action == ActionHash && ruleResourceType(r.Path) == rt {
			f.Set(rt + "/" + ctx.Hash(id))
			return
		}
	}
}

// ruleResourceType extracts the resource type a rule's own id-path names,
// e.g. "Patient" from "Patient.id".
func ruleResourceType(path string) string {
	if i := strings.Index(path, "."); i >= 0 {
		return path[:i]
	}
	return path
}

// applyField applies rules (already in canonical execution order) to f in
// sequence.
func applyField(f Field, rules []Rule, ctx RunContext, linkID string) {
	for _, r := range rules {
		switch r.Action {
		case ActionNone:
			// no-op — the value is released unchanged, deliberately.
		case ActionHash:
			if val, ok := f.Get(); ok {
				f.Set(ctx.Hash(val))
			}
		case ActionShift:
			applyDateTransform(f, func(t time.Time) time.Time {
				return ShiftDate(t, ctx.ShiftOffsetDays(linkID, *r.MaxDays))
			}, true)
		case ActionFirstOfMonth:
			applyDateTransform(f, FirstOfMonth, false)
		case ActionClampAge:
			applyDateTransform(f, func(t time.Time) time.Time {
				return ClampAge(t, *r.MinAge, *r.MaxAge, time.Now())
			}, false)
		}
	}
}

// applyDateTransform parses f's current value, applies transform, and
// writes the result back. preserveLayout keeps shift's original granularity
// (a dateTime with a time component stays a dateTime); first-of-month and
// clamp-age always write back a plain date, since both intentionally
// collapse to date-level precision.
func applyDateTransform(f Field, transform func(time.Time) time.Time, preserveLayout bool) {
	val, ok := f.Get()
	if !ok {
		return
	}
	if f.Kind == KindDate {
		t, err := time.Parse(dateOnlyLayout, val)
		if err != nil {
			return
		}
		f.Set(transform(t).Format(dateOnlyLayout))
		return
	}
	t, layout, err := ParseFHIRDateish(val)
	if err != nil {
		return
	}
	out := transform(t)
	if preserveLayout {
		f.Set(out.Format(layout))
	} else {
		f.Set(out.Format(dateOnlyLayout))
	}
}

// resolvePatientAnchor finds the original id of the patient a resource is
// about: its own id, if it is a Patient; otherwise the id half of its
// subject/patient reference. Must run before any hashing so it captures the
// pre-hash id, matching what a dependent resource's own anchor lookup (via
// its still-original subject reference, since this all runs before pass 1)
// will also resolve to.
func resolvePatientAnchor(resourceType string, fields []Field) string {
	if resourceType == "Patient" {
		for _, f := range fields {
			if f.Path == "Patient.id" {
				if v, ok := f.Get(); ok {
					return v
				}
			}
		}
		return ""
	}
	for _, f := range fields {
		if f.Kind != KindReference {
			continue
		}
		tail := f.Path
		if i := strings.LastIndex(f.Path, "."); i >= 0 {
			tail = f.Path[i+1:]
		}
		if tail != "subject" && tail != "patient" {
			continue
		}
		v, ok := f.Get()
		if !ok || refType(v) != "Patient" {
			continue
		}
		return refID(v)
	}
	return ""
}

// linkIDFor returns the linkID a resource's date fields should shift by: the
// LinkID-resolved anchor of the patient it's about, or — for the rare
// resource with no discoverable patient anchor — its own type as a
// deterministic-but-unlinked fallback (documented limitation: such a
// resource's dates won't necessarily shift in step with its patient's).
func linkIDFor(w walked, related []fhir.RelatedPerson) string {
	if w.patientAnchor == "" {
		return w.res.Type
	}
	return LinkID(w.patientAnchor, related)
}

// restrictToDeclared keeps only fields whose top-level element (the segment
// right after resourceType, e.g. "code" in "Observation.code.coding.system")
// is named in declared — matching FHIR _elements semantics, where selecting
// a composite element includes its whole subtree. "id" is always kept
// regardless of declared, since Bulk Data _elements never excludes it.
func restrictToDeclared(resourceType string, fields []Field, declared []string) []Field {
	allowed := make(map[string]bool, len(declared)+1)
	allowed["id"] = true
	for _, d := range declared {
		allowed[d] = true
	}
	prefix := resourceType + "."
	out := fields[:0]
	for _, f := range fields {
		rest := strings.TrimPrefix(f.Path, prefix)
		top := rest
		if i := strings.Index(rest, "."); i >= 0 {
			top = rest[:i]
		}
		if allowed[top] {
			out = append(out, f)
		}
	}
	return out
}
