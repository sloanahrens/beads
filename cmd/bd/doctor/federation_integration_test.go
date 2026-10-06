//go:build cgo && integration

package doctor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
)

func TestCheckLegacyCLIRemotesDetectsServerRootOnlyRemote(t *testing.T) {
	port := doctorTestServerPort()
	if port == 0 {
		t.Skip("Dolt test server not available")
	}
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skipf("dolt binary not available: %v", err)
	}

	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	doltDir := filepath.Join(beadsDir, "dolt")
	cliDir := filepath.Join(doltDir, testSharedDB)
	if err := os.MkdirAll(cliDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &configfile.Config{
		Backend:        configfile.BackendDolt,
		DoltMode:       "server",
		DoltServerHost: "127.0.0.1",
		DoltServerPort: port,
		DoltDatabase:   testSharedDB,
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	runDoctorTestCmd(t, doltDir, "dolt", "init", "--name", "test", "--email", "test@test.com")
	runDoctorTestCmd(t, cliDir, "dolt", "init", "--name", "test", "--email", "test@test.com")
	rootOnlyURL := "file:///tmp/root-only-remote.git"
	runDoctorTestCmd(t, doltDir, "dolt", "remote", "add", "rootonly", rootOnlyURL)

	check := CheckLegacyCLIRemotes(tmpDir)
	if check.Status != StatusWarning {
		t.Fatalf("expected StatusWarning for root-only legacy remote, got %s: %s\nDetail: %s", check.Status, check.Message, check.Detail)
	}
	if !strings.Contains(check.Detail, "Dolt server root") {
		t.Fatalf("expected detail to identify Dolt server root, got: %s", check.Detail)
	}
	if !strings.Contains(check.Detail, "rootonly="+rootOnlyURL) &&
		!strings.Contains(check.Detail, "rootonly=git+"+rootOnlyURL) {
		t.Fatalf("expected root-only remote in detail, got: %s", check.Detail)
	}
}
