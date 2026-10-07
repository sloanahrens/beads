package workspace

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// townTree mirrors the Gas Town layout that exposed be-h0k:
//
//	town/.beads/config.yaml                      issue-prefix: hq (unrelated ancestor)
//	town/rig/mayor/rig/.beads/{config.yaml,metadata.json}   issue-prefix: zz
//	town/rig/polecats/amber/rig/.beads/redirect  ../../../mayor/rig/.beads
//
// The worktree's .beads holds only the redirect (gastown hides the tracked
// config.yaml there), so an ancestor walk that ignores the redirect lands on
// the town config.
type townTree struct {
	town, rigBeads, worktree, worktreeBeads string
}

func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTownTree(t *testing.T) townTree {
	t.Helper()
	town := realTempDir(t)
	tr := townTree{
		town:     town,
		rigBeads: filepath.Join(town, "rig", "mayor", "rig", ".beads"),
		worktree: filepath.Join(town, "rig", "polecats", "amber", "rig"),
	}
	tr.worktreeBeads = filepath.Join(tr.worktree, ".beads")
	writeFile(t, filepath.Join(town, ".beads", "config.yaml"), "issue-prefix: hq\n")
	writeFile(t, filepath.Join(tr.rigBeads, "config.yaml"), "issue-prefix: zz\n")
	writeFile(t, filepath.Join(tr.rigBeads, "metadata.json"), `{"backend":"dolt"}`+"\n")
	writeFile(t, filepath.Join(tr.worktreeBeads, RedirectFileName), "../../../mayor/rig/.beads\n")
	return tr
}

func noEnv(string) string { return "" }

func envOf(kv map[string]string) Env {
	return func(k string) string { return kv[k] }
}

func assertRigWorkspace(t *testing.T, ws Workspace, tr townTree) {
	t.Helper()
	if ws.BeadsDir != tr.rigBeads {
		t.Errorf("BeadsDir = %q, want %q", ws.BeadsDir, tr.rigBeads)
	}
	if want := filepath.Join(tr.rigBeads, "config.yaml"); ws.ConfigPath != want {
		t.Errorf("ConfigPath = %q, want %q", ws.ConfigPath, want)
	}
	if want := filepath.Join(tr.rigBeads, "metadata.json"); ws.MetadataPath != want {
		t.Errorf("MetadataPath = %q, want %q", ws.MetadataPath, want)
	}
	if want := tr.rigBeads + ".gate.lock"; ws.GatePath != want {
		t.Errorf("GatePath = %q, want %q", ws.GatePath, want)
	}
}

func TestResolve_RedirectedWorktreeUsesRigForConfigDatabaseAndGate(t *testing.T) {
	tr := newTownTree(t)
	for _, cwd := range []string{tr.worktree, filepath.Join(tr.worktree, "sub", "dir")} {
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		ws, err := Resolve(cwd, noEnv)
		if err != nil {
			t.Fatalf("Resolve(%s): %v", cwd, err)
		}
		assertRigWorkspace(t, ws, tr)
		if !ws.Redirected || ws.SourceDir != tr.worktreeBeads || ws.FromEnv {
			t.Errorf("provenance = {Redirected:%v SourceDir:%q FromEnv:%v}, want redirected from %q",
				ws.Redirected, ws.SourceDir, ws.FromEnv, tr.worktreeBeads)
		}
	}
}

// The live layout is a git worktree of the rig clone. Run the same assertion
// with real git so the worktree branches of discovery execute.
func TestResolve_RedirectedGitWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	tr := newTownTree(t)
	rigRoot := filepath.Dir(tr.rigBeads)
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git(rigRoot, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(rigRoot, "README"), "rig\n")
	git(rigRoot, "add", "README")
	git(rigRoot, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "-m", "init")
	// Replace the plain worktree dir with a real git worktree carrying only the redirect.
	if err := os.RemoveAll(tr.worktree); err != nil {
		t.Fatal(err)
	}
	git(rigRoot, "worktree", "add", "-q", "-b", "polecat", tr.worktree)
	writeFile(t, filepath.Join(tr.worktreeBeads, RedirectFileName), "../../../mayor/rig/.beads\n")

	ws, err := Resolve(tr.worktree, noEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertRigWorkspace(t, ws, tr)
}

