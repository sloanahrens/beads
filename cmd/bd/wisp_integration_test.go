//go:build cgo && integration

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestWispCreateMaterializesChildDAG is the regression test for GH#3872.
// Before the fix, wisps were silently forced to create only the root unless
// the formula set pour=true, which broke ephemeral lifecycle testing of
// multi-step formulas. A wisp pour now always materializes the full child
// DAG, just marked Ephemeral=true so it doesn't sync via git.
func TestWispCreateMaterializesChildDAG(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")

	rootID := wispTestProto(t, ctx, s, 2)

	withWispTestGlobals(t, s, ctx)

	_ = captureStdout(t, func() error {
		runWispCreate(makeWispTestCmd(false), []string{rootID})
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

// TestWispCreateDryRunFanoutMessage verifies the dry-run printout reflects
// full DAG materialization. Catches regressions in user-facing messaging.
func TestWispCreateDryRunFanoutMessage(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")

	rootID := wispTestProto(t, ctx, s, 2)

	withWispTestGlobals(t, s, ctx)

	output := captureStdout(t, func() error {
		runWispCreate(makeWispTestCmd(true), []string{rootID})
		return nil
	})
	if !strings.Contains(output, "would create wisp with 3 issues") {
		t.Errorf("dry-run should mention 3 issues (root + 2 children), got:\n%s", output)
	}
}
