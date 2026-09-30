package testutil

import (
	"fmt"
	"os"
	"testing"

	"github.com/steveyegge/beads/internal/testtier"
)

// SkipOrFailUnavailable reports that test infrastructure (a Dolt server, a
// Docker container) is unavailable. Outside the integration tier it skips the
// test. In the integration tier (BD_TEST_TIER=integration, make
// test-integration) it fails the test: that tier exists to run these tests,
// so "could not run" must not read as "passed" (be-b23.3, B5-17). A missing
// dolt binary is not this case; RequireDoltBinary still skips for it.
func SkipOrFailUnavailable(t testing.TB, format string, args ...any) {
	t.Helper()
	if testtier.Integration() {
		t.Fatalf("integration tier require-mode: "+format, args...)
		return
	}
	t.Skipf(format, args...)
}

// requireModeExit is os.Exit, replaceable in tests.
var requireModeExit = os.Exit

// requireModeTestMainGate applies require-mode to a TestMain's container
// start: in the integration tier a failed start exits the test binary with
// status 1 instead of letting every test skip. Outside it the error is
// returned unchanged for the caller's existing warn-and-skip handling.
func requireModeTestMainGate(err error) error {
	if err != nil && testtier.Integration() {
		fmt.Fprintf(os.Stderr, "FATAL: integration tier require-mode: Dolt container unavailable: %v\n", err)
		requireModeExit(1)
	}
	return err
}
