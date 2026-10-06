//go:build cgo

package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// TestSingleIssueSnapshot tests that the snapshot captures status and update time.
func TestSingleIssueSnapshot(t *testing.T) {
	t.Parallel()
	now := time.Now()
	issue := &types.Issue{
		ID:        "test-001",
		Status:    types.StatusOpen,
		UpdatedAt: now,
	}

	snap1 := singleIssueSnapshot(issue)
	expected := fmt.Sprintf("test-001:open:%d", now.UnixNano())
	if snap1 != expected {
		t.Errorf("snapshot = %q, want %q", snap1, expected)
	}

	// Changing status changes the snapshot
	issue.Status = types.StatusClosed
	snap2 := singleIssueSnapshot(issue)
	if snap1 == snap2 {
		t.Error("snapshot should change when status changes from open to closed")
	}

	// Changing UpdatedAt changes the snapshot
	issue.UpdatedAt = now.Add(time.Second)
	snap3 := singleIssueSnapshot(issue)
	if snap2 == snap3 {
		t.Error("snapshot should change when UpdatedAt changes")
	}
}

// TestWatchIssueFlags tests that watch flag is properly registered.
func TestWatchIssueFlags(t *testing.T) {
	flag := showCmd.Flags().Lookup("watch")
	if flag == nil {
		t.Fatal("watch flag should be registered in showCmd")
	}
	if flag.DefValue != "false" {
		t.Errorf("watch flag default should be 'false', got '%s'", flag.DefValue)
	}
}
