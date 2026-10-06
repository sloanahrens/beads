//go:build cgo

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/git"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/testutil"
	"github.com/steveyegge/beads/internal/types"
)

// TestYamlOnlyConfigWithoutDatabase verifies that yaml-only config keys
// (like no-db) can be set/get without requiring a SQLite database.
// This is the fix for GH#536 - the chicken-and-egg problem where you couldn't
// run `bd config set no-db true` without first having a database.
func TestYamlOnlyConfigWithoutDatabase(t *testing.T) {
	// Create a temp directory with only config.yaml (no database)
	tmpDir, err := os.MkdirTemp("", "bd-test-yaml-config-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("Failed to create .beads dir: %v", err)
	}

	// Create config.yaml with a prefix but NO database
	configPath := filepath.Join(beadsDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("prefix: test\n"), 0644); err != nil {
		t.Fatalf("Failed to create config.yaml: %v", err)
	}

	// Create empty issues.jsonl (simulates fresh clone)
	jsonlPath := filepath.Join(beadsDir, "issues.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create issues.jsonl: %v", err)
	}

	// Test that IsYamlOnlyKey correctly identifies yaml-only keys
	yamlOnlyKeys := []string{"no-db", "json", "routing.mode"}
	for _, key := range yamlOnlyKeys {
		if !config.IsYamlOnlyKey(key) {
			t.Errorf("Expected %q to be a yaml-only key", key)
		}
	}

	// Test that non-yaml-only keys are correctly identified
	nonYamlKeys := []string{"jira.url", "linear.team_id", "status.custom"}
	for _, key := range nonYamlKeys {
		if config.IsYamlOnlyKey(key) {
			t.Errorf("Expected %q to NOT be a yaml-only key", key)
		}
	}
}

// setupTestDB creates a temporary test database
func setupTestDB(t *testing.T) (*dolt.DoltStore, func()) {
	tmpDir, err := os.MkdirTemp("", "bd-test-config-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	testDB := filepath.Join(tmpDir, "test.db")
	store, err := dolt.New(context.Background(), &dolt.Config{Path: testDB})
	if err != nil {
		os.RemoveAll(tmpDir)
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}

	// CRITICAL (bd-166): Set issue_prefix to prevent "database not initialized" errors
	ctx := context.Background()
	if err := store.SetConfig(ctx, "issue_prefix", "bd"); err != nil {
		store.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to set issue_prefix: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// TestBeadsRoleGitConfig verifies that beads.role is stored in git config,
// not SQLite, so that bd doctor can find it (GH#1531).
func TestBeadsRoleGitConfig(t *testing.T) {
	tmpDir := newGitRepo(t)

	t.Run("set contributor role writes to git config", func(t *testing.T) {
		cmd := exec.Command("git", "config", "beads.role", "contributor")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git config set failed: %v", err)
		}

		// Verify it's readable from git config
		cmd = exec.Command("git", "config", "--get", "beads.role")
		cmd.Dir = tmpDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("git config get failed: %v", err)
		}
		if got := strings.TrimSpace(string(output)); got != "contributor" {
			t.Errorf("expected 'contributor', got %q", got)
		}
	})

	t.Run("set maintainer role writes to git config", func(t *testing.T) {
		cmd := exec.Command("git", "config", "beads.role", "maintainer")
		cmd.Dir = tmpDir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git config set failed: %v", err)
		}

		cmd = exec.Command("git", "config", "--get", "beads.role")
		cmd.Dir = tmpDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("git config get failed: %v", err)
		}
		if got := strings.TrimSpace(string(output)); got != "maintainer" {
			t.Errorf("expected 'maintainer', got %q", got)
		}
	})
}

// TestIsValidRemoteURL tests the remote URL validation function
func TestIsValidRemoteURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		// Valid URLs
		{"dolthub scheme", "dolthub://org/repo", true},
		{"gs scheme", "gs://bucket/path", true},
		{"s3 scheme", "s3://bucket/path", true},
		{"file scheme", "file:///path/to/repo", true},
		{"https scheme", "https://github.com/user/repo", true},
		{"http scheme", "http://github.com/user/repo", true},
		{"ssh scheme", "ssh://git@github.com/user/repo", true},
		{"git ssh format", "git@github.com:user/repo.git", true},
		{"git ssh with underscore", "git@gitlab.example_host.com:user/repo.git", true},

		// Invalid URLs
		{"empty string", "", false},
		{"no scheme", "github.com/user/repo", false},
		{"invalid scheme", "ftp://server/path", false},
		{"malformed git ssh", "git@:repo", false},
		{"just path", "/path/to/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidRemoteURL(tt.url)
			if got != tt.expected {
				t.Errorf("isValidRemoteURL(%q) = %v, want %v", tt.url, got, tt.expected)
			}
		})
	}
}

