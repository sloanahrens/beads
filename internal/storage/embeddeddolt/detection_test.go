package embeddeddolt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasRepository(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, beadsDir string)
		want  bool
	}{
		{name: "missing root", want: false},
		{
			name: "empty root",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{
			name: "empty repository marker",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{
			name: "nonempty repository marker",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				marker := filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt")
				if err := os.MkdirAll(marker, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(marker, "opaque-entry"), []byte("not inspected"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			want: true,
		},
		{
			name: "symlink marker is refused",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				databaseDir := filepath.Join(beadsDir, "embeddeddolt", "beads")
				if err := os.MkdirAll(databaseDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(beadsDir, filepath.Join(databaseDir, ".dolt")); err != nil {
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
			if got := HasRepository(beadsDir); got != tt.want {
				t.Fatalf("HasRepository() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSoleRepository pins the naming half of the same probe. Its caller recovers
// a workspace's database name from this listing when metadata.json cannot be
// read, so it must accept exactly the directories HasRepository accepts — and
// name nothing when there are two, because the unreadable file was the only
// record of which one the workspace used.
func TestSoleRepository(t *testing.T) {
	writeRepository := func(t *testing.T, beadsDir, database string) {
		t.Helper()
		marker := filepath.Join(beadsDir, "embeddeddolt", database, ".dolt")
		if err := os.MkdirAll(marker, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(marker, "opaque-entry"), []byte("not inspected"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name  string
		setup func(t *testing.T, beadsDir string)
		want  string
	}{
		{name: "missing root", want: ""},
		{
			name: "empty root",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: "",
		},
		{
			name: "single repository",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeRepository(t, beadsDir, "beads")
			},
			want: "beads",
		},
		{
			name: "two repositories name nothing",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeRepository(t, beadsDir, "beads")
				writeRepository(t, beadsDir, "other")
			},
			want: "",
		},
		{
			name: "empty marker is not a repository",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt", "beads", ".dolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: "",
		},
		{
			name: "symlinked marker is refused even beside a usable one",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeRepository(t, beadsDir, "beads")
				linkedDir := filepath.Join(beadsDir, "embeddeddolt", "linked")
				if err := os.MkdirAll(linkedDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(beadsDir, filepath.Join(linkedDir, ".dolt")); err != nil {
					t.Fatal(err)
				}
			},
			want: "beads",
		},
		{
			name: "symlinked root names nothing",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				realRoot := filepath.Join(t.TempDir(), "embeddeddolt")
				if err := os.MkdirAll(filepath.Join(realRoot, "beads", ".dolt"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(realRoot, "beads", ".dolt", "opaque-entry"), []byte("opaque"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(realRoot, filepath.Join(beadsDir, "embeddeddolt")); err != nil {
					t.Fatal(err)
				}
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beadsDir := t.TempDir()
			if tt.setup != nil {
				tt.setup(t, beadsDir)
			}
			got, ok := SoleRepository(beadsDir)
			if got != tt.want {
				t.Fatalf("SoleRepository() = %q, want %q", got, tt.want)
			}
			if ok != (tt.want != "") {
				t.Fatalf("SoleRepository() ok = %v, want %v", ok, tt.want != "")
			}
			// The probes are deliberately not the same question — HasRepository
			// asks whether any exists, SoleRepository whether exactly one does,
			// so they diverge only on the two-repository case. What must hold is
			// the containment: a name SoleRepository is willing to record is a
			// directory HasRepository already accepts.
			if ok && !HasRepository(beadsDir) {
				t.Fatalf("SoleRepository() named %q but HasRepository() = false — the probes disagree about a directory", got)
			}
		})
	}
}

// TestHasDatabase pins the per-name form of the marker probe. HasRepository
// asks "does this directory hold an embedded repository at all"; a
// refuse-to-create open needs "does it hold one under the name I am about to
// open", and the two diverge on exactly the fossil shape — a directory holding
// a repository under a name nobody is opening any more (be-n2s). The
// containment is asserted as well: a name HasDatabase accepts must be one
// HasRepository accepts, or a refusal would be decided against a directory
// that the rest of the adapter considers empty.
func TestHasDatabase(t *testing.T) {
	// writeRepository writes a usable embedded repository — a directory whose
	// .dolt marker is non-empty — and, when markerEntries is 0, the empty
	// marker that is deliberately not one.
	writeRepository := func(t *testing.T, beadsDir, database string, markerEntries int) {
		t.Helper()
		marker := filepath.Join(beadsDir, "embeddeddolt", database, ".dolt")
		if err := os.MkdirAll(marker, 0o700); err != nil {
			t.Fatal(err)
		}
		for i := range markerEntries {
			entry := filepath.Join(marker, "opaque-entry-"+string(rune('a'+i)))
			if err := os.WriteFile(entry, []byte("not inspected"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	tests := []struct {
		name     string
		database string
		setup    func(t *testing.T, beadsDir string)
		want     bool
	}{
		{name: "missing root", database: "beads", want: false},
		{
			name:     "empty embeddeddolt root",
			database: "beads",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{
			name:     "database directory without a marker",
			database: "beads",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt", "beads"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{
			name:     "empty marker is not a repository",
			database: "beads",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeRepository(t, beadsDir, "beads", 0)
			},
			want: false,
		},
		{
			name:     "nonempty marker",
			database: "beads",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeRepository(t, beadsDir, "beads", 1)
			},
			want: true,
		},
		{
			// The fossil shape: a repository exists, but under a name the
			// caller is not opening. HasRepository is true here; HasDatabase
			// must be false, or the open would add a second repository beside
			// the first in a directory nothing ever declared a workspace.
			name:     "marker belongs to a different database",
			database: "hq",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeRepository(t, beadsDir, "beads", 1)
			},
			want: false,
		},
		{
			name:     "symlink marker is refused",
			database: "beads",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				databaseDir := filepath.Join(beadsDir, "embeddeddolt", "beads")
				if err := os.MkdirAll(databaseDir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(beadsDir, filepath.Join(databaseDir, ".dolt")); err != nil {
					t.Fatal(err)
				}
			},
			want: false,
		},
		{name: "empty name", database: "", want: false},
		{
			name:     "name that is not a single path element",
			database: filepath.Join("..", "beads"),
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeRepository(t, beadsDir, "beads", 1)
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
			if got := HasDatabase(beadsDir, tt.database); got != tt.want {
				t.Fatalf("HasDatabase(%q) = %v, want %v", tt.database, got, tt.want)
			}
			// Containment: a name HasDatabase accepts is a name HasRepository
			// accepts. The probes ask different questions, so the reverse does
			// not hold — see the different-database row.
			if tt.want && !HasRepository(beadsDir) {
				t.Fatalf("HasDatabase(%q) = true but HasRepository() = false — the probes disagree about a directory", tt.database)
			}
		})
	}
}
