//go:build cgo

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
)

// TestWispPurgeClosedProtectsOpenMoleculeSteps is the regression test for
// be-96h. A completed step of a molecule is itself a CLOSED wisp, so the
// sanctioned mid-cycle `bd mol wisp gc --closed --force` used to delete the
// running patrol's own finished steps and regress its progress (a deacon
// patrol went 2/28 complete -> 0/26). A closed wisp whose parent molecule is
// still open must survive; a closed wisp whose molecule is closed, and a
// closed wisp with no molecule parent, must still be purged.
func TestWispPurgeClosedProtectsOpenMoleculeSteps(t *testing.T) {
	ctx := context.Background()
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")

	// Molecule A is still running: its completed step must be protected.
	openMol := newGCTestIssue(t, ctx, s, "Open molecule", types.TypeMolecule, types.StatusOpen)
	openStep := newGCTestIssue(t, ctx, s, "Completed step of open molecule", types.TypeTask, types.StatusOpen)
	linkGCTestStep(t, ctx, s, openStep, openMol)
	closeGCTestIssue(t, ctx, s, openStep)

	// Molecule B is closed: both the molecule and its step are purgeable.
	closedMol := newGCTestIssue(t, ctx, s, "Closed molecule", types.TypeMolecule, types.StatusOpen)
	closedStep := newGCTestIssue(t, ctx, s, "Step of closed molecule", types.TypeTask, types.StatusOpen)
	linkGCTestStep(t, ctx, s, closedStep, closedMol)
	closeGCTestIssue(t, ctx, s, closedStep)
	closeGCTestIssue(t, ctx, s, closedMol)

	// A closed wisp with no parent at all is purgeable.
	orphan := newGCTestIssue(t, ctx, s, "Standalone closed wisp", types.TypeTask, types.StatusOpen)
	closeGCTestIssue(t, ctx, s, orphan)

	withWispTestGlobals(t, s, ctx)

	// Dry run must name the skipped protected step and delete nothing.
	out := captureStdout(t, func() error {
		return runWispPurgeClosed(ctx, true, false, nil)
	})
	if !strings.Contains(out, "Skipping 1 step(s) of open molecules (protected from cleanup)") {
		t.Errorf("dry run did not report the protected step, output:\n%s", out)
	}
	for _, id := range []string{openStep.ID, closedStep.ID, closedMol.ID, orphan.ID} {
		if !wispExists(t, ctx, s, id) {
			t.Fatalf("dry run deleted %s", id)
		}
	}

	// The real run with --force deletes everything except the open molecule's
	// completed step.
	out = captureStdout(t, func() error {
		return runWispPurgeClosed(ctx, false, true, nil)
	})
	if !strings.Contains(out, "Skipping 1 step(s) of open molecules (protected from cleanup)") {
		t.Errorf("purge did not report the protected step, output:\n%s", out)
	}
	if !wispExists(t, ctx, s, openStep.ID) {
		t.Errorf("step %s of the still-open molecule was deleted by --closed purge", openStep.ID)
	}
	if wispExists(t, ctx, s, closedStep.ID) {
		t.Errorf("step %s of a closed molecule should have been purged", closedStep.ID)
	}
	if wispExists(t, ctx, s, closedMol.ID) {
		t.Errorf("closed molecule root %s should have been purged", closedMol.ID)
	}
	if wispExists(t, ctx, s, orphan.ID) {
		t.Errorf("standalone closed wisp %s should have been purged", orphan.ID)
	}
}

// newGCTestIssue creates an ephemeral (wisp) issue in the temporary store and
// returns it with its store-assigned ID populated.
func newGCTestIssue(t *testing.T, ctx context.Context, s *dolt.DoltStore, title string, issueType types.IssueType, status types.Status) *types.Issue {
	t.Helper()
	issue := &types.Issue{
		Title:     title,
		Status:    status,
		Priority:  2,
		IssueType: issueType,
		Ephemeral: true,
	}
	if err := s.CreateIssue(ctx, issue, "test"); err != nil {
		t.Fatalf("CreateIssue(%s): %v", title, err)
	}
	return issue
}

// linkGCTestStep makes step a parent-child child of the molecule root, the edge
// findParentMolecules walks to identify a wisp's molecule.
func linkGCTestStep(t *testing.T, ctx context.Context, s *dolt.DoltStore, step, root *types.Issue) {
	t.Helper()
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     step.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("AddDependency(%s -> %s): %v", step.ID, root.ID, err)
	}
}

func closeGCTestIssue(t *testing.T, ctx context.Context, s *dolt.DoltStore, issue *types.Issue) {
	t.Helper()
	if err := s.CloseIssue(ctx, issue.ID, "done", "test", ""); err != nil {
		t.Fatalf("CloseIssue(%s): %v", issue.ID, err)
	}
}

func wispExists(t *testing.T, ctx context.Context, s *dolt.DoltStore, id string) bool {
	t.Helper()
	issue, err := s.GetIssue(ctx, id)
	return err == nil && issue != nil
}