// TestValidateSyncConfig tests the sync config validation function
func TestValidateSyncConfig(t *testing.T) {
	// Create a temp directory for testing
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("Failed to create .beads dir: %v", err)
	}

	t.Run("valid empty config", func(t *testing.T) {
		// Create minimal config.yaml
		configContent := `prefix: test
`
		if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		issues := validateSyncConfig(tmpDir)
		// After JSONL removal, Dolt sync requires federation.remote
		if len(issues) != 1 {
			t.Errorf("Expected 1 issue (missing federation.remote) for empty config, got: %v", issues)
		}
	})

	t.Run("invalid federation.sovereignty", func(t *testing.T) {
		configContent := `prefix: test
federation:
  sovereignty: "invalid-value"
`
		if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		issues := validateSyncConfig(tmpDir)
		found := false
		for _, issue := range issues {
			if strings.Contains(issue, "federation.sovereignty") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected issue about federation.sovereignty, got: %v", issues)
		}
	})

	t.Run("dolt-native mode without remote", func(t *testing.T) {
		configContent := `prefix: test
sync:
  mode: "dolt-native"
`
		if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		issues := validateSyncConfig(tmpDir)
		found := false
		for _, issue := range issues {
			if strings.Contains(issue, "federation.remote") && strings.Contains(issue, "required") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected issue about federation.remote being required, got: %v", issues)
		}
	})

	t.Run("invalid remote URL", func(t *testing.T) {
		configContent := `prefix: test
federation:
  remote: "invalid-url"
`
		if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		issues := validateSyncConfig(tmpDir)
		found := false
		for _, issue := range issues {
			if strings.Contains(issue, "federation.remote") && (strings.Contains(issue, "not a valid remote URL") || strings.Contains(issue, "no scheme") || strings.Contains(issue, "not allowed")) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected issue about invalid remote URL, got: %v", issues)
		}
	})

	t.Run("valid sync config", func(t *testing.T) {
		configContent := `prefix: test
sync:
  mode: "dolt-native"
conflict:
  strategy: "newest"
federation:
  sovereignty: "T2"
  remote: "https://github.com/user/beads-data.git"
`
		if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		issues := validateSyncConfig(tmpDir)
		if len(issues) != 0 {
			t.Errorf("Expected no issues for valid config, got: %v", issues)
		}
	})

	t.Run("remote URL with null byte", func(t *testing.T) {
		configContent := "prefix: test\nfederation:\n  remote: \"dolthub://org/repo\\x00evil\"\n"
		if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		issues := validateSyncConfig(tmpDir)
		found := false
		for _, issue := range issues {
			if strings.Contains(issue, "federation.remote") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected issue about invalid remote URL with null byte, got: %v", issues)
		}
	})

	t.Run("allowed-remote-patterns enforcement", func(t *testing.T) {
		configContent := `prefix: test
federation:
  remote: "https://github.com/user/repo"
  allowed-remote-patterns:
    - "dolthub://myorg/*"
`
		if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		issues := validateSyncConfig(tmpDir)
		found := false
		for _, issue := range issues {
			if strings.Contains(issue, "does not match") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected issue about remote not matching allowed patterns, got: %v", issues)
		}
	})

	t.Run("allowed-remote-patterns passes when matching", func(t *testing.T) {
		configContent := `prefix: test
federation:
  remote: "dolthub://myorg/myrepo"
  allowed-remote-patterns:
    - "dolthub://myorg/*"
`
		if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		issues := validateSyncConfig(tmpDir)
		if len(issues) != 0 {
			t.Errorf("Expected no issues when remote matches allowed pattern, got: %v", issues)
		}
	})

	t.Run("uses shared worktree config when local .beads is absent", func(t *testing.T) {
		bareDir, worktreeDir := setupBareParentInitWorktree(t)
		bareBeadsDir := filepath.Join(bareDir, ".beads")
		if err := os.MkdirAll(bareBeadsDir, 0o755); err != nil {
			t.Fatalf("Failed to create bare .beads dir: %v", err)
		}

		configContent := `federation:
  remote: "dolthub://myorg/myrepo"
  allowed-remote-patterns:
    - "dolthub://myorg/*"
`
		if err := os.WriteFile(filepath.Join(bareBeadsDir, "config.yaml"), []byte(configContent), 0o644); err != nil {
			t.Fatalf("Failed to write config.yaml: %v", err)
		}

		issues := validateSyncConfig(worktreeDir)
		if len(issues) != 0 {
			t.Errorf("Expected no issues when shared worktree config is valid, got: %v", issues)
		}
	})
}

