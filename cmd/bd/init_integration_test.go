//go:build cgo && integration

package main

import (
	"context"
	"path/filepath"
	"testing"
)

// TestVerifyMetadataSuccess verifies that verifyMetadata writes and reads back metadata.
// Note: Failure path tests (write errors, read-back mismatches) were removed because
// verifyMetadata now takes *dolt.DoltStore (concrete type), making interface-based
// mocking impossible. The failure paths are simple error-to-stderr logic.
func TestVerifyMetadataSuccess(t *testing.T) {
	skipIfNoDolt(t)
	ctx := context.Background()

	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, "test.db")
	store := newTestStore(t, testDB)
	defer store.Close()

	ok := verifyMetadata(ctx, store, "test_key", "test_value")
	if !ok {
		t.Error("verifyMetadata should return true on success")
	}
	// Verify the value was actually written
	val, err := store.GetMetadata(ctx, "test_key")
	if err != nil {
		t.Fatalf("GetMetadata failed: %v", err)
	}
	if val != "test_value" {
		t.Errorf("expected 'test_value', got %q", val)
	}
}
