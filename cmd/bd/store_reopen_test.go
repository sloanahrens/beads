//go:build cgo

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/utils"
)

func TestResolveBeadsDirForDBPath_UsesRawBeadsDirForSymlinkedDBPath(t *testing.T) {
	repoDir := t.TempDir()
	beadsDir := filepath.Join(repoDir, ".beads")
	actualDBPath := filepath.Join(repoDir, "external-dolt")
	linkDBPath := filepath.Join(beadsDir, "dolt")

	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir beads dir: %v", err)
	}
	if err := os.MkdirAll(actualDBPath, 0o755); err != nil {
		t.Fatalf("mkdir external dolt dir: %v", err)
	}
	if err := os.Symlink(actualDBPath, linkDBPath); err != nil {
		t.Fatalf("symlink db path: %v", err)
	}

	cfg := &configfile.Config{
		Database: "dolt",
		Backend:  configfile.BackendDolt,
	}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	if got := resolveBeadsDirForDBPath(linkDBPath); !utils.PathsEqual(got, beadsDir) {
		t.Fatalf("resolveBeadsDirForDBPath(%q) = %q, want %q", linkDBPath, got, beadsDir)
	}
}

func TestResolveCommandBeadsDir_NoCWDFallbackForExplicitPath(t *testing.T) {
	// Set up project A with metadata so FindBeadsDir() discovers it from CWD.
	projectA := t.TempDir()
	beadsDirA := filepath.Join(projectA, ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDirA, "dolt"), 0o755); err != nil {
		t.Fatalf("mkdir beads dir A: %v", err)
	}
	cfgA := &configfile.Config{
		Database:     "dolt",
		Backend:      configfile.BackendDolt,
		DoltDatabase: "project_a_db",
	}
	if err := cfgA.Save(beadsDirA); err != nil {
		t.Fatalf("save metadata A: %v", err)
	}

	// Project B: .beads/dolt exists but metadata.json is missing.
	// This triggers the bug: filepath.Dir(dbPath) gives the correct
	// .beads dir but configfile.Load returns nil, so the old code falls
	// through to FindBeadsDir() which discovers project A instead.
	projectB := t.TempDir()
	beadsDirB := filepath.Join(projectB, ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDirB, "dolt"), 0o755); err != nil {
		t.Fatalf("mkdir beads dir B: %v", err)
	}

	// CWD is inside project A so FindBeadsDir() discovers A
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectA); err != nil {
		t.Fatalf("chdir to project A: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	// Simulate --db pointing to project B's database path
	dbPathB := filepath.Join(beadsDirB, "dolt")
	got := resolveCommandBeadsDir(dbPathB)

	// Must resolve to project B's .beads, NOT project A's.
	// The old code falls back to FindBeadsDir() and returns beadsDirA.
	if !utils.PathsEqual(got, beadsDirB) {
		t.Fatalf("resolveCommandBeadsDir(%q) = %q, want %q", dbPathB, got, beadsDirB)
	}
}
