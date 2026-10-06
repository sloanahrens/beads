//go:build cgo && integration

package main

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/testutil"
	"github.com/steveyegge/beads/internal/types"
)

func TestAutoExportGitAddFailureExitsNonZero(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	bd := buildBDForInitTests(t)
	dir := t.TempDir()
	env := append(autoExportDataLossTestEnv(dir), "BD_NON_INTERACTIVE=1")

	runGit := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bd, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}

	run(append([]string{"init", "--prefix", "agf", "--quiet", "--non-interactive", "--skip-hooks", "--skip-agents"}, serverInitArgs(t)...)...)
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".beads/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("config", "set", "export.interval", "1ms")
	run("config", "set", "export.auto", "true")
	run("config", "set", "export.git-add", "true")
	if err := os.Remove(filepath.Join(dir, ".beads", exportAutoStateFile)); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)

	cmd := exec.Command(bd, "create", "caller visible git add failure", "-p", "2")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("bd create succeeded despite auto-export git add failure:\n%s", out)
	}
	output := string(out)
	if !strings.Contains(output, "Error: auto-export: git add failed") {
		t.Fatalf("expected caller-visible auto-export git add error, got:\n%s", output)
	}
	if !strings.Contains(strings.ToLower(output), "ignored") {
		t.Fatalf("expected git add stderr to explain ignored path, got:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(dir, ".beads", exportAutoStateFile)); !os.IsNotExist(err) {
		t.Fatalf("git-add failure should not save export state, stat err=%v", err)
	}
}

func TestAutoExportSkipsEmptyExportOverPopulatedJSONL(t *testing.T) {
	bd := buildBDForInitTests(t)
	dir := t.TempDir()
	env := autoExportDataLossTestEnv(dir)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bd, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}

	run(append([]string{"init", "--prefix", "dl", "--non-interactive"}, serverInitArgs(t)...)...)
	run("config", "set", "export.path", "custom.jsonl")

	jsonlPath := filepath.Join(dir, ".beads", "custom.jsonl")
	original := []byte(`{"_type":"issue","id":"dl-1","title":"Recovered issue","priority":1,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}` + "\n")
	if err := os.WriteFile(jsonlPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	run("config", "set", "export.auto", "true")
	out := run("remember", "private context that should not be auto-exported")
	if !strings.Contains(out, "refusing to overwrite") {
		t.Fatalf("expected auto-export refusal warning, got:\n%s", out)
	}

	got, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("expected populated JSONL to remain: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("populated JSONL was modified:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".beads", exportAutoStateFile)); !os.IsNotExist(err) {
		t.Fatalf("empty skipped auto-export should not save export state, stat err=%v", err)
	}
}

