package utils

import "testing"

func TestTownRootPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		gtTownRoot string
		gtRoot     string
		want       string
	}{
		{"only GT_TOWN_ROOT", "/town", "", "/town"},
		{"only GT_ROOT (deprecated alias)", "", "/legacy", "/legacy"},
		{"both set and differ: GT_TOWN_ROOT wins", "/town", "/legacy", "/town"},
		{"neither set", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GT_TOWN_ROOT", tt.gtTownRoot)
			t.Setenv("GT_ROOT", tt.gtRoot)
			if got := TownRoot(); got != tt.want {
				t.Fatalf("TownRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTownRootEnvNamesPrecedenceOrder(t *testing.T) {
	want := []string{"GT_TOWN_ROOT", "GT_ROOT"}
	got := TownRootEnvNames()
	if len(got) != len(want) {
		t.Fatalf("TownRootEnvNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("TownRootEnvNames() = %v, want %v", got, want)
		}
	}
}
