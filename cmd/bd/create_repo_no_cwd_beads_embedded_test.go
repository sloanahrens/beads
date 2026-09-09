//go:build cgo

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbeddedCreateRepoFromNonBeadsCwd reproduces GH#3686: running
// `bd create --repo=<local path>` from a directory that has no .beads/
// workspace of its own must resolve the target repo's workspace instead of
// failing with "no beads database found".
//
// Before the fix, PersistentPreRun exited early with that error because the
// current directory had no discoverable database, so create.go's --repo
// handling never ran. The reproduction, contributor bug report, and expected
// behavior are due to kevglynn (GH#3774).
func TestEmbeddedCreateRepoFromNonBeadsCwd(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt create tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)

	t.Run("repo_flag_resolves_target_workspace", func(t *testing.T) {
		// Target repo with a real .beads/ workspace.
		targetDir, targetBeadsDir, _ := bdInit(t, bd, "--prefix", "rp")

		// A separate directory with NO .beads/ workspace (and no .beads
		// ancestor, since it is an independent temp dir).
		noBeadsCwd := t.TempDir()

		// Sanity: the cwd genuinely has no .beads workspace.
		if _, err := os.Stat(noBeadsCwd + "/.beads"); err == nil {
			t.Fatalf("test setup: %s unexpectedly has a .beads dir", noBeadsCwd)
		}

		// Create from the non-beads cwd, targeting the other repo. Before the
		// fix this failed with "no beads database found".
		issue := bdCreate(t, bd, noBeadsCwd, "Routed from non-beads cwd", "--repo", targetDir)
		if issue.ID == "" {
			t.Fatal("expected issue ID")
		}
		if !strings.HasPrefix(issue.ID, "rp-") {
			t.Errorf("ID should have target prefix rp-, got %q", issue.ID)
		}
		if issue.Title != "Routed from non-beads cwd" {
			t.Errorf("title: got %q, want %q", issue.Title, "Routed from non-beads cwd")
		}

		// The issue must land in the target repo's store.
		assertIssueInStore(t, targetBeadsDir, "rp", issue.ID)
	})

	t.Run("no_repo_flag_still_errors_in_non_beads_cwd", func(t *testing.T) {
		// Regression guard: the no-database-found error must still fire for an
		// ordinary create with no --repo when the cwd has no workspace, so the
		// fix does not swallow the diagnostic for the common mistake.
		noBeadsCwd := t.TempDir()
		out := bdCreateFail(t, bd, noBeadsCwd, "should fail")
		if !strings.Contains(out, "no beads database found") {
			t.Errorf("expected 'no beads database found' error, got:\n%s", out)
		}
	})

	// be-6mk: an ambiguous (relative/bare) --repo value naming a target with
	// no existing workspace must be refused outright, and must never create
	// anything on disk at the guessed target path. Before the fix, this early
	// GH#3686 resolution unconditionally routed dbPath at the target and let
	// the normal store-open auto-vivify a brand-new embedded Dolt DB there —
	// silently, before create.go's own isAmbiguousRepoTarget guard ever ran.
	t.Run("ambiguous_missing_repo_target_does_not_auto_vivify", func(t *testing.T) {
		noBeadsCwd := t.TempDir()

		out := bdCreateFail(t, bd, noBeadsCwd, "should fail", "--repo", "some-unrelated-rig-name")
		if !strings.Contains(out, "won't be auto-created here") {
			t.Errorf("expected the ambiguous-repo-target error, got:\n%s", out)
		}

		guessedTarget := filepath.Join(noBeadsCwd, "some-unrelated-rig-name")
		if _, err := os.Stat(guessedTarget); err == nil {
			t.Errorf("expected no directory to be created at %s, but one exists", guessedTarget)
		}
	})

	// be-dxx: an --repo target whose .beads dir has ONLY a redirect file (no
	// local metadata.json) is a fully valid, already-initialized workspace —
	// this is exactly what a Gas Town rig root looks like. Before the fix,
	// the target-existence check looked for metadata.json at the literal
	// joined path without following the redirect, saw "nothing there", and
	// (for an unambiguous absolute --repo path, which be-6mk's relative-path
	// guard does not cover) auto-vivified a brand-new phantom embedded Dolt
	// DB right next to the redirect — bricking the real rig's writes with a
	// PROJECT IDENTITY MISMATCH on the next ordinary command run from it.
	t.Run("repo_flag_follows_redirect_instead_of_auto_vivifying_sibling", func(t *testing.T) {
		// The real workspace the redirect ultimately points at.
		_, realBeadsDir, _ := bdInit(t, bd, "--prefix", "rd")

		// A separate rig root whose .beads/ contains only a redirect to the
		// real workspace above — no local metadata.json, matching a healthy
		// redirected Gas Town rig (e.g. gastown/.beads -> mayor/rig/.beads).
		rigRoot := t.TempDir()
		rigBeadsDir := filepath.Join(rigRoot, ".beads")
		if err := os.MkdirAll(rigBeadsDir, 0o750); err != nil {
			t.Fatalf("mkdir rig .beads: %v", err)
		}
		if err := os.WriteFile(filepath.Join(rigBeadsDir, "redirect"), []byte(realBeadsDir+"\n"), 0o600); err != nil {
			t.Fatalf("write redirect: %v", err)
		}

		noBeadsCwd := t.TempDir()

		issue := bdCreate(t, bd, noBeadsCwd, "Routed through redirect", "--repo", rigRoot)
		if !strings.HasPrefix(issue.ID, "rd-") {
			t.Errorf("ID should have real repo's prefix rd-, got %q", issue.ID)
		}

		// The issue must land in the REAL store, reached via the redirect.
		assertIssueInStore(t, realBeadsDir, "rd", issue.ID)

		// No phantom metadata.json or embedded DB may appear next to the
		// redirect — that would mean a sibling database was auto-vivified
		// instead of the redirect being followed.
		if _, err := os.Stat(filepath.Join(rigBeadsDir, "metadata.json")); err == nil {
			t.Errorf("expected no metadata.json to be created at %s (redirect should have been followed)", rigBeadsDir)
		}
		if _, err := os.Stat(filepath.Join(rigBeadsDir, "embeddeddolt")); err == nil {
			t.Errorf("expected no embeddeddolt/ to be created at %s (redirect should have been followed)", rigBeadsDir)
		}
	})
}
