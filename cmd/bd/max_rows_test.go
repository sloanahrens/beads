//go:build cgo

// be-u8z9: CLI-layer behavioral tests for BEADS_MAX_ROWS / --max-rows on the
// command paths beyond `bd list`. Each subtest:
//   - inits a fresh rig (own DB so the row counts are exact)
//   - exercises the command via exec.Command(bd, ...)
//   - asserts the process exited with code 2 (cap exceeded) and that stderr
//     names the source ("--max-rows=N" or "BEADS_MAX_ROWS=N").
//
// The doctor-family commands (lint, doctor-conventions, doctor-pollution)
// are env-only by design (designer §4); they do NOT register --max-rows
// as a flag. A separate subtest asserts cobra rejects the flag.
//
// Gating matches the other embedded-dolt CLI tests: requires
// BEADS_TEST_EMBEDDED_DOLT=1 to opt in.

package main

import (
	"testing"

	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/workapi"
)

// TestWithFetchOneExtra_LimitEqualsCap_BumpsBothForTruncationProbe verifies
// the exact bump mechanism workapi.WithFetchOneExtra uses to reconcile the
// >N-matches truncation probe (GH#3212) with a MaxRows cap equal to the
// user's --limit (be-x42v.4 round-3 follow-up).
//
// The end-to-end LimitEqualsCap_TruncatesNotErrors subprocess test in
// TestEmbeddedMaxRowsList can only assert the *absence* of the false cap
// error and the trimmed row count — it can't observe the truncation
// *notice* text, which printTruncationHint gates on ui.IsStderrTerminal()
// and is unconditionally suppressed for a subprocess's piped stderr (see
// list_embedded_test.go's limit_truncation_hint subtest, which documents
// the same TTY constraint). This unit test instead asserts the underlying
// signal directly: with Limit==MaxRows, both must bump by one so the query
// still over-fetches by one row (restoring len(results) > effectiveLimit
// detection) while EnforceMaxRowsCap doesn't trip on that extra row.
func TestWithFetchOneExtra_LimitEqualsCap_BumpsBothForTruncationProbe(t *testing.T) {
	got := workapi.WithFetchOneExtra(types.IssueFilter{Limit: 5, MaxRows: 5, MaxRowsSource: "--max-rows"})
	if got.Limit != 6 {
		t.Errorf("Limit == MaxRows: Limit = %d, want 6 (bumped so the query still fetches the truncation-detection probe row)", got.Limit)
	}
	if got.MaxRows != 6 {
		t.Errorf("Limit == MaxRows: MaxRows = %d, want 6 (bumped in lockstep so the probe row alone doesn't trip EnforceMaxRowsCap)", got.MaxRows)
	}
	if got.MaxRowsSource != "--max-rows" {
		t.Errorf("MaxRowsSource must be preserved unchanged, got %q", got.MaxRowsSource)
	}
}

// TestWithFetchOneExtra_LimitOverCap_OnlyBumpsLimit covers the tighter-cap
// case (--limit 100 --max-rows 5, LimitSet_CapTighter): MaxRows must NOT
// bump, or a genuine cap violation would report the wrong Cap value (N+1
// instead of the user's true --max-rows=N) in the error message.
func TestWithFetchOneExtra_LimitOverCap_OnlyBumpsLimit(t *testing.T) {
	got := workapi.WithFetchOneExtra(types.IssueFilter{Limit: 100, MaxRows: 5})
	if got.Limit != 101 {
		t.Errorf("Limit = %d, want 101", got.Limit)
	}
	if got.MaxRows != 5 {
		t.Errorf("MaxRows must stay unbumped so a real cap violation reports the true cap; got %d, want 5", got.MaxRows)
	}
}

// TestWithFetchOneExtra_LimitUnderCap_OnlyBumpsLimit covers the
// looser-cap case (--limit 5 --max-rows 100, LimitSet_CapLooser): the
// probe-row bump alone never crosses EffectiveSearchLimit's `limit >
// maxRows` branch here, so no MaxRows adjustment is needed.
func TestWithFetchOneExtra_LimitUnderCap_OnlyBumpsLimit(t *testing.T) {
	got := workapi.WithFetchOneExtra(types.IssueFilter{Limit: 5, MaxRows: 100})
	if got.Limit != 6 {
		t.Errorf("Limit = %d, want 6", got.Limit)
	}
	if got.MaxRows != 100 {
		t.Errorf("MaxRows must stay unbumped, got %d, want 100", got.MaxRows)
	}
}

// TestWithFetchOneExtra_NoLimit_Unaffected covers the unlimited case
// (Limit == 0): workapi.WithFetchOneExtra is a no-op regardless of MaxRows.
func TestWithFetchOneExtra_NoLimit_Unaffected(t *testing.T) {
	got := workapi.WithFetchOneExtra(types.IssueFilter{Limit: 0, MaxRows: 5})
	if got.Limit != 0 || got.MaxRows != 5 {
		t.Errorf("unlimited Limit must pass through unchanged, got Limit=%d MaxRows=%d", got.Limit, got.MaxRows)
	}
}
