package testtier

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRefuseInUnitTier(t *testing.T) {
	tests := []struct {
		tier    string
		wantErr bool
	}{
		{tier: "", wantErr: false},
		{tier: "integration", wantErr: false},
		{tier: "unit", wantErr: true},
		{tier: " unit ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run("tier="+tt.tier, func(t *testing.T) {
			t.Setenv(EnvVar, tt.tier)
			err := Refuse("schema migration")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Refuse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				return
			}
			if !errors.Is(err, ErrUnitTier) {
				t.Fatalf("Refuse() error = %v, want errors.Is ErrUnitTier", err)
			}
			for _, want := range []string{"schema migration", EnvVar, "make test-integration"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("Refuse() error %q does not name %q", err, want)
				}
			}
		})
	}
}

func TestUnitReportsTier(t *testing.T) {
	t.Setenv(EnvVar, "unit")
	if !Unit() {
		t.Fatal("Unit() = false with BD_TEST_TIER=unit")
	}
	t.Setenv(EnvVar, "integration")
	if Unit() {
		t.Fatal("Unit() = true with BD_TEST_TIER=integration")
	}
}

func TestIntegrationReportsTier(t *testing.T) {
	t.Setenv(EnvVar, "integration")
	if !Integration() {
		t.Fatal("Integration() = false with BD_TEST_TIER=integration")
	}
	t.Setenv(EnvVar, "unit")
	if Integration() {
		t.Fatal("Integration() = true with BD_TEST_TIER=unit")
	}
}

// TestBuildTierAppliesWhenEnvIsAbsent pins the stripped-environment case:
// cmd/bd helpers that drop every BEADS_* and BD_* variable before spawning
// bd still get the tripwire, because scripts/test.sh links the tier into the
// prebuilt bd. A present variable, even empty, wins over the linked value.
func TestBuildTierAppliesWhenEnvIsAbsent(t *testing.T) {
	orig := buildTier
	t.Cleanup(func() { buildTier = orig })
	buildTier = "unit"

	t.Setenv(EnvVar, "")
	if Unit() {
		t.Fatal("an explicitly empty BD_TEST_TIER must override the linked tier")
	}
	if err := os.Unsetenv(EnvVar); err != nil {
		t.Fatal(err)
	}
	if !Unit() {
		t.Fatal("Unit() = false with BD_TEST_TIER absent and buildTier=unit")
	}
	if err := Refuse("schema migration"); !errors.Is(err, ErrUnitTier) {
		t.Fatalf("Refuse() = %v, want ErrUnitTier from the linked tier", err)
	}
	buildTier = ""
	if Unit() || Integration() {
		t.Fatal("no env and no linked tier must be no tier")
	}
}
