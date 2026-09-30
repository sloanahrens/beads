package testutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/testtier"
)

// recordingTB captures Skipf/Fatalf instead of acting on them.
type recordingTB struct {
	testing.TB
	skipped, failed string
}

func (r *recordingTB) Helper() {}
func (r *recordingTB) Skipf(format string, args ...any) {
	r.skipped = fmt.Sprintf(format, args...)
}
func (r *recordingTB) Fatalf(format string, args ...any) {
	r.failed = fmt.Sprintf(format, args...)
}

// TestSkipOrFailUnavailable pins be-b23.3: an infra-unavailable skip is a
// skip outside the integration tier and a failure inside it (require-mode).
func TestSkipOrFailUnavailable(t *testing.T) {
	for _, tt := range []struct {
		tier     string
		wantFail bool
	}{
		{tier: "", wantFail: false},
		{tier: "unit", wantFail: false},
		{tier: "integration", wantFail: true},
	} {
		t.Run("tier="+tt.tier, func(t *testing.T) {
			t.Setenv(testtier.EnvVar, tt.tier)
			rec := &recordingTB{TB: t}
			SkipOrFailUnavailable(rec, "Dolt test server crashed: %v", "exit 137")
			got := rec.skipped
			if tt.wantFail {
				got = rec.failed
				if rec.skipped != "" {
					t.Fatalf("require-mode skipped: %q", rec.skipped)
				}
				if !strings.Contains(got, "require-mode") {
					t.Fatalf("failure %q does not say require-mode", got)
				}
			} else if rec.failed != "" {
				t.Fatalf("outside the integration tier it failed: %q", rec.failed)
			}
			if !strings.Contains(got, "Dolt test server crashed: exit 137") {
				t.Fatalf("message %q lost the reason", got)
			}
		})
	}
}

// TestEnsureDoltContainerForTestMainExitsInRequireMode: in the integration
// tier a TestMain that cannot get its Dolt container exits 1 instead of
// letting every test skip and the package report ok.
func TestEnsureDoltContainerForTestMainExitsInRequireMode(t *testing.T) {
	t.Setenv(testtier.EnvVar, "integration")
	var exitCode = -1
	orig := requireModeExit
	requireModeExit = func(code int) { exitCode = code }
	t.Cleanup(func() { requireModeExit = orig })

	err := requireModeTestMainGate(fmt.Errorf("Docker not available"))
	if err == nil {
		t.Fatal("gate returned nil for a failed container start")
	}
	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1 in require-mode", exitCode)
	}

	t.Setenv(testtier.EnvVar, "unit")
	exitCode = -1
	if err := requireModeTestMainGate(fmt.Errorf("Docker not available")); err == nil {
		t.Fatal("outside require-mode the error must still be returned")
	}
	if exitCode != -1 {
		t.Fatalf("outside require-mode exit was called with %d", exitCode)
	}
	if err := requireModeTestMainGate(nil); err != nil {
		t.Fatalf("nil error became %v", err)
	}
}
