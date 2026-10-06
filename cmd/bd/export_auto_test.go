//go:build cgo

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/testutil"
	"github.com/steveyegge/beads/internal/types"
)

// TestGitAddFile_InWorktreeHook_StagesCorrectPath is a regression test for
// GH#3311: when bd's pre-commit hook calls git add with GIT_DIR inherited
// from the parent hook invocation, git defaults the work-tree to cwd and
// mis-stages the file at the root of the repo instead of under .beads/.
//
// This test verifies the file ends up staged at .beads/issues.jsonl, not
// at repo-root "issues.jsonl".
func TestGitAddFile_InWorktreeHook_StagesCorrectPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir, err := os.MkdirTemp("", "bd-gh3311-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Resolve symlinks so toplevel comparisons below match git's canonical view
	// (on macOS /var -> /private/var).
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	mainRepo := filepath.Join(tmpDir, "main")
	if err := os.MkdirAll(mainRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
		}
	}
	runGit(mainRepo, "init", "-q")
	runGit(mainRepo, "config", "user.email", "t@t")
	runGit(mainRepo, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(mainRepo, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(mainRepo, "add", "README.md")
	runGit(mainRepo, "commit", "-qm", "init")

	worktree := filepath.Join(tmpDir, "wt")
	runGit(mainRepo, "worktree", "add", worktree, "-b", "feat")
	t.Cleanup(func() {
		c := exec.Command("git", "worktree", "remove", "--force", worktree)
		c.Dir = mainRepo
		_ = c.Run()
	})

	beadsDir := filepath.Join(worktree, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(beadsDir, "issues.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(`{"id":"x"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate the environment inside a git pre-commit hook: GIT_DIR points
	// at the worktree's per-worktree gitdir.
	out, err := exec.Command("git", "-C", worktree, "rev-parse", "--git-dir").Output()
	if err != nil {
		t.Fatal(err)
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktree, gitDir)
	}
	if gitDir, err = filepath.EvalSymlinks(gitDir); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_DIR", gitDir)

	// Call the function under test from a state that matches the hook
	// subprocess: cwd not particularly interesting here, but gitAddFile sets
	// cmd.Dir = filepath.Dir(path) internally.
	t.Chdir(worktree)
	if err := gitAddFile(jsonlPath); err != nil {
		t.Fatalf("gitAddFile: %v", err)
	}

	// Inspect the worktree's index: the staged path must be ".beads/issues.jsonl",
	// NOT bare "issues.jsonl" at repo root.
	lsFiles := exec.Command("git", "ls-files", "--stage")
	lsFiles.Dir = worktree
	data, err := lsFiles.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-files: %v\n%s", err, data)
	}
	staged := string(data)
	if !strings.Contains(staged, ".beads/issues.jsonl") {
		t.Errorf("expected .beads/issues.jsonl to be staged, got:\n%s", staged)
	}
	// Regression guard: the pre-fix bug stages bare "issues.jsonl" at the root.
	for _, line := range strings.Split(strings.TrimSpace(staged), "\n") {
		// Each line is "<mode> <sha> <stage>\t<path>"
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		if parts[1] == "issues.jsonl" {
			t.Errorf("regression: issues.jsonl staged at repo root (GH#3311):\n%s", staged)
		}
	}
}

// TestScrubGitHookEnv verifies that the env-scrubbing helper drops exactly
// the git-hook-injected variables that would otherwise poison `git add`'s
// repo auto-discovery (or divert its object writes / config).
func TestScrubGitHookEnv(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"GIT_DIR=/some/.git",
		"GIT_WORK_TREE=/some",
		"GIT_INDEX_FILE=/some/.git/index",
		"GIT_COMMON_DIR=/some/.git",
		"GIT_PREFIX=sub/",
		"GIT_OBJECT_DIRECTORY=/some/.git/objects",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=/elsewhere/.git/objects",
		"GIT_CEILING_DIRECTORIES=/home",
		"GIT_DISCOVERY_ACROSS_FILESYSTEM=1",
		"GIT_CONFIG=/etc/some.conf",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=core.worktree",
		"GIT_CONFIG_VALUE_0=/elsewhere",
		"GIT_CONFIG_PARAMETERS='core.worktree=/elsewhere'",
		"GIT_CONFIG_GLOBAL=/tmp/gcfg",
		"GIT_CONFIG_SYSTEM=/tmp/scfg",
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME=/home/u",
		// Non-discovery vars that must pass through.
		"GIT_AUTHOR_NAME=kept",
		"GIT_COMMITTER_EMAIL=kept@example.com",
		"GIT_EDITOR=vim",
		"GIT_PAGER=less",
	}
	out := scrubGitHookEnv(in)
	joined := strings.Join(out, "\n")
	banned := []string{
		"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE=", "GIT_COMMON_DIR=",
		"GIT_PREFIX=", "GIT_OBJECT_DIRECTORY=", "GIT_ALTERNATE_OBJECT_DIRECTORIES=",
		"GIT_CEILING_DIRECTORIES=", "GIT_DISCOVERY_ACROSS_FILESYSTEM=",
		"GIT_CONFIG=", "GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_0=", "GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_PARAMETERS=", "GIT_CONFIG_GLOBAL=", "GIT_CONFIG_SYSTEM=", "GIT_CONFIG_NOSYSTEM=",
	}
	for _, b := range banned {
		if strings.Contains(joined, b) {
			t.Errorf("scrubGitHookEnv leaked %s\nresult:\n%s", b, joined)
		}
	}
	kept := []string{
		"PATH=/usr/bin", "HOME=/home/u",
		"GIT_AUTHOR_NAME=kept", "GIT_COMMITTER_EMAIL=kept@example.com",
		"GIT_EDITOR=vim", "GIT_PAGER=less",
	}
	for _, k := range kept {
		if !strings.Contains(joined, k) {
			t.Errorf("scrubGitHookEnv dropped %s\nresult:\n%s", k, joined)
		}
	}
}

