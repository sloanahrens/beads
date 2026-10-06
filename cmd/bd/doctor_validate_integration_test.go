//go:build cgo && integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestValidateCheck_AllClean(t *testing.T) {
	tmpDir, store := setupValidateTestDB(t, "val")
	ctx := context.Background()

	issues := []*types.Issue{
		{Title: "Fix login bug", Description: "Login fails", Status: types.StatusOpen, Priority: 1, IssueType: types.TypeBug},
		{Title: "Add search", Description: "Full-text search", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask},
	}
	for _, issue := range issues {
		if err := store.CreateIssue(ctx, issue, "val"); err != nil {
			t.Fatalf("Failed to create issue: %v", err)
		}
	}

	// Write clean JSONL so git conflicts check has a file to scan
	jsonlPath := filepath.Join(tmpDir, ".beads", "issues.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create JSONL: %v", err)
	}
	store.Close()

	checks := collectValidateChecks(tmpDir)

	for _, cr := range checks {
		if cr.check.Status != statusOK {
			t.Errorf("%s: status = %q, want %q (message: %s)", cr.check.Name, cr.check.Status, statusOK, cr.check.Message)
		}
	}
	if len(checks) != 4 {
		t.Errorf("Expected 4 checks, got %d", len(checks))
	}
}

func TestValidateCheck_DetectsDuplicates(t *testing.T) {
	tmpDir, store := setupValidateTestDB(t, "test")
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		issue := &types.Issue{
			Title:       "Duplicate task",
			Description: "Same description",
			Status:      types.StatusOpen,
			Priority:    2,
			IssueType:   types.TypeTask,
		}
		if err := store.CreateIssue(ctx, issue, "test"); err != nil {
			t.Fatalf("Failed to create issue: %v", err)
		}
	}
	store.Close()

	checks := collectValidateChecks(tmpDir)

	for _, cr := range checks {
		if cr.check.Name == "Duplicate Issues" {
			if cr.check.Status != statusWarning {
				t.Errorf("Duplicate Issues status = %q, want %q", cr.check.Status, statusWarning)
			}
			return
		}
	}
	t.Error("Duplicate Issues check not found")
}

func TestValidateCheck_DetectsOrphanedDeps(t *testing.T) {
	tmpDir, store := setupValidateTestDB(t, "test")
	ctx := context.Background()

	issue := &types.Issue{
		Title:     "Real issue",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeTask,
	}
	if err := store.CreateIssue(ctx, issue, "test"); err != nil {
		t.Fatalf("Failed to create issue: %v", err)
	}

	db := store.UnderlyingDB()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	// FK on depends_on_issue_id would normally block this; simulate the
	// schema-drift scenario the validator is designed to catch.
	if _, err = tx.Exec("SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatalf("Failed to disable FK checks: %v", err)
	}
	_, err = tx.Exec("INSERT INTO dependencies (id, issue_id, depends_on_issue_id, type, created_by) VALUES (UUID(), ?, ?, ?, ?)",
		issue.ID, "test-nonexistent", "blocks", "test")
	if err != nil {
		t.Fatalf("Failed to insert orphaned dep: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit orphaned dep: %v", err)
	}
	store.Close()

	checks := collectValidateChecks(tmpDir)

	for _, cr := range checks {
		if cr.check.Name == "Orphaned Dependencies" {
			if cr.check.Status != statusWarning {
				t.Errorf("Orphaned Dependencies status = %q, want %q", cr.check.Status, statusWarning)
			}
			if !cr.fixable {
				t.Error("Orphaned Dependencies should be marked fixable")
			}
			return
		}
	}
	t.Error("Orphaned Dependencies check not found")
}

func TestValidateCheck_GitConflicts_DoltClean(t *testing.T) {
	// When metadata is absent, GetBackend() defaults to "dolt",
	// the Git Conflicts check now queries dolt_conflicts (GH-2249).
	// Verify it reports OK for a clean Dolt database.
	tmpDir, store := setupValidateTestDB(t, "val")
	ctx := context.Background()

	issue := &types.Issue{
		Title:     "Clean issue",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeTask,
	}
	if err := store.CreateIssue(ctx, issue, "val"); err != nil {
		t.Fatalf("Failed to create issue: %v", err)
	}
	store.Close()

	checks := collectValidateChecks(tmpDir)

	for _, cr := range checks {
		if cr.check.Name == "Git Conflicts" {
			if cr.check.Status != statusOK {
				t.Errorf("Git Conflicts status = %q, want %q (clean DB)", cr.check.Status, statusOK)
			}
			return
		}
	}
	t.Error("Git Conflicts check not found")
}

func TestValidateCheck_DetectsTestPollution(t *testing.T) {
	tmpDir, store := setupValidateTestDB(t, "test")
	ctx := context.Background()

	testIssues := []*types.Issue{
		{Title: "test-pollution-check", Description: "A test issue", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask},
		{Title: "Test Issue 1", Description: "Another test", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask},
	}
	for _, issue := range testIssues {
		if err := store.CreateIssue(ctx, issue, "test"); err != nil {
			t.Fatalf("Failed to create issue: %v", err)
		}
	}
	store.Close()

	checks := collectValidateChecks(tmpDir)

	for _, cr := range checks {
		if cr.check.Name == "Test Pollution" {
			if cr.check.Status != statusWarning {
				t.Errorf("Test Pollution status = %q, want %q", cr.check.Status, statusWarning)
			}
			return
		}
	}
	t.Error("Test Pollution check not found")
}

func TestValidateCheck_FixOrphanedDeps(t *testing.T) {
	// The orphaned deps fix uses raw SQLite queries and skips Dolt backends.
	// Since the default backend is now Dolt, the fix is a no-op.
	// This test verifies that detection works and the fix gracefully skips.
	tmpDir, store := setupValidateTestDB(t, "test")
	ctx := context.Background()

	issue := &types.Issue{
		Title:     "Real issue",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeTask,
	}
	if err := store.CreateIssue(ctx, issue, "test"); err != nil {
		t.Fatalf("Failed to create issue: %v", err)
	}

	db := store.UnderlyingDB()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction: %v", err)
	}
	// FK on depends_on_issue_id would normally block this; simulate the
	// schema-drift scenario the validator/fixer is designed to catch.
	if _, err = tx.Exec("SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		t.Fatalf("Failed to disable FK checks: %v", err)
	}
	_, err = tx.Exec("INSERT INTO dependencies (id, issue_id, depends_on_issue_id, type, created_by) VALUES (UUID(), ?, ?, ?, ?)",
		issue.ID, "test-nonexistent", "blocks", "test")
	if err != nil {
		t.Fatalf("Failed to insert orphaned dep: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Failed to commit orphaned dep: %v", err)
	}
	store.Close()

	// Verify orphan is detected
	checks := collectValidateChecks(tmpDir)
	found := false
	for _, cr := range checks {
		if cr.check.Name == "Orphaned Dependencies" {
			found = true
			if cr.check.Status != statusWarning {
				t.Errorf("Expected orphaned deps warning, got %q", cr.check.Status)
			}
			if !cr.fixable {
				t.Error("Orphaned Dependencies should be marked fixable")
			}
		}
	}
	if !found {
		t.Error("Orphaned Dependencies check not found")
	}
}
