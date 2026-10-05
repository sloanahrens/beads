package dolt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/steveyegge/beads/internal/types"
	publicops "github.com/steveyegge/beads/issueops"
)

// This file is the be-hy2 matrix: every way a blocker can end, checked against
// the denormalized is_blocked flag its dependents carry.
//
// is_blocked is derived state maintained at named write points
// (issueops.RecomputeIsBlockedInTx and the close/delete callers around it), not
// a value computed at read time. Three stale cases were observed on 2026-09-30
// in wisp chains — a dependent read is_blocked=1 while `bd blocked` named a
// CLOSED blocker — and none reproduced by 2026-10-05 (0 of 89 blocked beads
// were blocked only by closed blockers). This matrix settles whether any
// blocker-ending path can still leave the flag set. It changes NO production
// code: a path that fails today becomes a named t.Skip naming the path, and is
// recorded in the bead notes so the fix gets its own bead.
//
// THE MATRIX. For each combination of
//
//	blocker plane   {regular issue, wisp},
//	dependent plane {regular issue, wisp},
//	edge shape      {direct blocks edge, chain of two through a parent-child},
//	ending path     {bd close, bulk close, delete, supersede, molecule burn,
//	                 wisp gc of a closed wisp},
//
// the case seeds blocker and dependent, ends the blocker by that path, then
// asserts the dependent's STORED flag is cleared and it has left the blocked
// list. The chain shape is a two-edge path — dependent --parent-child--> mid
// --blocks--> blocker — so the dependent's flag has no direct blocker edge and
// can only clear if the recompute reaches it transitively. The mid bead and
// the blocker live in the same plane; only the dependent's plane varies.
//
// `wisp gc of a closed wisp` is wisp-only: it collects wisps, so an issue
// blocker has no such form (the issue column runs the other five paths).
//
// HOST SAFETY: every case builds its own store through setupTestStore — a
// temporary directory plus a copy-on-write branch on the suite's own Dolt test
// server (see testmain_test.go). Nothing here reads or writes the production
// Dolt on :3307, ~/gt, or an installed bd binary, and no case sleeps.
func TestIsBlockedClearsWhenBlockerEnds(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	for _, blockerPlane := range []string{"issue", "wisp"} {
		for _, dependentPlane := range []string{"issue", "wisp"} {
			for _, shape := range []string{"direct", "chain"} {
				for _, path := range blockerEndingPaths() {
					if path.wispBlockersOnly && blockerPlane != "wisp" {
						continue
					}
					prefix := matrixPrefix(blockerPlane, dependentPlane, shape, path.name)
					t.Run(prefix, func(t *testing.T) {
						ctx, cancel := testContext(t)
						defer cancel()

						seeded := seedBlockerEndCase(t, ctx, store, prefix, blockerPlane, dependentPlane, shape)

						// Precondition: the dependent really is blocked before
						// the blocker ends, or the post-condition would be
						// vacuous — an unblocked dependent never "clears".
						requireBlocked(t, ctx, store, seeded.dependent, "dependent")
						if seeded.mid != "" {
							requireBlocked(t, ctx, store, seeded.mid, "mid")
						}
						if blocked := blockedListIDs(t, ctx, store); !blocked[seeded.dependent] {
							t.Fatalf("precondition: %s is not in the blocked list while its blocker is open", seeded.dependent)
						}

						path.end(t, ctx, store, seeded)

						requireUnblocked(t, ctx, store, seeded.dependent, "dependent")
						if seeded.mid != "" {
							requireUnblocked(t, ctx, store, seeded.mid, "mid")
						}
						if blocked := blockedListIDs(t, ctx, store); blocked[seeded.dependent] {
							t.Fatalf("%s is still in the blocked list after its blocker ended by %s", seeded.dependent, path.name)
						}
					})
				}
			}
		}
	}
}

// blockerEndCase is one seeded row of the matrix: the bead ids the case ends
// and the bystanders a path needs.
type blockerEndCase struct {
	blocker     string // the bead that ends
	mid         string // chain intermediate; empty for the direct shape
	dependent   string // the bead whose stored flag must clear
	replacement string // supersede target
	extra       string // second item of the bulk close
}

// blockerEndingPath is one way a blocker stops blocking, expressed as the
// storage-layer call sequence the corresponding CLI path performs.
type blockerEndingPath struct {
	name string
	// wispBlockersOnly marks a path with no issue-blocker form: `bd mol wisp
	// gc` collects wisps, so a regular issue cannot end through it.
	wispBlockersOnly bool
	end              func(t *testing.T, ctx context.Context, store *DoltStore, seeded blockerEndCase)
}