func TestResolve_BeadsDirEnvWinsAndIsRedirectFollowed(t *testing.T) {
	tr := newTownTree(t)
	elsewhere := realTempDir(t)

	ws, err := Resolve(elsewhere, envOf(map[string]string{"BEADS_DIR": tr.worktreeBeads}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	assertRigWorkspace(t, ws, tr)
	if !ws.FromEnv || !ws.Redirected {
		t.Errorf("FromEnv=%v Redirected=%v, want both true", ws.FromEnv, ws.Redirected)
	}

	// BEADS_DIR beats discovery from a cwd that would find the town.
	ws, err = Resolve(tr.town, envOf(map[string]string{"BEADS_DIR": tr.rigBeads}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ws.BeadsDir != tr.rigBeads || !ws.FromEnv || ws.Redirected {
		t.Errorf("got %+v, want rig from env without redirect", ws)
	}
}

// BEADS_DIR naming a directory that does not hold a workspace yet (bd init
// with an explicit target) still selects it: config comes from there or
// nowhere, never from the caller's ancestors.
func TestResolve_BeadsDirEnvWithoutWorkspaceStillSelectsIt(t *testing.T) {
	tr := newTownTree(t)
	fresh := filepath.Join(realTempDir(t), ".beads")
	ws, err := Resolve(tr.town, envOf(map[string]string{"BEADS_DIR": fresh}))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ws.BeadsDir != fresh || ws.ConfigPath != filepath.Join(fresh, "config.yaml") {
		t.Errorf("got %+v, want %s", ws, fresh)
	}
}

func TestResolve_NoWorkspace(t *testing.T) {
	_, err := Resolve(realTempDir(t), noEnv)
	if !errors.Is(err, ErrNoWorkspace) {
		t.Fatalf("err = %v, want ErrNoWorkspace", err)
	}
}

func TestResolve_BrokenRedirectIsAnError(t *testing.T) {
	tr := newTownTree(t)
	writeFile(t, filepath.Join(tr.worktreeBeads, RedirectFileName), "../../../nope/.beads\n")
	_, err := Resolve(tr.worktree, noEnv)
	if !errors.Is(err, ErrRedirectTarget) {
		t.Fatalf("err = %v, want ErrRedirectTarget", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(tr.worktreeBeads, RedirectFileName)) {
		t.Errorf("error %q does not name the redirect file", err)
	}
}

func TestFollowRedirect(t *testing.T) {
	t.Run("no redirect", func(t *testing.T) {
		dir := filepath.Join(realTempDir(t), ".beads")
		writeFile(t, filepath.Join(dir, "config.yaml"), "")
		got, redirected, err := FollowRedirect(dir)
		if err != nil || redirected || got != dir {
			t.Fatalf("got (%q,%v,%v)", got, redirected, err)
		}
	})
	t.Run("relative to project root, comments and blanks skipped", func(t *testing.T) {
		tr := newTownTree(t)
		writeFile(t, filepath.Join(tr.worktreeBeads, RedirectFileName), "# shared rig db\n\n  ../../../mayor/rig/.beads  \n")
		got, redirected, err := FollowRedirect(tr.worktreeBeads)
		if err != nil || !redirected || got != tr.rigBeads {
			t.Fatalf("got (%q,%v,%v), want %q", got, redirected, err, tr.rigBeads)
		}
	})
	t.Run("absolute target", func(t *testing.T) {
		tr := newTownTree(t)
		writeFile(t, filepath.Join(tr.worktreeBeads, RedirectFileName), tr.rigBeads+"\n")
		got, _, err := FollowRedirect(tr.worktreeBeads)
		if err != nil || got != tr.rigBeads {
			t.Fatalf("got (%q,%v)", got, err)
		}
	})
	t.Run("empty redirect is ignored", func(t *testing.T) {
		tr := newTownTree(t)
		writeFile(t, filepath.Join(tr.worktreeBeads, RedirectFileName), "# nothing\n")
		got, redirected, err := FollowRedirect(tr.worktreeBeads)
		if err != nil || redirected || got != tr.worktreeBeads {
			t.Fatalf("got (%q,%v,%v)", got, redirected, err)
		}
	})
	t.Run("self loop", func(t *testing.T) {
		root := realTempDir(t)
		dir := filepath.Join(root, ".beads")
		writeFile(t, filepath.Join(dir, "config.yaml"), "")
		writeFile(t, filepath.Join(dir, RedirectFileName), ".beads\n")
		if _, _, err := FollowRedirect(dir); !errors.Is(err, ErrRedirectLoop) {
			t.Fatalf("err = %v, want ErrRedirectLoop", err)
		}
	})
	t.Run("two-node loop", func(t *testing.T) {
		root := realTempDir(t)
		a, b := filepath.Join(root, "a", ".beads"), filepath.Join(root, "b", ".beads")
		writeFile(t, filepath.Join(a, "config.yaml"), "")
		writeFile(t, filepath.Join(b, "config.yaml"), "")
		writeFile(t, filepath.Join(a, RedirectFileName), "../b/.beads\n")
		writeFile(t, filepath.Join(b, RedirectFileName), "../a/.beads\n")
		if _, _, err := FollowRedirect(a); !errors.Is(err, ErrRedirectLoop) {
			t.Fatalf("err = %v, want ErrRedirectLoop", err)
		}
	})
	t.Run("chain follows exactly one hop", func(t *testing.T) {
		root := realTempDir(t)
		a, b, c := filepath.Join(root, "a", ".beads"), filepath.Join(root, "b", ".beads"), filepath.Join(root, "c", ".beads")
		for _, d := range []string{a, b, c} {
			writeFile(t, filepath.Join(d, "config.yaml"), "")
		}
		writeFile(t, filepath.Join(a, RedirectFileName), "../b/.beads\n")
		writeFile(t, filepath.Join(b, RedirectFileName), "../c/.beads\n")
		got, redirected, err := FollowRedirect(a)
		if err != nil || !redirected || got != b {
			t.Fatalf("got (%q,%v,%v), want %q", got, redirected, err, b)
		}
	})
	t.Run("missing target", func(t *testing.T) {
		tr := newTownTree(t)
		writeFile(t, filepath.Join(tr.worktreeBeads, RedirectFileName), "../../../gone/.beads\n")
		if _, _, err := FollowRedirect(tr.worktreeBeads); !errors.Is(err, ErrRedirectTarget) {
			t.Fatalf("err = %v, want ErrRedirectTarget", err)
		}
	})
	t.Run("target without workspace files", func(t *testing.T) {
		tr := newTownTree(t)
		empty := filepath.Join(tr.town, "empty", ".beads")
		if err := os.MkdirAll(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(tr.worktreeBeads, RedirectFileName), empty+"\n")
		if _, _, err := FollowRedirect(tr.worktreeBeads); !errors.Is(err, ErrRedirectTarget) {
			t.Fatalf("err = %v, want ErrRedirectTarget", err)
		}
	})
}

// Config loading runs discovery on every bd invocation. A main checkout whose
// .beads holds no local database (server mode: metadata.json and config.yaml
// only) must resolve without spawning git.
func TestResolve_ServerModeMainCheckoutSpawnsNoGit(t *testing.T) {
	root := realTempDir(t)
	proj := filepath.Join(root, "proj")
	writeFile(t, filepath.Join(proj, ".beads", "config.yaml"), "issue-prefix: sv\n")
	writeFile(t, filepath.Join(proj, ".beads", "metadata.json"), `{"backend":"dolt","dolt_mode":"server"}`)
	if err := os.MkdirAll(filepath.Join(proj, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(proj, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	logPath := filepath.Join(root, "git.log")
	writeFile(t, filepath.Join(bin, "git"), "#!/bin/sh\necho called >> \"$FAKE_GIT_LOG\"\nexit 1\n")
	if err := os.Chmod(filepath.Join(bin, "git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("FAKE_GIT_LOG", logPath)

	for _, cwd := range []string{proj, sub} {
		ws, err := Resolve(cwd, noEnv)
		if err != nil {
			t.Fatalf("Resolve(%s): %v", cwd, err)
		}
		if ws.BeadsDir != filepath.Join(proj, ".beads") {
			t.Errorf("BeadsDir = %q", ws.BeadsDir)
		}
	}
	if data, err := os.ReadFile(logPath); err == nil {
		t.Errorf("discovery spawned git %d time(s); want none", strings.Count(string(data), "called"))
	}
}

// runGit runs a git command in dir, skipping the test when git is unavailable.
// The empty global/system config keeps the developer's own git state out.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// initRepo creates a real git repository at dir with one commit, so the tree
// carries the .git directory the walk bounds on.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "README"), "repo\n")
	runGit(t, dir, "add", "README")
	runGit(t, dir, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "-m", "init")
}

// writeWorkspace gives a .beads directory the marker files discovery accepts.
func writeWorkspace(t *testing.T, beadsDir string) {
	t.Helper()
	writeFile(t, filepath.Join(beadsDir, "config.yaml"), "issue-prefix: hq\n")
	writeFile(t, filepath.Join(beadsDir, "metadata.json"), `{"backend":"dolt"}`+"\n")
}

// be-8ff: the walk stops at the nearest repo root. A workspace above the repo
// root belongs to a different project, so a cwd inside the repo with no .beads
// of its own resolves to nothing -- database and config alike.
func TestDiscover_RepoRootBoundsWalk(t *testing.T) {
	town := realTempDir(t)
	ancestorBeads := filepath.Join(town, ".beads")
	writeWorkspace(t, ancestorBeads)

	repo := filepath.Join(town, "repo")
	initRepo(t, repo)
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	if source, resolved, err := Discover(sub, FollowRedirect); err != nil || source != "" || resolved != "" {
		t.Errorf("Discover(%s) = (%q, %q, %v), want no workspace: %s is above the repo root",
			sub, source, resolved, err, ancestorBeads)
	}
	if ws, err := Resolve(sub, noEnv); !errors.Is(err, ErrNoWorkspace) {
		t.Errorf("Resolve(%s) = %+v, %v; want ErrNoWorkspace so no config comes from %s",
			sub, ws, err, ancestorBeads)
	}
}

// A nested clone is its own repo: the outer repo's .beads is above the inner
// clone's root and is not the inner project's workspace.
func TestDiscover_NestedCloneBoundsWalk(t *testing.T) {
	town := realTempDir(t)
	outer := filepath.Join(town, "outer")
	initRepo(t, outer)
	writeWorkspace(t, filepath.Join(outer, ".beads"))

	inner := filepath.Join(outer, "inner")
	initRepo(t, inner)
	sub := filepath.Join(inner, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	if source, resolved, err := Discover(sub, FollowRedirect); err != nil || source != "" || resolved != "" {
		t.Errorf("Discover(%s) = (%q, %q, %v), want no workspace: the outer .beads is above the inner clone's root",
			sub, source, resolved, err)
	}
}

// A linked worktree is bounded by the worktree root, so a workspace above the
// main checkout is out of reach, while the shared database at that main
// checkout is still found through the worktree fallback.
func TestDiscover_WorktreeRootBoundsWalk(t *testing.T) {
	town := realTempDir(t)
	writeWorkspace(t, filepath.Join(town, ".beads"))

	main := filepath.Join(town, "main")
	initRepo(t, main)
	sharedBeads := filepath.Join(main, ".beads")
	writeWorkspace(t, sharedBeads)

	worktree := filepath.Join(town, "wt")
	runGit(t, main, "worktree", "add", "-q", worktree, "HEAD")
	t.Cleanup(func() { _ = exec.Command("git", "-C", main, "worktree", "remove", "--force", worktree).Run() })
	sub := filepath.Join(worktree, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	source, resolved, err := Discover(sub, FollowRedirect)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if source != sharedBeads || resolved != sharedBeads {
		t.Errorf("Discover(%s) = (%q, %q), want the shared worktree workspace %q",
			sub, source, resolved, sharedBeads)
	}
}

// A jujutsu secondary workspace is bounded by its own root even though it sits
// inside the primary's working tree, so a workspace above the primary is out
// of reach.
func TestDiscover_JJSecondaryRootBoundsWalk(t *testing.T) {
	town := realTempDir(t)
	writeWorkspace(t, filepath.Join(town, ".beads"))

	// A colocated jj+git primary: .jj/repo is a directory.
	primary := filepath.Join(town, "primary")
	if err := os.MkdirAll(filepath.Join(primary, ".jj", "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	initRepo(t, primary)

	// The secondary points at the primary's repo directory with a file.
	secondary := filepath.Join(primary, "ws", "secondary")
	if err := os.MkdirAll(filepath.Join(secondary, ".jj"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(secondary, ".jj", "repo"), filepath.Join(primary, ".jj", "repo")+"\n")

	if source, resolved, err := Discover(secondary, FollowRedirect); err != nil || source != "" || resolved != "" {
		t.Errorf("Discover(%s) = (%q, %q, %v), want no workspace: the ancestor %s is above the primary root",
			secondary, source, resolved, err, filepath.Join(town, ".beads"))
	}
}

// Outside any repo there is no bound: the walk runs to the filesystem root,
// and an ancestor .beads is the workspace for database and config alike.
func TestDiscover_OutsideRepoWalksToFilesystemRoot(t *testing.T) {
	town := realTempDir(t)
	ancestorBeads := filepath.Join(town, ".beads")
	writeWorkspace(t, ancestorBeads)

	sub := filepath.Join(town, "not", "a", "repo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	source, resolved, err := Discover(sub, FollowRedirect)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if source != ancestorBeads || resolved != ancestorBeads {
		t.Errorf("Discover(%s) = (%q, %q), want the ancestor workspace %q",
			sub, source, resolved, ancestorBeads)
	}
	ws, err := Resolve(sub, noEnv)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ws.BeadsDir != ancestorBeads || ws.ConfigPath != filepath.Join(ancestorBeads, "config.yaml") {
		t.Errorf("Resolve(%s) = %+v, want workspace %q", sub, ws, ancestorBeads)
	}
}
