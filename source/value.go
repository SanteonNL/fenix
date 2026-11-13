// Converts source values to the text that is stored in staging.
//
// Staging stores every value as text, exactly as it appears in the source. Why text:
// staging must accept any source without knowing its schema in advance, and a text column
// never rejects a value. JSON also has no separate integer type, only "number". Guessing
// column types while loading would make a load fail on the first wrong guess; typing is
// done in the step after staging, where the queries know what a column means.
//
// All sources and the file output use StagingText, so a primary key and the link value
// in a child table always have the same text and a plain join works.
package source

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// StagingText returns value as the text stored in staging, and false when the value is NULL.
//
//   - JSON numbers (json.Number) keep their original digits: 111 stays "111", a 19-digit id keeps every digit.
//   - Other numbers are written without exponent and without a trailing ".0": 1234567 becomes "1234567".
//   - Times are written as RFC 3339 (ISO 8601): "2025-03-01T00:00:00Z".
//   - Bytes are read as text; drivers return text-like types such as DECIMAL this way.
//     Binary columns must be converted by the source before calling this function.
//   - Lists and objects are written as JSON.
func StagingText(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", false
	case string:
		return typed, true
	case json.Number:
		return typed.String(), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32), true
	case time.Time:
		return typed.Format(time.RFC3339Nano), true
	case []byte:
		return string(typed), true
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed), true
		}
		return string(encoded), true
	}
}

// StagingValue returns value as a database parameter for a staging column: its text,
// or nil for NULL.
func StagingValue(value any) any {
	text, isPresent := StagingText(value)
	if !isPresent {
		return nil
	}
	return text
}
