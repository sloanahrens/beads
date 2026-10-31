package dolt

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
)

// countingDBTX wraps the read transaction the batch detail read runs on and
// counts the statements it issues. Wrapping the seam rather than the driver is
// what makes the count meaningful here: every statement the read sends goes
// through one of these three methods, so a per-id read (or a loop hidden inside
// one of the helpers) shows up as growth.
type countingDBTX struct {
	tx         *sql.Tx
	statements *int
}

func (c countingDBTX) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	*c.statements++
	return c.tx.ExecContext(ctx, query, args...)
}

func (c countingDBTX) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	*c.statements++
	return c.tx.QueryContext(ctx, query, args...)
}

func (c countingDBTX) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	*c.statements++
	return c.tx.QueryRowContext(ctx, query, args...)
}

// TestGetIssueDetailsBatchStatementCountIsFixed is the cost half of be-2wkz:
// `bd show --json` over many ids used to run the detail query set once per id,
// which is why 1,010 ids cost 40-60 seconds. The batch read must cost a FIXED
// number of statements per chunk instead, so the count for 50 ids is the count
// for 10 ids — the two sizes carry the same shape (durable rows, one wisp, the
// same edge and comment setup), and only the number of ids differs.
//
// The counts are taken through the transaction seam issueops.GetIssueDetailsBatchInTx
// runs on, which is exactly the seam the DoltStore method hands it.
func TestGetIssueDetailsBatchStatementCountIsFixed(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	const (
		smallSize = 10
		largeSize = 50
	)
	seedIssues := func(t *testing.T, prefix string, n int, blockerID string) []string {
		t.Helper()
		ids := make([]string, 0, n)
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("%s-%03d", prefix, i)
			issue := &types.Issue{
				ID:        id,
				Title:     "batch " + id,
				Status:    types.StatusOpen,
				Priority:  2,
				IssueType: types.TypeTask,
			}
			if err := store.CreateIssue(ctx, issue, "tester"); err != nil {
				t.Fatalf("create %s: %v", id, err)
			}
			if err := store.AddDependency(ctx, &types.Dependency{
				IssueID: id, DependsOnID: blockerID, Type: types.DepBlocks,
			}, "tester"); err != nil {
				t.Fatalf("dependency %s: %v", id, err)
			}
			if _, err := store.AddIssueComment(ctx, id, "tester", "a comment for "+id); err != nil {
				t.Fatalf("comment %s: %v", id, err)
			}
			ids = append(ids, id)
		}
		return ids
	}

	blocker := &types.Issue{ID: "batchcount-blocker", Title: "blocker", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}
	if err := store.CreateIssue(ctx, blocker, "tester"); err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	// One wisp in both lists, with an edge, so the wisp arms of every read are
	// non-empty for both sizes and the comparison is like for like.
	wisp := &types.Issue{
		ID:        "batchcount-wisp",
		Title:     "a wisp the batch must read",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
		Ephemeral: true,
	}
	if err := store.CreateIssue(ctx, wisp, "tester"); err != nil {
		t.Fatalf("create wisp: %v", err)
	}
	if err := store.AddDependency(ctx, &types.Dependency{
		IssueID: wisp.ID, DependsOnID: blocker.ID, Type: types.DepBlocks,
	}, "tester"); err != nil {
		t.Fatalf("wisp dependency: %v", err)
	}

	small := append([]string{wisp.ID}, seedIssues(t, "batchcount-small", smallSize, blocker.ID)...)
	large := append([]string{wisp.ID}, seedIssues(t, "batchcount-large", largeSize, blocker.ID)...)

	run := func(ids []string, req storage.IssueDetailsBatchRequest) (*storage.IssueDetailsBatch, int) {
		t.Helper()
		req.IDs = ids
		statements := 0
		var out *storage.IssueDetailsBatch
		if err := store.withReadTx(ctx, func(tx *sql.Tx) error {
			var err error
			out, err = issueops.GetIssueDetailsBatchInTx(ctx, countingDBTX{tx: tx, statements: &statements}, req)
			return err
		}); err != nil {
			t.Fatalf("GetIssueDetailsBatchInTx over %d ids: %v", len(ids), err)
		}
		return out, statements
	}

	// The ceiling is a guard against an accidental per-id statement creeping
	// back in: the read is a fixed handful per chunk, nowhere near one per id.
	const fixedCeiling = 40
	for _, opts := range []struct {
		name string
		req  storage.IssueDetailsBatchRequest
	}{
		{"counts only", storage.IssueDetailsBatchRequest{}},
		{"every row list", storage.IssueDetailsBatchRequest{IncludeDependents: true, IncludeComments: true}},
	} {
		t.Run(opts.name, func(t *testing.T) {
			_, smallStatements := run(small, opts.req)
			largeOut, largeStatements := run(large, opts.req)

			if smallStatements != largeStatements {
				t.Errorf("statements for %d ids = %d, for %d ids = %d: the read costs per id, not per chunk",
					len(small), smallStatements, len(large), largeStatements)
			}
			if largeStatements > fixedCeiling {
				t.Errorf("%d statements for %d ids exceeds the fixed ceiling of %d",
					largeStatements, len(large), fixedCeiling)
			}

			// The count is only meaningful if the reads answered: a batch that
			// issued nothing would also satisfy the growth check.
			if len(largeOut.Issues) != len(large) {
				t.Fatalf("batch over %d ids answered %d issue rows", len(large), len(largeOut.Issues))
			}
			for _, id := range large {
				if got := len(largeOut.Dependencies[id]); got != 1 {
					t.Errorf("dependencies for %s = %d rows, want 1", id, got)
				}
				if got := largeOut.DependencyCounts[id]; got != 1 {
					t.Errorf("DependencyCount for %s = %d, want 1", id, got)
				}
				if got := largeOut.CommentCounts[id]; got != 1 {
					t.Errorf("CommentCount for %s = %d, want 1", id, got)
				}
				if opts.req.IncludeComments && len(largeOut.Comments[id]) != 1 {
					t.Errorf("comment rows for %s = %d, want 1", id, len(largeOut.Comments[id]))
				}
			}
		})
	}
}
