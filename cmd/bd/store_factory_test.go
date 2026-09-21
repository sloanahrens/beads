//go:build cgo

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
)

// TestNewDoltStoreFromConfig_NoMetadata verifies that newDoltStoreFromConfig
// succeeds when the beads directory has no metadata.json (fresh project).
// Regression test for GH#2988: "no database selected" error.
func TestNewDoltStoreFromConfig_NoMetadata(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt tests")
	}

	beadsDir := t.TempDir()

	// Confirm no config exists.
	cfg, err := configfile.Load(beadsDir)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config for empty dir")
	}

	// This should succeed using the default database name, not fail with
	// "no database selected".
	store, err := newDoltStoreFromConfig(t.Context(), beadsDir)
	if err != nil {
		t.Fatalf("newDoltStoreFromConfig failed: %v", err)
	}
	defer store.Close()
}

// TestEmbeddedOpen_EmptyDatabaseRejected verifies that embeddeddolt.Open fails
// with a clear error when called with an empty database name, rather than
// deferring to a confusing "no database selected" SQL error.
// Belt-and-suspenders defense for be-sy8 / GH#2988.
func TestEmbeddedOpen_EmptyDatabaseRejected(t *testing.T) {
	_, err := embeddeddolt.Open(t.Context(), t.TempDir(), "", "main")
	if err == nil {
		t.Fatal("expected error for empty database name")
	}
	if !strings.Contains(err.Error(), "database name must not be empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestNewDoltStoreFromConfig_HyphenatedDBName verifies that
// newDoltStoreFromConfig auto-sanitizes hyphenated database names for embedded
// mode and persists the fix to metadata.json.
// Regression test for GH#3231: pre-#2142 projects break on embedded upgrade.
func TestNewDoltStoreFromConfig_HyphenatedDBName(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt tests")
	}

	beadsDir := t.TempDir()

	cfg := &configfile.Config{
		Database:     "dolt",
		DoltDatabase: "my-cool-project",
		DoltMode:     configfile.DoltModeEmbedded,
	}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	store, err := newDoltStoreFromConfig(t.Context(), beadsDir)
	if err != nil {
		t.Fatalf("newDoltStoreFromConfig failed (should have auto-sanitized): %v", err)
	}
	defer store.Close()

	reloaded, err := configfile.Load(beadsDir)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if reloaded.DoltDatabase != "my_cool_project" {
		t.Errorf("expected dolt_database to be sanitized to %q, got %q", "my_cool_project", reloaded.DoltDatabase)
	}
}

// TestMigrateHyphenatedDB_PersistsToMetadata verifies that migrateHyphenatedDB
// updates metadata.json with the sanitized database name.
func TestMigrateHyphenatedDB_PersistsToMetadata(t *testing.T) {
	beadsDir := t.TempDir()

	cfg := &configfile.Config{
		Database:     "dolt",
		DoltDatabase: "my-project",
	}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	if err := migrateHyphenatedDB(beadsDir, cfg, "my-project", "my_project"); err != nil {
		t.Fatalf("migrateHyphenatedDB failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("failed to read metadata.json: %v", err)
	}

	var saved configfile.Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("failed to parse metadata.json: %v", err)
	}
	if saved.DoltDatabase != "my_project" {
		t.Errorf("expected dolt_database %q in metadata.json, got %q", "my_project", saved.DoltDatabase)
	}
}

// TestMigrateHyphenatedDB_RenamesDirectory verifies that migrateHyphenatedDB
// renames the old hyphenated database directory to the sanitized name.
func TestMigrateHyphenatedDB_RenamesDirectory(t *testing.T) {
	beadsDir := t.TempDir()

	dataDir := filepath.Join(beadsDir, "embeddeddolt")
	oldDir := filepath.Join(dataDir, "my-project")
	newDir := filepath.Join(dataDir, "my_project")

	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("failed to create old dir: %v", err)
	}
	sentinel := filepath.Join(oldDir, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to write sentinel: %v", err)
	}

	cfg := &configfile.Config{DoltDatabase: "my-project"}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	if err := migrateHyphenatedDB(beadsDir, cfg, "my-project", "my_project"); err != nil {
		t.Fatalf("migrateHyphenatedDB failed: %v", err)
	}

	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("old directory should no longer exist after rename")
	}
	if _, err := os.Stat(filepath.Join(newDir, "sentinel.txt")); err != nil {
		t.Error("sentinel file should exist in renamed directory")
	}
}

