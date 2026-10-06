//go:build cgo && integration

package main

import (
	"context"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestDuplicatesIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	testStore := newTestStore(t, tmpDir+"/.beads/beads.db")
	ctx := context.Background()

	// Create duplicate issues (let DB assign IDs)
	issues := []*types.Issue{
		{
			Title:       "Fix authentication bug",
			Description: "Users can't login",
			Status:      types.StatusOpen,
			Priority:    1,
			IssueType:   types.TypeBug,
		},
		{
			Title:       "Fix authentication bug",
			Description: "Users can't login",
			Status:      types.StatusOpen,
			Priority:    1,
			IssueType:   types.TypeBug,
		},
		{
			Title:       "Different task",
			Description: "Different description",
			Status:      types.StatusOpen,
			Priority:    2,
			IssueType:   types.TypeTask,
		},
	}

	for _, issue := range issues {
		if err := testStore.CreateIssue(ctx, issue, "test"); err != nil {
			t.Fatalf("CreateIssue failed: %v", err)
		}
	}

	// Fetch all issues
	allIssues, err := testStore.SearchIssues(ctx, "", types.IssueFilter{})
	if err != nil {
		t.Fatalf("SearchIssues failed: %v", err)
	}

	// Find duplicates
	groups := findDuplicateGroups(allIssues)

	if len(groups) != 1 {
		t.Fatalf("Expected 1 duplicate group, got %d", len(groups))
	}

	if len(groups[0]) != 2 {
		t.Fatalf("Expected 2 issues in group, got %d", len(groups[0]))
	}

	// Verify the duplicate group contains the two issues with "Fix authentication bug"
	dupCount := 0
	for _, issue := range groups[0] {
		if issue.Title == "Fix authentication bug" {
			dupCount++
		}
	}

	if dupCount != 2 {
		t.Errorf("Expected duplicate group to contain 2 'Fix authentication bug' issues, got %d", dupCount)
	}
}

func TestPerformMerge(t *testing.T) {
	tmpDir := t.TempDir()
	testStore := newTestStore(t, tmpDir+"/.beads/beads.db")
	ctx := context.Background()

	// Set up global state needed by performMerge
	oldStore := store
	oldRootCtx := rootCtx
	oldActor := actor
	store = testStore
	rootCtx = ctx
	actor = "test-user"
	defer func() {
		store = oldStore
		rootCtx = oldRootCtx
		actor = oldActor
	}()

	// Create duplicate issues
	target := &types.Issue{
		Title:       "Main issue",
		Description: "This is the target",
		Status:      types.StatusOpen,
		Priority:    1,
		IssueType:   types.TypeTask,
	}
	source1 := &types.Issue{
		Title:       "Main issue",
		Description: "This is the target",
		Status:      types.StatusOpen,
		Priority:    1,
		IssueType:   types.TypeTask,
	}
	source2 := &types.Issue{
		Title:       "Main issue",
		Description: "This is the target",
		Status:      types.StatusOpen,
		Priority:    1,
		IssueType:   types.TypeTask,
	}

	for _, issue := range []*types.Issue{target, source1, source2} {
		if err := testStore.CreateIssue(ctx, issue, "test"); err != nil {
			t.Fatalf("CreateIssue failed: %v", err)
		}
	}

	// Perform the merge
	result := performMerge(target.ID, []string{source1.ID, source2.ID})

	// Verify result structure
	closedIDs := result["closed"].([]string)
	linkedIDs := result["linked"].([]string)
	errors := result["errors"].([]string)

	if len(closedIDs) != 2 {
		t.Errorf("Expected 2 closed issues, got %d", len(closedIDs))
	}
	if len(linkedIDs) != 2 {
		t.Errorf("Expected 2 linked issues, got %d", len(linkedIDs))
	}
	if len(errors) != 0 {
		t.Errorf("Expected 0 errors, got %d: %v", len(errors), errors)
	}

	// Verify source issues are closed
	for _, sourceID := range []string{source1.ID, source2.ID} {
		issue, err := testStore.GetIssue(ctx, sourceID)
		if err != nil {
			t.Fatalf("GetIssue(%s) failed: %v", sourceID, err)
		}
		if issue.Status != types.StatusClosed {
			t.Errorf("Issue %s should be closed, got status %s", sourceID, issue.Status)
		}
	}

	// Verify target is still open
	targetIssue, err := testStore.GetIssue(ctx, target.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) failed: %v", target.ID, err)
	}
	if targetIssue.Status != types.StatusOpen {
		t.Errorf("Target issue should still be open, got status %s", targetIssue.Status)
	}

	// Verify dependencies were created (GetDependencies returns issues this depends on)
	for _, sourceID := range []string{source1.ID, source2.ID} {
		deps, err := testStore.GetDependencies(ctx, sourceID)
		if err != nil {
			t.Fatalf("GetDependencies(%s) failed: %v", sourceID, err)
		}
		found := false
		for _, dep := range deps {
			if dep.ID == target.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected dependency from %s to %s", sourceID, target.ID)
		}
	}
}
