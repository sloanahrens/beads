//go:build cgo && integration

// The bd init tests that need a real store: each runs the real init path
// against a Dolt server or drives the bd binary, so they belong to the
// integration tier (see engdocs/TESTING.md, "The Two Tiers"). The helpers
// they share with the unit tier stay in init_test.go and its siblings.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/git"
)

func TestInitCommand(t *testing.T) {
	skipIfNoDolt(t)
	tests := []struct {
		name           string
		prefix         string
		quiet          bool
		wantOutputText string
		wantNoOutput   bool
	}{
		{
			name:           "init with default prefix",
			prefix:         "",
			quiet:          false,
			wantOutputText: "bd initialized successfully",
		},
		{
			name:           "init with custom prefix",
			prefix:         "myproject",
			quiet:          false,
			wantOutputText: "myproject-<hash>",
		},
		{
			name:         "init with quiet flag",
			prefix:       "test",
			quiet:        true,
			wantNoOutput: true,
		},
		{
			name:           "init with prefix ending in hyphen",
			prefix:         "test-",
			quiet:          false,
			wantOutputText: "test-<hash>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global state
			origDBPath := dbPath
			origStore := store
			defer func() {
				if store != nil && store != origStore {
					store.Close()
				}
				store = origStore
				dbPath = origDBPath
			}()
			dbPath = ""
			store = nil

			// Reset Cobra command state
			rootCmd.SetArgs([]string{})
			initCmd.Flags().Set("prefix", "")
			initCmd.Flags().Set("quiet", "false")

			tmpDir := t.TempDir()
			t.Chdir(tmpDir)

			// Capture output
			var buf bytes.Buffer
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() {
				os.Stdout = oldStdout
			}()

			// Build command arguments
			args := []string{"init"}
			if tt.prefix != "" {
				args = append(args, "--prefix", tt.prefix)
			}
			if tt.quiet {
				args = append(args, "--quiet")
			}

			rootCmd.SetArgs(args)

			// Run command
			var err error
			err = rootCmd.Execute()

			// Restore stdout and read output
			w.Close()
			buf.ReadFrom(r)
			os.Stdout = oldStdout
			output := buf.String()

			if err != nil {
				t.Fatalf("init command failed: %v", err)
			}

			// Check output
			if tt.wantNoOutput {
				if output != "" {
					t.Errorf("Expected no output with --quiet, got: %s", output)
				}
			} else if tt.wantOutputText != "" {
				if !strings.Contains(output, tt.wantOutputText) {
					t.Errorf("Expected output to contain %q, got: %s", tt.wantOutputText, output)
				}
			}

			// Verify .beads directory was created
			beadsDir := filepath.Join(tmpDir, ".beads")
			if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
				t.Error(".beads directory was not created")
			}

			// Verify .gitignore was created with proper content
			gitignorePath := filepath.Join(beadsDir, ".gitignore")
			gitignoreContent, err := os.ReadFile(gitignorePath)
			if err != nil {
				t.Errorf(".gitignore file was not created: %v", err)
			} else {
				// Check for essential patterns
				gitignoreStr := string(gitignoreContent)
				expectedPatterns := []string{
					"*.db",
					"*.db?*",
					"*.db-journal",
					"*.db-wal",
					"*.db-shm",
					"bd.sock",
					"dolt/",
				}
				for _, pattern := range expectedPatterns {
					if !strings.Contains(gitignoreStr, pattern) {
						t.Errorf(".gitignore missing expected pattern: %s", pattern)
					}
				}
			}

			// Verify Dolt database directory was created
			doltPath := filepath.Join(beadsDir, "dolt")
			if info, err := os.Stat(doltPath); os.IsNotExist(err) {
				t.Errorf("Dolt database directory was not created at %s", doltPath)
			} else if !info.IsDir() {
				t.Errorf("Expected %s to be a directory", doltPath)
			}

			// Database content verification (prefix, metadata) is skipped here because
			// embedded Dolt's Close() can timeout, leaving file locks held and preventing
			// re-opening the DB in the same process. The init command's own internal logic
			// verifies these writes succeed; prefix/metadata correctness is also covered
			// by dedicated Dolt storage tests.
		})
	}
}

