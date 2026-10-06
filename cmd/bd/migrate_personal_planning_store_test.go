//go:build cgo

package main

import (
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
)

// TestOpenMigrationPlanningStore_RefusesEmbeddedPlanningWorkspace pins the
// shape be-nqt gave migrate-personal, in the world after embedded Dolt was
// removed (be-xu2.2): the planning workspace is opened by its own
// metadata.json, exactly as create/--repo opens any foreign workspace, and an
// embedded workspace's metadata meets the same fail-closed refusal as any
// other — naming the removal.
//
// The message is the assertion. A bare dolt.New (the pre-be-nqt shape, server
// mode regardless of metadata) fails this the other way: it connects to
// 127.0.0.1:0 and reports "Dolt server unreachable", which reads as a
// connectivity problem rather than a workspace that can no longer be opened.
func TestOpenMigrationPlanningStore_RefusesEmbeddedPlanningWorkspace(t *testing.T) {
	planningBeadsDir := t.TempDir()
	cfg := &configfile.Config{
		Database:     "dolt",
		DoltDatabase: "planning",
		DoltMode:     configfile.DoltModeEmbedded,
	}
	if err := cfg.Save(planningBeadsDir); err != nil {
		t.Fatalf("save planning config: %v", err)
	}

	_, err := openMigrationPlanningStore(t.Context(), planningBeadsDir)
	if err == nil {
		t.Fatal("openMigrationPlanningStore opened an embedded planning workspace; want the embedded-removal refusal")
	}
	if !strings.Contains(err.Error(), embeddedRemovedErrMsg) {
		t.Errorf("want the embedded-removal refusal, got: %v", err)
	}
}
