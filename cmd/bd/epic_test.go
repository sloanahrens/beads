//go:build cgo

package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
)

type epicTestHelper struct {
	s   *dolt.DoltStore
	ctx context.Context
}

func newEpicTestHelper(t *testing.T) *epicTestHelper {
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	return &epicTestHelper{
		s:   newTestStore(t, testDB),
		ctx: context.Background(),
	}
}

func (h *epicTestHelper) createIssue(t *testing.T, issue *types.Issue) {
	t.Helper()
	if err := h.s.CreateIssue(h.ctx, issue, "test"); err != nil {
		t.Fatal(err)
	}
}

func (h *epicTestHelper) addDependency(t *testing.T, dep *types.Dependency) {
	t.Helper()
	if err := h.s.AddDependency(h.ctx, dep, "test"); err != nil {
		t.Fatal(err)
	}
}

func (h *epicTestHelper) getEpicStatus(t *testing.T, epicID string) *types.EpicStatus {
	t.Helper()
	epics, err := h.s.GetEpicsEligibleForClosure(h.ctx)
	if err != nil {
		t.Fatalf("GetEpicsEligibleForClosure failed: %v", err)
	}

	for _, epic := range epics {
		if epic.Epic.ID == epicID {
			return epic
		}
	}
	return nil
}

func TestEpicCommandInit(t *testing.T) {
	if epicCmd == nil {
		t.Fatal("epicCmd should be initialized")
	}

	if epicCmd.Use != "epic" {
		t.Errorf("Expected Use='epic', got %q", epicCmd.Use)
	}

	var hasStatusCmd bool
	for _, cmd := range epicCmd.Commands() {
		if cmd.Use == "status" {
			hasStatusCmd = true
		}
	}

	if !hasStatusCmd {
		t.Error("epic command should have status subcommand")
	}
}
