package deident

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/SanteonNL/fenix/internal/models/fhir"
)

// RunContext carries the secret key and the non-secret per-run identifier
// that together derive a per-run seed (docs/fenix_architecture.md,
// "Pseudonymisation and the per-export seed"). RunID rotates the seed so
// repeated calls aren't linkable to each other — it is not a job or export
// identifier and nothing needs to track it afterward.
type RunContext struct {
	Key   []byte // FENIX_DEIDENT_KEY, loaded once at startup — never logged or serialized
	RunID string // non-secret, generated per request/batch
}

// seed derives this run's HMAC seed from the secret Key and the non-secret
// RunID. hash and ShiftOffsetDays both key off seed, never off Key
// directly — if hash(value) = HMAC(Key, value), the same patient would hash
// identically across every run, which would violate the "unlinkable across
// runs" property the spec requires. Reversal recomputes seed from Key +
// RunID inside the hospital; RunID alone (without Key) reveals nothing.
func (c RunContext) seed() []byte {
	mac := hmac.New(sha256.New, c.Key)
	mac.Write([]byte(c.RunID))
	return mac.Sum(nil)
}

// Hash replaces value with its HMAC-SHA256 under this run's seed, hex-encoded.
func (c RunContext) Hash(value string) string {
	mac := hmac.New(sha256.New, c.seed())
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// ShiftOffsetDays implements the deterministic per-patient offset
// construction from the spec ("Illustrative construction of the shift
// offset"): digest = HMAC-SHA256(seed, linkID); N = first 4 bytes as a
// big-endian uint32; k = N mod (2*maxDays); k is then mapped onto
// [-maxDays,-1] ∪ [1,maxDays], skipping zero.
func (c RunContext) ShiftOffsetDays(linkID string, maxDays int) int {
	if maxDays <= 0 {
		return 0
	}
	mac := hmac.New(sha256.New, c.seed())
	mac.Write([]byte(linkID))
	digest := mac.Sum(nil)

	n := binary.BigEndian.Uint32(digest[:4])
	k := int(n % uint32(2*maxDays))
	if k < maxDays {
		return k - maxDays
	}
	return k - maxDays + 1
}

// LinkID resolves the identifier used to derive a patient's shift offset.
// Related patients who must move together (most commonly mother and child)
// share an offset by sharing a linkID: when patientID is the subject of a
// RelatedPerson entry in related, the shared anchor is that RelatedPerson's
// own business identifier (or, failing that, its resource id) — so every
// Patient linked through the same RelatedPerson resource resolves to the
// same linkID. A patient with no such link uses their own id, unchanged.
//
// This is a best-effort implementation of a part of the spec that isn't
// pinned to a concrete FENIX data model yet (RelatedPerson isn't produced by
// today's converter for any resource type) — see the plan's "explicitly out
// of scope" notes.
func LinkID(patientID string, related []fhir.RelatedPerson) string {
	for _, rp := range related {
		if rp.Patient.Reference == nil || refID(*rp.Patient.Reference) != patientID {
			continue
		}
		if len(rp.Identifier) > 0 && rp.Identifier[0].Value != nil {
			return *rp.Identifier[0].Value
		}
		if rp.Id != nil {
			return *rp.Id
		}
	}
	return patientID
}
