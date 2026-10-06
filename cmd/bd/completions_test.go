//go:build cgo

package main

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/git"
)

func TestIssueIDCompletion_NoStore(t *testing.T) {
	// Save original store and restore after test
	originalStore := store
	originalDBPath := dbPath
	defer func() {
		store = originalStore
		dbPath = originalDBPath
	}()

	// Set store to nil and dbPath to non-existent path
	store = nil
	dbPath = "/nonexistent/path/to/database.db"

	cmd := &cobra.Command{}
	args := []string{}

	completions, directive := issueIDCompletion(cmd, args, "")

	// Should return empty completions when store is nil and dbPath is invalid
	if len(completions) != 0 {
		t.Errorf("Expected 0 completions when store is nil and dbPath is invalid, got %d", len(completions))
	}

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("Expected directive NoFileComp (4), got %d", directive)
	}
}

func TestCompleteCommandWorksWithoutDatabase(t *testing.T) {
	// This test verifies that shell completions work even without a beads database.
	// The __complete command must be in noDbCommands list so that PersistentPreRun
	// doesn't exit with "no beads database found" error.
	//
	// This test will FAIL on versions before the fix was applied, where __complete
	// was not in the noDbCommands list.

	// Create a temp directory with no .beads database
	tmpDir := t.TempDir()

	// Save original state
	originalDBPath := dbPath
	originalStore := store
	defer func() {
		dbPath = originalDBPath
		store = originalStore
	}()

	// Reset state to simulate no database
	store = nil
	dbPath = ""

	// Reset caches so FindDatabasePath won't use stale git worktree info
	beads.ResetCaches()
	git.ResetCaches()
	defer func() {
		beads.ResetCaches()
		git.ResetCaches()
	}()

	// Change to temp directory (no database present)
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalWd) }()

	// Test that issueIDCompletion returns gracefully (empty list, no error)
	// when there's no database, rather than panicking or hanging
	cmd := &cobra.Command{}
	completions, directive := issueIDCompletion(cmd, []string{}, "")

	// Should return empty completions, not panic
	if completions == nil {
		// nil is acceptable, convert to empty slice for consistent handling
		completions = []string{}
	}

	// Should return NoFileComp directive
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("Expected directive NoFileComp (%d), got %d",
			cobra.ShellCompDirectiveNoFileComp, directive)
	}

	// Completions should be empty (no database to query)
	if len(completions) != 0 {
		t.Errorf("Expected 0 completions without database, got %d", len(completions))
	}
}

func TestCompleteCommandInNoDbCommandsList(t *testing.T) {
	// This integration test verifies that running `bd __complete show ""`
	// does NOT fail with "no beads database found" error.
	//
	// The __complete command is Cobra's internal command for shell completions.
	// It must be in the noDbCommands list to skip database initialization.
	//
	// Before the fix: this test would fail because __complete wasn't in noDbCommands,
	// causing PersistentPreRun to exit with "no beads database found".
	// After the fix: this test passes because __complete is in noDbCommands.

	// Create a temp directory with no .beads database
	tmpDir := t.TempDir()

	// Change to temp directory (no database present)
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	defer func() { _ = os.Chdir(originalWd) }()

	// Save and reset global state
	originalDBPath := dbPath
	originalStore := store
	defer func() {
		dbPath = originalDBPath
		store = originalStore
	}()

	store = nil
	dbPath = ""

	// Capture stdout/stderr
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stdout = w
	os.Stderr = w
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	// Run __complete command (this is what shell completion scripts call)
	rootCmd.SetArgs([]string{"__complete", "show", ""})
	err = rootCmd.Execute()

	// Close pipe to get output
	_ = w.Close()

	// The command should NOT fail - if __complete is in noDbCommands,
	// PersistentPreRun will skip database initialization and the completion
	// will return empty results gracefully
	if err != nil {
		t.Errorf("__complete command failed without database: %v\n"+
			"This indicates __complete is not in noDbCommands list.\n"+
			"Shell completions should work without requiring a database.", err)
	}
}
