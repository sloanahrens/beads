package doltserver

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/testtier"
)

// TestStartRefusedInUnitTier pins the be-b23 tripwire: the unit test tier
// never starts a dolt sql-server (in-process or from a spawned bd).
func TestStartRefusedInUnitTier(t *testing.T) {
	t.Setenv(testtier.EnvVar, "unit")
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	state, err := Start(beadsDir)
	if !errors.Is(err, testtier.ErrUnitTier) {
		t.Fatalf("Start() = %+v, %v; want testtier.ErrUnitTier", state, err)
	}
}
