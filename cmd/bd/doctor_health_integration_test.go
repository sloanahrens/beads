//go:build cgo && integration

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDoctorCheckHealthReportsVersionMismatchOnRepoLocalPort(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("doctor health integration test not supported on windows")
	}

	tmpDir := createTempDirWithCleanup(t)

	if err := runCommandInDir(tmpDir, "git", "init"); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	_ = runCommandInDir(tmpDir, "git", "config", "user.email", "test@example.com")
	_ = runCommandInDir(tmpDir, "git", "config", "user.name", "Test User")

	if testDoltServerPort == 0 {
		t.Skip("skipping: Dolt test container not available")
	}

	env := append(os.Environ(), "BEADS_TEST_MODE=1")

	initOut, initErr := runBDExecAllowErrorWithEnv(t, tmpDir, env, "init", "--backend", "dolt", "--prefix", "test", "--quiet")
	if initErr != nil {
		lower := strings.ToLower(initOut)
		if strings.Contains(lower, "dolt") && (strings.Contains(lower, "not supported") || strings.Contains(lower, "not available") || strings.Contains(lower, "unknown")) {
			t.Skipf("dolt backend not available: %s", initOut)
		}
		t.Fatalf("bd init --backend dolt failed: %v\n%s", initErr, initOut)
	}

	startOut, startErr := runBDExecAllowErrorWithEnv(t, tmpDir, env, "dolt", "start")
	if startErr != nil {
		t.Fatalf("bd dolt start failed: %v\n%s", startErr, startOut)
	}

	portBytes, err := os.ReadFile(filepath.Join(tmpDir, ".beads", "dolt-server.port"))
	if err != nil {
		t.Fatalf("read dolt-server.port: %v", err)
	}
	port := strings.TrimSpace(string(portBytes))
	if port == "" {
		t.Fatal("expected non-empty dolt-server.port")
	}
	if port == "3307" {
		t.Skip("derived repo-local port unexpectedly matched 3307; not exercising regression")
	}

	sqlOut, sqlErr := runBDExecAllowErrorWithEnv(t, tmpDir, env, "sql", "UPDATE local_metadata SET value = '0.0.0' WHERE `key` = 'bd_version'")
	if sqlErr != nil {
		t.Fatalf("bd sql UPDATE failed: %v\n%s", sqlErr, sqlOut)
	}

	healthOut, healthErr := runBDExecAllowErrorWithEnv(t, tmpDir, env, "doctor", "--check-health")
	if healthErr == nil {
		t.Fatalf("expected bd doctor --check-health to fail on version mismatch; output:\n%s", healthOut)
	}
	if !strings.Contains(healthOut, "Version mismatch") {
		t.Fatalf("expected version mismatch in doctor --check-health output; output:\n%s", healthOut)
	}
	if !strings.Contains(healthOut, "CLI: "+Version) {
		t.Fatalf("expected CLI version %q in output; output:\n%s", Version, healthOut)
	}
	if !strings.Contains(healthOut, "database: 0.0.0") {
		t.Fatalf("expected database version in output; output:\n%s", healthOut)
	}
}

func TestHintsDisabledDB_UsesQuotedConfigKey(t *testing.T) {
	saveAndRestoreGlobals(t)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".beads", "beads.db")
	store := newTestStoreIsolatedDB(t, dbPath, "test")

	ctx := context.Background()
	if err := store.SetConfig(ctx, ConfigKeyHintsDoctor, "false"); err != nil {
		t.Fatalf("SetConfig(%q): %v", ConfigKeyHintsDoctor, err)
	}

	if !hintsDisabledDB(store.DB()) {
		t.Fatalf("expected hintsDisabledDB to read %q from config table", ConfigKeyHintsDoctor)
	}
}

func TestCheckVersionMismatchDB_UsesQuotedMetadataKey(t *testing.T) {
	saveAndRestoreGlobals(t)

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, ".beads", "beads.db")
	store := newTestStoreIsolatedDB(t, dbPath, "test")

	ctx := context.Background()
	if err := store.SetLocalMetadata(ctx, "bd_version", "0.0.0"); err != nil {
		t.Fatalf("SetLocalMetadata(bd_version): %v", err)
	}

	issue := checkVersionMismatchDB(store.DB())
	if issue == "" {
		t.Fatal("expected version mismatch to be reported")
	}
	if !strings.Contains(issue, "CLI: "+Version) {
		t.Fatalf("expected mismatch to mention CLI version %q, got %q", Version, issue)
	}
	if !strings.Contains(issue, "database: 0.0.0") {
		t.Fatalf("expected mismatch to mention database version, got %q", issue)
	}
}