func TestShouldRunPostCommandAutoExportSkipsReadOnlyCommands(t *testing.T) {
	if shouldRunPostCommandAutoExport(&cobra.Command{Use: "search"}) {
		t.Fatal("search is read-only and must not trigger post-command auto-export")
	}
	if !shouldRunPostCommandAutoExport(&cobra.Command{Use: "create"}) {
		t.Fatal("write commands should still trigger post-command auto-export")
	}
}

// fakeStateHashStore implements the storage.StateHasher optional interface
// plus the minimal DoltStorage surface maybeAutoExport touches. Any
// non-overridden method panics via the embedded nil interface.
type fakeStateHashStore struct {
	storage.DoltStorage
	stateHash          string
	stateHashCalls     int
	currentCommitCalls int
	issues             []*types.Issue
}

func (f *fakeStateHashStore) GetStateHash(_ context.Context) (string, error) {
	f.stateHashCalls++
	return f.stateHash, nil
}

func (f *fakeStateHashStore) GetCurrentCommit(_ context.Context) (string, error) {
	f.currentCommitCalls++
	return "head-commit-hash", nil
}

func (f *fakeStateHashStore) GetInfraTypes(_ context.Context) map[string]bool { return nil }

// GetConfig lets buildOwnerExcludeSet's database fallback lookup (for
// export.exclude_owners / export.exclude_owner) run without panicking;
// this fake has no config store, so every key is unset.
func (f *fakeStateHashStore) GetConfig(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (f *fakeStateHashStore) SearchIssues(_ context.Context, _ string, _ types.IssueFilter) ([]*types.Issue, error) {
	return f.issues, nil
}

// fakeHeadOnlyStore does NOT implement storage.StateHasher, forcing the
// GetCurrentCommit fallback.
type fakeHeadOnlyStore struct {
	storage.DoltStorage
	currentCommitCalls int
	issues             []*types.Issue
}

func (f *fakeHeadOnlyStore) GetCurrentCommit(_ context.Context) (string, error) {
	f.currentCommitCalls++
	return "head-commit-hash", nil
}

func (f *fakeHeadOnlyStore) GetInfraTypes(_ context.Context) map[string]bool { return nil }

// GetConfig lets buildOwnerExcludeSet's database fallback lookup (for
// export.exclude_owners / export.exclude_owner) run without panicking; this
// fake has no config store, so every key is unset.
func (f *fakeHeadOnlyStore) GetConfig(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (f *fakeHeadOnlyStore) SearchIssues(_ context.Context, _ string, _ types.IssueFilter) ([]*types.Issue, error) {
	return f.issues, nil
}

// The four bulk-relation loaders below let exportToFile's full-export path
// (maybeAutoExport's fallback when incremental isn't available) run to
// completion against this fake: it unconditionally calls all of them
// whenever SearchIssues returns at least one issue.
func (f *fakeHeadOnlyStore) GetLabelsForIssues(_ context.Context, _ []string) (map[string][]string, error) {
	return nil, nil
}

func (f *fakeHeadOnlyStore) GetDependencyRecordsForIssues(_ context.Context, _ []string) (map[string][]*types.Dependency, error) {
	return nil, nil
}

func (f *fakeHeadOnlyStore) GetCommentsForIssues(_ context.Context, _ []string) (map[string][]*types.Comment, error) {
	return nil, nil
}

func (f *fakeHeadOnlyStore) GetCommentCounts(_ context.Context, _ []string) (map[string]int, error) {
	return nil, nil
}

func (f *fakeHeadOnlyStore) GetDependencyCounts(_ context.Context, _ []string) (map[string]*types.DependencyCounts, error) {
	return nil, nil
}

// spyDiffStore wraps a real DoltStorage and counts ChangedIssueIDs calls,
// while delegating every method — including the optional StateHasher/
// DiffStore capability interfaces — to the wrapped store. It does NOT
// implement storage.Unwrapper, so storage.UnwrapStore returns the spy
// itself unchanged: maybeAutoExport's internal
// storage.UnwrapStore(store).(storage.DiffStore) / .(storage.StateHasher)
// type-assertions land on the spy's own overrides below, letting a test
// observe whether the real dolt_diff-backed incremental path actually ran,
// end-to-end through maybeAutoExport, against a real dolt server.
type spyDiffStore struct {
	storage.DoltStorage
	changedIssueIDsCalls int
}

func (s *spyDiffStore) ChangedIssueIDs(ctx context.Context, fromCommit, toCommit string) (storage.ChangedIssueIDs, error) {
	s.changedIssueIDsCalls++
	ds, ok := storage.UnwrapStore(s.DoltStorage).(storage.DiffStore)
	if !ok {
		return storage.ChangedIssueIDs{}, fmt.Errorf("spyDiffStore: wrapped store does not implement DiffStore")
	}
	return ds.ChangedIssueIDs(ctx, fromCommit, toCommit)
}

func (s *spyDiffStore) GetStateHash(ctx context.Context) (string, error) {
	sh, ok := storage.UnwrapStore(s.DoltStorage).(storage.StateHasher)
	if !ok {
		return s.DoltStorage.GetCurrentCommit(ctx)
	}
	return sh.GetStateHash(ctx)
}

func autoExportTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"database":"beads","backend":"dolt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("BEADS_DIR", beadsDir)
	return dir
}

// Regression test for wy-4ope: server-mode clients used to skip auto-export
// entirely, so `git push` published stale JSONL. maybeAutoExport must reach
// change detection and consult the working-set-aware state hash, not HEAD —
// server mode runs with dolt auto-commit off, so HEAD does not advance on
// writes.
func TestMaybeAutoExportUsesStateHashForChangeDetection(t *testing.T) {
	initConfigForTest(t)
	config.Set("export.auto", true)

	saveAndRestoreGlobals(t)
	fake := &fakeStateHashStore{stateHash: "working-set-hash"}
	store = fake

	dir := autoExportTestDir(t)
	saveExportAutoState(filepath.Join(dir, ".beads"), &exportAutoState{
		LastDoltCommit: "working-set-hash",
		Timestamp:      time.Now(),
	})

	if err := maybeAutoExport(context.Background(), false); err != nil {
		t.Fatalf("maybeAutoExport: %v", err)
	}

	if fake.stateHashCalls != 1 {
		t.Fatalf("GetStateHash calls = %d, want 1", fake.stateHashCalls)
	}
	if fake.currentCommitCalls != 0 {
		t.Fatalf("GetCurrentCommit calls = %d, want 0 (StateHasher must take precedence)", fake.currentCommitCalls)
	}
	if _, err := os.Stat(filepath.Join(dir, ".beads", "issues.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("unchanged state hash must not export, stat err=%v", err)
	}
}

func TestMaybeAutoExportExportsOnStateHashChange(t *testing.T) {
	initConfigForTest(t)
	config.Set("export.auto", true)

	saveAndRestoreGlobals(t)
	fake := &fakeStateHashStore{stateHash: "hash-after-write"}
	store = fake

	dir := autoExportTestDir(t)
	saveExportAutoState(filepath.Join(dir, ".beads"), &exportAutoState{
		LastDoltCommit: "hash-before-write",
		Timestamp:      time.Time{}, // zero: throttle window open
	})

	if err := maybeAutoExport(context.Background(), false); err != nil {
		t.Fatalf("maybeAutoExport: %v", err)
	}

	state := loadExportAutoState(filepath.Join(dir, ".beads"))
	if state.LastDoltCommit != "hash-after-write" {
		t.Fatalf("state LastDoltCommit = %q, want %q (export must run when the state hash moves)",
			state.LastDoltCommit, "hash-after-write")
	}
}

func TestMaybeAutoExportFallsBackToHeadCommit(t *testing.T) {
	initConfigForTest(t)
	config.Set("export.auto", true)

	saveAndRestoreGlobals(t)
	fake := &fakeHeadOnlyStore{}
	store = fake

	dir := autoExportTestDir(t)
	saveExportAutoState(filepath.Join(dir, ".beads"), &exportAutoState{
		LastDoltCommit: "head-commit-hash",
		Timestamp:      time.Now(),
	})

	if err := maybeAutoExport(context.Background(), false); err != nil {
		t.Fatalf("maybeAutoExport: %v", err)
	}

	if fake.currentCommitCalls != 1 {
		t.Fatalf("GetCurrentCommit calls = %d, want 1 (fallback when StateHasher is absent)", fake.currentCommitCalls)
	}
}

func TestGuardAutoExportOverwriteAllowsViewerScopedJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issues.jsonl")
	writeJSONLLines(t, path,
		map[string]any{"_type": "issue", "id": "bd-1", "issue_type": "task", "title": "kept"},
		map[string]any{"id": "bd-legacy", "issue_type": "bug", "title": "legacy issue record"},
	)

	if err := guardAutoExportOverwrite(path, map[string]bool{"agent": true}, false, nil); err != nil {
		t.Fatalf("guardAutoExportOverwrite: %v", err)
	}
}

// TestGuardAutoExportOverwriteBlocksRicherJSONL is #4069's regression test:
// infra/template rows that are STILL IN THE STORE must block the shrink
// guard, or the next auto-export silently overwrites the richer JSONL with
// the filtered subset (GH#4069 — 89% data loss in the reporter's workspace).
// bd-wisp is out-of-scope AND absent from the store (a compacted wisp,
// GH#4988's actual bug) and must NOT count toward the block — see the
// complement test TestGuardAutoExportOverwriteAllowsStaleEphemeralWisp,
// which isolates that half of the rule on its own.
func TestGuardAutoExportOverwriteBlocksRicherJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issues.jsonl")
	writeJSONLLines(t, path,
		map[string]any{"_type": "issue", "id": "bd-1", "issue_type": "task", "title": "kept"},
		map[string]any{"_type": "memory", "key": "keep-me", "value": "private context"},
		map[string]any{"_type": "issue", "id": "bd-agent", "issue_type": "agent", "title": "infra"},
		map[string]any{"_type": "issue", "id": "bd-template", "issue_type": "task", "is_template": true},
		map[string]any{"_type": "issue", "id": "bd-wisp", "issue_type": "task", "ephemeral": true},
		map[string]any{"_type": "event", "id": "bd-event"},
	)
	// bd-agent and bd-template are still in the store (the #4069 scenario);
	// bd-wisp has been compacted out of the store already (the #4988
	// scenario) and is deliberately absent here.
	storeIDs := map[string]struct{}{
		"bd-1":        {},
		"bd-agent":    {},
		"bd-template": {},
	}

	err := guardAutoExportOverwrite(path, map[string]bool{"agent": true}, false, storeIDs)
	if err == nil {
		t.Fatal("expected guardAutoExportOverwrite to reject richer JSONL, got nil")
	}
	msg := err.Error()
	for _, want := range []string{
		"refusing to overwrite",
		"4 record(s) outside auto-export scope",
		"1 memories",
		"2 infra/template/ephemeral issues",
		"1 unknown",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("guard error %q does not contain %q", msg, want)
		}
	}
}

