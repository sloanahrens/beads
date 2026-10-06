//go:build cgo

package main

import (
	"context"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// newProvenanceTestIssue creates an issue the provenance events can hang off
// (the FK requires a real issue row).
func newProvenanceTestIssue(t *testing.T, ctx context.Context, s interface {
	CreateIssue(context.Context, *types.Issue, string) error
}) string {
	t.Helper()
	issue := &types.Issue{
		Title:     "Provenance subject",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeTask,
		CreatedAt: time.Now(),
	}
	if err := s.CreateIssue(ctx, issue, "test"); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	return issue.ID
}
