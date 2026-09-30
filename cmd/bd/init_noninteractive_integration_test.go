//go:build integration

// Integration tier (be-b23): moved verbatim from the unit-tier sibling file.
// These tests run bd init against a fresh store, which migrates; the unit
// tier's BD_TEST_TIER tripwire refuses that.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInitNonInteractiveAutoExportDefaultOffAndOptIn(t *testing.T) {
	bd := buildBDForInitTests(t)
	dir := t.TempDir()

	runBDForAutoExportInitTest(t, bd, dir, "init", "--prefix", "test", "--quiet", "--non-interactive", "--skip-hooks", "--skip-agents")

	if got := strings.TrimSpace(runBDStdoutForAutoExportInitTest(t, bd, dir, "config", "get", "export.auto")); got != "false" {
		t.Fatalf("export.auto default = %q, want false", got)
	}
	if got := strings.TrimSpace(runBDStdoutForAutoExportInitTest(t, bd, dir, "config", "get", "export.git-add")); got != "false" {
		t.Fatalf("export.git-add default = %q, want false", got)
	}

	runBDForAutoExportInitTest(t, bd, dir, "create", "default-off issue", "-p", "2")
	jsonlPath := filepath.Join(dir, ".beads", "issues.jsonl")
	if _, err := os.Stat(jsonlPath); !os.IsNotExist(err) {
		t.Fatalf("default-off create wrote %s; stat err=%v", jsonlPath, err)
	}

	runBDForAutoExportInitTest(t, bd, dir, "config", "set", "export.interval", "1ms")
	runBDForAutoExportInitTest(t, bd, dir, "config", "set", "export.auto", "true")
	if got := strings.TrimSpace(runBDStdoutForAutoExportInitTest(t, bd, dir, "config", "get", "export.auto")); got != "true" {
		t.Fatalf("explicit export.auto = %q, want true", got)
	}
	time.Sleep(10 * time.Millisecond)
	runBDForAutoExportInitTest(t, bd, dir, "create", "explicit export issue", "-p", "2")
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("explicit export.auto did not write %s: %v", jsonlPath, err)
	}
	if !strings.Contains(string(data), "explicit export issue") {
		t.Fatalf("JSONL export did not contain created issue:\n%s", data)
	}
}

func runBDStdoutForAutoExportInitTest(t *testing.T, bd, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bd, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bd %v failed: %v", args, err)
	}
	return string(out)
}

func runBDForAutoExportInitTest(t *testing.T, bd, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bd, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BD_NON_INTERACTIVE=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}