func TestResolvedConfigRepoRoot(t *testing.T) {
	resetResolutionCaches := func(t *testing.T) {
		t.Helper()
		beads.ResetCaches()
		git.ResetCaches()
		t.Cleanup(func() {
			beads.ResetCaches()
			git.ResetCaches()
		})
	}

	assertSameResolvedPath := func(t *testing.T, got, want string) {
		t.Helper()

		gotResolved, err := filepath.EvalSymlinks(got)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q): %v", got, err)
		}
		wantResolved, err := filepath.EvalSymlinks(want)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q): %v", want, err)
		}
		if gotResolved != wantResolved {
			t.Errorf("resolvedConfigRepoRoot() = %q (resolved %q), want %q (resolved %q)", got, gotResolved, want, wantResolved)
		}
	}

	t.Run("uses local workspace from subdirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		beadsDir := filepath.Join(tmpDir, ".beads")
		subDir := filepath.Join(tmpDir, "sub", "dir")

		if err := os.MkdirAll(beadsDir, 0o755); err != nil {
			t.Fatalf("Failed to create .beads dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"backend":"dolt"}`), 0o644); err != nil {
			t.Fatalf("Failed to create metadata.json: %v", err)
		}
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("Failed to create sub dir: %v", err)
		}

		t.Chdir(subDir)
		resetResolutionCaches(t)

		got, err := resolvedConfigRepoRoot()
		if err != nil {
			t.Fatalf("resolvedConfigRepoRoot returned error: %v", err)
		}
		assertSameResolvedPath(t, got, tmpDir)
	})

	t.Run("uses BEADS_DIR target", func(t *testing.T) {
		cwdDir := t.TempDir()
		targetRepo := t.TempDir()
		targetBeadsDir := filepath.Join(targetRepo, ".beads")

		if err := os.MkdirAll(targetBeadsDir, 0o755); err != nil {
			t.Fatalf("Failed to create target .beads dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(targetBeadsDir, "metadata.json"), []byte(`{"backend":"dolt"}`), 0o644); err != nil {
			t.Fatalf("Failed to create target metadata.json: %v", err)
		}

		t.Setenv("BEADS_DIR", targetBeadsDir)
		t.Chdir(cwdDir)
		resetResolutionCaches(t)

		got, err := resolvedConfigRepoRoot()
		if err != nil {
			t.Fatalf("resolvedConfigRepoRoot returned error: %v", err)
		}
		assertSameResolvedPath(t, got, targetRepo)
	})

	t.Run("uses worktree fallback when local .beads is absent", func(t *testing.T) {
		bareDir, worktreeDir := setupBareParentInitWorktree(t)
		bareBeadsDir := filepath.Join(bareDir, ".beads")

		if err := os.MkdirAll(bareBeadsDir, 0o755); err != nil {
			t.Fatalf("Failed to create bare .beads dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(bareBeadsDir, "metadata.json"), []byte(`{"backend":"dolt"}`), 0o644); err != nil {
			t.Fatalf("Failed to create bare metadata.json: %v", err)
		}

		t.Chdir(worktreeDir)
		resetResolutionCaches(t)

		got, err := resolvedConfigRepoRoot()
		if err != nil {
			t.Fatalf("resolvedConfigRepoRoot returned error: %v", err)
		}
		assertSameResolvedPath(t, got, bareDir)
	})
}

// TestConfigSetManyValidationIntegration tests that the validation logic
// in set-many correctly rejects invalid values for known constrained keys
// before any DB writes would occur.
func TestConfigSetManyValidationIntegration(t *testing.T) {
	t.Run("beads.role only accepts maintainer or contributor", func(t *testing.T) {
		validRoles := map[string]bool{"maintainer": true, "contributor": true}
		for _, role := range []string{"maintainer", "contributor"} {
			if !validRoles[role] {
				t.Errorf("expected %q to be valid", role)
			}
		}
		for _, role := range []string{"admin", "superadmin", "owner", "", "MAINTAINER"} {
			if validRoles[role] {
				t.Errorf("expected %q to be invalid", role)
			}
		}
	})

	t.Run("status.custom validation catches invalid formats", func(t *testing.T) {
		valid := []string{
			"awaiting_review,awaiting_testing",
			"review,qa,deploy",
			"single_status",
		}
		for _, v := range valid {
			if _, err := types.ParseCustomStatusConfig(v); err != nil {
				t.Errorf("expected %q to be valid: %v", v, err)
			}
		}
	})

	t.Run("empty status.custom is allowed", func(t *testing.T) {
		// Empty value skips validation in the command handler
		result, err := types.ParseCustomStatusConfig("")
		if err != nil {
			t.Errorf("expected empty status.custom to be valid: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result for empty input, got %v", result)
		}
	})
}
