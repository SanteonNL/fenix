package local

import (
	"fmt"
	"strings"
)

// RowLoadError aggregates row-level write failures for a single table load.
// Duplicate-key failures are counted/sampled rather than listed individually,
// since the common case is the same source key repeated many times.
type RowLoadError struct {
	Table              string
	Failed             []error // non-duplicate row failures, each already wrapped with its row key
	DuplicateKeys      int
	DuplicateKeySample []string // up to 20 ids
}

func (e *RowLoadError) Error() string {
	var parts []string
	if len(e.Failed) > 0 {
		parts = append(parts, fmt.Sprintf("%d row(s) failed (first: %s)", len(e.Failed), e.Failed[0]))
	}
	if e.DuplicateKeys > 0 {
		parts = append(parts, fmt.Sprintf("%d duplicate key(s) skipped (ids: %s)",
			e.DuplicateKeys, strings.Join(e.DuplicateKeySample, ", ")))
	}
	return fmt.Sprintf("table %s: %s", e.Table, strings.Join(parts, "; "))
}

// empty reports whether the error carries no failures at all.
func (e *RowLoadError) empty() bool {
	return e == nil || (len(e.Failed) == 0 && e.DuplicateKeys == 0)
}

// addFailure records a non-duplicate row failure, tagged with its key column
// value when one is known (rows with no id_field, e.g. flat CSV/JSON inserts,
// pass a nil keyVal and are recorded untagged).
func (e *RowLoadError) addFailure(keyField string, keyVal interface{}, err error) {
	if keyVal == nil {
		e.Failed = append(e.Failed, err)
		return
	}
	e.Failed = append(e.Failed, fmt.Errorf("%s=%v: %w", logKeyField(keyField), keyVal, err))
}

// addDuplicate records a duplicate-key row, sampling up to 20 ids.
func (e *RowLoadError) addDuplicate(id string) {
	e.DuplicateKeys++
	if len(e.DuplicateKeySample) < 20 {
		e.DuplicateKeySample = append(e.DuplicateKeySample, id)
	}
}
