package dolt

import "testing"

// TestShouldFailOnDoltUnavailable pins the be-r18 decision: an unavailable
// Dolt container fails TestMain loudly unless the operator explicitly opted
// out via BEADS_TEST_SKIP. A test suite that cannot run its subject must not
// report a false "ok" to a merge gate reading only the exit code.
func TestShouldFailOnDoltUnavailable(t *testing.T) {
	tests := []struct {
		name              string
		explicitlySkipped bool
		want              bool
	}{
		{"no opt-out present -> fail loudly", false, true},
		{"explicit opt-out present -> stay silent", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldFailOnDoltUnavailable(tt.explicitlySkipped); got != tt.want {
				t.Errorf("shouldFailOnDoltUnavailable(%v) = %v, want %v", tt.explicitlySkipped, got, tt.want)
			}
		})
	}
}
