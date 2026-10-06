//go:build cgo

package main

import (
	"context"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// seedIssue creates a minimal open task in the given store for test fixtures.
func seedIssue(t *testing.T, ctx context.Context, s storage.DoltStorage, id string) {
	t.Helper()
	issue := &types.Issue{
		ID:        id,
		Title:     "test issue " + id,
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, issue, "test"); err != nil {
		t.Fatalf("seed issue %s: %v", id, err)
	}
}