// TestGuardAutoExportOverwriteAllowsStaleEphemeralWisp is the complement of
// TestGuardAutoExportOverwriteBlocksRicherJSONL: an out-of-scope row that is
// ALSO absent from the store (compacted away — GH#4988) does not block the
// rewrite, because nothing extra is lost versus what Dolt already lost.
func TestGuardAutoExportOverwriteAllowsStaleEphemeralWisp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issues.jsonl")
	writeJSONLLines(t, path,
		map[string]any{"_type": "issue", "id": "bd-1", "issue_type": "task", "title": "kept"},
		map[string]any{"_type": "issue", "id": "bd-wisp", "issue_type": "task", "ephemeral": true, "title": "stale wisp"},
	)
	// bd-1 is in the store; bd-wisp has been compacted away — not present.
	storeIDs := map[string]struct{}{"bd-1": {}}

	if err := guardAutoExportOverwrite(path, map[string]bool{"agent": true}, false, storeIDs); err != nil {
		t.Fatalf("stale ephemeral-only richer JSONL should be rewritable: %v", err)
	}
}

func TestIssueRecordsInJSONL_SkipsTombstoneAndNonIssue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issues.jsonl")
	writeJSONLLines(t, path,
		map[string]any{"_type": "issue", "id": "bd-1", "issue_type": "task", "ephemeral": false},
		map[string]any{"_type": "issue", "id": "bd-wisp", "issue_type": "task", "ephemeral": true},
		map[string]any{"_type": "issue", "id": "bd-gone", "status": "tombstone"},
		map[string]any{"_type": "memory", "key": "k", "value": "v"},
	)
	recs, err := issueRecordsInJSONL(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2 (tombstone+memory skipped): %+v", len(recs), recs)
	}
	byID := map[string]jsonlIssueRecord{}
	for _, r := range recs {
		byID[r.ID] = r
	}
	if !byID["bd-wisp"].Ephemeral {
		t.Fatal("expected bd-wisp ephemeral=true")
	}
}

