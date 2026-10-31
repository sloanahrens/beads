package dolt

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
)

// runExactIDSet answers an exact-id lookup through the same transaction seam
// the store's ExactIssueIDs hands issueops, counting the statements it issues.
func runExactIDSet(t *testing.T, ctx context.Context, store *DoltStore, ids []string) (map[string]struct{}, int) {
	t.Helper()
	statements := 0
	var set map[string]struct{}
	if err := store.withReadTx(ctx, func(tx *sql.Tx) error {
		var err error
		set, err = issueops.ExactIDSetInTx(ctx, countingDBTX{tx: tx, statements: &statements}, ids)
		return err
	}); err != nil {
		t.Fatalf("ExactIDSetInTx over %d ids: %v", len(ids), err)
	}
	return set, statements
}

// TestExactIDSetStatementCountIsFixed is the cost half of be-mfu5.
//
// `bd show --json` resolved every id with its own store lookup before the
// detail read (be-mfu5 measured 15-27 ms of the per-id cost as resolution).
// The batched exact-id lookup must cost a FIXED number of statements instead:
// 500 and 1,010 exact local ids issue the same statement count as 50, because
// the round trip removed is per id, not per chunk. The fixture holds no
// wisps, so the wisp half is its own emptiness probe at every size, and the
// leases probe is one statement at every size.
func TestExactIDSetStatementCountIsFixed(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// The largest size is the list the bead measured 1,010-id shows at.
	const largeSize = 1010
	seeded := make([]*types.Issue, 0, largeSize)
	ids := make([]string, 0, largeSize)
	for i := 0; i < largeSize; i++ {
		id := fmt.Sprintf("exactcount-%03d", i)
		seeded = append(seeded, &types.Issue{
			ID:        id,
			Title:     "exact id " + id,
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
		})
		ids = append(ids, id)
	}
	if err := store.CreateIssues(ctx, seeded, "tester"); err != nil {
		t.Fatalf("seed %d issues: %v", len(seeded), err)
	}

	small := ids[:50]
	medium := ids[:500]
	large := ids

	smallSet, smallStatements := runExactIDSet(t, ctx, store, small)
	mediumSet, mediumStatements := runExactIDSet(t, ctx, store, medium)
	largeSet, largeStatements := runExactIDSet(t, ctx, store, large)

	if smallStatements != mediumStatements || smallStatements != largeStatements {
		t.Errorf("statements: %d ids = %d, %d ids = %d, %d ids = %d: the lookup costs per id, not per chunk",
			len(small), smallStatements, len(medium), mediumStatements, len(large), largeStatements)
	}
	// The ceiling guards against an accidental per-id statement creeping back
	// in: three statements (leases probe, wisp probe, one issues statement)
	// plus slack.
	const fixedCeiling = 8
	if largeStatements > fixedCeiling {
		t.Errorf("%d statements for %d ids exceeds the fixed ceiling of %d",
			largeStatements, len(large), fixedCeiling)
	}

	// The count is only meaningful if the lookup answered: an empty set would
	// also satisfy the growth check.
	if len(smallSet) != len(small) {
		t.Errorf("set over %d ids held %d ids", len(small), len(smallSet))
	}
	if len(mediumSet) != len(medium) {
		t.Errorf("set over %d ids held %d ids", len(medium), len(mediumSet))
	}
	if len(largeSet) != len(large) {
		t.Errorf("set over %d ids held %d ids", len(large), len(largeSet))
	}
	for _, id := range large {
		if _, ok := largeSet[id]; !ok {
			t.Fatalf("%s missing from the exact-id set", id)
		}
	}
}

// TestExactIDSetAnswersBothPlanesAndOnlyExactIDs pins what the set means: a
// durable id and a wisp id are both exact, while a missing id, a partial id
// and an id of another rig are not — those must still reach the caller's
// resolver, which is what keeps the fast path from changing their answer.
func TestExactIDSetAnswersBothPlanesAndOnlyExactIDs(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	durable := &types.Issue{ID: "exactset-durable", Title: "durable", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}
	wisp := &types.Issue{ID: "exactset-wisp", Title: "wisp", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask, Ephemeral: true}
	if err := store.CreateIssue(ctx, durable, "tester"); err != nil {
		t.Fatalf("seed durable issue: %v", err)
	}
	if err := store.CreateIssue(ctx, wisp, "tester"); err != nil {
		t.Fatalf("seed wisp: %v", err)
	}

	ids := []string{durable.ID, wisp.ID, "exactset-missing", "exactset-dur", "hr-1abc"}
	set, _ := runExactIDSet(t, ctx, store, ids)

	for _, id := range []string{durable.ID, wisp.ID} {
		if _, ok := set[id]; !ok {
			t.Errorf("%s names a row exactly but is not in the set", id)
		}
	}
	for _, id := range []string{"exactset-missing", "exactset-dur", "hr-1abc"} {
		if _, ok := set[id]; ok {
			t.Errorf("%q is not an exact id in this store but is in the set", id)
		}
	}
}
