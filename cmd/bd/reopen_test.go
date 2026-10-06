//go:build cgo

package main

import (
	"context"
	"testing"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
)

type reopenTestHelper struct {
	s   *dolt.DoltStore
	ctx context.Context
	t   *testing.T
}

func (h *reopenTestHelper) createIssue(title string, issueType types.IssueType, priority int) *types.Issue {
	issue := &types.Issue{
		Title:     title,
		Priority:  priority,
		IssueType: issueType,
		Status:    types.StatusOpen,
	}
	if err := h.s.CreateIssue(h.ctx, issue, "test-user"); err != nil {
		h.t.Fatalf("Failed to create issue: %v", err)
	}
	return issue
}

func (h *reopenTestHelper) closeIssue(issueID, reason string) {
	if err := h.s.CloseIssue(h.ctx, issueID, "test-user", reason, ""); err != nil {
		h.t.Fatalf("Failed to close issue: %v", err)
	}
}

func (h *reopenTestHelper) reopenIssue(issueID string) {
	// Reopen through the lifecycle operation so the fixture carries the full
	// closure teardown, mirroring closeIssue above.
	if err := h.s.ReopenIssue(h.ctx, issueID, "", "test-user"); err != nil {
		h.t.Fatalf("Failed to reopen issue: %v", err)
	}
}

func (h *reopenTestHelper) getIssue(issueID string) *types.Issue {
	issue, err := h.s.GetIssue(h.ctx, issueID)
	if err != nil {
		h.t.Fatalf("Failed to get issue: %v", err)
	}
	return issue
}

func (h *reopenTestHelper) addComment(issueID, comment string) {
	if err := h.s.AddComment(h.ctx, issueID, "test-user", comment); err != nil {
		h.t.Fatalf("Failed to add comment: %v", err)
	}
}

func (h *reopenTestHelper) assertStatus(issueID string, expected types.Status) {
	issue := h.getIssue(issueID)
	if issue.Status != expected {
		h.t.Errorf("Expected status %s, got %s", expected, issue.Status)
	}
}

func (h *reopenTestHelper) assertClosedAtSet(issueID string) {
	issue := h.getIssue(issueID)
	if issue.ClosedAt == nil {
		h.t.Error("Expected ClosedAt to be set")
	}
}

func (h *reopenTestHelper) assertClosedAtNil(issueID string) {
	issue := h.getIssue(issueID)
	if issue.ClosedAt != nil {
		h.t.Errorf("Expected ClosedAt to be nil, got %v", issue.ClosedAt)
	}
}

func (h *reopenTestHelper) assertCommentEvent(issueID, comment string) {
	events, err := h.s.GetEvents(h.ctx, issueID, 100)
	if err != nil {
		h.t.Fatalf("Failed to get events: %v", err)
	}

	for _, e := range events {
		if e.EventType == types.EventCommented && e.Comment != nil && *e.Comment == comment {
			return
		}
	}
	h.t.Errorf("Expected to find comment event with reason '%s'", comment)
}
