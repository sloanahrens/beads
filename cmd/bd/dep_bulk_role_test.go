//go:build cgo

package main

import (
	"context"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
)

// The bulk `bd dep add --file` route asserts its edges through the
// DependencyEditor role, so the invariants the hand-rolled bulk transaction
// used to carry are pinned here against the role the route now calls — entered
// through the store's own accessor, exactly as addDependencyEdgesDirect enters
// it.

func seedBulkRoleIssues(ctx context.Context, t *testing.T, s *dolt.DoltStore, ids ...string) {
	t.Helper()
	for _, id := range ids {
		issue := &types.Issue{
			ID: id, Title: id, Status: types.StatusOpen,
			Priority: 1, IssueType: types.TypeTask, CreatedAt: time.Now(),
		}
		if err := s.CreateIssue(ctx, issue, "test"); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
}
