//go:build cgo && integration

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/git"
	"github.com/steveyegge/beads/internal/types"
)

func TestIssueIDCompletion(t *testing.T) {
	// Save original store and restore after test
	originalStore := store
	originalRootCtx := rootCtx
	defer func() {
		store = originalStore
		rootCtx = originalRootCtx
	}()

	// Set up test context
	ctx := context.Background()
	rootCtx = ctx

	// Create dolt store for testing
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, "test.db")
	testStore := newTestStoreWithPrefix(t, testDB, "bd")
	store = testStore

	// Create test issues
	now := time.Now()
	testIssues := []*types.Issue{
		{
			ID:        "bd-abc1",
			Title:     "Test issue 1",
			Status:    types.StatusOpen,
			Priority:  1,
			IssueType: types.TypeTask,
		},
		{
			ID:        "bd-abc2",
			Title:     "Test issue 2",
			Status:    types.StatusInProgress,
			Priority:  2,
			IssueType: types.TypeBug,
		},
		{
			ID:        "bd-xyz1",
			Title:     "Another test issue",
			Status:    types.StatusOpen,
			Priority:  1,
			IssueType: types.TypeFeature,
		},
		{
			ID:        "bd-xyz2",
			Title:     "Yet another test",
			Status:    types.StatusClosed,
			Priority:  3,
			IssueType: types.TypeTask,
			ClosedAt:  &now,
		},
	}

	for _, issue := range testIssues {
		if err := testStore.CreateIssue(ctx, issue, "test"); err != nil {
			t.Fatalf("Failed to create test issue: %v", err)
		}
	}

	tests := []struct {
		name             string
		toComplete       string
		expectedCount    int
		shouldContain    []string
		shouldNotContain []string
	}{
		{
			name:          "Empty prefix returns all issues",
			toComplete:    "",
			expectedCount: 4,
			shouldContain: []string{"bd-abc1", "bd-abc2", "bd-xyz1", "bd-xyz2"},
		},
		{
			name:             "Prefix 'bd-a' returns matching issues",
			toComplete:       "bd-a",
			expectedCount:    2,
			shouldContain:    []string{"bd-abc1", "bd-abc2"},
			shouldNotContain: []string{"bd-xyz1", "bd-xyz2"},
		},
		{
			name:             "Prefix 'bd-abc' returns matching issues",
			toComplete:       "bd-abc",
			expectedCount:    2,
			shouldContain:    []string{"bd-abc1", "bd-abc2"},
			shouldNotContain: []string{"bd-xyz1", "bd-xyz2"},
		},
		{
			name:             "Prefix 'bd-abc1' returns exact match",
			toComplete:       "bd-abc1",
			expectedCount:    1,
			shouldContain:    []string{"bd-abc1"},
			shouldNotContain: []string{"bd-abc2", "bd-xyz1", "bd-xyz2"},
		},
		{
			name:             "Prefix 'bd-xyz' returns matching issues",
			toComplete:       "bd-xyz",
			expectedCount:    2,
			shouldContain:    []string{"bd-xyz1", "bd-xyz2"},
			shouldNotContain: []string{"bd-abc1", "bd-abc2"},
		},
		{
			name:             "Non-matching prefix returns empty",
			toComplete:       "bd-zzz",
			expectedCount:    0,
			shouldContain:    []string{},
			shouldNotContain: []string{"bd-abc1", "bd-abc2", "bd-xyz1", "bd-xyz2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a dummy command (not actually used by the function)
			cmd := &cobra.Command{}
			args := []string{}

			// Call the completion function
			completions, directive := issueIDCompletion(cmd, args, tt.toComplete)

			// Check directive
			if directive != cobra.ShellCompDirectiveNoFileComp {
				t.Errorf("Expected directive NoFileComp (4), got %d", directive)
			}

			// Check count
			if len(completions) != tt.expectedCount {
				t.Errorf("Expected %d completions, got %d", tt.expectedCount, len(completions))
			}

			// Check that expected IDs are present
			for _, expectedID := range tt.shouldContain {
				found := false
				for _, completion := range completions {
					// Completion format is "ID\tTitle"
					if len(completion) > 0 && completion[:len(expectedID)] == expectedID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected completion to contain '%s', but it was not found", expectedID)
				}
			}

			// Check that unexpected IDs are NOT present
			for _, unexpectedID := range tt.shouldNotContain {
				for _, completion := range completions {
					if len(completion) > 0 && completion[:len(unexpectedID)] == unexpectedID {
						t.Errorf("Did not expect completion to contain '%s', but it was found", unexpectedID)
					}
				}
			}

			// Verify format: each completion should be "ID\tTitle"
			for _, completion := range completions {
				if len(completion) == 0 {
					t.Errorf("Got empty completion string")
					continue
				}
				// Check that it contains a tab character
				foundTab := false
				for _, c := range completion {
					if c == '\t' {
						foundTab = true
						break
					}
				}
				if !foundTab {
					t.Errorf("Completion '%s' doesn't contain tab separator", completion)
				}
			}
		})
	}
}