// blockerEndingPaths returns the six ending paths the bead lists. Each closure
// performs ONLY the storage calls that settle the blocker, so a failure points
// at the recompute the path runs rather than at test scaffolding.
func blockerEndingPaths() []blockerEndingPath {
	return []blockerEndingPath{
		{
			// `bd close <blocker>` — the single-id lifecycle close
			// (DoltStore.CloseIssue, issueops.CloseIssueInTx).
			name: "bd_close",
			end: func(t *testing.T, ctx context.Context, store *DoltStore, seeded blockerEndCase) {
				if err := store.CloseIssue(ctx, seeded.blocker, "completed", "tester", "matrix-session"); err != nil {
					t.Fatalf("CloseIssue(%s): %v", seeded.blocker, err)
				}
			},
		},
		{
			// `bd close <blocker> <extra>` — the BatchCloser role: one
			// transaction over both ids, both closes landing.
			name: "bulk_close",
			end: func(t *testing.T, ctx context.Context, store *DoltStore, seeded blockerEndCase) {
				closer, err := store.BatchCloser()
				if err != nil {
					t.Fatalf("BatchCloser(): %v", err)
				}
				result, err := closer.CloseBatch(ctx, publicops.CloseBatchRequest{
					Actor: "tester",
					Items: []publicops.BatchCloseItem{
						{IssueID: seeded.blocker},
						{IssueID: seeded.extra},
					},
				})
				if err != nil {
					t.Fatalf("CloseBatch: %v", err)
				}
				for i, outcome := range result.Outcomes {
					if outcome.Err != nil || !outcome.Changed {
						t.Fatalf("CloseBatch outcome[%d] = %#v, want a landed close", i, outcome)
					}
				}
			},
		},
		{
			// `bd delete <blocker> --force` — force erases the blocker and
			// drops its edges, orphaning the dependents.
			name: "delete",
			end: func(t *testing.T, ctx context.Context, store *DoltStore, seeded blockerEndCase) {
				if _, err := store.DeleteIssues(ctx, []string{seeded.blocker}, false, true, false); err != nil {
					t.Fatalf("DeleteIssues(%s): %v", seeded.blocker, err)
				}
			},
		},
		{
			// `bd supersede <blocker> --with <replacement>` — a supersedes
			// edge plus a lifecycle close (cmd/bd/duplicate.go runSupersede).
			name: "supersede",
			end: func(t *testing.T, ctx context.Context, store *DoltStore, seeded blockerEndCase) {
				if err := store.AddDependency(ctx, &types.Dependency{
					IssueID:     seeded.blocker,
					DependsOnID: seeded.replacement,
					Type:        types.DepSupersedes,
				}, "tester"); err != nil {
					t.Fatalf("supersede edge %s -> %s: %v", seeded.blocker, seeded.replacement, err)
				}
				if err := store.CloseIssue(ctx, seeded.blocker, "superseded", "tester", "matrix-session"); err != nil {
					t.Fatalf("CloseIssue(%s): %v", seeded.blocker, err)
				}
			},
		},
		{
			// `bd mol burn` — burnWisps erases each wisp with DeleteIssue
			// inside one transaction (cmd/bd/mol_burn.go burnWispsInto), and
			// the persistent form erases each issue the same way.
			name: "molecule_burn",
			end: func(t *testing.T, ctx context.Context, store *DoltStore, seeded blockerEndCase) {
				if err := store.DeleteIssue(ctx, seeded.blocker); err != nil {
					t.Fatalf("DeleteIssue(%s): %v", seeded.blocker, err)
				}
			},
		},
		{
			// `bd mol wisp gc --closed` — the wisp is closed, then collected;
			// the CLI's deleteBatch runs with force=true, cascade=false
			// (cmd/bd/wisp.go runWispPurgeClosed).
			name:             "wisp_gc_closed_wisp",
			wispBlockersOnly: true,
			end: func(t *testing.T, ctx context.Context, store *DoltStore, seeded blockerEndCase) {
				if err := store.CloseIssue(ctx, seeded.blocker, "completed", "tester", "matrix-session"); err != nil {
					t.Fatalf("CloseIssue(%s) before gc: %v", seeded.blocker, err)
				}
				if _, err := store.DeleteIssues(ctx, []string{seeded.blocker}, false, true, false); err != nil {
					t.Fatalf("DeleteIssues(%s) gc: %v", seeded.blocker, err)
				}
			},
		},
	}
}

