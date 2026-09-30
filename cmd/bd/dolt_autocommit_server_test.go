//go:build cgo

package main

import (
	"context"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	storageissueops "github.com/steveyegge/beads/internal/storage/issueops"
)

func saveStorageMode(t *testing.T) {
	t.Helper()
	oldServerMode := serverMode
	oldCmdCtx := cmdCtx
	oldStore := store
	oldUseGlobals := testModeUseGlobals
	oldDoltAutoCommit := doltAutoCommit
	t.Setenv("BEADS_DOLT_SHARED_SERVER", "0")
	testModeUseGlobals = true
	cmdCtx = nil
	doltAutoCommit = string(doltAutoCommitOn)
	t.Cleanup(func() {
		serverMode = oldServerMode
		cmdCtx = oldCmdCtx
		store = oldStore
		testModeUseGlobals = oldUseGlobals
		doltAutoCommit = oldDoltAutoCommit
	})
}

// fakeTransactStore captures the commit message RunInTransaction receives so
// tests can assert the auto-commit policy blanked (or preserved) it.
type fakeTransactStore struct {
	storage.DoltStorage
	msgs []string
}

func (f *fakeTransactStore) RunInTransaction(_ context.Context, commitMsg string, fn func(tx storage.Transaction) error) error {
	f.msgs = append(f.msgs, commitMsg)
	return fn(nil)
}

// TestIssueOpsContextDefersByModeInServerMode covers bd-4wamg: batch/off must
// defer the storage layer's per-write version commit in SQL-server mode, not
// only embedded mode. An unset mode means no Dolt default was resolved
// (non-Dolt backend) and must not defer.
func TestIssueOpsContextDefersByModeInServerMode(t *testing.T) {
	for _, tc := range []struct {
		mode         string
		wantDeferred bool
	}{
		{mode: string(doltAutoCommitOn), wantDeferred: false},
		{mode: string(doltAutoCommitBatch), wantDeferred: true},
		{mode: string(doltAutoCommitOff), wantDeferred: true},
		{mode: "", wantDeferred: false},
	} {
		t.Run("mode="+tc.mode, func(t *testing.T) {
			saveStorageMode(t)
			serverMode = true
			doltAutoCommit = tc.mode

			ctx, err := issueOpsContext(context.Background())
			if err != nil {
				t.Fatalf("issueOpsContext: %v", err)
			}
			if got := storageissueops.VersionCommitDeferred(ctx); got != tc.wantDeferred {
				t.Fatalf("VersionCommitDeferred = %v, want %v for mode %q in server mode", got, tc.wantDeferred, tc.mode)
			}
		})
	}
}

// TestTransactHonoringAutoCommitBlanksMessageInServerBatch: the blank message
// is what makes StageAndCommit skip the version commit, and it must happen in
// server mode too (bd-4wamg).
func TestTransactHonoringAutoCommitBlanksMessageInServerBatch(t *testing.T) {
	for _, tc := range []struct {
		mode    string
		wantMsg string
	}{
		{mode: string(doltAutoCommitOn), wantMsg: "bd: test write"},
		{mode: string(doltAutoCommitBatch), wantMsg: ""},
		{mode: string(doltAutoCommitOff), wantMsg: ""},
	} {
		t.Run("mode="+tc.mode, func(t *testing.T) {
			saveStorageMode(t)
			serverMode = true
			doltAutoCommit = tc.mode
			commandDidExplicitDoltCommit = false

			fake := &fakeTransactStore{}
			err := transactHonoringAutoCommit(context.Background(), fake, "bd: test write", func(tx storage.Transaction) error {
				return nil
			})
			if err != nil {
				t.Fatalf("transactHonoringAutoCommit: %v", err)
			}
			if len(fake.msgs) != 1 || fake.msgs[0] != tc.wantMsg {
				t.Fatalf("RunInTransaction messages = %q, want [%q] for mode %q", fake.msgs, tc.wantMsg, tc.mode)
			}
			wantExplicit := tc.wantMsg != ""
			if commandDidExplicitDoltCommit != wantExplicit {
				t.Fatalf("commandDidExplicitDoltCommit = %v, want %v for mode %q", commandDidExplicitDoltCommit, wantExplicit, tc.mode)
			}
		})
	}
}
