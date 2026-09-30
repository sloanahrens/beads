//go:build integration

// Integration tier (be-b23): moved verbatim from init_safety_test.go.
// These tests run bd init against a fresh store, which migrates; the unit
// tier's BD_TEST_TIER tripwire refuses that.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestInitReinitLocalConfiguredRemoteWithoutDoltDataSucceeds is the
// caller-level regression test for GH#4861: sync.remote (via BD_SYNC_REMOTE,
// not --remote, to hit initSyncRemoteConfigured) has no refs/dolt/data, so
// `bd init --reinit-local` must succeed rather than refuse. Supersedes
// TestCheckRemoteSafety_ConfiguredRemoteWithoutDoltData, which called the
// unmodified CheckRemoteSafety directly and tested nothing about this fix
// (both its cases are already in TestCheckRemoteSafety_GuardMatrix).
func TestInitReinitLocalConfiguredRemoteWithoutDoltDataSucceeds(t *testing.T) {
	bdBin := buildBDForInitTests(t)

	bareDir := filepath.Join(t.TempDir(), "bare.git")
	runGitForBootstrapTest(t, "", "init", "--bare", bareDir)
	// bareDir has ordinary git plumbing but no refs/dolt/data (GH#4861 shape).

	workDir := t.TempDir()
	runGitForBootstrapTest(t, workDir, "init", "-b", "main")
	runGitForBootstrapTest(t, workDir, "config", "core.hooksPath", ".git/hooks")

	homeDir := t.TempDir() // must differ from workDir: metrics.go caches to $HOME/.beads

	cmd := exec.Command(bdBin, "init", "--reinit-local", "--prefix", "cfg", "--quiet", "--non-interactive", "--skip-hooks", "--skip-agents")
	cmd.Dir = workDir
	cmd.Env = hermeticInitEnv(homeDir, "BD_SYNC_REMOTE="+bareDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("bd init --reinit-local with a Dolt-data-free sync.remote failed: %v\nstderr:\n%s", err, stderr.String())
	}

	beadsDir := filepath.Join(workDir, ".beads")
	if _, err := os.Stat(beadsDir); err != nil {
		t.Fatalf(".beads should have been created by a successful init: %v", err)
	}
}

// TestInitFreshWithUnreachableGitOriginSucceeds pins the plain-init arm of
// the tri-state probe: a git origin the probe cannot reach at all (exit 128 —
// the credential-less private-remote shape every CI container has) resolves
// to UNKNOWN, which fails closed for the refusal gate but must NOT convert a
// plain `bd init` into a bootstrap clone against that same unreachable
// origin. There is no local history to protect here, and the clone can only
// fail the same way the probe did, so init falls back to a fresh local
// database — the behavior this path always had before the probe existed.
// Regression: #5136 broke TestE2E_InitDoltMetadataRoundtrip (Main-lane only,
// so its PR ran green) exactly this way.
func TestInitFreshWithUnreachableGitOriginSucceeds(t *testing.T) {
	bdBin := buildBDForInitTests(t)

	workDir := t.TempDir()
	runGitForBootstrapTest(t, workDir, "init", "-b", "main")
	runGitForBootstrapTest(t, workDir, "config", "core.hooksPath", ".git/hooks")
	// An origin whose probe errors (missing path -> git ls-remote exit 128),
	// not one that answers "no refs/dolt/data".
	runGitForBootstrapTest(t, workDir, "remote", "add", "origin",
		filepath.Join(t.TempDir(), "does-not-exist.git"))

	homeDir := t.TempDir() // must differ from workDir: metrics.go caches to $HOME/.beads

	cmd := exec.Command(bdBin, "init", "--backend", "dolt", "--prefix", "org", "--quiet", "--non-interactive", "--skip-hooks", "--skip-agents")
	cmd.Dir = workDir
	cmd.Env = hermeticInitEnv(homeDir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		lower := strings.ToLower(out.String())
		if strings.Contains(lower, "dolt") && (strings.Contains(lower, "not supported") || strings.Contains(lower, "not available") || strings.Contains(lower, "unknown")) {
			t.Skipf("dolt backend not available: %s", out.String())
		}
		t.Fatalf("bd init with an unreachable git origin failed: %v\noutput:\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(workDir, ".beads")); err != nil {
		t.Fatalf(".beads should have been created by a successful init: %v", err)
	}

	if _, err := os.Stat(filepath.Join(homeDir, ".beads", "eventsData")); !os.IsNotExist(err) {
		t.Errorf("metrics queue dir was created under the isolated HOME (err=%v); "+
			"the detached send-metrics child will race t.TempDir cleanup "+
			"(see test_repo_beads_guard_test.go:118)", err)
	}
}
