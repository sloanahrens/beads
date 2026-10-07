package beads

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/git"
	"github.com/steveyegge/beads/internal/workspace"
)

// be-h0k: database discovery and workspace.Resolve must agree on a worktree
// whose .beads holds only a redirect to its rig, even with an unrelated
// ancestor workspace above both.
func TestFindBeadsDir_AgreesWithWorkspaceResolveThroughRedirect(t *testing.T) {
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
	write(filepath.Join(town, ".beads", "config.yaml"), "issue-prefix: hq\n")
	write(filepath.Join(town, ".beads", "metadata.json"), `{"backend":"dolt"}`)
	write(filepath.Join(rigBeads, "config.yaml"), "issue-prefix: zz\n")
	write(filepath.Join(rigBeads, "metadata.json"), `{"backend":"dolt"}`)
	if err := os.MkdirAll(filepath.Join(rigBeads, "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(worktree, ".beads", RedirectFileName), "../../../mayor/rig/.beads\n")

	t.Setenv("BEADS_DIR", "")
	t.Setenv("BEADS_DB", "")
	t.Chdir(worktree)
	git.ResetCaches()
	t.Cleanup(git.ResetCaches)

	ws, err := workspace.Resolve(worktree, os.Getenv)
	if err != nil {
		t.Fatalf("workspace.Resolve: %v", err)
	}
	if ws.BeadsDir != rigBeads {
		t.Fatalf("Resolve.BeadsDir = %q, want %q", ws.BeadsDir, rigBeads)
	}
	if got := FindBeadsDir(); got != ws.BeadsDir {
		t.Errorf("FindBeadsDir() = %q, want %q", got, ws.BeadsDir)
	}
	if got := FindBeadsDirFrom(worktree); got != ws.BeadsDir {
		t.Errorf("FindBeadsDirFrom() = %q, want %q", got, ws.BeadsDir)
	}
	if got := FindDatabasePath(); !strings.HasPrefix(got, rigBeads+string(filepath.Separator)) {
		t.Errorf("FindDatabasePath() = %q, want a path under %q", got, rigBeads)
	}

	// BEADS_DIR naming the worktree's .beads is interpreted the same way.
	t.Setenv("BEADS_DIR", filepath.Join(worktree, ".beads"))
	if got := FindBeadsDir(); got != rigBeads {
		t.Errorf("FindBeadsDir() with BEADS_DIR = %q, want %q", got, rigBeads)
	}
	if got := FindDatabasePath(); !strings.HasPrefix(got, rigBeads+string(filepath.Separator)) {
		t.Errorf("FindDatabasePath() with BEADS_DIR = %q, want a path under %q", got, rigBeads)
	}
}

// setupDiscoveryRepo creates a plain git repo (not a worktree) whose root
// .beads is a valid ancestor workspace, and returns the canonical repo root.
// The caller chdirs into a subdirectory; FindBeadsDir's step 2 walk stops at
// the repo root and step 4 checks it, so an ancestor workspace is what a
// walk-past would bind to.
func setupDiscoveryRepo(t *testing.T) string {
	t.Helper()
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", repo).CombinedOutput(); err != nil {
		t.Skipf("git not available: %v (%s)", err, out)
	}
	ancestor := filepath.Join(repo, ".beads")
	if err := os.MkdirAll(filepath.Join(ancestor, "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ancestor, "metadata.json"), []byte(`{"backend":"dolt","dolt_database":"ancestor"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BEADS_DIR", "")
	t.Setenv("BEADS_DB", "")
	t.Cleanup(git.ResetCaches)
	return repo
}

// chdir is a small wrapper so every discovery test resets the git caches after
// the process working directory moves.
func chdir(t *testing.T, dir string) {
	t.Helper()
	t.Chdir(dir)
	git.ResetCaches()
}

// be-929: a .beads/redirect the walk cannot follow ends discovery. The
// directory is a deliberate pointer at a workspace; returning an unrelated
// ancestor workspace instead would aim writes at the wrong database.
func TestFindBeadsDir_BrokenRedirectEndsWalk(t *testing.T) {
	cases := []struct {
		name     string
		redirect func(repo, source string) string
		target   func(repo, source string) string
	}{
		{
			name:     "target missing",
			redirect: func(repo, _ string) string { return filepath.Join(repo, "gone", ".beads") + "\n" },
			target:   func(repo, _ string) string { return filepath.Join(repo, "gone", ".beads") },
		},
		{
			name: "target has no workspace files",
			redirect: func(repo, _ string) string {
				return filepath.Join(repo, "empty", ".beads") + "\n"
			},
			target: func(repo, _ string) string { return filepath.Join(repo, "empty", ".beads") },
		},
		{
			name:     "redirect loop",
			redirect: func(_, source string) string { return source + "\n" },
			target:   func(_, source string) string { return source },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupDiscoveryRepo(t)
			source := filepath.Join(repo, "sub", ".beads")
			if err := os.MkdirAll(source, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.name == "target has no workspace files" {
				if err := os.MkdirAll(filepath.Join(repo, "empty", ".beads"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(source, "redirect"), []byte(tc.redirect(repo, source)), 0o644); err != nil {
				t.Fatal(err)
			}
			chdir(t, filepath.Join(repo, "sub"))

			var got string
			stderr := captureStderr(t, func() { got = FindBeadsDir() })

			if got != "" {
				t.Errorf("FindBeadsDir() = %q, want \"\" (broken redirect must not bind to an ancestor workspace)", got)
			}
			if n := strings.Count(stderr, "refusing to search parent directories"); n != 1 {
				t.Errorf("want exactly one walk-refusal warning, got %d: %q", n, stderr)
			}
			if sourcePath := source; !strings.Contains(stderr, sourcePath) {
				t.Errorf("warning should name the .beads directory %q, got: %q", sourcePath, stderr)
			}
			if targetPath := tc.target(repo, source); !strings.Contains(stderr, targetPath) {
				t.Errorf("warning should name the redirect target %q, got: %q", targetPath, stderr)
			}
		})
	}
}

// A good redirect found by the walk still resolves to its target.
func TestFindBeadsDir_GoodRedirectFollowedInWalk(t *testing.T) {
	repo := setupDiscoveryRepo(t)
	target := filepath.Join(repo, "real", ".beads")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "metadata.json"), []byte(`{"backend":"dolt","dolt_database":"real"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(repo, "sub", ".beads")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "redirect"), []byte("../real/.beads\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, filepath.Join(repo, "sub"))

	if got := FindBeadsDir(); got != target {
		t.Errorf("FindBeadsDir() = %q, want %q (good redirect still followed)", got, target)
	}
}

// A .beads directory with no redirect is skipped as before: the walk
// continues to the ancestor workspace.
func TestFindBeadsDir_NoRedirectStillWalksToAncestor(t *testing.T) {
	repo := setupDiscoveryRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, filepath.Join(repo, "sub"))

	want := filepath.Join(repo, ".beads")
	if got := FindBeadsDir(); got != want {
		t.Errorf("FindBeadsDir() = %q, want %q (no redirect should walk up as before)", got, want)
	}
}

// writeWorkspaceFiles gives a .beads directory the marker files discovery
// accepts as an existing workspace.
func writeWorkspaceFiles(t *testing.T, beadsDir string) {
	t.Helper()
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"backend":"dolt"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("issue-prefix: hq\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// initTestRepo creates a real git repository at dir with one commit, so the
// tree carries the .git entry the walk bounds on.
func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := initGitRepoWithCommit(dir); err != nil {
		t.Fatal(err)
	}
}

// be-8ff: FindBeadsDir and workspace.Resolve (through workspace.Discover)
// answer with the same .beads -- or with no workspace at all -- in every
// layout the walk bound distinguishes. The ancestor .beads above the town is
// valid in every case, so a layout that must not reach it fails loudly when
// the bound is missing.
func TestFindBeadsDir_AgreesWithResolveOnWalkBound(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	cases := []struct {
		name string
		// build returns the cwd to discover from, inside the town tree.
		build func(t *testing.T, town string) string
		// want is the .beads discovery must land on, "" for none.
		want func(town string) string
		// brokenRedirect is true when the walk must fail closed rather than
		// find no workspace at all (be-929).
		brokenRedirect bool
	}{
		{
			name: "plain repo under an ancestor .beads",
			build: func(t *testing.T, town string) string {
				repo := filepath.Join(town, "repo")
				initTestRepo(t, repo)
				return filepath.Join(repo, "sub")
			},
			want: func(string) string { return "" },
		},
		{
			name: "nested clone under the outer repo's .beads",
			build: func(t *testing.T, town string) string {
				outer := filepath.Join(town, "outer")
				initTestRepo(t, outer)
				writeWorkspaceFiles(t, filepath.Join(outer, ".beads"))
				inner := filepath.Join(outer, "inner")
				initTestRepo(t, inner)
				return filepath.Join(inner, "sub")
			},
			want: func(string) string { return "" },
		},
		{
			name: "outside any repo walks to the ancestor .beads",
			build: func(t *testing.T, town string) string {
				return filepath.Join(town, "not", "a", "repo")
			},
			want: func(town string) string { return filepath.Join(town, ".beads") },
		},
		{
			name: "linked worktree uses the shared database",
			build: func(t *testing.T, town string) string {
				main := filepath.Join(town, "main")
				initTestRepo(t, main)
				writeWorkspaceFiles(t, filepath.Join(main, ".beads"))
				worktree := filepath.Join(town, "wt")
				runGitInDir(t, main, "worktree", "add", "-q", worktree, "HEAD")
				t.Cleanup(func() {
					_ = exec.Command("git", "-C", main, "worktree", "remove", "--force", worktree).Run()
				})
				return filepath.Join(worktree, "sub")
			},
			want: func(town string) string { return filepath.Join(town, "main", ".beads") },
		},
		{
			name: "jj secondary uses the primary database",
			build: func(t *testing.T, town string) string {
				primary := filepath.Join(town, "primary")
				// A colocated jj+git primary: .jj/repo is a directory.
				if err := os.MkdirAll(filepath.Join(primary, ".jj", "repo"), 0o755); err != nil {
					t.Fatal(err)
				}
				initTestRepo(t, primary)
				writeWorkspaceFiles(t, filepath.Join(primary, ".beads"))

				// The secondary points at the primary's repo with a file.
				secondary := filepath.Join(primary, "ws", "secondary")
				if err := os.MkdirAll(filepath.Join(secondary, ".jj"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(secondary, ".jj", "repo"),
					[]byte(filepath.Join(primary, ".jj", "repo")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return secondary
			},
			want: func(town string) string { return filepath.Join(town, "primary", ".beads") },
		},
		{
			name: "broken redirect inside the repo ends discovery",
			build: func(t *testing.T, town string) string {
				repo := filepath.Join(town, "repo")
				initTestRepo(t, repo)
				sub := filepath.Join(repo, "sub")
				if err := os.MkdirAll(filepath.Join(sub, ".beads"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(sub, ".beads", workspace.RedirectFileName),
					[]byte(filepath.Join(town, "gone", ".beads")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return sub
			},
			want:           func(string) string { return "" },
			brokenRedirect: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			town, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			writeWorkspaceFiles(t, filepath.Join(town, ".beads"))
			cwd := tc.build(t, town)
			if err := os.MkdirAll(cwd, 0o755); err != nil {
				t.Fatal(err)
			}

			t.Setenv("BEADS_DIR", "")
			t.Setenv("BEADS_DB", "")
			t.Cleanup(git.ResetCaches)
			chdir(t, cwd)

			want := tc.want(town)
			got := FindBeadsDir()
			if got != want {
				t.Errorf("FindBeadsDir() = %q, want %q", got, want)
			}
			if !tc.brokenRedirect {
				if from := FindBeadsDirFrom(cwd); from != want {
					t.Errorf("FindBeadsDirFrom() = %q, want %q", from, want)
				}
			}

			ws, err := workspace.Resolve(cwd, os.Getenv)
			switch {
			case want != "":
				if err != nil {
					t.Fatalf("workspace.Resolve: %v", err)
				}
				if ws.BeadsDir != want {
					t.Errorf("Resolve().BeadsDir = %q, want %q", ws.BeadsDir, want)
				}
			case tc.brokenRedirect:
				if !errors.Is(err, workspace.ErrRedirectTarget) {
					t.Errorf("workspace.Resolve() = %+v, %v; want ErrRedirectTarget", ws, err)
				}
			default:
				if !errors.Is(err, workspace.ErrNoWorkspace) {
					t.Errorf("workspace.Resolve() = %+v, %v; want ErrNoWorkspace", ws, err)
				}
			}
		})
	}
}

// be-929, worktree case: a broken redirect found by the walk ends discovery,
// so the shared worktree .beads (step 3c) is never substituted for the
// workspace the redirect names.
func TestFindBeadsDir_BrokenRedirectDoesNotFallThroughToWorktreeSharedBeads(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(tmp, "main")
	if out, err := exec.Command("git", "init", "-b", "main", main).CombinedOutput(); err != nil {
		t.Skipf("git not available: %v (%s)", err, out)
	}
	if out, err := exec.Command("git", "-C", main, "config", "user.email", "test@example.com").CombinedOutput(); err != nil {
		t.Fatalf("git config user.email: %v (%s)", err, out)
	}
	if out, err := exec.Command("git", "-C", main, "config", "user.name", "Test User").CombinedOutput(); err != nil {
		t.Fatalf("git config user.name: %v (%s)", err, out)
	}

	// The shared .beads that step 3c would fall back to.
	shared := filepath.Join(main, ".beads")
	if err := os.MkdirAll(filepath.Join(shared, "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "metadata.json"), []byte(`{"backend":"dolt","dolt_database":"shared"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", main, "add", "README.md").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v (%s)", err, out)
	}
	if out, err := exec.Command("git", "-C", main, "commit", "-m", "init").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v (%s)", err, out)
	}

	wt := filepath.Join(tmp, "wt")
	if out, err := exec.Command("git", "-C", main, "worktree", "add", wt, "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v (%s)", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", main, "worktree", "remove", "--force", wt).Run()
	})

	source := filepath.Join(wt, "sub", ".beads")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(tmp, "gone", ".beads")
	if err := os.WriteFile(filepath.Join(source, "redirect"), []byte(missing+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BEADS_DIR", "")
	t.Setenv("BEADS_DB", "")
	t.Cleanup(git.ResetCaches)
	chdir(t, filepath.Join(wt, "sub"))

	var got string
	stderr := captureStderr(t, func() { got = FindBeadsDir() })

	if got != "" {
		t.Errorf("FindBeadsDir() = %q, want \"\" (broken redirect must not fall through to the shared worktree .beads %q)", got, shared)
	}
	if n := strings.Count(stderr, "refusing to search parent directories"); n != 1 {
		t.Errorf("want exactly one walk-refusal warning, got %d: %q", n, stderr)
	}
}
