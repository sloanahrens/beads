package testutil

import "testing"

func TestDoltTestsExplicitlySkipped(t *testing.T) {
	tests := []struct {
		name string
		skip string
		want bool
	}{
		{"unset", "", false},
		{"unrelated service", "slow", false},
		{"blanket dolt token", "dolt", true},
		{"narrow dolt-container token", "dolt-container", true},
		{"blanket token among others", "slow,dolt,race", true},
		{"narrow token among others", "slow,dolt-container", true},
		{"prefix match rejected", "doltbinary", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BEADS_TEST_SKIP", tt.skip)
			if got := DoltTestsExplicitlySkipped(); got != tt.want {
				t.Errorf("DoltTestsExplicitlySkipped() with BEADS_TEST_SKIP=%q = %v, want %v", tt.skip, got, tt.want)
			}
		})
	}
}
