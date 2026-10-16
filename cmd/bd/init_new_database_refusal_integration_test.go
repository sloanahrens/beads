//go:build integration

// Integration tier (be-b23): this drives the real bd binary against a real
// dolt sql-server it starts itself, which the unit tier's BD_TEST_TIER tripwire
// refuses (the gate failed with "operation not allowed in the unit test tier").
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/doltserver"
)

// newDatabaseRefusalEnv is a hermetic bd environment that leaves the
// new-database opt-in unset. Unlike bdEnv it does not pin
// BEADS_DOLT_AUTO_START=0 unless autoStart is false, because the first init
// below must start a server of its own.
func newDatabaseRefusalEnv(home string, autoStart bool, extra ...string) []string {
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "BEADS_") || strings.HasPrefix(e, "BD_") {
			continue
		}
		env = append(env, e)
	}
	env = append(env,
		"HOME="+home,
		"BEADS_NO_DAEMON=1",
		"BD_NON_INTERACTIVE=1",
		"BD_DISABLE_METRICS=1",
		"BD_DISABLE_EVENT_FLUSH=1",
	)
	if !autoStart {
		env = append(env, "BEADS_DOLT_AUTO_START=0")
	}
	return append(env, extra...)
}

// TestInitNewDatabaseRefusalEndToEnd drives the real bd binary through the
// be-d4v contract: the first init on a server with no other databases needs no
// opt-in, and a later init against that now-populated server is refused with
// exit 1 and a message naming both opt-ins, until --allow-new-database or
// BD_ALLOW_NEW_DATABASE=1 is given.
func TestInitNewDatabaseRefusalEndToEnd(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("spawns a local dolt sql-server; covered on unix")
	}
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt binary not available")
	}
	bd := buildBDForInitTests(t)
	home := t.TempDir()

	initArgs := func(prefix string, extra ...string) []string {
		return append([]string{"init", "--prefix", prefix, "--quiet", "--non-interactive",
			"--skip-hooks", "--skip-agents"}, extra...)
	}

	// First init: a fresh server holds only dolt's own bootstrap database, so
	// it must succeed with no opt-in.
	wsA := t.TempDir()
	beadsA := filepath.Join(wsA, ".beads")
	t.Cleanup(func() { _ = doltserver.Stop(beadsA) })
	out, code := bdRunWithEnv(t, bd, wsA, newDatabaseRefusalEnv(home, true), initArgs("nda")...)
	if code != 0 {
		t.Fatalf("first init on a fresh server: exit %d, want 0\n%s", code, out)
	}
	state, err := doltserver.IsRunning(beadsA)
	if err != nil || !state.Running || state.Port == 0 {
		t.Fatalf("expected the first init to leave a server running: state=%+v err=%v", state, err)
	}
	serverArgs := []string{"--server", "--server-host", "127.0.0.1", "--server-port", strconv.Itoa(state.Port)}
	noAutoStart := newDatabaseRefusalEnv(home, false)

	// Second init against that server, no opt-in: refused.
	wsB := t.TempDir()
	out, code = bdRunWithEnv(t, bd, wsB, noAutoStart, initArgs("ndb", serverArgs...)...)
	if code != 1 {
		t.Fatalf("init on a populated server without the opt-in: exit %d, want 1\n%s", code, out)
	}
	for _, want := range []string{"REFUSED", "--allow-new-database", "BD_ALLOW_NEW_DATABASE=1", "nda"} {
		if !strings.Contains(out, want) {
			t.Errorf("refusal output missing %q:\n%s", want, out)
		}
	}

	// The refusal must not strand an empty .beads/dolt: it would make the
	// retry below look like a legacy workspace.
	if _, err := os.Stat(filepath.Join(wsB, ".beads", "dolt")); err == nil {
		t.Errorf("refused init left %s behind", filepath.Join(wsB, ".beads", "dolt"))
	}

	// The flag opts in.
	out, code = bdRunWithEnv(t, bd, wsB, noAutoStart, initArgs("ndb", append(serverArgs, "--allow-new-database")...)...)
	if code != 0 {
		t.Fatalf("init with --allow-new-database: exit %d, want 0\n%s", code, out)
	}

	// So does the environment variable.
	wsC := t.TempDir()
	out, code = bdRunWithEnv(t, bd, wsC, newDatabaseRefusalEnv(home, false, "BD_ALLOW_NEW_DATABASE=1"), initArgs("ndc", serverArgs...)...)
	if code != 0 {
		t.Fatalf("init with BD_ALLOW_NEW_DATABASE=1: exit %d, want 0\n%s", code, out)
	}
}

// bdRunWithEnv runs bd in dir with exactly env and returns combined output and
// the exit code.
func bdRunWithEnv(t *testing.T, bd, dir string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bd, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), exitErr.ExitCode()
	}
	t.Fatalf("bd %s: %v\n%s", strings.Join(args, " "), err, out)
	return string(out), -1
}
