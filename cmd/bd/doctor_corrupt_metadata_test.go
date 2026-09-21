package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateDoctorWorkspaceBackendCorruptMetadata pins doctor's half of the
// corrupt-metadata contract, which is what makes the corruption reportable at
// all: bd init is the repair, but doctor is what tells the user what is wrong,
// so doctor has to run. It accepts an unreadable metadata.json only where
// nothing depends on the answer — a sole embedded database names the storage
// mode and the database by itself, and no server is in play to be misidentified.
// Every other shape keeps the loud refusal that names the file.
func TestValidateDoctorWorkspaceBackendCorruptMetadata(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T, beadsDir string)
		wantRefusal bool
	}{
		{
			name: "sole embedded database is diagnosable",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeEmbeddedRepository(t, beadsDir, "cm")
			},
		},
		{
			name:        "no local database to diagnose",
			setup:       func(*testing.T, string) {},
			wantRefusal: true,
		},
		{
			name: "ambiguous local databases",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeEmbeddedRepository(t, beadsDir, "cm")
				writeEmbeddedRepository(t, beadsDir, "other")
			},
			wantRefusal: true,
		},
		{
			name: "shared server owns the target",
			setup: func(t *testing.T, beadsDir string) {
				t.Helper()
				writeEmbeddedRepository(t, beadsDir, "cm")
				t.Setenv("BEADS_DOLT_SHARED_SERVER", "1")
			},
			wantRefusal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pin the ambient shared-server answer first so a developer's own
			// config.yaml cannot turn a soft refusal into a hard one, then let
			// the case set it deliberately.
			t.Setenv("BEADS_DOLT_SHARED_SERVER", "0")
			repo := t.TempDir()
			beadsDir := filepath.Join(repo, ".beads")
			if err := os.MkdirAll(beadsDir, 0o700); err != nil {
				t.Fatal(err)
			}
			before := writeCorruptWorkspaceMetadata(t, beadsDir)
			tt.setup(t, beadsDir)

			var err error
			diagnosis := captureStderr(t, func() { err = validateDoctorWorkspaceBackend(repo) })

			if tt.wantRefusal {
				if err == nil {
					t.Fatalf("validateDoctorWorkspaceBackend() = nil, want a refusal naming metadata.json\nstderr:\n%s", diagnosis)
				}
				for _, want := range []string{"metadata.json", "no storage database was opened or modified"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("refusal error missing %q: %v", want, err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("validateDoctorWorkspaceBackend() = %v, want nil — the corruption must stay reportable", err)
			}
			// The soft refusal is only useful if it says what is wrong and how
			// to fix it; silence would read as a healthy workspace.
			if !strings.Contains(diagnosis, "metadata.json") || !strings.Contains(diagnosis, "bd init") {
				t.Errorf("doctor admitted the workspace without naming the file and its repair:\n%s", diagnosis)
			}
			assertMetadataUnchanged(t, beadsDir, before)
		})
	}
}
