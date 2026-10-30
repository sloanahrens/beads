//go:build cgo

package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
)

// wispTestProto creates a proto epic with the given number of parent-child
// children in the store, returning the root proto's ID. Used to exercise
// wisp DAG fanout.
func wispTestProto(t *testing.T, ctx context.Context, s *dolt.DoltStore, numChildren int) string {
	t.Helper()
	root := &types.Issue{
		Title:     "Proto Root",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create proto root: %v", err)
	}
	for i := 1; i <= numChildren; i++ {
		child := &types.Issue{
			Title:     fmt.Sprintf("Step %d", i),
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
		}
		if err := s.CreateIssue(ctx, child, "test"); err != nil {
			t.Fatalf("Failed to create step %d: %v", i, err)
		}
		if err := s.AddDependency(ctx, &types.Dependency{
			IssueID:     child.ID,
			DependsOnID: root.ID,
			Type:        types.DepParentChild,
		}, "test"); err != nil {
			t.Fatalf("Failed to add dependency for step %d: %v", i, err)
		}
	}
	return root.ID
}

// makeWispTestCmd builds a cobra.Command with the same flag schema as the
// real `bd mol wisp` command, with the given flag values set as defaults
// (so runWispCreate reads them back without needing arg parsing).
func makeWispTestCmd(dryRun bool) *cobra.Command {
	c := &cobra.Command{Use: "wisp"}
	c.Flags().StringArray("var", []string{}, "")
	c.Flags().Bool("dry-run", dryRun, "")
	return c
}

// TestWispCommandsRegisterNoRootOnlyFlag locks in that pouring a wisp always
// materializes every step, so the former opt-out flag is gone from both
// entrypoints (be-2gw.5). Re-registering it would restore the root-only
// escape hatch GH#3872 removed.
func TestWispCommandsRegisterNoRootOnlyFlag(t *testing.T) {
	for _, c := range []*cobra.Command{wispCmd, wispCreateCmd} {
		if f := c.Flags().Lookup("root-only"); f != nil {
			t.Errorf("%q still registers --root-only (%s); steps are always materialized", c.CommandPath(), f.Usage)
		}
	}
}

// countEphemeral returns the number of issues in the store with Ephemeral=true.
func countEphemeral(t *testing.T, ctx context.Context, s *dolt.DoltStore) int {
	t.Helper()
	tru := true
	results, err := s.SearchIssues(ctx, "", types.IssueFilter{Ephemeral: &tru})
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}
	return len(results)
}

// withWispTestGlobals saves and restores the package-level store/rootCtx/actor
// globals around a test, since runWispCreate reads them directly.
func withWispTestGlobals(t *testing.T, s *dolt.DoltStore, ctx context.Context) {
	t.Helper()
	oldStore, oldCtx, oldActor := store, rootCtx, actor
	t.Cleanup(func() { store, rootCtx, actor = oldStore, oldCtx, oldActor })
	store, rootCtx, actor = s, ctx, "test"
}