func TestInitAlreadyInitialized(t *testing.T) {
	skipIfNoDolt(t)
	// Reset global state
	origDBPath := dbPath
	defer func() { dbPath = origDBPath }()
	dbPath = ""

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// Initialize once
	rootCmd.SetArgs([]string{"init", "--prefix", "test", "--quiet"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("First init failed: %v", err)
	}

	// Initialize again with same prefix and --force flag (bd-emg: safety guard)
	// Without --force, init should refuse when database already exists
	rootCmd.SetArgs([]string{"init", "--prefix", "test", "--quiet", "--force"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Second init with --force failed: %v", err)
	}

	// Verify database still works
	dbPath := filepath.Join(tmpDir, ".beads", "dolt")
	store, err := openExistingTestDB(t, dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	prefix, err := store.GetConfig(ctx, "issue_prefix")
	if err != nil {
		t.Fatalf("Failed to get prefix after re-init: %v", err)
	}

	if prefix != "test" {
		t.Errorf("Expected prefix 'test', got %q", prefix)
	}
}

// GH#3490: `bd init --init-if-missing` makes init idempotent for scaffold
// scripts. When the workspace is already initialized, init must skip and
// return without error (exit 0), emitting a benign "Skipping init" message,
// instead of aborting via os.Exit(1). The default (no-flag) abort path calls
// os.Exit(1) and therefore cannot be exercised in-process (see the note above
// TestInitAlreadyInitialized); this test covers the new success path.
func TestInitIfMissing(t *testing.T) {
	skipIfNoDolt(t)
	// Reset global state
	origDBPath := dbPath

	// Cobra flag values persist across Execute() calls on the shared command
	// tree (a sibling test may have left --force set, and our own first init
	// sets --quiet). Normalize the flags this test depends on before each run,
	// and restore defaults afterward so we neither inherit nor leak state.
	resetInitFlags := func() {
		_ = initCmd.Flags().Set("force", "false")
		_ = initCmd.Flags().Set("reinit-local", "false")
		_ = initCmd.Flags().Set("init-if-missing", "false")
		_ = initCmd.Flags().Set("quiet", "false")
		_ = initCmd.Flags().Set("prefix", "")
	}
	defer func() {
		dbPath = origDBPath
		resetInitFlags()
	}()
	dbPath = ""
	resetInitFlags()

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// First init creates the workspace.
	rootCmd.SetArgs([]string{"init", "--prefix", "test", "--quiet"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("First init failed: %v", err)
	}

	// Re-running with --init-if-missing must be a benign no-op that exits 0:
	// Execute() returns nil rather than the process aborting via os.Exit(1).
	// Clear --quiet (set by the first run) so the skip message is emitted, and
	// ensure no stale --force bypasses the already-initialized guard.
	resetInitFlags()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	rootCmd.SetArgs([]string{"init", "--prefix", "test", "--init-if-missing"})
	execErr := rootCmd.Execute()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	stderr := buf.String()

	if execErr != nil {
		t.Fatalf("init --init-if-missing on an initialized workspace should succeed, got: %v", execErr)
	}
	if !strings.Contains(stderr, "Skipping init: workspace already initialized") {
		t.Errorf("expected a 'Skipping init: workspace already initialized' message on stderr, got:\n%s", stderr)
	}

	// The skip is a no-op: the existing workspace must remain in place (init
	// returned before touching any data). We assert on the .beads directory
	// rather than a specific backend layout so the test holds for both the
	// embedded and server test modes.
	beadsDir := filepath.Join(tmpDir, ".beads")
	if info, err := os.Stat(beadsDir); err != nil || !info.IsDir() {
		t.Fatalf("expected existing .beads workspace to remain at %s, stat err: %v", beadsDir, err)
	}
}

// TestInitIfMissingMatchingPrefixSkips verifies the mismatch guard does not
// regress the happy path: re-running with the SAME explicit --prefix as the
// existing workspace still skips cleanly (exit 0) rather than aborting.
func TestInitIfMissingMatchingPrefixSkips(t *testing.T) {
	skipIfNoDolt(t)
	origDBPath := dbPath
	resetInitFlags := func() {
		_ = initCmd.Flags().Set("force", "false")
		_ = initCmd.Flags().Set("reinit-local", "false")
		_ = initCmd.Flags().Set("init-if-missing", "false")
		_ = initCmd.Flags().Set("quiet", "false")
		_ = initCmd.Flags().Set("prefix", "")
	}
	defer func() {
		dbPath = origDBPath
		resetInitFlags()
	}()
	dbPath = ""
	resetInitFlags()

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	rootCmd.SetArgs([]string{"init", "--prefix", "test", "--quiet"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("First init failed: %v", err)
	}

	resetInitFlags()
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = oldStderr }()

	// Same prefix as the existing workspace: must skip, not abort.
	rootCmd.SetArgs([]string{"init", "--prefix", "test", "--init-if-missing"})
	execErr := rootCmd.Execute()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	stderr := buf.String()

	if execErr != nil {
		t.Fatalf("init --init-if-missing with matching prefix should succeed, got: %v", execErr)
	}
	if !strings.Contains(stderr, "Skipping init: workspace already initialized") {
		t.Errorf("expected benign skip message, got:\n%s", stderr)
	}
}

// TestInitPromptNonGitRepo verifies prompt is skipped in non-git directories
func TestInitPromptNonGitRepo(t *testing.T) {
	skipIfNoDolt(t)
	// Reset global state
	origDBPath := dbPath
	defer func() { dbPath = origDBPath }()
	dbPath = ""

	// Reset caches so RepoContext picks up new directory
	beads.ResetCaches()
	git.ResetCaches()
	defer func() {
		beads.ResetCaches()
		git.ResetCaches()
	}()

	// Reset Cobra flags that may be set from previous tests
	initCmd.Flags().Set("contributor", "false")
	initCmd.Flags().Set("team", "false")

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	// DON'T initialize git repo

	// Run bd init - should succeed without prompting (no git repo)
	rootCmd.SetArgs([]string{"init", "--prefix", "test", "--quiet"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Init should succeed in non-git directory: %v", err)
	}

	// Verify .beads was created
	beadsDir := filepath.Join(tmpDir, ".beads")
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		t.Error(".beads directory should be created even without git")
	}
}

// TestInitPromptExistingRole verifies behavior when beads.role is already set
func TestInitPromptExistingRole(t *testing.T) {
	skipIfNoDolt(t)
	t.Run("existing role is preserved on reinit with --force", func(t *testing.T) {
		// Reset global state
		origDBPath := dbPath
		defer func() { dbPath = origDBPath }()
		dbPath = ""

		// Reset caches so RepoContext picks up new directory
		beads.ResetCaches()
		git.ResetCaches()
		defer func() {
			beads.ResetCaches()
			git.ResetCaches()
		}()

		// Reset Cobra flags that may be set from previous tests
		initCmd.Flags().Set("contributor", "false")
		initCmd.Flags().Set("team", "false")
		initCmd.Flags().Set("force", "false")

		tmpDir := newGitRepo(t)
		t.Chdir(tmpDir)

		// Set role before init
		if err := setBeadsRole("contributor"); err != nil {
			t.Fatalf("Failed to set beads.role: %v", err)
		}

		// Run bd init (non-interactive, so prompt is skipped)
		rootCmd.SetArgs([]string{"init", "--prefix", "test", "--quiet"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Init failed: %v", err)
		}

		// Verify role is still set
		role, hasRole := getBeadsRole()
		if !hasRole {
			t.Error("Expected beads.role to still be set after init")
		}
		if role != "contributor" {
			t.Errorf("Expected role 'contributor' to be preserved, got %q", role)
		}

		// Reset Cobra flags for reinit
		initCmd.Flags().Set("force", "false")

		// Reinit with --force (non-interactive)
		rootCmd.SetArgs([]string{"init", "--prefix", "test", "--quiet", "--force"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Reinit failed: %v", err)
		}

		// Verify role is still set (not cleared by reinit)
		role, hasRole = getBeadsRole()
		if !hasRole {
			t.Error("Expected beads.role to still be set after reinit")
		}
		if role != "contributor" {
			t.Errorf("Expected role 'contributor' to be preserved after reinit, got %q", role)
		}
	})
}

func TestInitContributorSetsBeadsRoleContributor(t *testing.T) {
	skipIfNoDolt(t)

	origDBPath := dbPath
	defer func() { dbPath = origDBPath }()
	dbPath = ""

	beads.ResetCaches()
	git.ResetCaches()
	defer func() {
		beads.ResetCaches()
		git.ResetCaches()
	}()

	initCmd.Flags().Set("contributor", "false")
	initCmd.Flags().Set("team", "false")
	initCmd.Flags().Set("force", "false")

	tmpDir := newGitRepo(t)
	t.Chdir(tmpDir)

	// Keep test isolated from the real home/planning repo.
	testHome := t.TempDir()
	t.Setenv("HOME", testHome)

	// Configure remotes so contributor wizard doesn't ask the "continue anyway" prompt.
	cmd := exec.Command("git", "remote", "add", "origin", "git@github.com:osamu2001/zmx.git")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add origin remote: %v", err)
	}
	cmd = exec.Command("git", "remote", "add", "upstream", "git@github.com:neurosnap/zmx.git")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to add upstream remote: %v", err)
	}

	// Wizard answers:
	// 1) "Do you want to use a separate planning repo anyway? [Y/n]" -> Enter (default yes)
	// 2) "Planning repo path [press Enter for default]" -> Enter (default ~/.beads-planning)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })
	t.Cleanup(func() { _ = r.Close() })
	_, _ = w.WriteString("\n\n")
	_ = w.Close()

	rootCmd.SetArgs([]string{"init", "--prefix", "test", "--contributor", "--quiet"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init --contributor failed: %v", err)
	}

	role, hasRole := getBeadsRole()
	if !hasRole {
		t.Fatal("expected beads.role to be configured")
	}
	if role != "contributor" {
		t.Fatalf("beads.role = %q, want %q", role, "contributor")
	}
}

// TestInitNonInteractiveAlwaysSetsRole verifies that bd init --non-interactive
// always leaves beads.role set, even when no --role flag is provided (GH#2950).
// This is the safety net for the init flow.
func TestInitNonInteractiveAlwaysSetsRole(t *testing.T) {
	skipIfNoDolt(t)

	origDBPath := dbPath
	defer func() { dbPath = origDBPath }()
	dbPath = ""

	beads.ResetCaches()
	git.ResetCaches()
	defer func() {
		beads.ResetCaches()
		git.ResetCaches()
	}()

	initCmd.Flags().Set("contributor", "false")
	initCmd.Flags().Set("team", "false")
	initCmd.Flags().Set("force", "false")
	initCmd.Flags().Set("role", "")

	tmpDir := newGitRepo(t)
	t.Chdir(tmpDir)

	// Ensure no role is set before init
	exec.Command("git", "config", "--unset", "beads.role").Run() //nolint:errcheck

	rootCmd.SetArgs([]string{"init", "--prefix", "test", "--quiet", "--non-interactive"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init --non-interactive failed: %v", err)
	}

	role, hasRole := getBeadsRole()
	if !hasRole {
		t.Fatal("expected beads.role to be configured after non-interactive init (GH#2950)")
	}
	if role != "maintainer" {
		t.Fatalf("beads.role = %q, want %q (default for non-interactive)", role, "maintainer")
	}
}

// TestInitWithRedirect verifies that bd init creates the database in the redirect target,
// not in the local .beads directory. (GH#bd-0qel)
// TestInitRedirect groups redirect-related init tests.
func TestInitRedirect(t *testing.T) {
	skipIfNoDolt(t)
	resetRedirectState := func(t *testing.T) {
		t.Helper()
		origDBPath := dbPath
		origBeadsDir := os.Getenv("BEADS_DIR")
		t.Cleanup(func() {
			dbPath = origDBPath
			if origBeadsDir != "" {
				os.Setenv("BEADS_DIR", origBeadsDir)
			} else {
				os.Unsetenv("BEADS_DIR")
			}
		})
		dbPath = ""
		os.Unsetenv("BEADS_DIR")
		initCmd.Flags().Set("prefix", "")
		initCmd.Flags().Set("quiet", "false")
		initCmd.Flags().Set("force", "false")
	}

	t.Run("RedirectCreatesDBInTarget", func(t *testing.T) {
		resetRedirectState(t)

		tmpDir := t.TempDir()

		projectDir := filepath.Join(tmpDir, "project")
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			t.Fatal(err)
		}

		localBeadsDir := filepath.Join(projectDir, ".beads")
		if err := os.MkdirAll(localBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		targetBeadsDir := filepath.Join(tmpDir, "canonical", ".beads")
		if err := os.MkdirAll(targetBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		redirectPath := filepath.Join(localBeadsDir, beads.RedirectFileName)
		if err := os.WriteFile(redirectPath, []byte("../canonical/.beads\n"), 0644); err != nil {
			t.Fatal(err)
		}

		t.Chdir(projectDir)

		rootCmd.SetArgs([]string{"init", "--prefix", "redirect-test", "--quiet"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("Init with redirect failed: %v", err)
		}

		targetDBPath := filepath.Join(targetBeadsDir, "dolt")
		if _, err := os.Stat(targetDBPath); os.IsNotExist(err) {
			t.Errorf("Dolt database was NOT created in redirect target: %s", targetDBPath)
		}

		localDBPath := filepath.Join(localBeadsDir, "dolt")
		if _, err := os.Stat(localDBPath); err == nil {
			t.Errorf("Database was incorrectly created in local .beads: %s (should be in redirect target)", localDBPath)
		}

		store, err := openExistingTestDB(t, targetDBPath)
		if err != nil {
			t.Fatalf("Failed to open database in redirect target: %v", err)
		}
		defer store.Close()

		ctx := context.Background()
		prefix, err := store.GetConfig(ctx, "issue_prefix")
		if err != nil {
			t.Fatalf("Failed to get issue prefix from database: %v", err)
		}
		if prefix != "redirect-test" {
			t.Errorf("Expected prefix 'redirect-test', got %q", prefix)
		}
	})

	// Verifies that bd init errors when the redirect target already has a database,
	// preventing accidental overwrites. (GH#bd-0qel)
	t.Run("ErrorWhenTargetHasExistingDB", func(t *testing.T) {
		resetRedirectState(t)

		tmpDir := t.TempDir()

		canonicalDir := filepath.Join(tmpDir, "canonical")
		canonicalBeadsDir := filepath.Join(canonicalDir, ".beads")
		if err := os.MkdirAll(canonicalBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		canonicalDBPath := filepath.Join(canonicalBeadsDir, "beads.db")
		// Create the db file so checkExistingBeadsData detects it
		if err := os.WriteFile(canonicalDBPath, []byte{}, 0644); err != nil {
			t.Fatalf("Failed to create canonical db file: %v", err)
		}

		projectDir := filepath.Join(tmpDir, "project")
		projectBeadsDir := filepath.Join(projectDir, ".beads")
		if err := os.MkdirAll(projectBeadsDir, 0755); err != nil {
			t.Fatal(err)
		}

		redirectPath := filepath.Join(projectBeadsDir, beads.RedirectFileName)
		if err := os.WriteFile(redirectPath, []byte("../canonical/.beads\n"), 0644); err != nil {
			t.Fatal(err)
		}

		// Use os.Chdir since checkExistingBeadsData reads CWD directly
		origWd, _ := os.Getwd()
		if err := os.Chdir(projectDir); err != nil {
			t.Fatal(err)
		}
		defer os.Chdir(origWd)

		err := checkExistingBeadsData("new-prefix")
		if err == nil {
			t.Fatal("Expected checkExistingBeadsData to return error when redirect target already has database")
		}

		errorMsg := err.Error()
		if !strings.Contains(errorMsg, "redirect target already has database") {
			t.Errorf("Expected error about redirect target having database, got: %s", errorMsg)
		}

		// Verify the canonical DB file still exists (wasn't deleted/overwritten)
		if _, statErr := os.Stat(canonicalDBPath); os.IsNotExist(statErr) {
			t.Error("Canonical database file should still exist after error")
		}
	})
}

func TestInitServerModeWritesDoltCompatibilityMarker(t *testing.T) {
	skipIfNoDolt(t)
	saveAndRestoreGlobals(t)
	ensureCleanGlobalState(t)
	dbPath = ""
	store = nil

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	beadsDir := filepath.Join(tmpDir, ".beads")
	doltDir := filepath.Join(beadsDir, "dolt")
	if err := os.MkdirAll(beadsDir, 0o700); err != nil {
		t.Fatalf("creating beads dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(doltDir, ".dolt"), 0o750); err != nil {
		t.Fatalf("creating simulated server data dir: %v", err)
	}

	database := uniqueTestDBName(t)
	t.Cleanup(func() {
		dropTestDatabase(database, testDoltServerPort)
	})

	rootCmd.SetArgs([]string{
		"init",
		"--server",
		"--external",
		"--server-host", "127.0.0.1",
		"--server-port", fmt.Sprintf("%d", testDoltServerPort),
		"--database", database,
		"--prefix", "marker",
		"--quiet",
		"--skip-hooks",
		"--skip-agents",
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("server init failed: %v", err)
	}

	markerPath := filepath.Join(doltDir, ".bd-dolt-ok")
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected server init to write %s: %v", markerPath, err)
	}
}

func TestInitServerModeWarnsOnMarkerFailureInQuietMode(t *testing.T) {
	skipIfNoDolt(t)
	saveAndRestoreGlobals(t)
	ensureCleanGlobalState(t)
	dbPath = ""
	store = nil

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	beadsDir := filepath.Join(tmpDir, ".beads")
	doltDir := filepath.Join(beadsDir, "dolt")
	if err := os.MkdirAll(doltDir, 0o700); err != nil {
		t.Fatalf("creating dolt dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(doltDir, ".dolt"), []byte("not a dir"), 0o600); err != nil {
		t.Fatalf("creating invalid dot-dolt marker: %v", err)
	}

	database := uniqueTestDBName(t)
	t.Cleanup(func() {
		dropTestDatabase(database, testDoltServerPort)
	})

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	rootCmd.SetArgs([]string{
		"init",
		"--server",
		"--external",
		"--server-host", "127.0.0.1",
		"--server-port", fmt.Sprintf("%d", testDoltServerPort),
		"--database", database,
		"--prefix", "marker",
		"--quiet",
		"--skip-hooks",
		"--skip-agents",
	})

	if err := rootCmd.Execute(); err != nil {
		w.Close()
		t.Fatalf("server init failed: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	var stderr bytes.Buffer
	if _, err := stderr.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	if !strings.Contains(stderr.String(), "Warning: failed to write Dolt compatibility marker") {
		t.Fatalf("expected marker warning in stderr, got:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), doltDir) {
		t.Fatalf("expected marker warning to include dolt dir %s, got:\n%s", doltDir, stderr.String())
	}
}
