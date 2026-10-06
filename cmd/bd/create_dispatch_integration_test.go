//go:build cgo && integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// TestCreateSingleIssueAllowEmptyDescriptionRoundTrip exercises the opt-in
// success path end to end: a single-issue (non --file/--graph) create with an
// empty description read from an external source (--body-file pointing at an
// empty file) plus --allow-empty-description should actually create the
// issue, not merely register the flag (the review's should-fix #2: thin
// coverage of the opt-in path).
func TestCreateSingleIssueAllowEmptyDescriptionRoundTrip(t *testing.T) {
	saveAndRestoreGlobals(t)
	ensureCleanGlobalState(t)

	// saveAndRestoreGlobals doesn't cover these three; restore them
	// explicitly so this test can't contaminate the package run.
	savedRootCtx, savedJSONOutput, savedActor := rootCtx, jsonOutput, actor
	t.Cleanup(func() {
		rootCtx, jsonOutput, actor = savedRootCtx, savedJSONOutput, savedActor
	})

	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)

	store = s
	rootCtx = context.Background()
	jsonOutput = false
	readonlyMode = false
	actor = "test-actor"
	t.Cleanup(func() { readonlyMode = false })

	bodyFilePath := filepath.Join(tmpDir, "empty-body.md")
	if err := os.WriteFile(bodyFilePath, nil, 0644); err != nil {
		t.Fatalf("write empty body file: %v", err)
	}

	bodyFileFlag := createCmd.Flags().Lookup("body-file")
	allowEmptyFlag := createCmd.Flags().Lookup("allow-empty-description")
	t.Cleanup(func() {
		_ = bodyFileFlag.Value.Set("")
		bodyFileFlag.Changed = false
		_ = allowEmptyFlag.Value.Set("false")
		allowEmptyFlag.Changed = false
	})

	if err := createCmd.Flags().Set("body-file", bodyFilePath); err != nil {
		t.Fatalf("set --body-file: %v", err)
	}
	if err := createCmd.Flags().Set("allow-empty-description", "true"); err != nil {
		t.Fatalf("set --allow-empty-description: %v", err)
	}

	title := "Allow empty description round trip"
	if err := createCmd.RunE(createCmd, []string{title}); err != nil {
		t.Fatalf("expected --allow-empty-description opt-in to succeed, got: %v", err)
	}

	issues, err := s.SearchIssues(rootCtx, "", types.IssueFilter{})
	if err != nil {
		t.Fatalf("search issues: %v", err)
	}
	var created bool
	for _, iss := range issues {
		if iss.Title == title {
			created = true
			if iss.Description != "" {
				t.Fatalf("expected empty description, got %q", iss.Description)
			}
		}
	}
	if !created {
		t.Fatalf("expected issue %q to be created via --allow-empty-description opt-in", title)
	}
}