func TestAutoExportSkipsWhenExistingJSONLHasIDsMissingFromStore(t *testing.T) {
	bd := buildBDForInitTests(t)
	dir := t.TempDir()
	env := autoExportDataLossTestEnv(dir)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bd, args...)
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bd %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}

	run(append([]string{"init", "--prefix", "dl", "--non-interactive"}, serverInitArgs(t)...)...)
	run("config", "set", "export.path", "custom.jsonl")
	run("create", "local issue", "-p", "2")

	jsonlPath := filepath.Join(dir, ".beads", "custom.jsonl")
	original := []byte(strings.Join([]string{
		`{"_type":"issue","id":"dl-1","title":"Local issue","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
		`{"_type":"issue","id":"dl-jsonl-only","title":"Only in JSONL","priority":1,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`,
		``,
	}, "\n"))
	if err := os.WriteFile(jsonlPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	run("config", "set", "export.interval", "1ms")
	run("config", "set", "export.auto", "true")
	out := run("create", "another local issue", "-p", "2")
	if !strings.Contains(out, "JSONL-only issue record") || !strings.Contains(out, "dl-jsonl-only") {
		t.Fatalf("expected JSONL-only refusal warning, got:\n%s", out)
	}

	got, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("expected JSONL to remain: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("JSONL-only records were overwritten:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".beads", exportAutoStateFile)); !os.IsNotExist(err) {
		t.Fatalf("skipped auto-export should not save export state, stat err=%v", err)
	}
}

func TestChangedIssueIDs_DetectsUpsertsAndRemovals(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)

	// Baseline: create three issues and commit.
	h.mustCreate(t, ctx, "cid-a", "Alpha")
	h.mustCreate(t, ctx, "cid-b", "Beta")
	h.mustCreate(t, ctx, "cid-c", "Gamma")
	c1 := h.mustCommit(t, ctx, "baseline")

	// Delta:
	//   - modify cid-a via UpdateIssue (touches issues row)
	//   - add a label to cid-b (touches labels row only)
	//   - delete cid-c (touches issues row, diff_type=removed)
	if err := h.store.UpdateIssue(ctx, "cid-a", map[string]interface{}{"title": "Alpha Prime"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if err := h.store.AddLabel(ctx, "cid-b", "priority", "tester"); err != nil {
		t.Fatalf("AddLabel: %v", err)
	}
	if err := h.store.DeleteIssue(ctx, "cid-c"); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	c2 := h.mustCommit(t, ctx, "delta")

	ds, ok := storage.UnwrapStore(h.store).(storage.DiffStore)
	if !ok {
		t.Fatal("DoltStore should implement DiffStore")
	}
	changed, err := ds.ChangedIssueIDs(ctx, c1, c2)
	if err != nil {
		t.Fatalf("ChangedIssueIDs: %v", err)
	}

	gotUpserted := idSetFromIDs(changed.Upserted)
	gotRemoved := idSetFromIDs(changed.Removed)

	for _, id := range []string{"cid-a", "cid-b"} {
		if !gotUpserted[id] {
			t.Errorf("%s missing from Upserted (got %v)", id, changed.Upserted)
		}
		if gotRemoved[id] {
			t.Errorf("%s wrongly in Removed", id)
		}
	}
	if !gotRemoved["cid-c"] {
		t.Errorf("cid-c missing from Removed (got %v)", changed.Removed)
	}
	if gotUpserted["cid-c"] {
		t.Error("cid-c wrongly in Upserted — a deleted issue must not be upserted even though cascade removes its label/dep rows")
	}
}

func TestTryIncrementalExport_PatchesChangedIssuesAndDropsRemoved(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)

	// Baseline: 5 issues, full export.
	h.mustCreate(t, ctx, "inc-a", "A")
	h.mustCreate(t, ctx, "inc-b", "B")
	h.mustCreate(t, ctx, "inc-c", "C")
	h.mustCreate(t, ctx, "inc-d", "D")
	h.mustCreate(t, ctx, "inc-e", "E")
	c1 := h.mustCommit(t, ctx, "baseline")

	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	if _, _, err := exportToFile(ctx, exportPath, true); err != nil {
		t.Fatalf("exportToFile: %v", err)
	}
	if got := countIssueLines(t, exportPath); got != 5 {
		t.Fatalf("baseline export has %d issues, want 5", got)
	}

	// Mutate: rename inc-a, delete inc-b.
	if err := h.store.UpdateIssue(ctx, "inc-a", map[string]interface{}{"title": "A-renamed"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if err := h.store.DeleteIssue(ctx, "inc-b"); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	c2 := h.mustCommit(t, ctx, "mutate")

	issueCount, memoryCount, _, didIncremental, err := tryIncrementalExport(ctx, exportPath, c1, c2, nil)
	if err != nil {
		t.Fatalf("tryIncrementalExport returned error: %v", err)
	}
	if !didIncremental {
		t.Fatal("expected incremental path to succeed")
	}
	if issueCount != 4 {
		t.Errorf("issueCount = %d, want 4 (5 baseline − 1 deleted)", issueCount)
	}
	_ = memoryCount

	// Verify file state: inc-b gone; inc-a has new title; others unchanged.
	titles := loadIssueTitles(t, exportPath)
	if _, ok := titles["inc-b"]; ok {
		t.Error("inc-b should have been dropped from export")
	}
	if titles["inc-a"] != "A-renamed" {
		t.Errorf("inc-a title = %q, want %q", titles["inc-a"], "A-renamed")
	}
	for _, id := range []string{"inc-c", "inc-d", "inc-e"} {
		if _, ok := titles[id]; !ok {
			t.Errorf("untouched issue %s missing from export", id)
		}
	}
}

func TestTryIncrementalExport_DropsIssueWhenFlippedToTemplate(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)

	h.mustCreate(t, ctx, "flip-a", "Alpha")
	h.mustCreate(t, ctx, "flip-b", "Beta")
	c1 := h.mustCommit(t, ctx, "baseline")

	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	if _, _, err := exportToFile(ctx, exportPath, true); err != nil {
		t.Fatalf("exportToFile: %v", err)
	}
	if got := countIssueLines(t, exportPath); got != 2 {
		t.Fatalf("baseline export has %d issues, want 2", got)
	}

	// Flip flip-a to a template in place. UpdateIssue doesn't toggle
	// is_template directly, so go through raw SQL — that mirrors what
	// bd's template-promotion flow eventually writes anyway.
	doltStore, ok := h.store.(interface {
		DB() *sql.DB
	})
	if !ok {
		t.Skip("store does not expose DB() for raw SQL; can't exercise template flip")
	}
	if _, err := doltStore.DB().ExecContext(ctx, `UPDATE issues SET is_template = 1 WHERE id = ?`, "flip-a"); err != nil {
		t.Fatalf("UPDATE is_template: %v", err)
	}
	c2 := h.mustCommit(t, ctx, "promote to template")

	_, _, _, didIncremental, err := tryIncrementalExport(ctx, exportPath, c1, c2, nil)
	if err != nil {
		t.Fatalf("tryIncrementalExport: %v", err)
	}
	if !didIncremental {
		t.Fatal("expected incremental path to run")
	}

	titles := loadIssueTitles(t, exportPath)
	if _, stillThere := titles["flip-a"]; stillThere {
		t.Error("flip-a should have been dropped from export once flipped to a template")
	}
	if _, ok := titles["flip-b"]; !ok {
		t.Error("flip-b (untouched) must remain in the export")
	}
}

func TestTryIncrementalExport_FallsBackWhenFileMissing(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)

	h.mustCreate(t, ctx, "fb-a", "A")
	c1 := h.mustCommit(t, ctx, "first")
	h.mustCreate(t, ctx, "fb-b", "B")
	c2 := h.mustCommit(t, ctx, "second")

	// No existing file → must return didIncremental=false and leave the
	// disk untouched so the caller falls back to the full-export path.
	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	issueCount, _, _, didIncremental, err := tryIncrementalExport(ctx, exportPath, c1, c2, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if didIncremental {
		t.Fatal("expected fallback when file is missing")
	}
	if issueCount != 0 {
		t.Errorf("issueCount on fallback = %d, want 0", issueCount)
	}
	if _, err := os.Stat(exportPath); !os.IsNotExist(err) {
		t.Error("fallback path must not create a file")
	}
}

func TestTryIncrementalExport_ThresholdExceededFallsBack(t *testing.T) {
	h, ctx := setupIncrementalExportTestWithReadTimeout(t, bulkSeedPoolReadTimeout)

	// Seed one issue so the file exists; baseline commit.
	h.mustCreate(t, ctx, "thr-0", "seed")
	c1 := h.mustCommit(t, ctx, "seed")

	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	if _, _, err := exportToFile(ctx, exportPath, true); err != nil {
		t.Fatalf("exportToFile: %v", err)
	}
	sizeBefore, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create more issues than the threshold in a single transaction/commit
	// (not incrementalExportThreshold+1 separate CreateIssue round trips —
	// be-fgd round-2: holding one connection open across 5001 sequential
	// writes intermittently tripped a mid-stream TCP read timeout ["write
	// commit result indeterminate after connection loss"], at a different
	// point in the loop on each of two consecutive runs).
	h.mustCreateBatch(t, ctx, incrementalExportThreshold+1, "thr-")
	c2 := h.mustCommit(t, ctx, "flood")

	_, _, _, didIncremental, err := tryIncrementalExport(ctx, exportPath, c1, c2, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if didIncremental {
		t.Fatal("expected fallback when change count exceeds threshold")
	}

	// File should be byte-for-byte unchanged since fallback was taken.
	sizeAfter, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(sizeBefore) != len(sizeAfter) {
		t.Errorf("file was touched on fallback (size %d → %d)", len(sizeBefore), len(sizeAfter))
	}
}

// TestMaybeAutoExport_SecondRunTakesIncrementalPath_ServerMode is the be-shbed
// regression test for bee-ghosttrack's PR #5806 review finding: the root
// cause was DOLT_HASHOF_DB() (a working-set root hash) being fed straight
// into dolt_diff(), which only accepts real commits or the literal
// 'WORKING' — so dolt_diff always errored and every "incremental" export
// silently fell back to a full rewrite. This test proves the fix by driving
// maybeAutoExport itself (not tryIncrementalExport directly) end-to-end
// against the real dolt test server, and observing that the DiffStore path
// actually executes.
func TestMaybeAutoExport_SecondRunTakesIncrementalPath_ServerMode(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)
	initConfigForTest(t)
	config.Set("export.auto", true)
	config.Set("export.interval", "1ms")

	spy := &spyDiffStore{DoltStorage: h.store}
	store = spy

	h.mustCreate(t, ctx, "e2e-a", "Alpha")
	h.mustCreate(t, ctx, "e2e-b", "Beta")
	h.mustCreate(t, ctx, "e2e-c", "Gamma")
	h.mustCommit(t, ctx, "baseline")

	if err := maybeAutoExport(ctx, false); err != nil {
		t.Fatalf("first maybeAutoExport: %v", err)
	}
	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	if got := countIssueLines(t, exportPath); got != 3 {
		t.Fatalf("after first export, %d issue lines, want 3", got)
	}

	// Mutate: rename e2e-a, delete e2e-b, leave e2e-c untouched. Committed
	// (not just working-set-dirty) so the state hash unambiguously moves and
	// a second export is triggered.
	if err := h.store.UpdateIssue(ctx, "e2e-a", map[string]interface{}{"title": "Alpha renamed"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	if err := h.store.DeleteIssue(ctx, "e2e-b"); err != nil {
		t.Fatalf("DeleteIssue: %v", err)
	}
	h.mustCommit(t, ctx, "mutate")

	if err := maybeAutoExport(ctx, false); err != nil {
		t.Fatalf("second maybeAutoExport: %v", err)
	}

	// Both ChangedIssueIDs call sites must have fired: the orphan guard's
	// proof-of-deletion probe (missingJSONLIssueIDsInStore, which runs
	// because deleting e2e-b leaves it JSONL-only) and tryIncrementalExport's
	// own diff. A bare "called at least once" check is NOT sufficient and was
	// the PR #5806 round-5 review finding: the guard's own call satisfies it
	// even when tryIncrementalExport falls back to a full export, so the test
	// would pass while proving nothing about the path it is named for.
	if spy.changedIssueIDsCalls != 2 {
		t.Errorf("ChangedIssueIDs called %d times, want 2 (orphan-guard deletion probe + incremental diff) — incremental export never actually reached the dolt_diff-backed DiffStore path (root-cause regression: WORKING-set hash fed to dolt_diff, silently falling back to full export every time)", spy.changedIssueIDsCalls)
	}

	// The load-bearing oracle for this test's name: dirtyIDs reaches
	// LastDirtyIDs only from a successful incremental patch — maybeAutoExport
	// explicitly nils it on the full-export fallback — so an exact {e2e-a}
	// here proves the export WAS incremental, not merely that a diff was
	// reached. e2e-a is the only upserted id: e2e-b was removed (not
	// upserted) and e2e-c was untouched.
	if got, want := loadExportAutoState(h.beadsDir).LastDirtyIDs, []string{"e2e-a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("LastDirtyIDs = %v, want %v — a nil/empty value means the export fell back to a full rewrite instead of taking the incremental path", got, want)
	}

	titles := loadIssueTitles(t, exportPath)
	if _, stillThere := titles["e2e-b"]; stillThere {
		t.Error("e2e-b should have been dropped from export")
	}
	if titles["e2e-a"] != "Alpha renamed" {
		t.Errorf("e2e-a title = %q, want %q", titles["e2e-a"], "Alpha renamed")
	}
	if _, ok := titles["e2e-c"]; !ok {
		t.Error("untouched issue e2e-c missing from export")
	}

	// Content-equivalence control: the incrementally-patched file must
	// describe the same issue set as a fresh full export of the same final
	// state (field-for-field, not byte-for-byte — line order and formatting
	// are allowed to differ).
	controlPath := filepath.Join(t.TempDir(), "control.jsonl")
	if _, _, err := exportToFile(ctx, controlPath, false); err != nil {
		t.Fatalf("control exportToFile: %v", err)
	}
	got := jsonlRecordsByID(t, exportPath)
	want := jsonlRecordsByID(t, controlPath)
	if len(got) != len(want) {
		t.Fatalf("incremental export has %d records, control full export has %d", len(got), len(want))
	}
	for id, wantRec := range want {
		gotRec, ok := got[id]
		if !ok {
			t.Errorf("record %s present in control export, missing from incremental export", id)
			continue
		}
		if !reflect.DeepEqual(gotRec, wantRec) {
			t.Errorf("record %s differs between incremental and full export:\n  incremental: %v\n  full:        %v", id, gotRec, wantRec)
		}
	}
}

// TestMaybeAutoExport_HistoryRewindDoesNotProveDeletion is the PR #5806
// round-5 regression test for review ask 2: "removed since anchor" is not the
// same claim as "deleted by `bd delete`". dolt_diff(anchor, WORKING) reports a
// row as removed whenever it is absent at WORKING, which is equally true after
// the history is rewound out from under the anchor. Without an ancestry
// precondition the orphan guard accepted that as proof of deletion and let the
// export silently drop a live record from issues.jsonl — the exact #4988
// corruption class the guard exists to prevent.
func TestMaybeAutoExport_HistoryRewindDoesNotProveDeletion(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)
	initConfigForTest(t)
	config.Set("export.auto", true)
	config.Set("export.interval", "1ms")

	h.mustCreate(t, ctx, "rw-a", "Alpha")
	c1 := h.mustCommit(t, ctx, "baseline")
	if err := maybeAutoExport(ctx, false); err != nil {
		t.Fatalf("first maybeAutoExport: %v", err)
	}
	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")

	// Second cycle: rw-x lands in both the store and issues.jsonl, and the
	// diff anchor advances past c1 to the commit that added it.
	h.mustCreate(t, ctx, "rw-x", "Rewound")
	h.mustCommit(t, ctx, "add rw-x")
	if err := maybeAutoExport(ctx, false); err != nil {
		t.Fatalf("second maybeAutoExport: %v", err)
	}
	if titles := loadIssueTitles(t, exportPath); titles["rw-x"] != "Rewound" {
		t.Fatalf("setup: rw-x must be exported before the rewind, got %v", titles)
	}

	// Rewind the data dir underneath the anchor. rw-x is now absent from the
	// store but still present in issues.jsonl — indistinguishable from a real
	// `bd delete` if you only consult dolt_diff(anchor, WORKING).
	raw, ok := storage.UnwrapStore(h.store).(storage.RawDBAccessor)
	if !ok {
		t.Skip("store does not expose raw DB access")
	}
	if _, err := raw.DB().ExecContext(ctx, "CALL DOLT_RESET('--hard', ?)", c1); err != nil {
		t.Fatalf("CALL DOLT_RESET('--hard', %s): %v", c1, err)
	}

	// The guard must refuse rather than honor the bogus "removed" verdict:
	// the anchor is no longer reachable from HEAD, so nothing is proven.
	// maybeAutoExport returns nil either way (a refusal is a warn + skip), so
	// the file content is the oracle.
	if err := maybeAutoExport(ctx, false); err != nil {
		t.Fatalf("third maybeAutoExport: %v", err)
	}
	if _, stillThere := loadIssueTitles(t, exportPath)["rw-x"]; !stillThere {
		t.Error("rw-x was silently dropped from issues.jsonl after a history rewind: the orphan guard treated a rewound-away row as a proven deletion instead of refusing to overwrite")
	}
}

func TestTryIncrementalExport_NeverLeaksMemoriesIntoAutoExport(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)

	h.mustCreate(t, ctx, "mem-a", "Alpha")
	c1 := h.mustCommit(t, ctx, "baseline")

	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	// Baseline matches real auto-export usage: includeMemories=false.
	if _, memCount, err := exportToFile(ctx, exportPath, false); err != nil {
		t.Fatalf("exportToFile: %v", err)
	} else if memCount != 0 {
		t.Fatalf("baseline memoryCount = %d, want 0", memCount)
	}

	// User remembers something AFTER the baseline auto-export.
	if err := h.store.SetConfig(ctx, kvPrefix+memoryPrefix+"secret-key", "private context"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	// An unrelated issue mutation triggers the next auto-export cycle.
	if err := h.store.UpdateIssue(ctx, "mem-a", map[string]interface{}{"title": "Alpha updated"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	c2 := h.mustCommit(t, ctx, "mutate")

	_, memCount, _, didIncremental, err := tryIncrementalExport(ctx, exportPath, c1, c2, nil)
	if err != nil {
		t.Fatalf("tryIncrementalExport: %v", err)
	}
	if !didIncremental {
		t.Fatal("expected incremental path to run")
	}
	if memCount != 0 {
		t.Errorf("incremental memoryCount = %d, want 0 (auto-export must never include memories)", memCount)
	}

	got := readFile(t, exportPath)
	if strings.Contains(got, "private context") {
		t.Error("incremental auto-export leaked a memory record that was written after the baseline export — auto-export must never regenerate memories from live config")
	}

	// Control: an explicit full export with memories included stays clean —
	// i.e. this isn't a case where the memory was never written at all.
	fullPath := filepath.Join(t.TempDir(), "full-control.jsonl")
	if _, memCount, err := exportToFile(ctx, fullPath, true); err != nil {
		t.Fatalf("exportToFile control: %v", err)
	} else if memCount != 1 {
		t.Fatalf("control export memoryCount = %d, want 1 (sanity: the memory really was written)", memCount)
	}
}

func TestTryIncrementalExport_PreservesPreExistingMemoryAcrossPatch(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)

	h.mustCreate(t, ctx, "mem-b", "Beta")
	c1 := h.mustCommit(t, ctx, "baseline")

	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	if err := h.store.SetConfig(ctx, kvPrefix+memoryPrefix+"kept-key", "kept context"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	// A memory already exists in the file BEFORE any incremental patching —
	// e.g. from an explicit `bd export --include-memories` the user ran by
	// hand. Auto-export's incremental path must not destroy it.
	if _, memCount, err := exportToFile(ctx, exportPath, true); err != nil {
		t.Fatalf("exportToFile: %v", err)
	} else if memCount != 1 {
		t.Fatalf("baseline memoryCount = %d, want 1", memCount)
	}

	if err := h.store.UpdateIssue(ctx, "mem-b", map[string]interface{}{"title": "Beta updated"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	c2 := h.mustCommit(t, ctx, "mutate")

	_, _, _, didIncremental, err := tryIncrementalExport(ctx, exportPath, c1, c2, nil)
	if err != nil {
		t.Fatalf("tryIncrementalExport: %v", err)
	}
	if !didIncremental {
		t.Fatal("expected incremental path to run")
	}

	got := readFile(t, exportPath)
	if !strings.Contains(got, "kept context") {
		t.Error("incremental patch destroyed a pre-existing memory record instead of preserving it")
	}
}

// TestMaybeAutoExport_WorkingSetRevertIsCorrected is the be-shbed regression
// test for PR #5806 review item 4 (LastDirtyIDs): in server mode, dolt
// auto-commit is off, so uncommitted edits live only in the working set.
// dolt_diff(anchorCommit, 'WORKING') only reports rows that differ from the
// anchor COMMIT — if an issue is dirtied in export cycle N and then reverted
// back to its committed value before cycle N+1, the working set once again
// matches the anchor commit for that row, so dolt_diff reports no change —
// even though the file on disk still shows the stale dirty value from cycle
// N. Carrying forward the previous cycle's dirty IDs and re-patching them
// unconditionally is what corrects this.
func TestMaybeAutoExport_WorkingSetRevertIsCorrected(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)
	initConfigForTest(t)
	config.Set("export.auto", true)
	config.Set("export.interval", "1ms")

	h.mustCreate(t, ctx, "revert-a", "Original title")
	h.mustCommit(t, ctx, "baseline")

	if err := maybeAutoExport(ctx, false); err != nil {
		t.Fatalf("baseline maybeAutoExport: %v", err)
	}
	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	if got := loadIssueTitles(t, exportPath)["revert-a"]; got != "Original title" {
		t.Fatalf("baseline title = %q, want %q", got, "Original title")
	}

	// Dirty the issue in the working set only (no commit) — matches server
	// mode with dolt auto-commit off.
	if err := h.store.UpdateIssue(ctx, "revert-a", map[string]interface{}{"title": "Dirty title"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue (dirty): %v", err)
	}
	if err := maybeAutoExport(ctx, false); err != nil {
		t.Fatalf("dirty maybeAutoExport: %v", err)
	}
	if got := loadIssueTitles(t, exportPath)["revert-a"]; got != "Dirty title" {
		t.Fatalf("dirty title = %q, want %q", got, "Dirty title")
	}

	// Revert to the committed value, again without committing. dolt_diff
	// between the anchor commit and 'WORKING' now reports NO change for
	// revert-a — without carrying it forward as a dirty ID from the
	// previous cycle, the file would incorrectly keep showing "Dirty title".
	if err := h.store.UpdateIssue(ctx, "revert-a", map[string]interface{}{"title": "Original title"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue (revert): %v", err)
	}
	if err := maybeAutoExport(ctx, false); err != nil {
		t.Fatalf("revert maybeAutoExport: %v", err)
	}
	if got := loadIssueTitles(t, exportPath)["revert-a"]; got != "Original title" {
		t.Errorf("after working-set revert, title = %q, want %q (LastDirtyIDs must carry revert-a forward so it gets re-patched even though dolt_diff(anchor, WORKING) reports no change for it)", got, "Original title")
	}
}

func TestTryIncrementalExport_ExcludesConfiguredOwnerFromPatchedIssues(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)
	initConfigForTest(t)
	config.Set("export.exclude_owners", "bot-user")

	h.mustCreate(t, ctx, "own-a", "Kept")
	if err := h.store.CreateIssue(ctx, &types.Issue{
		ID: "own-b", Title: "Original", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask,
		CreatedBy: "bot-user",
	}, "bot-user"); err != nil {
		t.Fatalf("CreateIssue own-b: %v", err)
	}
	c1 := h.mustCommit(t, ctx, "baseline")

	// Baseline full export: exportToFile already applies owner-exclusion, so
	// own-b is correctly absent from the start — this test is about the
	// INCREMENTAL patch path, not the baseline.
	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	if _, _, err := exportToFile(ctx, exportPath, true); err != nil {
		t.Fatalf("exportToFile: %v", err)
	}
	if _, ok := loadIssueTitles(t, exportPath)["own-b"]; ok {
		t.Fatal("sanity check: baseline full export should already exclude own-b")
	}

	// Mutate the excluded-owner issue. If tryIncrementalExport patches
	// changed issues into the file without re-applying owner-exclusion,
	// own-b would leak in here even though it never should have appeared.
	if err := h.store.UpdateIssue(ctx, "own-b", map[string]interface{}{"title": "Mutated"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue own-b: %v", err)
	}
	c2 := h.mustCommit(t, ctx, "mutate")

	_, _, _, didIncremental, err := tryIncrementalExport(ctx, exportPath, c1, c2, nil)
	if err != nil {
		t.Fatalf("tryIncrementalExport: %v", err)
	}
	if !didIncremental {
		t.Fatal("expected incremental path to run")
	}

	titles := loadIssueTitles(t, exportPath)
	if _, leaked := titles["own-b"]; leaked {
		t.Error("own-b belongs to an excluded owner but was patched into the export by the incremental path")
	}
	if _, ok := titles["own-a"]; !ok {
		t.Error("own-a (kept owner, untouched) missing from export")
	}
}

func TestTryIncrementalExport_PatchedLinesIncludeTypeField(t *testing.T) {
	h, ctx := setupIncrementalExportTest(t)

	h.mustCreate(t, ctx, "typ-a", "Alpha")
	c1 := h.mustCommit(t, ctx, "baseline")

	exportPath := filepath.Join(h.beadsDir, "issues.jsonl")
	if _, _, err := exportToFile(ctx, exportPath, true); err != nil {
		t.Fatalf("exportToFile: %v", err)
	}

	if err := h.store.UpdateIssue(ctx, "typ-a", map[string]interface{}{"title": "Alpha renamed"}, "tester"); err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}
	c2 := h.mustCommit(t, ctx, "mutate")

	_, _, _, didIncremental, err := tryIncrementalExport(ctx, exportPath, c1, c2, nil)
	if err != nil {
		t.Fatalf("tryIncrementalExport: %v", err)
	}
	if !didIncremental {
		t.Fatal("expected incremental path to run")
	}

	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec struct {
			ID   string `json:"id"`
			Type string `json:"_type"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal line %q: %v", line, err)
		}
		if rec.ID != "typ-a" {
			continue
		}
		found = true
		if rec.Type != "issue" {
			t.Errorf(`patched line for typ-a has _type=%q, want "issue" — every issue record, including incrementally-patched ones, must carry a _type field so readers (and the auto-export shrink guard's own classifyExistingAutoExportRecord) can distinguish it from a memory or unknown record`, rec.Type)
		}
	}
	if !found {
		t.Fatal("typ-a not found in export after incremental patch")
	}
}

// TestNewTestStoreWithReadTimeout_AppliesConfiguredTimeout proves the
// PoolReadTimeout parameter added for be-uoat round 2 actually reaches the
// live connection, rather than just being accepted and ignored. An
// unreasonably short timeout must make store creation fail fast (the
// configured value is live); a normal one must still succeed (the plumbing
// doesn't break the default path). TestBuildServerDSN_PoolTimeouts
// (internal/storage/dolt/store_unit_test.go) already covers that
// Config.PoolReadTimeout is formatted into the DSN correctly — this test is
// at the cmd/bd harness layer instead, against the real test Dolt server, to
// prove the new newTestStoreSharedBranchWithReadTimeout/
// newTestStoreWithPrefixAndReadTimeout plumbing actually threads the caller's
// value through to that mechanism.
func TestNewTestStoreWithReadTimeout_AppliesConfiguredTimeout(t *testing.T) {
	if testDoltServerPort == 0 {
		t.Skip("Dolt test server not available")
	}
	if testutil.DoltContainerCrashed() {
		t.Skipf("Dolt test server crashed: %v", testutil.DoltContainerCrashError())
	}
	ensureTestMode(t)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".beads", "dolt")
	// 1ns can never survive a real handshake/query round trip — this is
	// not a race with a slow-but-real server, it's a guaranteed trip.
	s, err := tryNewTestStoreWithReadTimeout(t, dbPath, 1*time.Nanosecond)
	if err == nil {
		s.Close()
		t.Fatal("expected store creation to fail with an unreasonably short PoolReadTimeout, but it succeeded")
	}

	t.Run("normal timeout still succeeds", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, ".beads", "dolt")
		s := newTestStoreWithPrefixAndReadTimeout(t, dbPath, "test", bulkSeedPoolReadTimeout)
		if s == nil {
			t.Fatal("expected non-nil store")
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
