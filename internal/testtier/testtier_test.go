package testtier

import (
	"errors"
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