// TestMigrateHyphenatedDB_CollisionError verifies that migrateHyphenatedDB
// returns an error when both old and new directories exist (GH#3231).
func TestMigrateHyphenatedDB_CollisionError(t *testing.T) {
	beadsDir := t.TempDir()

	dataDir := filepath.Join(beadsDir, "embeddeddolt")
	oldDir := filepath.Join(dataDir, "my-project")
	newDir := filepath.Join(dataDir, "my_project")

	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("failed to create old dir: %v", err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("failed to create new dir: %v", err)
	}

	cfg := &configfile.Config{DoltDatabase: "my-project"}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	err := migrateHyphenatedDB(beadsDir, cfg, "my-project", "my_project")
	if err == nil {
		t.Fatal("expected error when both directories exist, got nil")
	}
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("expected collision error message, got: %v", err)
	}
}

// TestMigrateHyphenatedDB_NoOldDir verifies that migrateHyphenatedDB still
// updates metadata.json even when the old directory doesn't exist (e.g., fresh
// project where only metadata.json has the bad name).
func TestMigrateHyphenatedDB_NoOldDir(t *testing.T) {
	beadsDir := t.TempDir()

	cfg := &configfile.Config{DoltDatabase: "my-project"}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	if err := migrateHyphenatedDB(beadsDir, cfg, "my-project", "my_project"); err != nil {
		t.Fatalf("migrateHyphenatedDB failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("failed to read metadata.json: %v", err)
	}
	var saved configfile.Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("failed to parse metadata.json: %v", err)
	}
	if saved.DoltDatabase != "my_project" {
		t.Errorf("expected %q, got %q", "my_project", saved.DoltDatabase)
	}
}

// TestNewDoltStoreFromConfig_DottedDBName verifies that dots are also
// auto-sanitized, not just hyphens (GH#3231).
func TestNewDoltStoreFromConfig_DottedDBName(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt tests")
	}

	beadsDir := t.TempDir()

	cfg := &configfile.Config{
		Database:     "dolt",
		DoltDatabase: "my.project",
		DoltMode:     configfile.DoltModeEmbedded,
	}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	store, err := newDoltStoreFromConfig(t.Context(), beadsDir)
	if err != nil {
		t.Fatalf("newDoltStoreFromConfig failed (should have auto-sanitized dots): %v", err)
	}
	defer store.Close()

	reloaded, err := configfile.Load(beadsDir)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if reloaded.DoltDatabase != "my_project" {
		t.Errorf("expected dolt_database %q, got %q", "my_project", reloaded.DoltDatabase)
	}
}

// TestNewDoltStore_StrictReadOnlyRefusesWritesOnFreshDatabase covers Blocker 2
// of the 2026-07-23 maintainer review on gastownhall/beads#4930: cfg.ReadOnly
// alone (an ordinary classified-read command) must route through
// OpenForReadOnlyCommand, which creates the embedded data directory on first
// use — but cfg.ReadOnly combined with cfg.DisableAutoStart (the strict
// --readonly signal) must route through the genuinely write-refusing
// OpenReadOnly instead, which fails rather than create anything for a fresh
// database.
func TestNewDoltStore_StrictReadOnlyRefusesWritesOnFreshDatabase(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt tests")
	}

	// Strict --readonly: must fail on a fresh database and must not create
	// the embeddeddolt data directory.
	strictBeadsDir := t.TempDir()
	strictDataDir := filepath.Join(strictBeadsDir, "embeddeddolt")
	_, err := newDoltStore(t.Context(), &dolt.Config{
		ReadOnly:         true,
		DisableAutoStart: true,
		BeadsDir:         strictBeadsDir,
		Database:         "testdb",
	})
	if err == nil {
		t.Fatal("newDoltStore(ReadOnly, DisableAutoStart) on a fresh database = nil error, want refusal")
	}
	if _, statErr := os.Stat(strictDataDir); !os.IsNotExist(statErr) {
		t.Fatalf("strict read-only open created %s (stat error: %v)", strictDataDir, statErr)
	}

	// Ordinary classified read (ReadOnly without DisableAutoStart): must
	// still succeed and initialize the embedded database on first use, per
	// the #4259 remote-migrate-gate exemption this backend relies on.
	classifiedBeadsDir := t.TempDir()
	classifiedDataDir := filepath.Join(classifiedBeadsDir, "embeddeddolt")
	store, err := newDoltStore(t.Context(), &dolt.Config{
		ReadOnly: true,
		BeadsDir: classifiedBeadsDir,
		Database: "testdb",
	})
	if err != nil {
		t.Fatalf("newDoltStore(ReadOnly) on a fresh database: %v", err)
	}
	defer store.Close()
	if _, statErr := os.Stat(classifiedDataDir); statErr != nil {
		t.Fatalf("classified-read open did not initialize %s: %v", classifiedDataDir, statErr)
	}
}

