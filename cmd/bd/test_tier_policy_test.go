package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/testtier"
)

// TestPrebuiltBDCarriesUnitTier is the unit-tier policy for subprocess tests
// (be-b23). A test that spawns bd with an environment stripped of every
// BEADS_* and BD_* variable (several helpers here do that) must still hit the
// tripwire, so a new unit-tier test cannot migrate a fresh store through a
// subprocess either. scripts/test.sh links the tier into the prebuilt bd;
// this spawns that bd with nothing but PATH and HOME and expects bd init to
// be refused before it migrates.
func TestPrebuiltBDCarriesUnitTier(t *testing.T) {
	if !testtier.Unit() {
		t.Skip("unit-tier policy: runs under make test (BD_TEST_TIER=unit)")
	}
	bd, err := findPrebuiltBDBinary()
	if err != nil {
		t.Fatal(err)
	}
	if bd == "" {
		t.Fatal("unit tier without BEADS_TEST_BD_BINARY: scripts/test.sh prebuilds bd with the tier linked in; run through it")
	}

	dir := t.TempDir()
	initGitRepoAt(t, dir)
	cmd := exec.Command(bd, "init", "--quiet", "--prefix", "tier", "--non-interactive", "--skip-hooks", "--skip-agents")
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("bd init with a stripped environment succeeded in the unit tier; the prebuilt bd lacks the linked tier:\n%s", out)
	}
	if !strings.Contains(string(out), testtier.ErrUnitTier.Error()) {
		t.Fatalf("bd init failed for a reason other than the unit-tier tripwire: %v\n%s", err, out)
	}
}
