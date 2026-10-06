//go:build cgo

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/storage/dolt"
)

// setupValidateTestDB creates a temp .beads workspace with a configured database.
// Uses newTestStoreWithPrefix to ensure metadata.json has the correct database name
// so that collectValidateChecks (which reads metadata.json) connects to the right DB.
func setupValidateTestDB(t *testing.T, prefix string) (tmpDir string, store *dolt.DoltStore) {
	t.Helper()
	tmpDir = t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.Mkdir(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(beadsDir, "dolt")
	store = newTestStoreIsolatedDB(t, dbPath, prefix)

	return tmpDir, store
}

func TestValidateCheck_NoBeadsDir(t *testing.T) {
	tmpDir := t.TempDir()

	checks := collectValidateChecks(tmpDir)

	for _, cr := range checks {
		if cr.check.Status != statusOK {
			t.Errorf("%s: status = %q, want %q when no .beads/ exists", cr.check.Name, cr.check.Status, statusOK)
		}
	}
}

func TestValidateOverallOK(t *testing.T) {
	allPass := []validateCheckResult{
		{check: doctorCheck{Status: statusOK}},
		{check: doctorCheck{Status: statusOK}},
	}
	if !validateOverallOK(allPass) {
		t.Error("Expected true when all checks pass")
	}

	hasWarning := []validateCheckResult{
		{check: doctorCheck{Status: statusOK}},
		{check: doctorCheck{Status: statusWarning}},
	}
	if validateOverallOK(hasWarning) {
		t.Error("Expected false when a check has warning")
	}

	hasError := []validateCheckResult{
		{check: doctorCheck{Status: statusOK}},
		{check: doctorCheck{Status: statusError}},
	}
	if validateOverallOK(hasError) {
		t.Error("Expected false when a check has error")
	}
}
