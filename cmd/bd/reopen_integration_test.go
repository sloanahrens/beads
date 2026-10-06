//go:build cgo && integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestReopenCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bd-test-reopen-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testDB := filepath.Join(tmpDir, "test.db")
	s := newTestStore(t, testDB)
	defer s.Close()

	ctx := context.Background()
	h := &reopenTestHelper{s: s, ctx: ctx, t: t}

	t.Run("reopen closed issue", func(t *testing.T) {
		issue := h.createIssue("Test Issue", types.TypeBug, 1)
		h.closeIssue(issue.ID, "Closing for test")
		h.assertStatus(issue.ID, types.StatusClosed)
		h.assertClosedAtSet(issue.ID)
		h.reopenIssue(issue.ID)
		h.assertStatus(issue.ID, types.StatusOpen)
		h.assertClosedAtNil(issue.ID)
	})

	t.Run("reopen with reason adds comment", func(t *testing.T) {
		issue := h.createIssue("Test Issue 2", types.TypeTask, 1)
		h.closeIssue(issue.ID, "Done")
		h.reopenIssue(issue.ID)
		reason := "Found a regression"
		h.addComment(issue.ID, reason)
		h.assertCommentEvent(issue.ID, reason)
	})

	t.Run("reopen multiple issues", func(t *testing.T) {
		issue1 := h.createIssue("Multi Test 1", types.TypeBug, 1)
		issue2 := h.createIssue("Multi Test 2", types.TypeBug, 1)
		h.closeIssue(issue1.ID, "Done")
		h.closeIssue(issue2.ID, "Done")
		h.reopenIssue(issue1.ID)
		h.reopenIssue(issue2.ID)
		h.assertStatus(issue1.ID, types.StatusOpen)
		h.assertStatus(issue2.ID, types.StatusOpen)
	})

	t.Run("reopen already open issue is no-op", func(t *testing.T) {
		issue := h.createIssue("Already Open", types.TypeTask, 1)
		h.reopenIssue(issue.ID)
		h.assertStatus(issue.ID, types.StatusOpen)
		h.assertClosedAtNil(issue.ID)
	})
}
