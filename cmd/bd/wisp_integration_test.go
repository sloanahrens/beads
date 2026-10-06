//go:build cgo && integration

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestWispCreateMaterializesChildDAG is the regression test for GH#3872.
// Before the fix, wisps were silently forced to root-only unless the
// formula set pour=true, making --root-only a no-op flag and breaking
// ephemeral lifecycle testing of multi-step formulas. After the fix,
// `bd mol wisp <proto>` materializes the full child DAG by default,
// just marked Ephemeral=true so it doesn't sync via git.
func TestWispCreateMaterializesChildDAG(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")

	rootID := wispTestProto(t, ctx, s, 2)

	withWispTestGlobals(t, s, ctx)

	_ = captureStdout(t, func() error {
		runWispCreate(makeWispTestCmd(false, false), []string{rootID})
		return nil
	})

	// Proto root + 2 children = 3 source issues, all materialized as wisp
	// copies with Ephemeral=true. The original proto issues are persistent
	// (Ephemeral=false), so counting ephemeral issues gives us exactly the
	// wisp set.
	if got := countEphemeral(t, ctx, s); got != 3 {
		t.Errorf("expected 3 ephemeral wisp issues (root + 2 children), got %d", got)
	}
}

// TestWispCreateRootOnly verifies that --root-only opts out of child
// materialization while still creating the root as ephemeral. Before the
// GH#3872 fix this was the silent default for all vapor formulas; after
// the fix it must be an explicit opt-in via the flag.
func TestWispCreateRootOnly(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")

	rootID := wispTestProto(t, ctx, s, 2)

	withWispTestGlobals(t, s, ctx)

	_ = captureStdout(t, func() error {
		runWispCreate(makeWispTestCmd(true, false), []string{rootID})
		return nil
	})

	if got := countEphemeral(t, ctx, s); got != 1 {
		t.Errorf("expected 1 ephemeral wisp issue (root only), got %d", got)
	}
}

// TestWispCreateDryRunFanoutMessage verifies the dry-run printout reflects
// full DAG materialization by default and switches to "root only" wording
// only under --root-only. Catches regressions in user-facing messaging.
func TestWispCreateDryRunFanoutMessage(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")

	rootID := wispTestProto(t, ctx, s, 2)

	withWispTestGlobals(t, s, ctx)

	t.Run("default fans out", func(t *testing.T) {
		output := captureStdout(t, func() error {
			runWispCreate(makeWispTestCmd(false, true), []string{rootID})
			return nil
		})
		if !strings.Contains(output, "would create wisp with 3 issues") {
			t.Errorf("dry-run should mention 3 issues (root + 2 children), got:\n%s", output)
		}
		if strings.Contains(output, "root only") {
			t.Errorf("dry-run without --root-only should NOT say 'root only', got:\n%s", output)
		}
	})

	t.Run("root-only shows opt-out wording", func(t *testing.T) {
		output := captureStdout(t, func() error {
			runWispCreate(makeWispTestCmd(true, true), []string{rootID})
			return nil
		})
		if !strings.Contains(output, "1 issue (root only)") {
			t.Errorf("dry-run with --root-only should mention 1 root issue, got:\n%s", output)
		}
		if !strings.Contains(output, "--root-only") {
			t.Errorf("skip message should reference --root-only flag, got:\n%s", output)
		}
	})
}
