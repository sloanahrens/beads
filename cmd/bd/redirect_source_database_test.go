package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/git"
)

// setupRedirectWorkspaces builds the two-workspace fixture be-qm8.2.2 needs:
// a cwd git repo whose .beads/redirect targets a shared workspace, and a
// second workspace that BEADS_DIR can name. The cwd repo's metadata.json
// carries a source dolt_database different from anything the shared target
// declares.
func setupRedirectWorkspaces(t *testing.T, sourceDatabase string) (cwdRepo, sharedBeads, otherBeads string) {
	t.Helper()

	cwdRepo = newGitRepo(t)
	cwdBeads := filepath.Join(cwdRepo, ".beads")
	if err := os.MkdirAll(cwdBeads, 0o700); err != nil {
		t.Fatalf("mkdir cwd .beads: %v", err)
	}

	sharedBeads = t.TempDir()
	writeTestConfigYAML(t, sharedBeads, "")

	if err := os.WriteFile(filepath.Join(cwdBeads, "redirect"), []byte(sharedBeads+"\n"), 0o600); err != nil {
		t.Fatalf("write redirect: %v", err)
	}
	metadata := []byte(`{"dolt_database":"` + sourceDatabase + `"}`)
	if err := os.WriteFile(filepath.Join(cwdBeads, "metadata.json"), metadata, 0o600); err != nil {
		t.Fatalf("write source metadata: %v", err)
	}

	otherRepo := newGitRepo(t)
	otherBeads = filepath.Join(otherRepo, ".beads")
	writeTestConfigYAML(t, otherBeads, "")

	return cwdRepo, sharedBeads, otherBeads
}

// runRootPreRunOnConfigShow drives the real root pre-run far enough to reach
// the redirect-source block, then returns through the no-store config path.
// config show is deliberately the command: it is not in noDbCommands, so the
// pre-run reaches the redirect-source block instead of the skip-store branch,
// and configCommandCanRunWithoutStore lets it finish without a database.
func runRootPreRunOnConfigShow() error {
	return rootCmd.PersistentPreRunE(configShowCmd, nil)
}

func TestPreserveRedirectSourceDatabaseOnlyForSelectedWorkspace(t *testing.T) {
	const sourceDatabase = "cwd_source_db"
	cwdRepo, sharedBeads, otherBeads := setupRedirectWorkspaces(t, sourceDatabase)

	tests := []struct {
		name         string
		beadsDir     string
		wantDatabase string
		wantReason   string
	}{
		{
			name:         "BEADS_DIR names another workspace",
			beadsDir:     otherBeads,
			wantDatabase: "",
			wantReason:   "do not resolve to the same directory",
		},
		{
			name:         "BEADS_DIR names the redirect target",
			beadsDir:     sharedBeads,
			wantDatabase: sourceDatabase,
		},
		{
			name:         "BEADS_DIR unset",
			beadsDir:     "",
			wantDatabase: sourceDatabase,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(cwdRepo)
			// git.GetRepoRoot is cached once per process; other tests in this
			// package populate it with their own tree, so it must be cleared
			// after the chdir and again when this subtest ends.
			beads.ResetCaches()
			git.ResetCaches()
			t.Cleanup(func() {
				beads.ResetCaches()
				git.ResetCaches()
			})
			t.Setenv("BEADS_DIR", tt.beadsDir)
			t.Setenv("BEADS_DOLT_SERVER_DATABASE", "")
			t.Setenv("BEADS_DOLT_SERVER_HOST", "")
			t.Setenv("BEADS_DOLT_SHARED_SERVER", "")
			t.Setenv("BEADS_DOLT_SERVER_PORT", "")
			t.Setenv("BD_DEBUG_ROUTING", "1")

			config.ResetForTesting()
			t.Cleanup(config.ResetForTesting)
			savePersistentPreRunState(t)

			var runErr error
			stderr := captureStderr(t, func() { runErr = runRootPreRunOnConfigShow() })
			if runErr != nil {
				t.Fatalf("PersistentPreRunE: %v\nstderr:\n%s", runErr, stderr)
			}

			if got := os.Getenv("BEADS_DOLT_SERVER_DATABASE"); got != tt.wantDatabase {
				t.Fatalf("BEADS_DOLT_SERVER_DATABASE = %q, want %q\nstderr:\n%s", got, tt.wantDatabase, stderr)
			}

			notApplied := 0
			for _, line := range strings.Split(stderr, "\n") {
				if !strings.Contains(line, "[routing]") || !strings.Contains(line, "not applied") {
					continue
				}
				notApplied++
				if tt.wantReason == "" {
					t.Errorf("unexpected routing refusal while the redirect leads to the selected workspace: %s", line)
					continue
				}
				if !strings.Contains(line, tt.wantReason) {
					t.Errorf("routing refusal does not say why: %s", line)
				}
			}
			wantLines := 0
			if tt.wantReason != "" {
				wantLines = 1
			}
			if notApplied != wantLines {
				t.Errorf("got %d [routing] not-applied lines, want %d\nstderr:\n%s", notApplied, wantLines, stderr)
			}
		})
	}
}

// TestRedirectLeadsToWorkspace pins the comparison the pre-run applies. Both
// producers (GetRedirectInfo and FindBeadsDir) canonicalise their results, so
// this helper is the only seam that can drive the symlink resolution and the
// "cannot resolve" refusals: a path that does not resolve is not the workspace
// and the redirect source database must not be applied to it.
func TestRedirectLeadsToWorkspace(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real-workspace")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatalf("mkdir real workspace: %v", err)
	}
	linkedDir := filepath.Join(root, "linked-workspace")
	if err := os.Symlink(realDir, linkedDir); err != nil {
		t.Fatalf("symlink workspace: %v", err)
	}
	otherDir := filepath.Join(root, "other-workspace")
	if err := os.Mkdir(otherDir, 0o700); err != nil {
		t.Fatalf("mkdir other workspace: %v", err)
	}
	missing := filepath.Join(root, "missing-workspace")

	tests := []struct {
		name           string
		redirectTarget string
		workspaceDir   string
		want           bool
	}{
		{name: "same directory", redirectTarget: realDir, workspaceDir: realDir, want: true},
		{name: "workspace reached through a symlink", redirectTarget: realDir, workspaceDir: linkedDir, want: true},
		{name: "redirect target reached through a symlink", redirectTarget: linkedDir, workspaceDir: realDir, want: true},
		{name: "different directories", redirectTarget: realDir, workspaceDir: otherDir, want: false},
		{name: "redirect target does not resolve", redirectTarget: missing, workspaceDir: realDir, want: false},
		{name: "workspace does not resolve", redirectTarget: realDir, workspaceDir: missing, want: false},
		{name: "neither path resolves", redirectTarget: missing, workspaceDir: missing, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redirectLeadsToWorkspace(tt.redirectTarget, tt.workspaceDir); got != tt.want {
				t.Errorf("redirectLeadsToWorkspace(%q, %q) = %v, want %v", tt.redirectTarget, tt.workspaceDir, got, tt.want)
			}
		})
	}
}
