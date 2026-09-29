package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
