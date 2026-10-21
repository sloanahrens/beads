package dolt

import (
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// batchRefIssue returns an issue with no ID, so CreateIssues must assign one.
func batchRefIssue(title string, ephemeral bool, deps ...*types.Dependency) *types.Issue {
	return &types.Issue{
		Title:        title,
		Status:       types.StatusOpen,
		Priority:     2,
		IssueType:    types.TypeTask,
		Ephemeral:    ephemeral,
		Dependencies: deps,
	}
}

// requireBatchRefEdge checks that issue has exactly one dependency, on target.
func requireBatchRefEdge(t *testing.T, store *DoltStore, issue, target *types.Issue) {
	t.Helper()
	ctx, cancel := testContext(t)
	defer cancel()

	records, err := store.GetDependencyRecords(ctx, issue.ID)
	if err != nil {
		t.Fatalf("GetDependencyRecords(%s): %v", issue.ID, err)
	}
	if len(records) != 1 {
		t.Fatalf("%s has %d dependencies, want 1", issue.ID, len(records))
	}
	if records[0].DependsOnID != target.ID {
		t.Fatalf("%s depends on %q, want %q", issue.ID, records[0].DependsOnID, target.ID)
	}
}

func TestCreateIssuesResolvesBatchRefs(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// batch:<index> names the slice position, so the edges are written before
	// any of the three rows exists.
	a := batchRefIssue("batch step a", false)
	b := batchRefIssue("batch step b", false, &types.Dependency{
		IssueID:     "batch:1",
		DependsOnID: "batch:0",
		Type:        types.DepBlocks,
	})
	c := batchRefIssue("batch step c", false, &types.Dependency{
		IssueID:     "batch:2",
		DependsOnID: "batch:1",
		Type:        types.DepBlocks,
	})
	issues := []*types.Issue{a, b, c}

	if err := store.CreateIssues(ctx, issues, "tester"); err != nil {
		t.Fatalf("CreateIssues: %v", err)
	}

	for _, issue := range issues {
		if issue.ID == "" {
			t.Fatalf("caller's issue %q has no assigned ID after CreateIssues", issue.Title)
		}
	}
	requireBatchRefEdge(t, store, b, a)
	requireBatchRefEdge(t, store, c, b)
}

func TestCreateIssuesResolvesBatchRefsForWispBatch(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// An all-wisp batch takes the single-transaction fast path.
	a := batchRefIssue("wisp step a", true)
	b := batchRefIssue("wisp step b", true, &types.Dependency{
		IssueID:     "batch:1",
		DependsOnID: "batch:0",
		Type:        types.DepBlocks,
	})
	issues := []*types.Issue{a, b}

	if err := store.CreateIssues(ctx, issues, "tester"); err != nil {
		t.Fatalf("CreateIssues all-wisp batch: %v", err)
	}
	if a.ID == "" || b.ID == "" {
		t.Fatalf("wisp IDs not assigned: a=%q b=%q", a.ID, b.ID)
	}
	requireBatchRefEdge(t, store, b, a)
}

func TestCreateIssuesResolvesBatchRefsAgainstFullSliceInTransaction(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// The batch mixes planes. Inside the transaction the regular and wisp
	// subsets are created separately, so a ref only lands on the right issue
	// if it is resolved against the caller's full slice: batch:1 is the wisp
	// at position 1, which is first in its own subset only by accident.
	a := batchRefIssue("regular step a", false)
	w := batchRefIssue("wisp step w", true)
	b := batchRefIssue("regular step b", false, &types.Dependency{
		IssueID:     "batch:2",
		DependsOnID: "batch:0",
		Type:        types.DepBlocks,
	})
	x := batchRefIssue("wisp step x", true, &types.Dependency{
		IssueID:     "batch:3",
		DependsOnID: "batch:1",
		Type:        types.DepBlocks,
	})
	issues := []*types.Issue{a, w, b, x}

	err := store.RunInTransaction(ctx, "test: batch refs across planes", func(tx storage.Transaction) error {
		return tx.CreateIssues(ctx, issues, "tester")
	})
	if err != nil {
		t.Fatalf("RunInTransaction CreateIssues: %v", err)
	}

	requireBatchRefEdge(t, store, b, a)
	requireBatchRefEdge(t, store, x, w)
}
