package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTestScriptReportsCoverageBoundary locks in be-1kk.
//
// `go test` prints `ok <pkg> <duration>` for a package whose every test
// skipped, which is indistinguishable from a package that ran and passed. So
// when scripts/test.sh runs a suite that is gated behind an opt-in env var it
// has not set, its green is not evidence about that suite — and that is how
// main sat red (be-bz4) behind a gate reporting
// `ok internal/storage/embeddeddolt 265.768s`.
//
// The gate cannot fix that by setting BEADS_TEST_EMBEDDED_DOLT (the var is
// process-wide, so it also switches on cmd/bd's 184 TestEmbedded* tests,
// which CI can only fit by sharding 20 ways — see scripts/ci/lib/test-env.sh
// for the measurement). What it can do is refuse to be silent about the
// boundary, which is what this test pins: the gate names the suites it is
// skipping, and stops naming them once they are actually enabled.
//
// The suite list is discovered through test-env.sh, so this test fails if
// someone drops the report from scripts/test.sh, or makes the report
// unconditional (the second case asserts the opposite output, so a report
// that always prints fails it too).
func TestTestScriptReportsCoverageBoundary(t *testing.T) {
	repoRoot := sourceRepoRoot(t)
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("bash is required to exercise scripts/test.sh: %v", err)
	}

	// A `go` that accepts anything and succeeds. Only the gate's own
	// reporting is under test here; the suites it would run are irrelevant,
	// and stubbing go keeps the assertion independent of build state.
	fakeBin := filepath.Join(t.TempDir(), "fake go bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatalf("create fake bin dir: %v", err)
	}
	fakeGo := filepath.Join(fakeBin, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}

	runGate := func(t *testing.T, extraEnv ...string) string {
		t.Helper()
		cmd := exec.Command(bash, "--noprofile", "--norc", filepath.Join(repoRoot, "scripts", "test.sh"),
			"-run", "^$", "./internal/types/")
		cmd.Dir = repoRoot
		cmd.Env = append(gateCoverageEnv(fakeBin), extraEnv...)
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			t.Fatalf("scripts/test.sh failed: %v\n%s", runErr, out)
		}
		return string(out)
	}

	t.Run("unset gate var is reported as a coverage gap", func(t *testing.T) {
		out := runGate(t, "BEADS_TEST_EMBEDDED_DOLT=")
		if !strings.Contains(out, "NOT COVERED by this run") {
			t.Errorf("gate did not report its coverage boundary:\n%s", out)
		}
		for _, want := range []string{
			"internal/storage/embeddeddolt/",
			"BEADS_TEST_EMBEDDED_DOLT unset",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("coverage-gap report is missing %q:\n%s", want, out)
			}
		}
		if !strings.Contains(out, "this green is not evidence about them") {
			t.Errorf("passing run did not restate the gap in its summary:\n%s", out)
		}
	})

	t.Run("enabled gate var is not reported", func(t *testing.T) {
		out := runGate(t, "BEADS_TEST_EMBEDDED_DOLT=1")
		if strings.Contains(out, "NOT COVERED by this run") {
			t.Errorf("gate reported a coverage gap it had enabled:\n%s", out)
		}
		if strings.Contains(out, "this green is not evidence about them") {
			t.Errorf("gate restated a coverage gap it had enabled:\n%s", out)
		}
	})
}

// gateCoverageEnv returns a deterministic gate environment: the caller's
// environment minus every variable that would either make the report
// inherited rather than computed (BEADS_TEST_ENV_ACTIVE short-circuits
// beads_test_env_enter) or pre-enable the suite under test.
func gateCoverageEnv(fakeBin string) []string {
	inherited := map[string]bool{
		"BEADS_TEST_ENV_ACTIVE":    true,
		"BEADS_TEST_ENV_ROOT":      true,
		"BEADS_TEST_EMBEDDED_DOLT": true,
		"BEADS_TEST_ENV_RUN_DOLT":  true,
		"BEADS_TEST_ENV_DISABLE":   true,
		"BEADS_TEST_ENV_KEEP":      true,
	}
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if key, _, ok := strings.Cut(kv, "="); ok && inherited[key] {
			continue
		}
		env = append(env, kv)
	}
	// Fake go first, real toolchain still reachable for everything else.
	return append(env, "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
