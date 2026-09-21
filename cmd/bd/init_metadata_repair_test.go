package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
)

// discoverRepairDatabase mirrors the call site: the repair only ever runs with
// the name embeddeddolt would itself accept.
func discoverRepairDatabase(t *testing.T, beadsDir string) string {
	t.Helper()
	database, _ := embeddeddolt.SoleRepository(beadsDir)
	return database
}

func assertMetadataUnchanged(t *testing.T, beadsDir string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata after the repair declined: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("repair declined but rewrote metadata:\nbefore: %q\nafter:  %q", before, after)
	}
}

// preservedMetadataCopies returns the paths of the files a declined-or-completed
// repair left behind, following the .corrupt.backup/ convention bd doctor --fix
// uses for recovered artifacts.
func preservedMetadataCopies(t *testing.T, beadsDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(beadsDir)
	if err != nil {
		t.Fatalf("read beads dir: %v", err)
	}
	var copies []string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "metadata.json.") || !strings.HasSuffix(entry.Name(), ".corrupt.backup") {
			continue
		}
		copies = append(copies, filepath.Join(beadsDir, entry.Name(), "metadata.json"))
	}
	return copies
}

// TestRepairUnreadableMetadataRefusesShapesItCannotName covers the workspaces the
// repair must decline: the unreadable file was the only record of which database
// this workspace used, so a shape that cannot name exactly one is left to the
// fail-closed refusal. The empty and symlinked markers matter because recording
// them would point metadata.json at a database the adapter cannot open.
func TestRepairUnreadableMetadataRefusesShapesItCannotName(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, beadsDir string)
	}{
		{
			name:  "no embedded root",
			setup: func(*testing.T, string) {},
		},
		{
			name: "empty embedded root",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "two embedded databases",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeEmbeddedRepository(t, beadsDir, "cm")
				writeEmbeddedRepository(t, beadsDir, "other")
			},
		},
		{
			name: "half-initialized marker",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt", "cm", ".dolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked marker",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				databaseDir := filepath.Join(beadsDir, "embeddeddolt", "cm")
				if err := os.MkdirAll(databaseDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(beadsDir, filepath.Join(databaseDir, ".dolt")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlinked embedded root",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				realRoot := filepath.Join(t.TempDir(), "embeddeddolt")
				if err := os.MkdirAll(filepath.Join(realRoot, "cm", ".dolt"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(realRoot, "cm", ".dolt", "opaque-entry"), []byte("opaque"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(realRoot, filepath.Join(beadsDir, "embeddeddolt")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beadsDir := t.TempDir()
			before := writeCorruptWorkspaceMetadata(t, beadsDir)
			tt.setup(t, beadsDir)

			database := discoverRepairDatabase(t, beadsDir)
			if database != "" {
				t.Fatalf("SoleRepository() = %q, want no name for the shape under test", database)
			}
			repaired, err := repairUnreadableMetadata(context.Background(), beadsDir, database)
			if err != nil {
				t.Fatalf("repairUnreadableMetadata() error = %v, want nil", err)
			}
			if repaired {
				t.Fatal("repairUnreadableMetadata() repaired a workspace whose database it cannot name")
			}
			assertMetadataUnchanged(t, beadsDir, before)
			if backups := preservedMetadataCopies(t, beadsDir); len(backups) != 0 {
				t.Fatalf("declined repair left backups behind: %v", backups)
			}
		})
	}
}

// TestRepairUnreadableMetadataRewritesFromDiskEvidence is the positive contract:
// the sole embedded database names both the storage mode and the database, so
// the file is rebuilt from that and the unparseable original is preserved rather
// than destroyed.
func TestRepairUnreadableMetadataRewritesFromDiskEvidence(t *testing.T) {
	beadsDir := t.TempDir()
	before := writeCorruptWorkspaceMetadata(t, beadsDir)
	writeEmbeddedRepository(t, beadsDir, "cm")

	database := discoverRepairDatabase(t, beadsDir)
	if database != "cm" {
		t.Fatalf("SoleRepository() = %q, want %q", database, "cm")
	}
	repaired, err := repairUnreadableMetadata(context.Background(), beadsDir, database)
	if err != nil {
		t.Fatalf("repairUnreadableMetadata() error = %v", err)
	}
	if !repaired {
		t.Fatal("repairUnreadableMetadata() = false, want the workspace repaired")
	}

	cfg, err := configfile.LoadForDiscovery(beadsDir)
	if err != nil {
		t.Fatalf("repaired metadata.json does not load: %v", err)
	}
	if cfg.DoltMode != configfile.DoltModeEmbedded {
		t.Errorf("dolt_mode = %q, want %q", cfg.DoltMode, configfile.DoltModeEmbedded)
	}
	if cfg.DoltDatabase != "cm" {
		t.Errorf("dolt_database = %q, want %q", cfg.DoltDatabase, "cm")
	}
	if cfg.Backend != configfile.BackendDolt {
		t.Errorf("backend = %q, want %q", cfg.Backend, configfile.BackendDolt)
	}

	backups := preservedMetadataCopies(t, beadsDir)
	if len(backups) != 1 {
		t.Fatalf("preserved copies = %v, want exactly one", backups)
	}
	preserved, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatalf("read preserved copy: %v", err)
	}
	if !bytes.Equal(preserved, before) {
		t.Fatalf("preserved copy = %q, want the unparseable original %q", preserved, before)
	}
}

// TestRepairUnreadableMetadataLeavesUnreadableFileAlone guards the error-class
// boundary: only a parse error means the file is corrupt in the way this repair
// exists to undo. A file that could not be *read* may still be the only pointer
// to its database, and overwriting it would destroy evidence on the strength of
// a failure that says nothing about its contents. EISDIR stands in for any read
// fault; it is deterministic and portable.
func TestRepairUnreadableMetadataLeavesUnreadableFileAlone(t *testing.T) {
	beadsDir := t.TempDir()
	writeEmbeddedRepository(t, beadsDir, "cm")
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	if err := os.Mkdir(metadataPath, 0o700); err != nil {
		t.Fatal(err)
	}

	database := discoverRepairDatabase(t, beadsDir)
	if database != "cm" {
		t.Fatalf("SoleRepository() = %q, want %q", database, "cm")
	}
	repaired, err := repairUnreadableMetadata(context.Background(), beadsDir, database)
	if err != nil {
		t.Fatalf("repairUnreadableMetadata() error = %v, want nil", err)
	}
	if repaired {
		t.Fatal("repairUnreadableMetadata() repaired a file it could not read")
	}

	info, statErr := os.Lstat(metadataPath)
	if statErr != nil {
		t.Fatalf("stat metadata.json after the declined repair: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatal("repairUnreadableMetadata() replaced a file it could not read")
	}
	if backups := preservedMetadataCopies(t, beadsDir); len(backups) != 0 {
		t.Fatalf("declined repair left backups behind: %v", backups)
	}
}

func TestRepairUnreadableMetadataLeavesHealthyWorkspacesAlone(t *testing.T) {
	t.Run("absent metadata.json is the fresh-workspace default", func(t *testing.T) {
		beadsDir := t.TempDir()
		writeEmbeddedRepository(t, beadsDir, "cm")

		repaired, err := repairUnreadableMetadata(context.Background(), beadsDir, discoverRepairDatabase(t, beadsDir))
		if err != nil {
			t.Fatalf("repairUnreadableMetadata() error = %v, want nil", err)
		}
		if repaired {
			t.Fatal("repairUnreadableMetadata() rewrote a workspace with no metadata.json")
		}
	})

	t.Run("readable metadata.json is not touched", func(t *testing.T) {
		beadsDir := t.TempDir()
		readable := []byte(`{"backend":"dolt","dolt_mode":"embedded","dolt_database":"cm"}`)
		if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), readable, 0o600); err != nil {
			t.Fatal(err)
		}
		writeEmbeddedRepository(t, beadsDir, "cm")

		repaired, err := repairUnreadableMetadata(context.Background(), beadsDir, discoverRepairDatabase(t, beadsDir))
		if err != nil {
			t.Fatalf("repairUnreadableMetadata() error = %v, want nil", err)
		}
		if repaired {
			t.Fatal("repairUnreadableMetadata() repaired a readable metadata.json")
		}
		assertMetadataUnchanged(t, beadsDir, readable)
	})
}

// repairGateCommand builds the flag surface explicitRepairConflict inspects,
// marking exactly the selectors the caller passes.
func repairGateCommand(t *testing.T, selectors map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("database", "", "")
	cmd.Flags().String("prefix", "", "")
	for name, value := range selectors {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return cmd
}

// TestExplicitRepairConflict pins the gate that keeps the rewrite from silently
// swallowing an explicit selector: the repair records the database name the disk
// evidence names, so a request for a different one is refused rather than
// ignored. bd init --prefix cm is the documented repair, so a selector that
// agrees — or was never passed — must not be treated as a conflict.
func TestExplicitRepairConflict(t *testing.T) {
	tests := []struct {
		name       string
		selectors  map[string]string
		prefix     string
		discovered string
		want       bool
	}{
		{name: "no selectors", discovered: "cm"},
		{name: "matching database", selectors: map[string]string{"database": "cm"}, discovered: "cm"},
		{name: "case-insensitive matching database", selectors: map[string]string{"database": "CM"}, discovered: "cm"},
		{name: "matching prefix", selectors: map[string]string{"prefix": "cm"}, prefix: "cm", discovered: "cm"},
		{name: "hyphenated prefix matches underscore database", selectors: map[string]string{"prefix": "my-proj"}, prefix: "my-proj", discovered: "my_proj"},

		{name: "different database", selectors: map[string]string{"database": "foo"}, discovered: "cm", want: true},
		{name: "different prefix", selectors: map[string]string{"prefix": "foo"}, prefix: "foo", discovered: "cm", want: true},
		{name: "database requested with nothing discoverable", selectors: map[string]string{"database": "cm"}, discovered: "", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := repairGateCommand(t, tt.selectors)
			if got := explicitRepairConflict(cmd, tt.prefix, cmd.Flag("database").Value.String(), tt.discovered); got != tt.want {
				t.Fatalf("explicitRepairConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExplicitRepairConflictWithoutFlagsRegistered proves the predicate is safe
// on a command that never declared the selectors: a missing flag is not a
// conflict, so the repair is not skipped for a reason that does not exist.
func TestExplicitRepairConflictWithoutFlagsRegistered(t *testing.T) {
	if explicitRepairConflict(&cobra.Command{}, "", "", "cm") {
		t.Fatal("explicitRepairConflict() = true with no selectors registered")
	}
}
