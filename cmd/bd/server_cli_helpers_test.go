package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/doltserver"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// Shared helpers for server-mode tests, kept from the deleted embedded suite.

func bdEnv(dir string) []string {
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "BEADS_") {
			continue
		}
		env = append(env, e)
	}
	return append(env,
		"HOME="+dir,
		"BEADS_DOLT_AUTO_START=0",
		"BEADS_NO_DAEMON=1",
		"BD_DISABLE_METRICS=1",
		"BD_DISABLE_EVENT_FLUSH=1",
	)
}

// bdRunRaw runs bd with the given args and env extras (appended to bdEnv(dir)).
// Returns combined stdout+stderr and the process exit code. Unlike the other
// helpers in this package it does not call t.Fatal on non-zero exits — that's
// the success case for the max-rows tests.
func bdRunRaw(t *testing.T, bd, dir string, envExtras []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bd, args...)
	cmd.Dir = dir
	env := bdEnv(dir)
	env = append(env, envExtras...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return string(out), exitErr.ExitCode()
	}
	t.Fatalf("bd %s unexpected non-exit error: %v\n%s", strings.Join(args, " "), err, out)
	return string(out), -1
}

// runBatchScriptInTx is a tiny helper that mirrors what batchCmd.RunE does,
// minus the cobra/flag plumbing, so tests can drive batch execution against
// a *dolt.DoltStore without spawning a 'bd' subprocess.
func runBatchScriptInTx(t *testing.T, ctx context.Context, st storage.DoltStorage, script string) error {
	t.Helper()
	ops, err := parseBatchScript(strings.NewReader(script))
	if err != nil {
		return err
	}
	return st.RunInTransaction(ctx, "test: bd batch", func(tx storage.Transaction) error {
		for _, op := range ops {
			if _, err := runBatchOp(ctx, tx, op); err != nil {
				return err
			}
		}
		return nil
	})
}

// seedBatchTestIssues creates three open issues for batch tests to operate on.
func seedBatchTestIssues(t *testing.T, ctx context.Context, st storage.DoltStorage, ids ...string) {
	t.Helper()
	for _, id := range ids {
		issue := &types.Issue{
			ID:        id,
			Title:     "seed " + id,
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
		}
		if err := st.CreateIssue(ctx, issue, "test"); err != nil {
			t.Fatalf("seed CreateIssue %s: %v", id, err)
		}
	}
}

// writeContractBackendConfig writes a metadata.json naming backend into a fresh
// directory and returns it.
func writeContractBackendConfig(t *testing.T, backend string) string {
	t.Helper()
	beadsDir := t.TempDir()
	if err := (&configfile.Config{Backend: backend}).Save(beadsDir); err != nil {
		t.Fatalf("save metadata.json: %v", err)
	}
	return beadsDir
}

// captureBootstrapStderr redirects os.Stderr for the duration of fn and returns
// what was written. bd bootstrap surfaces the guard message through HandleError,
// which writes to os.Stderr and returns an opaque exit error, so the message is
// only observable here — not on the error returned by rootCmd.Execute().
func captureBootstrapStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	os.Stderr = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// serverInitArgs returns the `bd init` flags that point a subprocess at the
// shared test Dolt server with a database of its own, dropped on cleanup. It
// skips the test when no test server is running: with embedded Dolt gone, a
// bare `bd init` needs a server.
func serverInitArgs(t *testing.T) []string {
	t.Helper()
	if testDoltServerPort == 0 {
		t.Skip("Dolt test server not available")
	}
	database := uniqueTestDBName(t)
	t.Cleanup(func() { dropTestDatabase(database, testDoltServerPort) })
	return []string{"--server-port", strconv.Itoa(testDoltServerPort), "--database", database}
}

// sharedServerEnvExtras returns the BEADS_*/BD_* entries a subprocess bd needs
// to reach the shared test Dolt server instead of starting one of its own.
// Callers strip the ambient BEADS_*/BD_* variables to stay hermetic, which also
// removes TestMain's own BEADS_TEST_SERVER opt-in, so the entries are re-added
// here.
//
// BEADS_DOLT_AUTO_START=0 is the one that matters: without it bd opens a
// workspace whose metadata names no server by starting one — detached, so it
// outlives the test process (be-gnt).
func sharedServerEnvExtras() []string {
	return []string{
		"BEADS_DOLT_AUTO_START=0",
		"BEADS_TEST_SERVER=1",
		"BEADS_NO_DAEMON=1",
		"BD_DISABLE_METRICS=1",
		"BD_DISABLE_EVENT_FLUSH=1",
	}
}

// autoStartedServerChecks dedupes the leak check per workspace: the env helpers
// are called once per subprocess bd invocation, and a test that runs a dozen of
// them must not register a dozen identical cleanups.
var autoStartedServerChecks sync.Map

// requireNoAutoStartedServer registers a cleanup that fails the test if a Dolt
// server bd started for beadsDir is still running when the test ends.
//
// These tests run the real bd binary against their own workspace. Stripping
// every BEADS_*/BD_* variable from the child environment also removes the
// auto-start opt-out, so bd opens the store by starting a server of its own.
// Nothing stops it — the start is detached on purpose — and t.TempDir() then
// deletes the workspace out from under the live process, leaving it reparented
// to launchd with ~155 MB and a port until the host is rebooted. Eight such
// servers were found on 2026-10-07 (be-gnt), and the whole point of the
// shared-server wiring in the two helpers is that it cannot happen again.
//
// The check reads the pid file of THIS workspace, and stops only the server
// that file names: never a process pattern, since other sessions run these same
// tests concurrently and may own identical-looking servers. doltserver.IsRunning
// is the right reader for that file — it also confirms the pid still looks like
// a dolt sql-server, so a pid the kernel recycled is treated as the stale state
// it is rather than killed. It fails the test as well as stopping the server,
// because a live one means the env wiring regressed — much cheaper to catch in
// the run that caused it than on the host a day later.
func requireNoAutoStartedServer(t *testing.T, beadsDir string) {
	t.Helper()
	if _, seen := autoStartedServerChecks.LoadOrStore(beadsDir, struct{}{}); seen {
		return
	}
	t.Cleanup(func() {
		state, err := doltserver.IsRunning(beadsDir)
		if err != nil {
			t.Errorf("checking %s for a leaked dolt server: %v", beadsDir, err)
			return
		}
		if !state.Running {
			return
		}
		// Report before Stop: it clears the state files this message reads.
		t.Errorf("bd auto-started a dolt sql-server (pid %d, port %d) in %s and left it running; subprocess bd envs must pin BEADS_DOLT_AUTO_START=0 and init against the shared test server (be-gnt)",
			state.PID, state.Port, beadsDir)
		if err := doltserver.Stop(beadsDir); err != nil {
			t.Errorf("stopping leaked dolt sql-server pid %d in %s: %v", state.PID, beadsDir, err)
		}
	})
}
