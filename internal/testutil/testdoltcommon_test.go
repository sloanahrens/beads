package testutil

import "testing"

// TestDoltTestsExplicitlySkipped covers the explicit-opt-out detection that
// TestMain functions use to distinguish "the operator asked to skip Dolt
// tests" from "Dolt happens to be unavailable in this environment" (be-r18).
func TestDoltTestsExplicitlySkipped(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"unset", "", false},
		{"dolt only", "dolt", true},
		{"dolt among others", "slow,dolt,network", true},
		{"dolt with surrounding spaces", "slow, dolt", true},
		{"unrelated services only", "slow,network", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BEADS_TEST_SKIP", tc.value)
			if got := DoltTestsExplicitlySkipped(); got != tc.want {
				t.Errorf("DoltTestsExplicitlySkipped() with BEADS_TEST_SKIP=%q = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
