package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestIfAcceptanceMatches(t *testing.T) {
	const stored = "- [ ] builds\n- [ ] tests pass"
	sum := sha256.Sum256([]byte(stored))
	digest := hex.EncodeToString(sum[:])
	otherSum := sha256.Sum256([]byte("- [x] builds\n- [ ] tests pass"))
	otherDigest := hex.EncodeToString(otherSum[:])
	// A stored value that is itself 64 hex digits must still match by its text.
	hexText := strings.Repeat("ab", 32)
	emptySum := sha256.Sum256(nil)
	emptyDigest := hex.EncodeToString(emptySum[:])

	tests := []struct {
		name     string
		stored   string
		expected string
		want     bool
	}{
		{name: "full text matches", stored: stored, expected: stored, want: true},
		{name: "different text refuses", stored: stored, expected: "- [x] builds", want: false},
		{name: "sha256 of stored matches", stored: stored, expected: digest, want: true},
		{name: "uppercase sha256 of stored matches", stored: stored, expected: strings.ToUpper(digest), want: true},
		{name: "sha256 of other text refuses", stored: stored, expected: otherDigest, want: false},
		{name: "hash-shaped text matches its own text", stored: hexText, expected: hexText, want: true},
		{name: "hash-shaped text does not match a different text", stored: stored, expected: hexText, want: false},
		{name: "empty expected matches empty stored", stored: "", expected: "", want: true},
		{name: "empty expected refuses non-empty stored", stored: stored, expected: "", want: false},
		{name: "non-empty expected refuses empty stored", stored: "", expected: stored, want: false},
		{name: "sha256 of empty matches empty stored", stored: "", expected: emptyDigest, want: true},
		{name: "truncated digest refuses", stored: stored, expected: digest[:63], want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AcceptanceMatches(tt.stored, tt.expected); got != tt.want {
				t.Errorf("AcceptanceMatches(%q, %q) = %v, want %v", tt.stored, tt.expected, got, tt.want)
			}
		})
	}
}
