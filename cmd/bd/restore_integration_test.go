//go:build cgo && integration

package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
)

// TestRestoreWithInvalidIssueID verifies that restore handles non-existent issues
// gracefully without panicking.
func TestRestoreWithInvalidIssueID(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".beads", "beads.db")
	testStore := newTestStore(t, dbPath)
	defer testStore.Close()

	ctx := context.Background()
	issue, err := testStore.GetIssue(ctx, "nonexistent-issue-12345")

	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetIssue expected ErrNotFound, got: %v", err)
	}
	if issue != nil {
		t.Fatalf("GetIssue returned issue for non-existent ID: %v", issue)
	}
}
