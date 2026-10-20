package dolt

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
)

// TestGetReadyWorkWakesExpiredDeferOnReadOnlyStore is the `bd ready` path: the
// command is on the read-only allowlist, so its store opens ReadOnly with the
// sweep opted in, and the ready read must return a dated defer whose date has
// passed.
func TestGetReadyWorkWakesExpiredDeferOnReadOnlyStore(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()
	ctx := context.Background()

	iss := &types.Issue{ID: "bd-wake-ro", Title: "dated defer", IssueType: types.TypeTask, Status: types.StatusOpen}
	if err := store.CreateIssue(ctx, iss, "creator-1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.UpdateIssue(ctx, iss.ID, map[string]interface{}{
		"status":      "deferred",
		"defer_until": time.Now().UTC().Add(-48 * time.Hour),
	}, "deferrer-1"); err != nil {
		t.Fatalf("defer: %v", err)
	}

	store.readOnly, store.sweepDefersOnRead = true, true
	ready, err := store.GetReadyWork(ctx, types.WorkFilter{})
	store.readOnly, store.sweepDefersOnRead = false, false
	if err != nil {
		t.Fatalf("ready work: %v", err)
	}

	got, err := store.GetIssue(ctx, iss.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != types.StatusOpen {
		t.Errorf("status after ready read = %q, want %q", got.Status, types.StatusOpen)
	}
	if !containsIssue(ready, iss.ID) {
		t.Errorf("ready read did not return the woken defer %s", iss.ID)
	}

	var commits int
	if err := store.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM dolt_log WHERE message = ?",
		issueops.WakeDefersCommitMessage(1)).Scan(&commits); err != nil {
		t.Fatalf("count wake commits: %v", err)
	}
	if commits == 0 {
		t.Errorf("no dolt commit with message %q", issueops.WakeDefersCommitMessage(1))
	}
}

// TestGetReadyWorkReadOnlyWithoutSweepLeavesDeferAlone pins the other half of
// the contract: a read-only open that did not opt in (strict --readonly,
// preview, doctor) must not write, so the expired defer stays deferred.
func TestGetReadyWorkReadOnlyWithoutSweepLeavesDeferAlone(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()
	ctx := context.Background()

	iss := &types.Issue{ID: "bd-wake-strict", Title: "dated defer", IssueType: types.TypeTask, Status: types.StatusOpen}
	if err := store.CreateIssue(ctx, iss, "creator-1"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.UpdateIssue(ctx, iss.ID, map[string]interface{}{
		"status":      "deferred",
		"defer_until": time.Now().UTC().Add(-48 * time.Hour),
	}, "deferrer-1"); err != nil {
		t.Fatalf("defer: %v", err)
	}

	store.readOnly = true
	_, err := store.GetReadyWork(ctx, types.WorkFilter{})
	store.readOnly = false
	if err != nil {
		t.Fatalf("ready work: %v", err)
	}

	got, err := store.GetIssue(ctx, iss.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != types.StatusDeferred {
		t.Errorf("status after read-only ready read = %q, want %q (no sweep without opt-in)", got.Status, types.StatusDeferred)
	}
}

func containsIssue(issues []*types.Issue, id string) bool {
	for _, iss := range issues {
		if iss.ID == id {
			return true
		}
	}
	return false
}

// TestDeferWakeSkipWarnsOncePerProcess pins the stderr contract for the sweep's
// swallowed errors: circuit-open and store-closed are reported, each shape once.
func TestDeferWakeSkipWarnsOncePerProcess(t *testing.T) {
	var w deferWakeWarnings
	var out bytes.Buffer
	for _, err := range []error{ErrCircuitOpen, ErrCircuitOpen, ErrStoreClosed, ErrStoreClosed} {
		w.report(&out, fmt.Errorf("wake expired defers: %w", err))
	}
	if got := strings.Count(out.String(), "warning: defer-wake sweep skipped"); got != 2 {
		t.Errorf("warnings = %d, want 2 (one per shape):\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), "write circuit open") || !strings.Contains(out.String(), "store closed") {
		t.Errorf("warnings missing a shape:\n%s", out.String())
	}
}
