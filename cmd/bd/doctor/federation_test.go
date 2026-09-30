//go:build cgo

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

func TestDoltServerConfig_SuppressesCLIAutoStartWithConfiguredPort(t *testing.T) {
	t.Setenv("BEADS_TEST_MODE", "")
	t.Setenv("BEADS_DOLT_AUTO_START", "")
	t.Setenv("BEADS_DOLT_SERVER_MODE", "")
	t.Setenv("BEADS_DOLT_SHARED_SERVER", "")

	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &configfile.Config{
		Backend:        configfile.BackendDolt,
		DoltDatabase:   "beads_test",
		DoltServerPort: 12345,
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	result := doltServerConfig(beadsDir, filepath.Join(beadsDir, "dolt"))
	if result.AutoStart {
		t.Fatal("doltServerConfig should suppress CLI auto-start with an external configured server port")
	}
}

func TestDoltServerConfig_EnablesCLIAutoStartWithoutConfiguredPort(t *testing.T) {
	t.Setenv("BEADS_TEST_MODE", "")
	t.Setenv("BEADS_DOLT_AUTO_START", "")
	t.Setenv("BEADS_DOLT_SERVER_MODE", "")
	t.Setenv("BEADS_DOLT_SHARED_SERVER", "")

	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &configfile.Config{
		Backend:      configfile.BackendDolt,
		DoltDatabase: "beads_test",
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	result := doltServerConfig(beadsDir, filepath.Join(beadsDir, "dolt"))
	if !result.AutoStart {
		t.Fatal("doltServerConfig should enable CLI auto-start for owned standalone configs")
	}
}

func TestDoltServerConfig_HonorsAutoStartOptOut(t *testing.T) {
	t.Setenv("BEADS_TEST_MODE", "")
	t.Setenv("BEADS_DOLT_AUTO_START", "0")

	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &configfile.Config{
		Backend:        configfile.BackendDolt,
		DoltDatabase:   "beads_test",
		DoltServerPort: 12345,
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	result := doltServerConfig(beadsDir, filepath.Join(beadsDir, "dolt"))
	if result.AutoStart {
		t.Fatal("doltServerConfig should honor BEADS_DOLT_AUTO_START=0")
	}
}

func TestCheckDoltServerModeMismatch_NonDoltBackend(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &configfile.Config{
		Backend: "sqlite",
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	check := CheckDoltServerModeMismatch(tmpDir)

	if check.Status != StatusOK {
		t.Errorf("expected StatusOK for non-Dolt backend, got %s", check.Status)
	}
}

func TestCheckDoltServerModeMismatch_NoDoltDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &configfile.Config{
		Backend: configfile.BackendDolt,
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	check := CheckDoltServerModeMismatch(tmpDir)

	if check.Status != StatusOK {
		t.Errorf("expected StatusOK for missing dolt database, got %s", check.Status)
	}
}

func TestDoltServerConfig_PopulatesFromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	doltDir := filepath.Join(beadsDir, "dolt")
	if err := os.MkdirAll(doltDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &configfile.Config{
		Backend:        configfile.BackendDolt,
		DoltServerHost: "192.168.1.10",
		DoltServerUser: "testuser",
		DoltDatabase:   "mydb",
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	result := doltServerConfig(beadsDir, doltDir)

	if result.Path != doltDir {
		t.Errorf("expected Path %q, got %q", doltDir, result.Path)
	}
	if !result.ReadOnly {
		t.Error("expected ReadOnly=true")
	}
	if result.Database != "mydb" {
		t.Errorf("expected Database 'mydb', got %q", result.Database)
	}
	if result.ServerHost != "192.168.1.10" {
		t.Errorf("expected ServerHost '192.168.1.10', got %q", result.ServerHost)
	}
	if result.ServerUser != "testuser" {
		t.Errorf("expected ServerUser 'testuser', got %q", result.ServerUser)
	}
}

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

func TestDoltDatabaseName_Default(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// No config file — should fall back to default
	name := doltDatabaseName(beadsDir)
	if name != configfile.DefaultDoltDatabase {
		t.Errorf("expected default %q, got %q", configfile.DefaultDoltDatabase, name)
	}
}

func TestDoltDatabaseName_FromConfig(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &configfile.Config{
		DoltDatabase: "custom_db",
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	name := doltDatabaseName(beadsDir)
	if name != "custom_db" {
		t.Errorf("expected 'custom_db', got %q", name)
	}
}

func runDoctorTestCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed in %s: %v\nOutput: %s", name, args, dir, err, out)
	}
}

// findDoltPIDs returns PIDs of running dolt sql-server processes on the host.
func findDoltPIDs(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("pgrep", "-f", "dolt sql-server").Output()
	if err != nil {
		return nil
	}
	var pids []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			pids = append(pids, line)
		}
	}
	return pids
}
