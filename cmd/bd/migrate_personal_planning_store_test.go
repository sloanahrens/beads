//go:build cgo

package main

import (
	"os"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
)

// be-nqt: migrate-personal opened the planning workspace with a bare
// dolt.New, i.e. server mode regardless of how that workspace was
// initialized. An embedded planning repo (what `bd init` creates by default)
// therefore failed with "Dolt server unreachable at 127.0.0.1:0", and the
// end-to-end test's skip clause matched "server" in that output and hid it.
// The planning store must be opened the way every other foreign workspace is:
// by its own metadata.json.
func TestOpenMigrationPlanningStore_HonorsEmbeddedPlanningWorkspace(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt tests")
	}
	planningBeadsDir := t.TempDir()
	cfg := &configfile.Config{
		Database:     "dolt",
		DoltDatabase: "planning",
		DoltMode:     configfile.DoltModeEmbedded,
	}
	if err := cfg.Save(planningBeadsDir); err != nil {
		t.Fatalf("save planning config: %v", err)
	}

	store, err := openMigrationPlanningStore(t.Context(), planningBeadsDir)
	if err != nil {
		t.Fatalf("openMigrationPlanningStore on an embedded planning workspace: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
