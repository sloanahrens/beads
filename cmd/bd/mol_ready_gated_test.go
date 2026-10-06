//go:build cgo

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/testutil"
)

// setupGatedTestDB creates a temporary file-based test database
func setupGatedTestDB(t *testing.T) (*dolt.DoltStore, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "bd-test-gated-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	testDB := filepath.Join(tmpDir, "test.db")
	store, err := dolt.New(context.Background(), &dolt.Config{Path: testDB})
	if err != nil {
		os.RemoveAll(tmpDir)
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}

	// Set issue_prefix (required for beads)
	ctx := context.Background()
	if err := store.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		store.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to set issue_prefix: %v", err)
	}

	// Configure orchestrator custom types for test compatibility (bd-find4)
	if err := store.SetConfig(ctx, "types.custom", "molecule,gate,convoy,merge-request,slot,agent,role,rig,event,message"); err != nil {
		store.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to set types.custom: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

// =============================================================================
// mol ready --gated Tests (bd-lhalq: Gate-resume discovery)
// =============================================================================
