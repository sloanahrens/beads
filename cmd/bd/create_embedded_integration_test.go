//go:build cgo && integration

// Integration tier (be-b23): moved verbatim from the unit-tier sibling file.
// These tests run bd init against a fresh store, which migrates; the unit
// tier's BD_TEST_TIER tripwire refuses that.
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
	"github.com/steveyegge/beads/internal/storage/schema"
)

func TestEmbeddedCreateDryRunDoesNotMigrate(t *testing.T) {
	bd := buildEmbeddedBD(t)
	dir, beadsDir, _ := bdInit(t, bd, "--prefix", "dnm")
	bdCreate(t, bd, dir, "Existing issue")

	readSnapshot := func() embeddedStoreSnapshot {
		return readEmbeddedStoreSnapshot(t, beadsDir, "dnm")
	}

	regressEmbeddedSchemaCursor(t, beadsDir, "dnm")

	// Force the version-bump path that previously opened a second writable
	// store and migrated before create.RunE reached --dry-run handling.
	if err := os.WriteFile(filepath.Join(beadsDir, localVersionFile), []byte("0.9.0\n"), 0o600); err != nil {
		t.Fatalf("write old local version: %v", err)
	}

	before := readSnapshot()
	if before.schemaVersion != schema.LatestVersion()-1 {
		t.Fatalf("fixture schema version = %d, want %d", before.schemaVersion, schema.LatestVersion()-1)
	}

	cmd := exec.Command(bd, "create", "--dry-run", "Preview only", "--json")
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	stdout, stderr, err := runCommandBuffers(t, cmd)
	if err != nil {
		t.Fatalf("bd create --dry-run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	after := readSnapshot()
	if after.schemaVersion != before.schemaVersion {
		t.Errorf("schema version changed during dry-run: before=%d after=%d", before.schemaVersion, after.schemaVersion)
	}
	if after.head != before.head {
		t.Errorf("Dolt HEAD changed during dry-run: before=%s after=%s", before.head, after.head)
	}
	if after.issueCount != before.issueCount {
		t.Errorf("issue count changed during dry-run: before=%d after=%d", before.issueCount, after.issueCount)
	}
}

// TestEmbeddedCreateDryRunCrossRepoDoesNotMigrateTarget covers the second
// store a dry-run can reach: `create --dry-run --parent X --repo <other>`
// resolves the parent against the OTHER repo, and openDryRunTargetStore used
// to open it with the writable factory. The command's own store being opened
// read-only says nothing about that one — the mutation lands in a repository
// the user only named as a lookup target.
func TestEmbeddedCreateDryRunCrossRepoDoesNotMigrateTarget(t *testing.T) {
	bd := buildEmbeddedBD(t)
	targetDir, targetBeadsDir, _ := bdInit(t, bd, "--prefix", "xtgt")
	parent := bdCreate(t, bd, targetDir, "Parent in the target repo")
	if parent.ID == "" {
		t.Fatal("parent issue has no ID")
	}

	callerDir, callerBeadsDir, _ := bdInit(t, bd, "--prefix", "xsrc")

	// The tripwire goes in the TARGET repo: only a writable open of that repo
	// restores its cursor and commits.
	regressEmbeddedSchemaCursor(t, targetBeadsDir, "xtgt")

	// Force the version-bump path in the caller repo too, so this exercises
	// the same post-upgrade window as the single-repo test.
	if err := os.WriteFile(filepath.Join(callerBeadsDir, localVersionFile), []byte("0.9.0\n"), 0o600); err != nil {
		t.Fatalf("write old local version: %v", err)
	}

	before := readEmbeddedStoreSnapshot(t, targetBeadsDir, "xtgt")
	if before.schemaVersion != schema.LatestVersion()-1 {
		t.Fatalf("fixture schema version = %d, want %d", before.schemaVersion, schema.LatestVersion()-1)
	}

	cmd := exec.Command(bd, "create", "--dry-run",
		"--parent", parent.ID, "--repo", targetDir, "Preview only", "--json")
	cmd.Dir = callerDir
	cmd.Env = bdEnv(callerDir)
	stdout, stderr, err := runCommandBuffers(t, cmd)
	if err != nil {
		t.Fatalf("bd create --dry-run --parent --repo failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	after := readEmbeddedStoreSnapshot(t, targetBeadsDir, "xtgt")
	if after.schemaVersion != before.schemaVersion {
		t.Errorf("target repo schema version changed during cross-repo dry-run: before=%d after=%d", before.schemaVersion, after.schemaVersion)
	}
	if after.head != before.head {
		t.Errorf("target repo Dolt HEAD changed during cross-repo dry-run: before=%s after=%s", before.head, after.head)
	}
	if after.issueCount != before.issueCount {
		t.Errorf("target repo issue count changed during cross-repo dry-run: before=%d after=%d", before.issueCount, after.issueCount)
	}
}

// TestEmbeddedPreviewDoesNotConsumeVersionMarker is the two-invocation
// regression for the one-shot upgrade signal: a preview run first after an
// upgrade correctly skips the version-bump reconciliation, so it must also
// leave .beads/.local_version alone. Burning the marker there would mean the
// next ordinary command sees a matching version and never reconciles —
// whichever command happened to run first would silently decide whether the
// upgrade was finished.
func TestEmbeddedPreviewDoesNotConsumeVersionMarker(t *testing.T) {
	bd := buildEmbeddedBD(t)
	dir, beadsDir, _ := bdInit(t, bd, "--prefix", "pvm")
	bdCreate(t, bd, dir, "Existing issue")

	localVersionPath := filepath.Join(beadsDir, localVersionFile)
	if err := os.WriteFile(localVersionPath, []byte("0.9.0\n"), 0o600); err != nil {
		t.Fatalf("write old local version: %v", err)
	}

	// Invocation 1: preview.
	preview := exec.Command(bd, "create", "--dry-run", "Preview only", "--json")
	preview.Dir = dir
	preview.Env = bdEnv(dir)
	stdout, stderr, err := runCommandBuffers(t, preview)
	if err != nil {
		t.Fatalf("bd create --dry-run failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	raw, err := os.ReadFile(localVersionPath)
	if err != nil {
		t.Fatalf("read local version after preview: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); got != "0.9.0" {
		t.Fatalf("preview consumed the version marker: .local_version = %q, want %q", got, "0.9.0")
	}

	// Invocation 2: an ordinary command, which must still see the upgrade.
	status := exec.Command(bd, "upgrade", "status", "--json")
	status.Dir = dir
	status.Env = bdEnv(dir)
	stdout, stderr, err = runCommandBuffers(t, status)
	if err != nil {
		t.Fatalf("bd upgrade status failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	var upgradeStatus struct {
		Upgraded        bool   `json:"upgraded"`
		PreviousVersion string `json:"previous_version"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &upgradeStatus); err != nil {
		t.Fatalf("parse upgrade status: %v\nstdout:\n%s", err, stdout.String())
	}
	if !upgradeStatus.Upgraded || upgradeStatus.PreviousVersion != "0.9.0" {
		t.Errorf("ordinary command after a preview no longer sees the upgrade: upgraded=%v previous=%q; stdout:\n%s",
			upgradeStatus.Upgraded, upgradeStatus.PreviousVersion, stdout.String())
	}

	raw, err = os.ReadFile(localVersionPath)
	if err != nil {
		t.Fatalf("read local version after ordinary command: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); got == "0.9.0" {
		t.Errorf("ordinary command left .local_version at %q; the marker should have been updated", got)
	}
}

func TestEmbeddedChangeDirOverridesInheritedBeadsDir(t *testing.T) {
	bd := buildEmbeddedBD(t)
	callerDir, callerBeadsDir, _ := bdInit(t, bd, "--prefix", "caller")
	targetDir, targetBeadsDir, _ := bdInit(t, bd, "--prefix", "target")

	cmd := exec.Command(bd, "-C", targetDir, "create", "Explicit target", "--json")
	cmd.Dir = callerDir
	cmd.Env = append(bdEnv(callerDir), "BEADS_DIR="+callerBeadsDir)
	stdout, stderr, err := runCommandBuffers(t, cmd)
	if err != nil {
		t.Fatalf("bd -C target create failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	countIssues := func(beadsDir, database string) int {
		t.Helper()
		db, cleanup, err := embeddeddolt.OpenSQL(
			t.Context(),
			filepath.Join(beadsDir, "embeddeddolt"),
			database,
			"main",
		)
		if err != nil {
			t.Fatalf("OpenSQL %s: %v", database, err)
		}
		defer func() {
			if err := cleanup(); err != nil {
				t.Errorf("cleanup OpenSQL %s: %v", database, err)
			}
		}()
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM issues").Scan(&count); err != nil {
			t.Fatalf("count issues in %s: %v", database, err)
		}
		return count
	}

	if got := countIssues(callerBeadsDir, "caller"); got != 0 {
		t.Fatalf("inherited BEADS_DIR received %d issues, want 0", got)
	}
	if got := countIssues(targetBeadsDir, "target"); got != 1 {
		t.Fatalf("-C target received %d issues, want 1", got)
	}
}

// embeddedStoreSnapshot is the "did anything write to this database?"
// tripwire the preview regression tests compare across a command run.
type embeddedStoreSnapshot struct {
	schemaVersion int
	head          string
	issueCount    int
}

func readEmbeddedStoreSnapshot(t *testing.T, beadsDir, database string) embeddedStoreSnapshot {
	t.Helper()
	db, cleanup, err := embeddeddolt.OpenSQL(
		t.Context(),
		filepath.Join(beadsDir, "embeddeddolt"),
		database,
		"main",
	)
	if err != nil {
		t.Fatalf("OpenSQL: %v", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Errorf("cleanup OpenSQL: %v", err)
		}
	}()

	var got embeddedStoreSnapshot
	if err := db.QueryRowContext(t.Context(),
		"SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&got.schemaVersion); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if err := db.QueryRowContext(t.Context(), "SELECT HASHOF('HEAD')").Scan(&got.head); err != nil {
		t.Fatalf("read HEAD: %v", err)
	}
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM issues").Scan(&got.issueCount); err != nil {
		t.Fatalf("read issue count: %v", err)
	}
	return got
}

// regressEmbeddedSchemaCursor rolls the recorded migration cursor back one
// version WITHOUT touching the physical schema. The latest migration is
// idempotent, so a writable open reapplies it, restores the cursor, and
// commits a new HEAD — which is exactly what makes the cursor a usable
// tripwire. Keeping the physical schema intact lets the preview's own reads
// still work against the older recorded version.
func regressEmbeddedSchemaCursor(t *testing.T, beadsDir, database string) {
	t.Helper()
	db, cleanup, err := embeddeddolt.OpenSQL(
		t.Context(),
		filepath.Join(beadsDir, "embeddeddolt"),
		database,
		"main",
	)
	if err != nil {
		t.Fatalf("OpenSQL for regression fixture: %v", err)
	}
	if _, err := db.ExecContext(t.Context(),
		"DELETE FROM schema_migrations WHERE version = ?", schema.LatestVersion()); err != nil {
		_ = cleanup()
		t.Fatalf("regress schema cursor: %v", err)
	}
	if _, err := db.ExecContext(t.Context(),
		"CALL DOLT_COMMIT('-am', 'test: regress schema before dry-run')"); err != nil {
		_ = cleanup()
		t.Fatalf("commit regressed schema cursor: %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup regression fixture: %v", err)
	}
}
