//go:build cgo && integration

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/testutil"
	"github.com/steveyegge/beads/internal/types"
)

// TestUpdateMetadataInlineJSON tests inline JSON metadata update
func TestUpdateMetadataInlineJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".beads", "beads.db")

	// Create storage
	ctx := context.Background()
	store, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer store.Close()

	// Initialize database with issue prefix
	if err := store.SetConfig(ctx, "issue_prefix", "bd"); err != nil {
		t.Fatalf("failed to set issue_prefix: %v", err)
	}

	// Create an issue
	issue := &types.Issue{
		ID:        "bd-test1",
		Title:     "Test Issue",
		Status:    "open",
		IssueType: "task",
	}
	if err := store.CreateIssue(ctx, issue, "test-actor"); err != nil {
		t.Fatalf("failed to create issue: %v", err)
	}

	// Update with inline JSON metadata
	metadata := `{"key": "value", "nested": {"foo": "bar"}}`
	updates := map[string]interface{}{
		"metadata": json.RawMessage(metadata),
	}
	if err := store.UpdateIssue(ctx, "bd-test1", updates, "test-actor"); err != nil {
		t.Fatalf("failed to update issue with metadata: %v", err)
	}

	// Verify the metadata was stored
	updated, err := store.GetIssue(ctx, "bd-test1")
	if err != nil {
		t.Fatalf("failed to get issue: %v", err)
	}
	if updated.Metadata == nil {
		t.Fatal("metadata should not be nil after update")
	}
	// Compare as parsed JSON (MySQL/Dolt normalizes JSON whitespace on storage)
	var expectedJSON, actualJSON interface{}
	if err := json.Unmarshal([]byte(metadata), &expectedJSON); err != nil {
		t.Fatalf("failed to parse expected metadata: %v", err)
	}
	if err := json.Unmarshal(updated.Metadata, &actualJSON); err != nil {
		t.Fatalf("failed to parse actual metadata: %v", err)
	}
	expectedBytes, _ := json.Marshal(expectedJSON)
	actualBytes, _ := json.Marshal(actualJSON)
	if string(expectedBytes) != string(actualBytes) {
		t.Errorf("expected metadata %s, got %s", expectedBytes, actualBytes)
	}
}
