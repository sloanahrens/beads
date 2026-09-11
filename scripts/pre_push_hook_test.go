package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrePushHookAcceptsNotesOnlyPush guards against gt-r8yl: a push of only
// refs/notes/* (e.g. the om-editorial proof note) must never be treated as a
// version-tag push, so it must not invoke — or be blocked by — the release
// version-consistency check.
func TestPrePushHookAcceptsNotesOnlyPush(t *testing.T) {
	repo := newPrePushHookFixture(t, failingCheckVersions)

	out, err := runPrePushHook(t, repo, "refs/notes/om oldsha refs/notes/om newsha\n")
	if err != nil {
		t.Fatalf("notes-only push should be accepted, got error: %v\noutput:\n%s", err, out)
	}
}

// TestPrePushHookStillGatesVersionTagPush confirms the notes exemption did
// not disable the existing version-tag consistency guard: a real version tag
// push must still invoke scripts/check-versions.sh and fail when it fails.
func TestPrePushHookStillGatesVersionTagPush(t *testing.T) {
	repo := newPrePushHookFixture(t, failingCheckVersions)

	out, err := runPrePushHook(t, repo, "refs/tags/v1.2.3 oldsha refs/tags/v1.2.3 newsha\n")
	if err == nil {
		t.Fatalf("version tag push should be blocked by a failing check-versions.sh, got success\noutput:\n%s", out)
	}
}

// TestPrePushHookGatesMixedNotesAndTagPush confirms a single push event that
// includes both a notes ref and a version tag ref still runs the version
// check — the notes exemption must not short-circuit the rest of the push.
func TestPrePushHookGatesMixedNotesAndTagPush(t *testing.T) {
	repo := newPrePushHookFixture(t, failingCheckVersions)

	out, err := runPrePushHook(t, repo, "refs/notes/om oldsha refs/notes/om newsha\nrefs/tags/v1.2.3 oldsha refs/tags/v1.2.3 newsha\n")
	if err == nil {
		t.Fatalf("mixed notes+tag push should still be blocked by a failing check-versions.sh, got success\noutput:\n%s", out)
	}
}

const failingCheckVersions = `#!/bin/sh
echo "rigged failure: versions inconsistent" >&2
exit 1
`

// newPrePushHookFixture creates a temp git repo whose scripts/check-versions.sh
// is the given (rigged) script, so tests can observe whether the pre-push
// hook actually invokes it.
func newPrePushHookFixture(t *testing.T, checkVersionsBody string) string {
	t.Helper()
	repo := t.TempDir()

	if out, err := exec.Command("git", "-C", repo, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	scriptsDir := filepath.Join(repo, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(scriptsDir, "check-versions.sh"), checkVersionsBody)

	return repo
}

// runPrePushHook runs the real .githooks/pre-push script from this source
// tree against a fixture repo, feeding it the given push-refs stdin. PATH
// deliberately excludes bd so the beads-integration section is a no-op —
// this test is only about the version-tag/notes classification above it.
func runPrePushHook(t *testing.T, repo, stdin string) (string, error) {
	t.Helper()
	repoRoot := sourceRepoRoot(t)
	hookPath := filepath.Join(repoRoot, ".githooks", "pre-push")

	cmd := exec.Command("bash", hookPath)
	cmd.Dir = repo
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	return string(out), err
}
