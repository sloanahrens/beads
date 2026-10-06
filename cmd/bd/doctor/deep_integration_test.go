//go:build cgo && integration

package doctor

import (
	"context"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// TestCheckParentConsistency_OrphanedDeps verifies detection of orphaned parent-child deps
func TestCheckParentConsistency_OrphanedDeps(t *testing.T) {
	store := newTestDoltStore(t, "bd")
	ctx := context.Background()

	// Create an issue
	issue := &types.Issue{
		ID:        "bd-1",
		Title:     "Test Issue",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
		CreatedAt: time.Now(),
	}
	if err := store.CreateIssue(ctx, issue, "test"); err != nil {
		t.Fatal(err)
	}

	// Insert a parent-child dep pointing to non-existent parent via raw SQL.
	// FK on depends_on_issue_id would normally block this; disable checks to
	// simulate the schema-drift scenario the validator is designed to catch.
	db := store.UnderlyingDB()
	if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatal(err)
	}
	_, err := db.ExecContext(ctx,
		"INSERT INTO dependencies (id, issue_id, depends_on_issue_id, type, created_at, created_by) VALUES (UUID(), ?, ?, ?, NOW(), ?)",
		"bd-1", "bd-missing", "parent-child", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1"); err != nil {
		t.Fatal(err)
	}

	check := checkParentConsistency(db)

	if check.Status != StatusError {
		t.Errorf("Status = %q, want %q", check.Status, StatusError)
	}
}

// TestCheckEpicCompleteness_CompletedEpic verifies detection of closeable epics
func TestCheckEpicCompleteness_CompletedEpic(t *testing.T) {
	store := newTestDoltStore(t, "epic")
	ctx := context.Background()

	// Insert an open epic
	epic := &types.Issue{
		ID:        "epic-1",
		Title:     "Epic",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
		CreatedAt: time.Now(),
	}
	if err := store.CreateIssue(ctx, epic, "test"); err != nil {
		t.Fatal(err)
	}

	// Insert a closed child task
	task := &types.Issue{
		ID:        "epic-1.1",
		Title:     "Task",
		Status:    types.StatusClosed,
		IssueType: types.TypeTask,
		ClosedAt:  ptrTime(time.Now()),
		CreatedAt: time.Now(),
	}
	if err := store.CreateIssue(ctx, task, "test"); err != nil {
		t.Fatal(err)
	}

	// Create parent-child relationship
	dep := &types.Dependency{
		IssueID:     "epic-1.1",
		DependsOnID: "epic-1",
		Type:        types.DepParentChild,
		CreatedAt:   time.Now(),
		CreatedBy:   "test",
	}
	if err := store.AddDependency(ctx, dep, "test"); err != nil {
		t.Fatal(err)
	}

	db := store.UnderlyingDB()
	check := checkEpicCompleteness(db)

	// Epic with all children closed should be detected
	if check.Status != StatusWarning {
		t.Errorf("Status = %q, want %q", check.Status, StatusWarning)
	}
}

func TestCheckEpicCompleteness_CountsWispChildren(t *testing.T) {
	store := newTestDoltStore(t, "epic")
	ctx := context.Background()

	epic := &types.Issue{
		ID:        "epic-wisp",
		Title:     "Epic",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
		CreatedAt: time.Now(),
	}
	if err := store.CreateIssue(ctx, epic, "test"); err != nil {
		t.Fatal(err)
	}

	child := &types.Issue{
		ID:        "epic-wisp.1",
		Title:     "Wisp child",
		Status:    types.StatusClosed,
		IssueType: types.TypeTask,
		ClosedAt:  ptrTime(time.Now()),
		NoHistory: true,
		CreatedAt: time.Now(),
	}
	if err := store.CreateIssue(ctx, child, "test"); err != nil {
		t.Fatal(err)
	}

	dep := &types.Dependency{
		IssueID:     child.ID,
		DependsOnID: epic.ID,
		Type:        types.DepParentChild,
		CreatedAt:   time.Now(),
		CreatedBy:   "test",
	}
	if err := store.AddDependency(ctx, dep, "test"); err != nil {
		t.Fatal(err)
	}

	check := checkEpicCompleteness(store.UnderlyingDB())
	if check.Status != StatusWarning {
		t.Errorf("Status = %q, want %q", check.Status, StatusWarning)
	}
}

func TestCheckEpicCompleteness_OpenWispChildPreventsCompletedEpic(t *testing.T) {
	store := newTestDoltStore(t, "epic")
	ctx := context.Background()

	epic := &types.Issue{
		ID:        "epic-open-wisp",
		Title:     "Epic",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
		CreatedAt: time.Now(),
	}
	if err := store.CreateIssue(ctx, epic, "test"); err != nil {
		t.Fatal(err)
	}

	child := &types.Issue{
		ID:        "epic-open-wisp.1",
		Title:     "Open wisp child",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
		NoHistory: true,
		CreatedAt: time.Now(),
	}
	if err := store.CreateIssue(ctx, child, "test"); err != nil {
		t.Fatal(err)
	}

	dep := &types.Dependency{
		IssueID:     child.ID,
		DependsOnID: epic.ID,
		Type:        types.DepParentChild,
		CreatedAt:   time.Now(),
		CreatedBy:   "test",
	}
	if err := store.AddDependency(ctx, dep, "test"); err != nil {
		t.Fatal(err)
	}

	check := checkEpicCompleteness(store.UnderlyingDB())
	if check.Status != StatusOK {
		t.Errorf("Status = %q, want %q; detail=%s", check.Status, StatusOK, check.Detail)
	}
}

// TestCheckMailThreadIntegrity_ValidThreads verifies valid thread references pass
func TestCheckMailThreadIntegrity_ValidThreads(t *testing.T) {
	store := newTestDoltStore(t, "thread")
	ctx := context.Background()

	// Insert issues
	root := &types.Issue{
		ID:        "thread-root",
		Title:     "Thread Root",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
		CreatedAt: time.Now(),
	}
	if err := store.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatal(err)
	}

	reply := &types.Issue{
		ID:        "thread-reply",
		Title:     "Reply",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
		CreatedAt: time.Now(),
	}
	if err := store.CreateIssue(ctx, reply, "test"); err != nil {
		t.Fatal(err)
	}

	// Insert a dependency with valid thread_id via raw SQL (replies-to with thread_id)
	db := store.UnderlyingDB()
	_, err := db.ExecContext(ctx,
		"INSERT INTO dependencies (id, issue_id, depends_on_issue_id, type, thread_id, created_at, created_by) VALUES (UUID(), ?, ?, ?, ?, NOW(), ?)",
		"thread-reply", "thread-root", "replies-to", "thread-root", "test")
	if err != nil {
		t.Fatalf("Failed to insert thread dep: %v", err)
	}

	check := checkMailThreadIntegrity(db)

	// On Dolt/MySQL, pragma_table_info is not available, so the check
	// returns StatusOK with "N/A" message. This is expected behavior —
	// the check functions will be updated to use Dolt-compatible queries
	// in later subtasks (bd-o0u.2+).
	if check.Status != StatusOK {
		t.Errorf("Status = %q, want %q: %s", check.Status, StatusOK, check.Message)
	}
}
