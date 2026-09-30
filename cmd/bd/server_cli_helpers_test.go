package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
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
