//go:build integration

// Integration tier (be-b23): moved verbatim from the unit-tier sibling file.
// These tests run bd init against a fresh store, which migrates; the unit
// tier's BD_TEST_TIER tripwire refuses that.
package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// showStrayMetadata fetches an issue's metadata map via bd show --json.
func showStrayMetadata(t *testing.T, bd, dir, id string) map[string]interface{} {
	t.Helper()
	stdout, stderr, code := runBDMultiID(t, bd, dir, "show", id, "--json")
	if code != 0 {
		t.Fatalf("bd show %s failed (exit %d):\nstdout:\n%s\nstderr:\n%s", id, code, stdout, stderr)
	}
	var details []struct {
		ID       string                 `json:"id"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(stdout), &details); err != nil {
		t.Fatalf("parsing show --json for %s: %v\n%s", id, err, stdout)
	}
	if len(details) != 1 || details[0].ID != id {
		t.Fatalf("show --json for %s returned unexpected issues:\n%s", id, stdout)
	}
	return details[0].Metadata
}

func TestUpdateStrayMetadataPositionalRefusedBeforeWrite(t *testing.T) {
	bd, dir := setupMultiIDUpdateDB(t)
	id := createMultiIDUpdateIssue(t, bd, dir, "stray metadata target")

	// Only `probe_a=1` binds to the flag; `probe_b=2` and `probe_c=3` land as
	// positional ids. The command must refuse before writing anything.
	stdout, stderr, code := runBDMultiID(t, bd, dir,
		"update", id, "--set-metadata", "probe_a=1", "probe_b=2", "probe_c=3")
	if code == 0 {
		t.Errorf("bd update with a '='-bearing positional exited 0, want nonzero\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(stderr, "probe_b=2") {
		t.Errorf("stderr does not name the mis-typed pair probe_b=2:\n%s", stderr)
	}

	// The refusal is before any write: no pair — not even the bound probe_a —
	// may have landed. This is the half a message-only fix would leave broken.
	meta := showStrayMetadata(t, bd, dir, id)
	if len(meta) != 0 {
		t.Errorf("metadata is %v, want empty: refusal must prevent the partial write", meta)
	}
}

func TestUpdateSetMetadataRepeatedFlagStillWrites(t *testing.T) {
	bd, dir := setupMultiIDUpdateDB(t)
	id := createMultiIDUpdateIssue(t, bd, dir, "correct form target")

	// The correct form (one flag per pair) has no stray positional and must
	// still apply normally — the guard does not break valid usage.
	stdout, stderr, code := runBDMultiID(t, bd, dir,
		"update", id, "--set-metadata", "probe_a=1", "--set-metadata", "probe_b=2")
	if code != 0 {
		t.Fatalf("bd update with repeated --set-metadata exited %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	meta := showStrayMetadata(t, bd, dir, id)
	if meta["probe_a"] == nil || meta["probe_b"] == nil {
		t.Errorf("metadata = %v, want both probe_a and probe_b written", meta)
	}
}
