package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureBeadsDirForPathUsesWorkspacePredicate is the be-n2s regression for
// the guard half of the disagreement. ensureBeadsDirForPath decided "is this
// an initialized workspace?" with a literal os.Stat(<target>/.beads/
// metadata.json), while discovery answered the same question through
// internal/beads.HasBeadsProjectFiles — which also accepts config.yaml and a
// bare database directory. For a target holding one of those and no
// metadata.json the two disagreed, and the guard's answer ("nothing here") is
// the one that lights the create path: a store open that auto-vivifies a
// database, and a metadata.json stamping the directory as a workspace the
// caller never targeted.
//
// The assertions use allowCreate=false because that is where the old
// agreement-gap is observable without building a source store: refusing an
// initialized workspace as uninitialized is exactly the wrong answer, and it
// is the one the literal metadata.json check gave. The bare-directory rows are
// the control — those must still be refused, or the fix has simply stopped
// guarding anything.
func TestEnsureBeadsDirForPathUsesWorkspacePredicate(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, beadsDir string)
		allowCreate bool
		wantErr     bool
		// hasMarker marks the rows whose setup legitimately writes
		// metadata.json, so the "nothing was stamped here" assertion below
		// does not re-assert what the row already created.
		hasMarker bool
	}{
		{
			// The divergence: initialized for discovery, uninitialized for the
			// guard, so a --repo target that is a real workspace was refused.
			name: "config.yaml only is an existing workspace",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("dolt:\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			allowCreate: false,
			wantErr:     false,
		},
		{
			name: "bare database directory is an existing workspace",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.Mkdir(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			allowCreate: false,
			wantErr:     false,
		},
		{
			name: "metadata.json is an existing workspace",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte("{}"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			allowCreate: false,
			wantErr:     false,
			hasMarker:   true,
		},
		{
			// Control: the guard still has to refuse a target with nothing in
			// it when creation was not authorized.
			name:        "empty beads directory is not a workspace",
			setup:       nil,
			allowCreate: false,
			wantErr:     true,
		},
		{
			// Control: creation that was authorized is unaffected.
			name:        "empty beads directory with authorized creation",
			setup:       nil,
			allowCreate: true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := t.TempDir()
			beadsDir := filepath.Join(target, ".beads")
			if err := os.Mkdir(beadsDir, 0o750); err != nil {
				t.Fatal(err)
			}
			if tt.setup != nil {
				tt.setup(t, beadsDir)
			}

			// A nil source store: with no source there is no prefix to inherit,
			// so the create path writes no metadata.json and opens no store.
			// That keeps the divergence observable on the guard's own answer.
			resolved, err := ensureBeadsDirForPath(t.Context(), target, nil, tt.allowCreate)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ensureBeadsDirForPath(allowCreate=%v) = %q, want an error", tt.allowCreate, resolved)
				}
				if !strings.Contains(err.Error(), "no beads workspace found") {
					t.Errorf("unexpected refusal wording: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ensureBeadsDirForPath(allowCreate=%v): %v", tt.allowCreate, err)
			}
			if resolved != beadsDir {
				t.Errorf("resolved beads dir = %q, want %q", resolved, beadsDir)
			}
			if _, statErr := os.Stat(filepath.Join(beadsDir, "metadata.json")); statErr == nil && !tt.hasMarker {
				t.Error("guard wrote metadata.json into a directory it did not target")
			}
		})
	}
}
