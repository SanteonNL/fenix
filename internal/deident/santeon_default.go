package deident

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed santeon_default.json
var santeonDefaultJSON []byte

// DefaultRuleset returns the santeon-default standard ruleset shipped with
// the IG (docs/fenix_architecture.md, "Two-layer configuration"): hash on
// Patient.id/Patient.identifier, a 15-day shift on every date, and
// first-of-month + clamp-age(18,85) on Patient.birthDate.
func DefaultRuleset() (Ruleset, error) {
	var rs Ruleset
	if err := json.Unmarshal(santeonDefaultJSON, &rs); err != nil {
		return Ruleset{}, fmt.Errorf("deident: parse embedded santeon-default ruleset: %w", err)
	}
	if err := rs.Validate(); err != nil {
		return Ruleset{}, fmt.Errorf("deident: embedded santeon-default ruleset is invalid: %w", err)
	}
	return rs, nil
}
