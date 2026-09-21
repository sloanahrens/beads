package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/config"
)

// writeCorruptWorkspaceMetadata lays down the state every case here starts from:
// a metadata.json that exists but cannot be parsed, which is what a reader sees
// when the file is caught mid-rewrite or hit by a transient read failure. It
// returns the bytes written so a caller can prove they were left alone.
func writeCorruptWorkspaceMetadata(t *testing.T, beadsDir string) []byte {
	t.Helper()
	metadata := []byte(`{"dolt_mode":"serv`)
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), metadata, 0o600); err != nil {
		t.Fatalf("write corrupt metadata.json: %v", err)
	}
	return metadata
}

// writeEmbeddedRepository lays down the marker shape embeddeddolt accepts as a
// repository: a database directory whose .dolt marker is a non-symlink
// directory with at least one entry. The entry is never interpreted.
func writeEmbeddedRepository(t *testing.T, beadsDir, database string) {
	t.Helper()
	marker := filepath.Join(beadsDir, "embeddeddolt", database, ".dolt")
	if err := os.MkdirAll(marker, 0o700); err != nil {
		t.Fatalf("mkdir embedded repository: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marker, "opaque-entry"), []byte("not inspected"), 0o600); err != nil {
		t.Fatalf("write repository marker entry: %v", err)
	}
}

// isolateGuardServerMode pins the ambient shared-server answers so a case that
// turns on the mode cannot leak into its neighbours through the environment or a
// developer's own config.yaml.
func isolateGuardServerMode(t *testing.T) {
	t.Helper()
	config.ResetForTesting()
	t.Cleanup(config.ResetForTesting)
	t.Setenv("BEADS_DOLT_SHARED_SERVER", "0")
	t.Setenv("BEADS_DOLT_SERVER_MODE", "")
	t.Setenv("BEADS_DOLT_PROXIED_SERVER", "")
}

// TestLegacyUpgradeGuardDoesNotClassifyUnreadableMetadata pins the fix for a
// current server workspace being reported as a legacy one. Every refusal the
// guard can make rests on what metadata.json says, so a file that cannot be
// parsed must make none of them: .beads/dolt is the pre-1.0 Dolt root *and* the
// current server physical root, and only the mode the unreadable file held tells
// them apart. Misreporting it sent the user to cross-era migration when the fix
// was to rewrite the file.
func TestLegacyUpgradeGuardDoesNotClassifyUnreadableMetadata(t *testing.T) {
	t.Run("current server root is not reported as a legacy layout", func(t *testing.T) {
		isolateGuardServerMode(t)
		beadsDir := t.TempDir()
		writeCorruptWorkspaceMetadata(t, beadsDir)
		if err := os.Mkdir(filepath.Join(beadsDir, "dolt"), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := guardLegacyUpgradeWorkspace(beadsDir); err != nil {
			t.Fatalf("guardLegacyUpgradeWorkspace() = %v, want nil — the load failure classifies nothing", err)
		}
	})

	t.Run("current embedded workspace admits the repair path", func(t *testing.T) {
		isolateGuardServerMode(t)
		beadsDir := t.TempDir()
		writeCorruptWorkspaceMetadata(t, beadsDir)
		writeEmbeddedRepository(t, beadsDir, "cm")

		if err := guardLegacyUpgradeWorkspace(beadsDir); err != nil {
			t.Fatalf("guardLegacyUpgradeWorkspace() = %v, want nil — bd init rewrites this file", err)
		}
	})

	// A genuinely absent metadata.json is a different thing from an unreadable
	// one — LoadForDiscovery reports it as (nil, nil) — and the on-disk legacy
	// classifications must keep firing for it.
	t.Run("absent metadata still classifies a legacy Dolt root", func(t *testing.T) {
		isolateGuardServerMode(t)
		beadsDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(beadsDir, "dolt"), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := guardLegacyUpgradeWorkspace(beadsDir); !isLegacyUpgradeRefusal(err) {
			t.Fatalf("guardLegacyUpgradeWorkspace() = %v, want migration refusal", err)
		}
	})

	// The relaxed guard must not become a repair. Doctor rewrites nothing, and
	// this guard runs ahead of the commands that do.
	t.Run("unreadable metadata is left byte-identical", func(t *testing.T) {
		isolateGuardServerMode(t)
		beadsDir := t.TempDir()
		before := writeCorruptWorkspaceMetadata(t, beadsDir)
		if err := os.Mkdir(filepath.Join(beadsDir, "dolt"), 0o700); err != nil {
			t.Fatal(err)
		}

		_ = guardLegacyUpgradeWorkspace(beadsDir)

		after, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
		if err != nil {
			t.Fatalf("read metadata after the guard: %v", err)
		}
		if !bytes.Equal(after, before) {
			t.Fatalf("guard rewrote unreadable metadata:\nbefore: %q\nafter:  %q", before, after)
		}
	})

	// Stopping at the load failure is only safe because the callers that can
	// still open or write a workspace fail closed on the same error. This pins
	// the boundary rather than the classification: a pre-1.0 SQLite layout
	// beside a corrupt metadata.json is no longer refused *here*.
	t.Run("legacy on-disk layout beside unreadable metadata is left to callers", func(t *testing.T) {
		isolateGuardServerMode(t)
		beadsDir := t.TempDir()
		writeCorruptWorkspaceMetadata(t, beadsDir)
		if err := os.WriteFile(filepath.Join(beadsDir, "vc.db"), []byte("SQLite format 3\x00"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := guardLegacyUpgradeWorkspace(beadsDir); err != nil {
			t.Fatalf("guardLegacyUpgradeWorkspace() = %v, want nil", err)
		}
		if err := checkExistingBeadsDataAt(beadsDir, "cm"); err == nil {
			t.Fatal("checkExistingBeadsDataAt() = nil, want the unreadable-metadata refusal that protects this workspace")
		}
	})
}
