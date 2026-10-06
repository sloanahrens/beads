//go:build cgo && integration

package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// TestUpdateClosePolicyDirectForceStillFencesAssigneeTransfer keeps the other
// half of `--force` intact. Conditioning it on an assignee edit must not turn
// it off when there IS one: a transfer away from a live foreign claim is still
// exactly what the flag authorizes.
func TestUpdateClosePolicyDirectForceStillFencesAssigneeTransfer(t *testing.T) {
	env := newParityEnv(t)
	env.seed("test-ucpa", "Held by another actor", func(i *types.Issue) {
		i.Assignee = "someone-else"
		i.Status = types.StatusInProgress
	})

	// Without --force the fence holds.
	env.setFlags(updateCmd, map[string]string{"assignee": "thief"})
	if res := env.run(updateCmd, "test-ucpa"); res.exitCode == 0 {
		t.Fatalf("unforced transfer succeeded; the fence is gone\nstderr:\n%s", res.stderr)
	}
	if got := env.get("test-ucpa").Assignee; got != "someone-else" {
		t.Fatalf("assignee = %q after a refused transfer, want someone-else", got)
	}

	// With it, the transfer is authorized.
	env.setFlags(updateCmd, map[string]string{"assignee": "thief", "force": "true"})
	if res := env.run(updateCmd, "test-ucpa"); res.exitCode != 0 {
		t.Fatalf("forced transfer exit = %d, want 0\nstderr:\n%s", res.exitCode, res.stderr)
	}
	if got := env.get("test-ucpa").Assignee; got != "thief" {
		t.Errorf("assignee = %q, want thief", got)
	}
}

// TestUpdateClosePolicyBatchCrossesIntoDone drives `bd batch update`, whose
// transaction reaches the same embedded write funnel without going through the
// facade at all.
func TestUpdateClosePolicyBatchCrossesIntoDone(t *testing.T) {
	tmpDir := t.TempDir()
	st := newTestStoreWithPrefix(t, filepath.Join(tmpDir, ".beads", "beads.db"), "tbc")
	ctx := context.Background()

	seedBatchTestIssues(t, ctx, st, "tbc-parent", "tbc-child", "tbc-blocker", "tbc-blocked")
	for _, dep := range []*types.Dependency{
		{IssueID: "tbc-child", DependsOnID: "tbc-parent", Type: types.DepParentChild},
		{IssueID: "tbc-blocked", DependsOnID: "tbc-blocker", Type: types.DepBlocks},
	} {
		if err := st.AddDependency(ctx, dep, "test"); err != nil {
			t.Fatalf("seed dependency %s -> %s: %v", dep.IssueID, dep.DependsOnID, err)
		}
	}

	// An unforced crossing refuses — and because the batch is one transaction,
	// it takes the WHOLE batch down, including the priority edit on a line that
	// had nothing to do with the refusal. That is the documented contract.
	script := "update tbc-blocker priority=0\nupdate tbc-parent status=closed\n"
	err := runBatchScriptInTx(t, ctx, st, script)
	if err == nil {
		t.Fatal("batch update into done with an open child succeeded, want a refusal")
	}
	if !errors.Is(err, storage.ErrCloseOpenChildren) {
		t.Errorf("batch error = %v, want ErrCloseOpenChildren", err)
	}
	rolledBack, getErr := st.GetIssue(ctx, "tbc-blocker")
	if getErr != nil {
		t.Fatalf("GetIssue tbc-blocker: %v", getErr)
	}
	if rolledBack.Priority != 2 {
		t.Errorf("tbc-blocker priority = %d; an unforced refusal must roll back the whole batch", rolledBack.Priority)
	}

	if err := runBatchScriptInTx(t, ctx, st, "update tbc-blocked status=closed\n"); !errors.Is(err, storage.ErrCloseBlocked) {
		t.Errorf("batch error = %v, want ErrCloseBlocked", err)
	}

	// force=true overrides both, in the same one transaction.
	forced := "update tbc-parent status=closed force=true\nupdate tbc-blocked status=closed force=true\n"
	if err := runBatchScriptInTx(t, ctx, st, forced); err != nil {
		t.Fatalf("forced batch update into done: %v", err)
	}
	for _, id := range []string{"tbc-parent", "tbc-blocked"} {
		got, err := st.GetIssue(ctx, id)
		if err != nil {
			t.Fatalf("GetIssue %s: %v", id, err)
		}
		if got.Status != types.StatusClosed {
			t.Errorf("%s status = %q, want closed", id, got.Status)
		}
	}
}
