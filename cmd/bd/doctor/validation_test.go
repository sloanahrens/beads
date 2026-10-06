//go:build cgo

package doctor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/configfile"
)

// TestCheckDuplicateIssues_NoDatabase verifies graceful handling when no database exists.
func TestCheckDuplicateIssues_NoDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.Mkdir(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write metadata.json pointing to a unique nonexistent database so that
	// openStoreDB doesn't fall back to the shared default "beads" database.
	h := sha256.Sum256([]byte(t.Name() + fmt.Sprintf("%d", time.Now().UnixNano())))
	noDbName := "doctest_nodb_" + hex.EncodeToString(h[:6])
	cfg := configfile.DefaultConfig()
	cfg.Backend = configfile.BackendDolt
	cfg.DoltDatabase = noDbName
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	check := CheckDuplicateIssues(tmpDir, false, 1000)

	if check.Status != StatusOK {
		t.Errorf("Status = %q, want %q", check.Status, StatusOK)
	}
	// When no Dolt database exists, openStoreDB may create an empty one but
	// the duplicate query will fail since no schema exists.
	wantMessages := []string{"N/A (no database)", "N/A (unable to query issues)"}
	found := false
	for _, msg := range wantMessages {
		if check.Message == msg {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Message = %q, want one of %v", check.Message, wantMessages)
	}
}

// TestCheckTestPollution_NoTestIssues_NoServer verifies StatusOK when no Dolt
// server is reachable. This isolates BEADS_DOLT_PORT set by TestMain (which
// starts a Docker-based Dolt container on Ubuntu but not macOS) so the test
// exercises the "no database" code path deterministically on all platforms.
func TestCheckTestPollution_NoTestIssues_NoServer(t *testing.T) {
	for _, key := range []string{"BEADS_DOLT_PORT", "BEADS_DOLT_SERVER_PORT"} {
		if orig, ok := os.LookupEnv(key); ok {
			t.Cleanup(func() { os.Setenv(key, orig) })
			os.Unsetenv(key)
		}
	}

	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	check := CheckTestPollution(tmpDir)

	if check.Status != StatusOK {
		t.Errorf("Status = %q, want %q", check.Status, StatusOK)
	}
	if check.Message != "N/A (no database)" {
		t.Errorf("Message = %q, want %q", check.Message, "N/A (no database)")
	}
}

// TestCheckGitConflicts_DoltBackend_NoDB verifies CheckGitConflicts returns N/A
// when the Dolt database directory doesn't exist.
func TestCheckGitConflicts_DoltBackend_NoDB(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Default backend is Dolt when no config exists
	check := CheckGitConflicts(tmpDir)

	if check.Status != StatusOK {
		t.Errorf("Status = %q, want %q", check.Status, StatusOK)
	}
	// Without a Dolt database, should report N/A
	wantMessages := []string{"N/A (no Dolt database)", "N/A (unable to open database)"}
	found := false
	for _, msg := range wantMessages {
		if check.Message == msg {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Message = %q, want one of %v", check.Message, wantMessages)
	}
}
