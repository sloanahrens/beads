package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/workspace"
)

// be-h0k / B3-07: a git worktree whose .beads holds only a redirect to its
// rig (Gas Town polecats; gastown hides the tracked config.yaml there) must
// load the rig's config.yaml. The old ancestor walk ignored the redirect and
// reached the town's config.yaml, so polecats ran with issue-prefix "hq"
// against the rig database.
func TestInitialize_FollowsBeadsRedirectForProjectConfig(t *testing.T) {
	town, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rigBeads := filepath.Join(town, "rig", "mayor", "rig", ".beads")
	worktree := filepath.Join(town, "rig", "polecats", "amber", "rig")
	write(filepath.Join(town, ".beads", "config.yaml"), "issue-prefix: hq\nrouting:\n  mode: explicit\n")
	write(filepath.Join(rigBeads, "config.yaml"), "issue-prefix: zz\nsync.remote: git+https://example.com/rig.git\n")
	write(filepath.Join(rigBeads, "metadata.json"), `{"backend":"dolt"}`)
	write(filepath.Join(worktree, ".beads", "redirect"), "../../../mayor/rig/.beads\n")
	sub := filepath.Join(worktree, "internal", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BEADS_DIR", "")
	t.Setenv("BEADS_TEST_IGNORE_REPO_CONFIG", "")
	for _, cwd := range []string{worktree, sub} {
		t.Run(strings.TrimPrefix(cwd, town), func(t *testing.T) {
			t.Chdir(cwd)
			if err := Initialize(); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			if got := GetString("issue-prefix"); got != "zz" {
				t.Errorf("issue-prefix = %q, want %q (rig config through the redirect)", got, "zz")
			}
			if got := GetString("sync.remote"); got != "git+https://example.com/rig.git" {
				t.Errorf("sync.remote = %q, want the rig's", got)
			}
			if got := GetString("routing.mode"); got != "" {
				t.Errorf("routing.mode = %q leaked in from the town config", got)
			}
			if got, want := ConfigFileUsed(), filepath.Join(rigBeads, "config.yaml"); got != want {
				t.Errorf("ConfigFileUsed() = %q, want %q", got, want)
			}
		})
	}

	// BEADS_DIR naming the worktree's .beads follows the same redirect.
	t.Run("BEADS_DIR", func(t *testing.T) {
		t.Chdir(town)
		t.Setenv("BEADS_DIR", filepath.Join(worktree, ".beads"))
		if err := Initialize(); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		if got := GetString("issue-prefix"); got != "zz" {
			t.Errorf("issue-prefix = %q, want %q", got, "zz")
		}
	})
}

// A redirect that cannot be followed is reported, and no ancestor config is
// substituted for the workspace the redirect was meant to select.
func TestInitialize_BrokenRedirectReportsAndLoadsNoAncestorConfig(t *testing.T) {
	town, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(town, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(town, ".beads", "config.yaml"), []byte("issue-prefix: hq\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtBeads := filepath.Join(town, "wt", ".beads")
	if err := os.MkdirAll(wtBeads, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtBeads, "redirect"), []byte("../gone/.beads\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEADS_DIR", "")
	t.Setenv("BEADS_TEST_IGNORE_REPO_CONFIG", "")
	t.Chdir(filepath.Join(town, "wt"))

	err = Initialize()
	if err == nil || !strings.Contains(err.Error(), filepath.Join(wtBeads, "redirect")) {
		t.Fatalf("Initialize err = %v, want an error naming the redirect file", err)
	}
	if got := GetString("issue-prefix"); got != "" {
		t.Errorf("issue-prefix = %q, want empty: the town config must not stand in", got)
	}
}

// BEADS_DIR whose redirect is broken: report it, and load BEADS_DIR's own
// config.yaml, the directory internal/beads falls back to for the database,
// so config and database stay together.
func TestInitialize_BeadsDirBrokenRedirectFallsBackToItsOwnConfig(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "ws", ".beads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("issue-prefix: own\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "redirect"), []byte("../gone/.beads\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEADS_TEST_IGNORE_REPO_CONFIG", "")
	t.Setenv("BEADS_DIR", dir)
	t.Chdir(root)

	err = Initialize()
	if err == nil || !strings.Contains(err.Error(), "BEADS_DIR") {
		t.Fatalf("Initialize err = %v, want the BEADS_DIR redirect reported", err)
	}
	if got := GetString("issue-prefix"); got != "own" {
		t.Errorf("issue-prefix = %q, want %q from BEADS_DIR's own config", got, "own")
	}
}

// ConfigFileUsed keeps the caller's spelling of a path that reaches the
// workspace through a symlink (macOS /var -> /private/var), and the
// canonical path once a redirect was followed.
func TestConfigPathAsSpelled(t *testing.T) {
	real, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	proj := filepath.Join(real, "proj")
	beadsDir := filepath.Join(proj, ".beads")
	sub := filepath.Join(proj, "a", "b")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	canonicalCfg := filepath.Join(beadsDir, "config.yaml")
	discovered := workspace.Workspace{BeadsDir: beadsDir, SourceDir: beadsDir, ConfigPath: canonicalCfg}

	tests := []struct {
		name   string
		ws     workspace.Workspace
		cwd    string
		envDir string
		want   string
	}{
		{"discovered from symlinked project root", discovered, filepath.Join(link, "proj"), "", filepath.Join(link, "proj", ".beads", "config.yaml")},
		{"discovered from symlinked subdirectory", discovered, filepath.Join(link, "proj", "a", "b"), "", filepath.Join(link, "proj", ".beads", "config.yaml")},
		{"discovered from canonical cwd", discovered, sub, "", canonicalCfg},
		{"no cwd", discovered, "", "", canonicalCfg},
		{"cwd outside the workspace", discovered, real, "", canonicalCfg},
		{"BEADS_DIR spelled through the symlink",
			workspace.Workspace{BeadsDir: beadsDir, SourceDir: beadsDir, FromEnv: true, ConfigPath: canonicalCfg},
			"", filepath.Join(link, "proj", ".beads"), filepath.Join(link, "proj", ".beads", "config.yaml")},
		{"redirected keeps canonical target",
			workspace.Workspace{BeadsDir: beadsDir, SourceDir: filepath.Join(link, "other", ".beads"), Redirected: true, ConfigPath: canonicalCfg},
			filepath.Join(link, "other"), "", canonicalCfg},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BEADS_DIR", tc.envDir)
			if got := configPathAsSpelled(tc.ws, tc.cwd); got != tc.want {
				t.Errorf("configPathAsSpelled = %q, want %q", got, tc.want)
			}
		})
	}
}
