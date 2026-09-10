package dolt

import (
	"errors"
	"testing"
)

// TestShouldFailOnDoltUnavailable covers the decision TestMain makes when
// the Dolt test server can't be started: fail the whole package (so a merge
// gate reads red instead of a false "ok"), except when the operator asked
// for the skip via BEADS_TEST_SKIP=dolt (be-r18).
func TestShouldFailOnDoltUnavailable(t *testing.T) {
	setupErr := errors.New("Docker not available")

	cases := []struct {
		name              string
		setupErr          error
		explicitlySkipped bool
		want              bool
	}{
		{"dolt started fine", nil, false, false},
		{"dolt started fine, skip flag irrelevant", nil, true, false},
		{"dolt unavailable, no opt-out: fail loud", setupErr, false, true},
		{"dolt unavailable, explicit opt-out: silent skip", setupErr, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFailOnDoltUnavailable(tc.setupErr, tc.explicitlySkipped); got != tc.want {
				t.Errorf("shouldFailOnDoltUnavailable(%v, %v) = %v, want %v", tc.setupErr, tc.explicitlySkipped, got, tc.want)
			}
		})
	}
}
