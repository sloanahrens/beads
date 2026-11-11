package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// AcceptanceMatches reports whether expected names the stored acceptance
// criteria: either the same text, or the sha256 of stored written as hex. That
// is the compare-and-set rule behind `bd update --if-acceptance`, where the hash
// form keeps argv short. An empty expected matches only an empty stored value.
//
// The text is tried first, so a stored value that is itself 64 hex digits
// still matches by its own text, not only by its hash.
func AcceptanceMatches(stored, expected string) bool {
	if expected == stored {
		return true
	}
	if len(expected) != sha256.Size*2 {
		return false
	}
	sum := sha256.Sum256([]byte(stored))
	return strings.EqualFold(expected, hex.EncodeToString(sum[:]))
}