func TestIssueIDCompletion_UsesWorktreeFallbackWhenStoreNil(t *testing.T) {
	originalStore := store
	originalDBPath := dbPath
	originalRootCtx := rootCtx
	defer func() {
		store = originalStore
		dbPath = originalDBPath
		rootCtx = originalRootCtx
	}()

	ctx := context.Background()
	rootCtx = ctx

	tmpDir := t.TempDir()
	mainRepoDir := filepath.Join(tmpDir, "main-repo")
	if err := os.MkdirAll(mainRepoDir, 0o755); err != nil {
		t.Fatalf("mkdir main repo: %v", err)
	}

	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run(mainRepoDir, "init")
	run(mainRepoDir, "config", "user.email", "test@example.com")
	run(mainRepoDir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(mainRepoDir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run(mainRepoDir, "add", "README.md")
	run(mainRepoDir, "commit", "-m", "Initial commit")

	worktreeDir := filepath.Join(tmpDir, "worktree")
	cmd := exec.Command("git", "worktree", "add", worktreeDir, "HEAD")
	cmd.Dir = mainRepoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add failed: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		cleanupCmd := exec.Command("git", "worktree", "remove", "--force", worktreeDir)
		cleanupCmd.Dir = mainRepoDir
		_ = cleanupCmd.Run()
	})

	testDB := filepath.Join(mainRepoDir, ".beads", "beads.db")
	testStore := newTestStoreWithPrefix(t, testDB, "wt")
	if err := testStore.CreateIssue(ctx, &types.Issue{
		ID:        "wt-abc1",
		Title:     "Worktree completion target",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeTask,
	}, "test"); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	store = nil
	dbPath = ""

	t.Chdir(worktreeDir)
	beads.ResetCaches()
	git.ResetCaches()
	t.Cleanup(func() {
		beads.ResetCaches()
		git.ResetCaches()
	})

	completions, directive := issueIDCompletion(&cobra.Command{}, nil, "wt-a")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("directive = %d, want %d", directive, cobra.ShellCompDirectiveNoFileComp)
	}
	if len(completions) != 1 {
		t.Fatalf("len(completions) = %d, want 1 (%v)", len(completions), completions)
	}
	if len(completions[0]) < len("wt-abc1") || completions[0][:len("wt-abc1")] != "wt-abc1" {
		t.Fatalf("completion = %q, want prefix %q", completions[0], "wt-abc1")
	}
}

func TestIssueIDCompletion_EmptyDatabase(t *testing.T) {
	// Save original store and restore after test
	originalStore := store
	originalRootCtx := rootCtx
	defer func() {
		store = originalStore
		rootCtx = originalRootCtx
	}()

	// Set up test context
	ctx := context.Background()
	rootCtx = ctx

	// Create empty dolt store
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, "test.db")
	testStore := newTestStoreWithPrefix(t, testDB, "bd")
	store = testStore

	cmd := &cobra.Command{}
	args := []string{}

	completions, directive := issueIDCompletion(cmd, args, "")

	// Should return empty completions when database is empty
	if len(completions) != 0 {
		t.Errorf("Expected 0 completions when database is empty, got %d", len(completions))
	}

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("Expected directive NoFileComp (4), got %d", directive)
	}
}
