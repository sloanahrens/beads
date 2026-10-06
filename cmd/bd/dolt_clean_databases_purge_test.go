//go:build cgo

package main

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// TestShouldPurgeDroppedDatabasesGatesOnFlagAlone pins the --purge-dropped
// gating contract at the pure-function level: purge fires if and only if
// the flag is set, regardless of how many databases this run dropped. In
// particular it fixes a P1 found in review of the initial version of this
// change: gating on "did this run drop anything" instead of the flag alone
// meant a run that found zero stale databases (stale == 0, e.g. because a
// prior run already dropped them but never purged) silently skipped the
// purge even with --purge-dropped set, leaving that residue unreclaimed
// forever.
func TestShouldPurgeDroppedDatabasesGatesOnFlagAlone(t *testing.T) {
	cases := []struct {
		name         string
		purgeDropped bool
		droppedCount int
		want         bool
	}{
		{"flag off, nothing dropped this run", false, 0, false},
		{"flag off, dropped this run", false, 3, false},
		{"flag on, nothing dropped this run (residue case)", true, 0, true},
		{"flag on, dropped this run", true, 3, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldPurgeDroppedDatabases(tc.purgeDropped, tc.droppedCount); got != tc.want {
				t.Errorf("shouldPurgeDroppedDatabases(%v, %d) = %v, want %v",
					tc.purgeDropped, tc.droppedCount, got, tc.want)
			}
		})
	}
}

func mustExec(t *testing.T, ctx context.Context, db *sql.DB, query string) {
	t.Helper()
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(execCtx, query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
