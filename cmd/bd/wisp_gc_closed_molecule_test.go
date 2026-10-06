//go:build cgo

package main

import (
	"context"
	"testing"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
)

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