// seedBlockerEndCase creates the bead graph for one matrix row through the
// normal write path, so is_blocked is maintained the way production maintains
// it. Beads are created before the edges that join them.
func seedBlockerEndCase(t *testing.T, ctx context.Context, store *DoltStore, prefix, blockerPlane, dependentPlane, shape string) blockerEndCase {
	t.Helper()
	seeded := blockerEndCase{
		blocker:     prefix + "-blk",
		mid:         prefix + "-mid",
		dependent:   prefix + "-dep",
		replacement: prefix + "-rep",
		extra:       prefix + "-x",
	}
	createPlaneBead(t, ctx, store, seeded.blocker, blockerPlane)
	createPlaneBead(t, ctx, store, seeded.dependent, dependentPlane)
	createPlaneBead(t, ctx, store, seeded.replacement, "issue")
	createPlaneBead(t, ctx, store, seeded.extra, "issue")

	if shape == "chain" {
		// Two edges: mid is blocked directly by the blocker, and the
		// dependent inherits the block as a parent-child child of mid. The
		// dependent has no blocking edge of its own, so only a recompute that
		// reaches it transitively can clear it.
		createPlaneBead(t, ctx, store, seeded.mid, blockerPlane)
		addPlaneEdge(t, ctx, store, seeded.mid, seeded.blocker, types.DepBlocks)
		addPlaneEdge(t, ctx, store, seeded.dependent, seeded.mid, types.DepParentChild)
		return seeded
	}
	seeded.mid = ""
	addPlaneEdge(t, ctx, store, seeded.dependent, seeded.blocker, types.DepBlocks)
	return seeded
}

// createPlaneBead creates an open task in the named plane: a wisp when plane is
// "wisp", a durable issue otherwise. Ephemeral beads use explicit ids that do
// not match the "-wisp-" pattern, exactly like the beads an agent files with
// --id, so the store routes them by wisps-table residency (isActiveWisp).
func createPlaneBead(t *testing.T, ctx context.Context, store *DoltStore, id, plane string) {
	t.Helper()
	iss := &types.Issue{
		ID:        id,
		Title:     id,
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
		Ephemeral: plane == "wisp",
	}
	if err := store.CreateIssue(ctx, iss, "tester"); err != nil {
		t.Fatalf("create %s bead %s: %v", plane, id, err)
	}
}

// addPlaneEdge adds one dependency edge through the normal write path, which is
// what maintains is_blocked.
func addPlaneEdge(t *testing.T, ctx context.Context, store *DoltStore, from, to string, depType types.DependencyType) {
	t.Helper()
	if err := store.AddDependency(ctx, &types.Dependency{IssueID: from, DependsOnID: to, Type: depType}, "tester"); err != nil {
		t.Fatalf("add %s edge %s -> %s: %v", depType, from, to, err)
	}
}

// requireBlocked fails unless the bead's stored is_blocked column is set. The
// flag is read straight from the owning table (issues or wisps) because that
// stored value — not a graph walk at read time — is what the bug is about.
func requireBlocked(t *testing.T, ctx context.Context, store *DoltStore, id, role string) {
	t.Helper()
	if !storedBlockedFlag(t, ctx, store, id) {
		t.Fatalf("precondition: %s %s reads is_blocked=0, want 1", role, id)
	}
}

// requireUnblocked fails unless the bead's stored is_blocked column is clear.
func requireUnblocked(t *testing.T, ctx context.Context, store *DoltStore, id, role string) {
	t.Helper()
	if storedBlockedFlag(t, ctx, store, id) {
		t.Fatalf("%s %s still reads is_blocked=1 after its blocker ended", role, id)
	}
}

// storedBlockedFlag reads the denormalized is_blocked column from whichever
// plane owns the id. A missing row in both planes is a test bug: the bead was
// seeded, and only the wisp-collecting paths remove it.
func storedBlockedFlag(t *testing.T, ctx context.Context, store *DoltStore, id string) bool {
	t.Helper()
	for _, table := range []string{"issues", "wisps"} {
		var blocked bool
		err := store.db.QueryRowContext(ctx, "SELECT is_blocked FROM "+table+" WHERE id = ?", id).Scan(&blocked)
		if err == nil {
			return blocked
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("read is_blocked from %s for %s: %v", table, id, err)
		}
	}
	t.Fatalf("%s is in neither plane (issues, wisps)", id)
	return false
}

// blockedListIDs is the set of ids the blocked-list read reports, so a case can
// assert the dependent left the list as well as clearing its stored flag. The
// read scans both planes, so a wisp dependent is visible here too.
func blockedListIDs(t *testing.T, ctx context.Context, store *DoltStore) map[string]bool {
	t.Helper()
	list, err := store.GetBlockedIssues(ctx, types.WorkFilter{})
	if err != nil {
		t.Fatalf("GetBlockedIssues: %v", err)
	}
	ids := make(map[string]bool, len(list))
	for _, blocked := range list {
		ids[blocked.ID] = true
	}
	return ids
}

// matrixPrefix builds a subtest name and bead-id prefix that is unique per
// matrix row and never contains the "-wisp-" infix, so an ephemeral bead's
// plane is decided by table residency and not by an id pattern.
func matrixPrefix(blockerPlane, dependentPlane, shape, path string) string {
	short := func(plane string) string {
		if plane == "wisp" {
			return "e"
		}
		return "r"
	}
	return fmt.Sprintf("behy2-%s%s-%s-%s", short(blockerPlane), short(dependentPlane), shape, path)
}
