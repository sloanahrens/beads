//go:build cgo && integration

package embeddeddolt_test

import (
	"runtime"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

func TestHookFiringStoreCreateIssuesFiresDependencyUpdatesFromEmbeddedStore(t *testing.T) {
	if runtime.GOOS == "windows" {
		// The hook runner on Windows executes hook files directly via CreateProcess,
		// which has no shebang dispatch. The extensionless #!/bin/sh hook written by
		// newEmbeddedHookStore cannot be executed as a shell script, so no payload is
		// ever logged and the assertions fail. Same limitation already skipped in
		// internal/hooks/hooks_test.go. See: https://github.com/gastownhall/beads/issues/3800
		t.Skip("hook script execution not supported on Windows - see GH#3800")
	}
	t.Run("non_transactional", func(t *testing.T) {
		te := newTestEnv(t, "hk")
		ctx := t.Context()
		store, logPath := newEmbeddedHookStore(t, te)

		source := &types.Issue{
			ID:        "hk-source",
			Title:     "Source",
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
			Dependencies: []*types.Dependency{
				{DependsOnID: "hk-target-a", Type: types.DepBlocks},
				{DependsOnID: "hk-target-b", Type: types.DepBlocks},
			},
		}
		targetA := &types.Issue{ID: "hk-target-a", Title: "Target A", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}
		targetB := &types.Issue{ID: "hk-target-b", Title: "Target B", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}

		if err := store.CreateIssues(ctx, []*types.Issue{source, targetA, targetB}, "tester"); err != nil {
			t.Fatalf("CreateIssues: %v", err)
		}

		assertDependencyHookPayloads(t, logPath, []string{"hk-target-a", "hk-target-b"})
	})

	t.Run("transactional", func(t *testing.T) {
		te := newTestEnv(t, "txh")
		ctx := t.Context()
		store, logPath := newEmbeddedHookStore(t, te)

		source := &types.Issue{
			ID:        "txh-source",
			Title:     "Source",
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
			Dependencies: []*types.Dependency{
				{DependsOnID: "txh-target-a", Type: types.DepBlocks},
				{DependsOnID: "txh-target-b", Type: types.DepBlocks},
			},
		}
		targetA := &types.Issue{ID: "txh-target-a", Title: "Target A", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}
		targetB := &types.Issue{ID: "txh-target-b", Title: "Target B", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}

		err := store.RunInTransaction(ctx, "test: hook batch deps", func(tx storage.Transaction) error {
			return tx.CreateIssues(ctx, []*types.Issue{source, targetA, targetB}, "tester")
		})
		if err != nil {
			t.Fatalf("RunInTransaction: %v", err)
		}

		assertDependencyHookPayloads(t, logPath, []string{"txh-target-a", "txh-target-b"})
	})
}
