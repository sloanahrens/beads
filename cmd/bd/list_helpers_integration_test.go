//go:build cgo && integration

package main

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

func TestLoadWatchedIssues_WithParentIncludesHierarchyAndStableOrder(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	store := newTestStore(t, testDB)

	createIssue := func(title string, issueType types.IssueType) *types.Issue {
		issue := &types.Issue{
			Title:     title,
			Priority:  2,
			IssueType: issueType,
			Status:    types.StatusOpen,
		}
		if err := store.CreateIssue(ctx, issue, "test-user"); err != nil {
			t.Fatalf("Failed to create issue %s: %v", title, err)
		}
		return issue
	}

	addParentChild := func(child, parent *types.Issue) {
		dep := &types.Dependency{
			IssueID:     child.ID,
			DependsOnID: parent.ID,
			Type:        types.DepParentChild,
			CreatedAt:   time.Now(),
			CreatedBy:   "test-user",
		}
		if err := store.AddDependency(ctx, dep, "test-user"); err != nil {
			t.Fatalf("Failed to add dependency %s -> %s: %v", child.ID, parent.ID, err)
		}
	}

	parent := createIssue("Parent epic", types.TypeEpic)
	child := createIssue("Child task", types.TypeTask)
	grandchild := createIssue("Grandchild task", types.TypeTask)
	addParentChild(child, parent)
	addParentChild(grandchild, child)

	filter := types.IssueFilter{ParentID: &parent.ID}
	first, err := loadWatchedIssues(ctx, store, filter, false, parent.ID, "", false)
	if err != nil {
		t.Fatalf("loadWatchedIssues first call failed: %v", err)
	}
	second, err := loadWatchedIssues(ctx, store, filter, false, parent.ID, "", false)
	if err != nil {
		t.Fatalf("loadWatchedIssues second call failed: %v", err)
	}

	if len(first) != 3 {
		t.Fatalf("expected parent path to include parent and descendants, got %d issues", len(first))
	}

	firstIDs := []string{first[0].ID, first[1].ID, first[2].ID}
	secondIDs := []string{second[0].ID, second[1].ID, second[2].ID}
	if !slices.Equal(firstIDs, secondIDs) {
		t.Fatalf("expected stable watched issue ordering, got %v then %v", firstIDs, secondIDs)
	}

	wantIDs := []string{parent.ID, child.ID, grandchild.ID}
	slices.Sort(wantIDs)
	if !slices.Equal(firstIDs, wantIDs) {
		t.Fatalf("expected watched issues to be normalized by id for snapshot stability, got %v want %v", firstIDs, wantIDs)
	}
}

func TestLoadWatchedIssues_ReadyWithParentPreservesReadySemantics(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	store := newTestStore(t, testDB)

	createIssue := func(title string, issueType types.IssueType) *types.Issue {
		issue := &types.Issue{
			Title:     title,
			Priority:  2,
			IssueType: issueType,
			Status:    types.StatusOpen,
		}
		if err := store.CreateIssue(ctx, issue, "test-user"); err != nil {
			t.Fatalf("Failed to create issue %s: %v", title, err)
		}
		return issue
	}
	addDep := func(child, parent *types.Issue, depType types.DependencyType) {
		dep := &types.Dependency{
			IssueID:     child.ID,
			DependsOnID: parent.ID,
			Type:        depType,
			CreatedAt:   time.Now(),
			CreatedBy:   "test-user",
		}
		if err := store.AddDependency(ctx, dep, "test-user"); err != nil {
			t.Fatalf("Failed to add dependency %s -> %s: %v", child.ID, parent.ID, err)
		}
	}

	parent := createIssue("Watch ready parent", types.TypeEpic)
	readyChild := createIssue("Watch ready child", types.TypeTask)
	blockedChild := createIssue("Watch blocked child", types.TypeTask)
	blocker := createIssue("Watch blocker", types.TypeTask)
	addDep(readyChild, parent, types.DepParentChild)
	addDep(blockedChild, parent, types.DepParentChild)
	addDep(blockedChild, blocker, types.DepBlocks)

	filter := types.IssueFilter{ParentID: &parent.ID}
	issues, err := loadWatchedIssues(ctx, store, filter, true, parent.ID, "", false)
	if err != nil {
		t.Fatalf("loadWatchedIssues ready parent failed: %v", err)
	}

	ids := make([]string, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	if !slices.Contains(ids, readyChild.ID) {
		t.Fatalf("expected ready child %s in watch ready parent result, got %v", readyChild.ID, ids)
	}
	if slices.Contains(ids, blockedChild.ID) {
		t.Fatalf("blocked child %s should not appear in watch ready parent result, got %v", blockedChild.ID, ids)
	}
}

func TestGetHierarchicalChildrenIncludesDescendantsBeyondDepthTen(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	store := newTestStore(t, testDB)

	root := &types.Issue{
		Title:     "Deep tree root",
		Priority:  2,
		IssueType: types.TypeEpic,
		Status:    types.StatusOpen,
	}
	if err := store.CreateIssue(ctx, root, "test-user"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	parent := root
	var leaf *types.Issue
	const depth = 12
	for i := 1; i <= depth; i++ {
		child := &types.Issue{
			Title:     "Deep tree child",
			Priority:  2,
			IssueType: types.TypeTask,
			Status:    types.StatusOpen,
		}
		if err := store.CreateIssue(ctx, child, "test-user"); err != nil {
			t.Fatalf("Failed to create child at depth %d: %v", i, err)
		}
		dep := &types.Dependency{
			IssueID:     child.ID,
			DependsOnID: parent.ID,
			Type:        types.DepParentChild,
			CreatedAt:   time.Now(),
			CreatedBy:   "test-user",
		}
		if err := store.AddDependency(ctx, dep, "test-user"); err != nil {
			t.Fatalf("Failed to add parent-child dependency at depth %d: %v", i, err)
		}
		parent = child
		leaf = child
	}

	issues, err := getHierarchicalChildren(ctx, store, "", root.ID, types.IssueFilter{})
	if err != nil {
		t.Fatalf("getHierarchicalChildren failed: %v", err)
	}
	ids := make([]string, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	if !slices.Contains(ids, leaf.ID) {
		t.Fatalf("expected descendant at depth %d (%s), got %v", depth, leaf.ID, ids)
	}
}