// TestRefuseToCreateEmbedded pins the decision boundary for be-n2s. Both
// directions matter. Refusing when the caller declared the create would break
// bd init outright: init creates .beads/embeddeddolt/ — for its lock — before
// it writes metadata.json, so at its store open the directory is marker-less
// with an embeddeddolt/ shell in it, which is otherwise exactly the fossil
// shape. Refusing a directory with no embeddeddolt/ at all would break the
// documented first-run paths (bd import on a bare directory, a fresh clone
// rebuilt from tracked issues.jsonl, `--repo` auto-vivify); refusing a
// directory that carries a marker would break rebuilding a workspace whose
// database is legitimately absent.
func TestRefuseToCreateEmbedded(t *testing.T) {
	tests := []struct {
		name            string
		createIfMissing bool
		setup           func(t *testing.T, beadsDir string)
		want            bool
	}{
		{name: "bare directory", want: false},
		{
			name: "metadata.json and no embeddeddolt tree",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeMetadataMarker(t, beadsDir)
			},
			want: false,
		},
		{
			name: "markerless embeddeddolt tree",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			name: "markerless embeddeddolt tree holding a database",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			// bd init's shape: the shell is already on disk, but the caller
			// declared that it is creating the database.
			name:            "markerless embeddeddolt tree with a declared create",
			createIfMissing: true,
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{
			name: "metadata.json beside an embeddeddolt tree",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
					t.Fatal(err)
				}
				writeMetadataMarker(t, beadsDir)
			},
			want: false,
		},
		{
			name: "config.yaml beside an embeddeddolt tree",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("dolt:\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{
			// A plain file named embeddeddolt is not a data directory, so the
			// directory is not workspace-shaped at all.
			name: "embeddeddolt as a regular file",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(beadsDir, "embeddeddolt"), []byte("not a dir"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beadsDir := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, beadsDir)
			}
			cfg := &dolt.Config{BeadsDir: beadsDir, Database: "testdb", CreateIfMissing: tt.createIfMissing}
			if got := refuseToCreateEmbedded(cfg); got != tt.want {
				t.Fatalf("refuseToCreateEmbedded() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestNewDoltStore_RefuseToCreateEmbedded is the end-to-end half: through the
// factory, a markerless embeddeddolt/ shell must fail the open rather than
// gain a database, while the shapes that legitimately need creation must still
// create. The refusal is asserted on the data directory's entries as well as
// on the named database's absence, because the failure mode this guards
// against is a database appearing on disk, not an error being returned late.
func TestNewDoltStore_RefuseToCreateEmbedded(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt tests")
	}

	// markerlessShell builds the fossil shape: .beads/ with an embeddeddolt/
	// tree (holding a real database when withDatabase is set) and no
	// metadata.json or config.yaml.
	markerlessShell := func(t *testing.T, withDatabase bool) (beadsDir, dataDir string) {
		t.Helper()
		beadsDir = t.TempDir()
		dataDir = filepath.Join(beadsDir, "embeddeddolt")
		databaseDir := filepath.Join(dataDir, "testdb")
		if !withDatabase {
			databaseDir = dataDir
		}
		if err := os.MkdirAll(databaseDir, 0o700); err != nil {
			t.Fatal(err)
		}
		return beadsDir, dataDir
	}

	t.Run("markerless shell refuses and creates nothing", func(t *testing.T) {
		beadsDir, dataDir := markerlessShell(t, false)
		before := embeddedDataDirEntries(t, dataDir)

		_, err := newDoltStore(t.Context(), &dolt.Config{BeadsDir: beadsDir, Database: "testdb"})
		if err == nil {
			t.Fatal("newDoltStore on a markerless embeddeddolt/ shell = nil error, want refusal")
		}
		if !strings.Contains(err.Error(), "refusing to create") {
			t.Errorf("refusal should name the reason, got: %v", err)
		}
		if got := embeddedDataDirEntries(t, dataDir); got != before {
			t.Errorf("refusal changed %s entries to %q, want %q; it must create nothing", dataDir, got, before)
		}
	})

	t.Run("markerless shell opens an existing database", func(t *testing.T) {
		beadsDir, _ := markerlessShell(t, false)

		// Create the database the way bd init does, then reopen without the
		// declared create: the shell is still marker-less, but the requested
		// database is on disk, so the open must find it rather than refuse.
		created, err := newDoltStore(t.Context(), &dolt.Config{
			BeadsDir:        beadsDir,
			Database:        "testdb",
			CreateIfMissing: true,
		})
		if err != nil {
			t.Fatalf("seed the markerless shell: %v", err)
		}
		if err := created.Close(); err != nil {
			t.Fatalf("close seeded store: %v", err)
		}

		store, err := newDoltStore(t.Context(), &dolt.Config{BeadsDir: beadsDir, Database: "testdb"})
		if err != nil {
			t.Errorf("newDoltStore on a markerless shell holding the requested database: %v", err)
			return
		}
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	t.Run("declared create still creates in a markerless shell", func(t *testing.T) {
		beadsDir, _ := markerlessShell(t, false)

		store, err := newDoltStore(t.Context(), &dolt.Config{
			BeadsDir:        beadsDir,
			Database:        "testdb",
			CreateIfMissing: true,
		})
		if err != nil {
			t.Fatalf("newDoltStore with CreateIfMissing on a markerless shell (bd init's shape): %v", err)
		}
		defer func() { _ = store.Close() }()
		if _, statErr := os.Stat(filepath.Join(beadsDir, "embeddeddolt", "testdb")); statErr != nil {
			t.Fatalf("declared create did not create the database: %v", statErr)
		}
	})

	t.Run("bare directory still creates", func(t *testing.T) {
		beadsDir := t.TempDir()
		store, err := newDoltStore(t.Context(), &dolt.Config{BeadsDir: beadsDir, Database: "testdb"})
		if err != nil {
			t.Fatalf("newDoltStore on a bare directory: %v", err)
		}
		defer func() { _ = store.Close() }()
		if _, statErr := os.Stat(filepath.Join(beadsDir, "embeddeddolt", "testdb")); statErr != nil {
			t.Fatalf("first-run open did not create the database: %v", statErr)
		}
	})

	t.Run("marked workspace with an empty data tree still creates", func(t *testing.T) {
		beadsDir := t.TempDir()
		writeMetadataMarker(t, beadsDir)
		if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
			t.Fatal(err)
		}

		store, err := newDoltStore(t.Context(), &dolt.Config{BeadsDir: beadsDir, Database: "testdb"})
		if err != nil {
			t.Fatalf("newDoltStore on a marked workspace: %v", err)
		}
		defer func() { _ = store.Close() }()
		if _, statErr := os.Stat(filepath.Join(beadsDir, "embeddeddolt", "testdb")); statErr != nil {
			t.Fatalf("marked workspace did not rebuild its database: %v", statErr)
		}
	})
}

// embeddedDataDirEntries renders the entries under an embeddeddolt data
// directory, for asserting that a refused open left the directory untouched.
func embeddedDataDirEntries(t *testing.T, dataDir string) string {
	t.Helper()
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("read %s: %v", dataDir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return strings.Join(names, ",")
}

// writeMetadataMarker writes a metadata.json declaring an embedded workspace,
// which is the marker HasWorkspaceMarker looks for.
func writeMetadataMarker(t *testing.T, beadsDir string) {
	t.Helper()
	cfg := configfile.DefaultConfig()
	cfg.Backend = configfile.BackendDolt
	cfg.DoltDatabase = "testdb"
	cfg.DoltMode = configfile.DoltModeEmbedded
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("save metadata.json: %v", err)
	}
}