func TestGuardAutoExportOverwriteAllowsMemoriesWhenIncluded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issues.jsonl")
	writeJSONLLines(t, path,
		map[string]any{"_type": "memory", "key": "keep-me", "value": "private context"},
	)

	if err := guardAutoExportOverwrite(path, nil, true, nil); err != nil {
		t.Fatalf("guardAutoExportOverwrite with memories included: %v", err)
	}
}

func writeJSONLLines(t *testing.T, path string, records ...map[string]any) {
	t.Helper()
	var b strings.Builder
	for _, rec := range records {
		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPathInsideDir covers the common structural cases plus the
// fresh-file + symlinked-parent case that tripped the initial fix
// (macOS /tmp -> /private/tmp asymmetry when the target file doesn't
// yet exist).
func TestPathInsideDir(t *testing.T) {
	tmpRaw, err := os.MkdirTemp("", "bd-pathinside-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpRaw) })

	// Provoke a symlinked-parent asymmetry: keep `raw` as the un-resolved
	// tmp form (/tmp/...) and derive `real` as the canonical form
	// (/private/tmp/...) so tests can compare across the boundary.
	real, err := filepath.EvalSymlinks(tmpRaw)
	if err != nil {
		t.Fatal(err)
	}

	wt := filepath.Join(real, "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	wtRaw := filepath.Join(tmpRaw, "wt") // un-resolved view of same dir

	existing := filepath.Join(wt, "existing.txt")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		dir  string
		want bool
	}{
		{"identical paths", wt, wt, true},
		{"existing descendant", existing, wt, true},
		{"fresh nonexistent descendant", filepath.Join(wt, "not-yet.txt"), wt, true},
		{"sibling path with shared prefix", filepath.Join(real, "wt-other/x"), wt, false},
		{"outside dir", filepath.Join(real, "elsewhere/x"), wt, false},
		// The regression: fresh path expressed via /tmp symlink vs dir
		// expressed via /private/tmp canonical. Must still say "inside".
		{"fresh path with symlinked parent form", filepath.Join(wtRaw, "fresh.txt"), wt, true},
		{"existing path with symlinked parent form", filepath.Join(wtRaw, "existing.txt"), wt, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pathInsideDir(tc.path, tc.dir)
			if got != tc.want {
				t.Errorf("pathInsideDir(%q, %q) = %v, want %v", tc.path, tc.dir, got, tc.want)
			}
		})
	}
}

// TestHookWorkTreeRoot covers the documented GIT_DIR shapes and the
// not-a-hook case.
func TestHookWorkTreeRoot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bd-hwt-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Case 1: GIT_DIR not set → "" (normal non-hook context).
	if err := os.Unsetenv("GIT_DIR"); err != nil {
		t.Fatal(err)
	}
	if got := hookWorkTreeRoot(); got != "" {
		t.Errorf("with GIT_DIR unset: hookWorkTreeRoot = %q, want \"\"", got)
	}

	// Case 2: linked-worktree style — GIT_DIR = main/.git/worktrees/<n>,
	// and that dir contains a `gitdir` file pointing at the worktree's
	// .git file. Worktree root = parent of that .git file.
	wtDotGit := filepath.Join(tmpDir, "wt", ".git")
	if err := os.MkdirAll(filepath.Dir(wtDotGit), 0o755); err != nil {
		t.Fatal(err)
	}
	linkedGitDir := filepath.Join(tmpDir, "main", ".git", "worktrees", "wt")
	if err := os.MkdirAll(linkedGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linkedGitDir, "gitdir"), []byte(wtDotGit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", linkedGitDir)
	if got, want := hookWorkTreeRoot(), filepath.Dir(wtDotGit); got != want {
		t.Errorf("linked worktree: hookWorkTreeRoot = %q, want %q", got, want)
	}

	// Case 3: plain repo — GIT_DIR = <repo>/.git. Worktree root is its parent.
	plainGitDir := filepath.Join(tmpDir, "plain", ".git")
	if err := os.MkdirAll(plainGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", plainGitDir)
	if got, want := hookWorkTreeRoot(), filepath.Dir(plainGitDir); got != want {
		t.Errorf("plain repo: hookWorkTreeRoot = %q, want %q", got, want)
	}

	// Case 4: unrecognized shape (no gitdir file, basename != .git) → "".
	// Bare-repo-ish; we conservatively decline to identify a worktree.
	bare := filepath.Join(tmpDir, "bare.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", bare)
	if got := hookWorkTreeRoot(); got != "" {
		t.Errorf("bare/unrecognized GIT_DIR: hookWorkTreeRoot = %q, want \"\"", got)
	}
}

// TestGitAddFile_NonHookContext_GuardDoesNotFire verifies the worktree
// guard is a no-op when GIT_DIR is not set (normal bd invocation, not
// inside a git hook). Regression guard so a future tightening of
// hookWorkTreeRoot does not silently break the common path.
func TestGitAddFile_NonHookContext_GuardDoesNotFire(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir, err := os.MkdirTemp("", "bd-nonhook-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "t@t")
	runGit("config", "user.name", "t")

	target := filepath.Join(repo, ".beads", "issues.jsonl")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"id":"x"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Unsetenv("GIT_DIR"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)
	if err := gitAddFile(target); err != nil {
		t.Fatalf("gitAddFile: %v", err)
	}

	c := exec.Command("git", "ls-files", "--stage")
	c.Dir = repo
	data, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("ls-files: %v\n%s", err, data)
	}
	if !strings.Contains(string(data), ".beads/issues.jsonl") {
		t.Errorf("non-hook path did not stage .beads/issues.jsonl:\n%s", data)
	}
}

// TestGitAddFile_RelativePathDoesNotDoubleRoot is a regression test for
// GH#4351: gitAddFile sets cmd.Dir to filepath.Dir(path). When path is
// relative (e.g. ".beads/issues.jsonl"), passing that full path as the
// git-add argument becomes `cd .beads && git add .beads/issues.jsonl`,
// which looks for a non-existent nested path and exits 128. The fix is
// to pass filepath.Base(path) so the pathspec is relative to cmd.Dir.
func TestGitAddFile_RelativePathDoesNotDoubleRoot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir, err := os.MkdirTemp("", "bd-gh4351-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "t@t")
	runGit("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README.md")
	runGit("commit", "-qm", "init")

	if err := os.MkdirAll(filepath.Join(repo, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".beads", "issues.jsonl"), []byte(`{"id":"x"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Unsetenv("GIT_DIR"); err != nil {
		t.Fatal(err)
	}
	// Repo-relative path — the double-root trigger. Must stage successfully.
	t.Chdir(repo)
	relPath := filepath.Join(".beads", "issues.jsonl")
	if err := gitAddFile(relPath); err != nil {
		t.Fatalf("gitAddFile(%q): %v", relPath, err)
	}

	c := exec.Command("git", "diff", "--cached", "--name-only")
	c.Dir = repo
	data, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git diff --cached: %v\n%s", err, data)
	}
	staged := strings.TrimSpace(string(data))
	if !strings.Contains(staged, ".beads/issues.jsonl") && !strings.Contains(staged, filepath.ToSlash(relPath)) {
		t.Errorf("expected .beads/issues.jsonl staged, got: %q", staged)
	}
}

// TestGitAddFile_CapturesStderrOnFailure verifies that when `git add` fails,
// the returned error wraps git's stderr text instead of just the bare exit
// status. Regression guard for the silent "Warning: auto-export: git add
// failed: exit status 1" noise where the user has no signal as to why.
func TestGitAddFile_CapturesStderrOnFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir, err := os.MkdirTemp("", "bd-stderr-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "t@t")
	runGit("config", "user.name", "t")

	// Force git add to fail by gitignoring the target. Common real-world
	// trigger: a parent .gitignore excluding .beads/ that the user is
	// unaware of.
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".beads/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(repo, ".beads", "issues.jsonl")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"id":"x"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Unsetenv("GIT_DIR"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)

	err = gitAddFile(target)
	if err == nil {
		t.Fatal("expected gitAddFile to fail on gitignored target, got nil")
	}
	msg := err.Error()
	// Bare-exit-status regression guard: pre-fix message was just "exit
	// status 1" with nothing else. Post-fix must include git's stderr.
	if !strings.Contains(strings.ToLower(msg), "ignored") {
		t.Errorf("expected error to surface git's stderr (containing 'ignored'), got: %q", msg)
	}
}

// TestGitAddFile_CapturesLockedIndexFailure verifies that a locked git index
// is surfaced as a rich, caller-visible error rather than a bare exit status.
func TestGitAddFile_CapturesLockedIndexFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir, err := os.MkdirTemp("", "bd-index-lock-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "t@t")
	runGit("config", "user.name", "t")

	target := filepath.Join(repo, ".beads", "issues.jsonl")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"id":"x"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	lockPath := filepath.Join(repo, ".git", "index.lock")
	if err := os.WriteFile(lockPath, []byte("held by another git process"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Unsetenv("GIT_DIR"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(repo)

	err = gitAddFile(target)
	if err == nil {
		t.Fatal("expected gitAddFile to fail while index.lock exists, got nil")
	}
	if msg := err.Error(); !strings.Contains(msg, "index is locked") || !strings.Contains(msg, "index.lock") {
		t.Fatalf("expected index.lock error, got: %q", msg)
	}

	c := exec.Command("git", "ls-files", "--stage")
	c.Dir = repo
	data, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("ls-files: %v\n%s", err, data)
	}
	if strings.Contains(string(data), ".beads/issues.jsonl") {
		t.Fatalf("gitAddFile staged target despite index.lock:\n%s", data)
	}
}

// TestGitAddFile_RedirectCase_DoesNotStageInMainRepo regresses the
// silent-stage-in-main follow-up from the GH#3311 review: when a worktree
// has .beads/redirect -> main/.beads, the worktree's pre-commit hook must
// NOT stage the redirected path into main's index. That would silently
// pollute a repo the user did not tell us to touch. Expected behavior is
// to skip staging entirely (the file content on disk is still correct).
func TestGitAddFile_RedirectCase_DoesNotStageInMainRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir, err := os.MkdirTemp("", "bd-gh3311-redirect-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	mainRepo := filepath.Join(tmpDir, "main")
	if err := os.MkdirAll(mainRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(dir string, args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
		}
	}
	runGit(mainRepo, "init", "-q")
	runGit(mainRepo, "config", "user.email", "t@t")
	runGit(mainRepo, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(mainRepo, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(mainRepo, "add", "README.md")
	runGit(mainRepo, "commit", "-qm", "init")

	// Create main's .beads directory with an issues.jsonl the hook would
	// target via the redirect.
	mainBeads := filepath.Join(mainRepo, ".beads")
	if err := os.MkdirAll(mainBeads, 0o755); err != nil {
		t.Fatal(err)
	}
	mainJSONL := filepath.Join(mainBeads, "issues.jsonl")
	if err := os.WriteFile(mainJSONL, []byte(`{"id":"from-redirect"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create worktree; GIT_DIR env var simulation captures the hook context.
	worktree := filepath.Join(tmpDir, "wt")
	runGit(mainRepo, "worktree", "add", worktree, "-b", "feat")
	t.Cleanup(func() {
		c := exec.Command("git", "worktree", "remove", "--force", worktree)
		c.Dir = mainRepo
		_ = c.Run()
	})

	out, err := exec.Command("git", "-C", worktree, "rev-parse", "--git-dir").Output()
	if err != nil {
		t.Fatal(err)
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(worktree, gitDir)
	}
	if gitDir, err = filepath.EvalSymlinks(gitDir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", gitDir)

	// Act: stage the main-repo-resident path from inside the worktree hook.
	t.Chdir(worktree)
	if err := gitAddFile(mainJSONL); err != nil {
		t.Fatalf("gitAddFile: %v", err)
	}

	// Assert: neither the worktree's index nor main's index got a bogus
	// staging entry from the worktree's hook firing.
	checkNoStage := func(label, repoDir string) {
		t.Helper()
		c := exec.Command("git", "ls-files", "--stage")
		c.Dir = repoDir
		data, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: ls-files: %v\n%s", label, err, data)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				continue
			}
			if strings.Contains(parts[1], "issues.jsonl") {
				t.Errorf("%s staged issues.jsonl when it should not have; ls-files output:\n%s", label, data)
			}
		}
	}
	// Both checks use env with GIT_DIR unset so we observe each repo's
	// own index rather than routing through the inherited hook gitdir.
	// t.Setenv can only set (not unset); the outer Setenv of GIT_DIR has
	// a Cleanup that restores it, so unsetting here is safe for the rest
	// of this test and the outer cleanup will re-set if another test
	// relies on the parent env.
	if err := os.Unsetenv("GIT_DIR"); err != nil {
		t.Fatal(err)
	}
	checkNoStage("worktree", worktree)
	checkNoStage("main", mainRepo)
}

func TestPreCommitHasStagedBeadsFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir, err := os.MkdirTemp("", "bd-staged-beads-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	repo := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "t@t")
	runGit("config", "user.name", "t")

	readme := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readme, []byte("code\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README.md")
	if preCommitHasStagedBeadsFiles(filepath.Join(repo, ".beads")) {
		t.Fatal("staged non-.beads file should not trigger pre-commit JSONL export")
	}

	configPath := filepath.Join(repo, ".beads", "config.yaml")
	if err := os.WriteFile(configPath, []byte("export:\n  auto: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".beads/config.yaml")
	if !preCommitHasStagedBeadsFiles(filepath.Join(repo, ".beads")) {
		t.Fatal("staged .beads file should trigger pre-commit JSONL export")
	}
}

func TestCommandAllowsEmptyAutoExport(t *testing.T) {
	commandMayEmptyJSONLExport.Store(false)
	t.Cleanup(func() { commandMayEmptyJSONLExport.Store(false) })

	if commandAllowsEmptyAutoExport(&cobra.Command{Use: "prune"}) {
		t.Fatal("prune should not allow an empty auto-export before deleting rows")
	}

	commandMayEmptyJSONLExport.Store(true)
	if !commandAllowsEmptyAutoExport(&cobra.Command{Use: "prune"}) {
		t.Fatal("prune should allow an intentional empty auto-export")
	}
	if !commandAllowsEmptyAutoExport(&cobra.Command{Use: "purge"}) {
		t.Fatal("purge should allow an intentional empty auto-export")
	}
	if commandAllowsEmptyAutoExport(&cobra.Command{Use: "create"}) {
		t.Fatal("create should not bypass empty auto-export protection")
	}
}

// TestShouldExport covers the pure throttle-window decision used by
// maybeAutoExport. Adapted from Jeremy Longshore's GH#4061 refactor.
func TestShouldExport(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		state    *exportAutoState
		interval time.Duration
		want     bool
	}{
		{
			name:     "first run always exports",
			state:    &exportAutoState{},
			interval: time.Minute,
			want:     true,
		},
		{
			name:     "throttle window active blocks",
			state:    &exportAutoState{Timestamp: now.Add(-10 * time.Second)},
			interval: time.Minute,
			want:     false,
		},
		{
			name:     "throttle window elapsed allows",
			state:    &exportAutoState{Timestamp: now.Add(-2 * time.Minute)},
			interval: time.Minute,
			want:     true,
		},
		{
			name:     "at interval boundary allows",
			state:    &exportAutoState{Timestamp: now.Add(-time.Minute)},
			interval: time.Minute,
			want:     true,
		},
		{
			name:     "zero interval allows",
			state:    &exportAutoState{Timestamp: now.Add(-time.Microsecond)},
			interval: 0,
			want:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldExport(tc.state, tc.interval); got != tc.want {
				t.Errorf("shouldExport(%+v, %s) = %v, want %v", tc.state, tc.interval, got, tc.want)
			}
		})
	}
}

func TestCountIssueRecordsInJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "issues.jsonl")
	data := strings.Join([]string{
		`{"_type":"issue","id":"bd-1","status":"open"}`,
		`{"id":"bd-2","status":"closed"}`,
		`{"_type":"issue","id":"bd-2","status":"closed"}`,
		`{"_type":"memory","key":"note","value":"private"}`,
		`{"_type":"issue","id":"bd-3","status":"tombstone"}`,
		`{"_type":"issue","title":"missing id"}`,
		``,
	}, "\n")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := countIssueRecordsInJSONL(path)
	if err != nil {
		t.Fatalf("countIssueRecordsInJSONL: %v", err)
	}
	if got != 2 {
		t.Fatalf("countIssueRecordsInJSONL = %d, want 2", got)
	}

	ids, err := issueIDsInJSONL(path)
	if err != nil {
		t.Fatalf("issueIDsInJSONL: %v", err)
	}
	if got := strings.Join(ids, ","); got != "bd-1,bd-2" {
		t.Fatalf("issueIDsInJSONL = %q, want bd-1,bd-2", got)
	}
}

func autoExportDataLossTestEnv(home string) []string {
	env := make([]string, 0, len(os.Environ())+3)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "BEADS_") {
			continue
		}
		env = append(env, e)
	}
	// BEADS_TEST_SERVER lets bd connect to the shared test Dolt server that
	// serverInitArgs points init at; without it the guard refuses test
	// databases on any server.
	return append(env, "HOME="+home, "BEADS_DOLT_AUTO_START=0", "BEADS_NO_DAEMON=1", "BD_DISABLE_METRICS=1", "BD_DISABLE_EVENT_FLUSH=1", "BEADS_TEST_SERVER=1")
}

// ---------------------------------------------------------------------------
// Unit tests — no Dolt required.
// ---------------------------------------------------------------------------

func TestOrderedIssueLines_PreservesInsertionOrderAndReplacesInPlace(t *testing.T) {
	o := newOrderedIssueLines()
	o.set("a", []byte(`{"id":"a","v":1}`))
	o.set("b", []byte(`{"id":"b","v":1}`))
	o.set("c", []byte(`{"id":"c","v":1}`))
	// Replace b in-place — must NOT move it to the end.
	o.set("b", []byte(`{"id":"b","v":2}`))
	// Remove a.
	o.remove("a")
	// Add d — appended at the end.
	o.set("d", []byte(`{"id":"d","v":1}`))

	var got []string
	o.each(func(id string, line []byte) {
		got = append(got, string(line))
	})
	want := []string{
		`{"id":"b","v":2}`,
		`{"id":"c","v":1}`,
		`{"id":"d","v":1}`,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoadExistingIssueLines_ParsesIssuesPreservesMemories(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "issues.jsonl")
	content := strings.Join([]string{
		`{"id":"one","title":"first"}`,
		`{"id":"two","title":"second","comments":[{"id":"c1","text":"hi"}]}`,
		`{"_type":"memory","key":"k","value":"v"}`,
		`   `,
		`not valid json`,
		`{"id":"","title":"empty id"}`,
		`{"id":"three","title":"third"}`,
		``,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	lines, err := loadExistingIssueLines(path)
	if err != nil {
		t.Fatalf("loadExistingIssueLines: %v", err)
	}

	var ids []string
	lines.each(func(id string, _ []byte) { ids = append(ids, id) })
	wantIDs := []string{"one", "two", "three"}
	if len(ids) != len(wantIDs) {
		t.Fatalf("got ids %v, want %v", ids, wantIDs)
	}
	for i, id := range wantIDs {
		if ids[i] != id {
			t.Errorf("order[%d] = %q, want %q", i, ids[i], id)
		}
	}

	// Memories must NOT be mixed into the issue-line ordering (a memory record
	// has no issue "id" to key on), but they also must not be silently
	// dropped: an incremental rewrite that never regenerates memories from
	// live config depends on loadExistingIssueLines carrying pre-existing
	// memory lines forward verbatim via memoryLines.
	lines.each(func(_ string, line []byte) {
		if strings.Contains(string(line), `"_type":"memory"`) {
			t.Errorf("memory record leaked into issue lines: %s", line)
		}
	})
	if len(lines.memoryLines) != 1 {
		t.Fatalf("got %d preserved memory lines, want 1: %v", len(lines.memoryLines), lines.memoryLines)
	}
	if !strings.Contains(string(lines.memoryLines[0]), `"key":"k"`) {
		t.Errorf("preserved memory line = %s, want the original memory record verbatim", lines.memoryLines[0])
	}

	// Comment records have an "id" field too — confirm we grabbed the
	// OUTER id (which the json.Unmarshal probe does correctly because
	// Go's decoder overwrites repeated keys from left to right, and the
	// issue's "id" is first in the object).
	var two []byte
	lines.each(func(id string, line []byte) {
		if id == "two" {
			two = line
		}
	})
	if two == nil {
		t.Fatal("issue 'two' not loaded")
	}
	if !strings.Contains(string(two), `"comments":[{"id":"c1"`) {
		t.Errorf("comments not preserved verbatim: %s", two)
	}
}

func TestLoadExistingIssueLines_MissingFileReturnsEmpty(t *testing.T) {
	lines, err := loadExistingIssueLines(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	called := false
	lines.each(func(_ string, _ []byte) { called = true })
	if called {
		t.Error("empty set yielded entries")
	}
}

// ---------------------------------------------------------------------------
// Integration tests — require the shared Dolt test server.
// ---------------------------------------------------------------------------

// setupIncrementalExportTest wires a fresh store, puts the cwd in a temp
// beads dir, and registers cleanup. Returns the store, beads dir, and ctx.
func setupIncrementalExportTest(t *testing.T) (*testHarness, context.Context) {
	t.Helper()
	return setupIncrementalExportTestWithReadTimeout(t, 0)
}

// bulkSeedPoolReadTimeout is the Config.PoolReadTimeout given to tests whose
// own write volume, not server health, needs more slack than
// defaultPoolReadTimeout's 10s (internal/storage/dolt/store.go). be-uoat
// round 2: TestTryIncrementalExport_ThresholdExceededFallsBack's 5001-row
// mustCreateBatch seed writes sequentially over one held connection
// (MaxOpenConns=1 on the shared-branch test path), so any single read
// stalling past 10s under host contention trips the pool timeout mid-batch
// ("invalid connection" / "i/o timeout") — independently reproduced 2 of 3
// runs at 269.44s/316.35s/67.80s wall-clock (release-gates/be-uoat-dolt-diff-
// export-gate.md), evidence of real contention, not a hung process. 5m
// reuses the same tier already established in this codebase for this exact
// class of host-contention stall (execWithLongTimeout's push/pull deadline,
// and testStoreOpenTimeout's 300s in test_helpers_test.go) rather than a
// guessed value — comfortably above any observed run, while still bounded
// so a genuine hang still fails.
const bulkSeedPoolReadTimeout = 5 * time.Minute

// setupIncrementalExportTestWithReadTimeout is setupIncrementalExportTest
// with a caller-specified Config.PoolReadTimeout (0 = defaultPoolReadTimeout,
// i.e. identical to setupIncrementalExportTest). See bulkSeedPoolReadTimeout.
func setupIncrementalExportTestWithReadTimeout(t *testing.T, readTimeout time.Duration) (*testHarness, context.Context) {
	t.Helper()
	if testDoltServerPort == 0 {
		t.Skip("Dolt test server not available")
	}
	if testutil.DoltContainerCrashed() {
		t.Skipf("Dolt test server crashed: %v", testutil.DoltContainerCrashError())
	}

	ensureTestMode(t)
	saveAndRestoreGlobals(t)

	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	dbName := uniqueTestDBName(t)
	testDBPath := filepath.Join(beadsDir, "dolt")
	writeTestMetadata(t, testDBPath, dbName)
	s := newTestStoreWithPrefixAndReadTimeout(t, testDBPath, "test", readTimeout)
	store = s
	storeMutex.Lock()
	storeActive = true
	storeMutex.Unlock()
	t.Cleanup(func() {
		store = nil
		storeMutex.Lock()
		storeActive = false
		storeMutex.Unlock()
	})

	ctx := context.Background()
	rootCtx = ctx

	return &testHarness{store: s, beadsDir: beadsDir}, ctx
}

type testHarness struct {
	store    storage.DoltStorage
	beadsDir string
}

func (h *testHarness) mustCreate(t *testing.T, ctx context.Context, id, title string) {
	t.Helper()
	iss := &types.Issue{
		ID:        id,
		Title:     title,
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := h.store.CreateIssue(ctx, iss, "tester"); err != nil {
		t.Fatalf("CreateIssue(%s): %v", id, err)
	}
}

// mustCreateBatch creates n issues (IDs "<prefix>NNNNN") in a single
// transaction via CreateIssuesWithFullOptions, instead of n separate
// CreateIssue round trips over the connection. SkipPrefixValidation matches
// mustCreate/CreateIssue's single-issue behavior so callers can keep using
// IDs that don't match the store's configured issue_prefix (as the existing
// "thr-*" ids here do).
func (h *testHarness) mustCreateBatch(t *testing.T, ctx context.Context, n int, prefix string) {
	t.Helper()
	issues := make([]*types.Issue, n)
	for i := 0; i < n; i++ {
		issues[i] = &types.Issue{
			ID:        fmt.Sprintf("%s%05d", prefix, i),
			Title:     fmt.Sprintf("t%05d", i),
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
		}
	}
	if err := h.store.CreateIssuesWithFullOptions(ctx, issues, "tester", storage.BatchCreateOptions{SkipPrefixValidation: true}); err != nil {
		t.Fatalf("CreateIssuesWithFullOptions(%d issues): %v", n, err)
	}
}

func (h *testHarness) mustCommit(t *testing.T, ctx context.Context, msg string) string {
	t.Helper()
	if err := h.store.Commit(ctx, msg); err != nil {
		t.Fatalf("Commit(%q): %v", msg, err)
	}
	hash, err := h.store.GetCurrentCommit(ctx)
	if err != nil {
		t.Fatalf("GetCurrentCommit: %v", err)
	}
	return hash
}

// TestMaybeAutoExport_EmbeddedModeFallsBackToFullExportCleanly documents and
// locks in embedded mode's contract: EmbeddedDoltStore implements neither
// StateHasher nor DiffStore, so it can never take the incremental path —
// but per the fix (be-shbed / PR #5806 review item 3) it must still fall
// back to a full export cleanly, rather than silently producing nothing.
// This is deliberately a fake-store unit test, not a real embedded-mode
// integration test: exercising a genuine EmbeddedDoltStore end-to-end is
// out of scope (the bead explicitly excludes implementing DiffStore/
// StateHasher on it), and fakeHeadOnlyStore already models exactly the
// capability gap that matters here — no StateHasher, no DiffStore.
func TestMaybeAutoExport_EmbeddedModeFallsBackToFullExportCleanly(t *testing.T) {
	initConfigForTest(t)
	config.Set("export.auto", true)

	saveAndRestoreGlobals(t)
	fake := &fakeHeadOnlyStore{
		issues: []*types.Issue{
			{ID: "emb-a", Title: "Embedded Alpha", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask},
		},
	}
	store = fake

	dir := autoExportTestDir(t)
	saveExportAutoState(filepath.Join(dir, ".beads"), &exportAutoState{
		LastDoltCommit: "stale-hash",
		Timestamp:      time.Time{}, // zero: throttle window open
	})

	if err := maybeAutoExport(context.Background(), false); err != nil {
		t.Fatalf("maybeAutoExport: %v", err)
	}

	exportPath := filepath.Join(dir, ".beads", "issues.jsonl")
	titles := loadIssueTitles(t, exportPath)
	if titles["emb-a"] != "Embedded Alpha" {
		t.Errorf("emb-a title = %q, want %q (embedded mode must still produce a full export, not silently no-op)", titles["emb-a"], "Embedded Alpha")
	}

	state := loadExportAutoState(filepath.Join(dir, ".beads"))
	if state.LastDoltCommit != "head-commit-hash" {
		t.Errorf("state LastDoltCommit = %q, want %q (anchor must advance on a successful fallback export)", state.LastDoltCommit, "head-commit-hash")
	}
}

func idSetFromIDs(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func countIssueLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, `"_type":"memory"`) {
			continue
		}
		n++
	}
	return n
}

func loadIssueTitles(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, `"_type":"memory"`) {
			continue
		}
		var iss struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(line), &iss); err != nil {
			t.Errorf("unmarshal line %q: %v", line, err)
			continue
		}
		if iss.ID != "" {
			out[iss.ID] = iss.Title
		}
	}
	return out
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}

// jsonlRecordsByID parses every issue record in an export file (skipping
// memory records) into a generic map, keyed by "id". Used for
// content-equivalence comparisons between an incrementally-patched export
// and a fresh full export of the same state, where line order and byte
// layout are allowed to differ but the field content must match exactly.
func jsonlRecordsByID(t *testing.T, path string) map[string]map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	out := make(map[string]map[string]interface{})
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, `"_type":"memory"`) {
			continue
		}
		var rec map[string]interface{}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal line %q: %v", line, err)
		}
		id, _ := rec["id"].(string)
		if id == "" {
			continue
		}
		out[id] = rec
	}
	return out
}
