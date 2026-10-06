//go:build cgo

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
)

// TestBlockedEnvVars tests that BD_BACKEND and BD_DATABASE_BACKEND are blocked (bd-hevyw).
func TestBlockedEnvVars(t *testing.T) {
	tests := []struct {
		name   string
		envVar string
		value  string
	}{
		{"BD_BACKEND blocked", "BD_BACKEND", "sqlite"},
		{"BD_DATABASE_BACKEND blocked", "BD_DATABASE_BACKEND", "sqlite"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envVar, tt.value)
			err := checkBlockedEnvVars()
			if err == nil {
				t.Errorf("expected error when %s is set, got nil", tt.envVar)
			}
			if err != nil && !strings.Contains(err.Error(), tt.envVar) {
				t.Errorf("expected error to mention %s, got: %v", tt.envVar, err)
			}
			if err != nil && !strings.Contains(err.Error(), "bd help init-safety") {
				t.Errorf("expected error to point to safe reinitialization guidance, got: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "bd migrate dolt") {
				t.Errorf("error must not recommend the removed backend-conversion command: %v", err)
			}
		})
	}

	// Verify no error when env vars are unset
	t.Run("no env vars set", func(t *testing.T) {
		t.Setenv("BD_BACKEND", "")
		t.Setenv("BD_DATABASE_BACKEND", "")
		// Unset them (t.Setenv("", "") sets to empty which Getenv returns as "")
		os.Unsetenv("BD_BACKEND")
		os.Unsetenv("BD_DATABASE_BACKEND")
		err := checkBlockedEnvVars()
		if err != nil {
			t.Errorf("expected no error when env vars are unset, got: %v", err)
		}
	})
}

// bd-6dnrw.5: shared-server mode overriding a pinned dolt_mode=embedded must
// warn and win for the session, but must never rewrite the committed
// metadata.json (per-machine env must not leak into shared config).
func TestSharedServerEmbeddedMismatchDoesNotRewriteMetadata(t *testing.T) {
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := configfile.DefaultConfig()
	cfg.Backend = configfile.BackendDolt
	cfg.DoltMode = configfile.DoltModeEmbedded
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("save metadata.json: %v", err)
	}
	before, err := os.ReadFile(configfile.ConfigPath(beadsDir))
	if err != nil {
		t.Fatalf("read metadata.json: %v", err)
	}

	t.Setenv("BEADS_DOLT_SHARED_SERVER", "1")
	t.Setenv("BEADS_DOLT_SERVER_MODE", "")

	oldServerMode, oldWarned := serverMode, sharedServerEmbeddedMismatchWarned
	defer func() {
		serverMode = oldServerMode
		sharedServerEmbeddedMismatchWarned = oldWarned
	}()
	sharedServerEmbeddedMismatchWarned = false

	captureStderr := func(fn func()) string {
		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatalf("pipe: %v", pipeErr)
		}
		oldStderr := os.Stderr
		os.Stderr = w
		defer func() { os.Stderr = oldStderr }()
		fn()
		_ = w.Close()
		out, readErr := io.ReadAll(r)
		if readErr != nil {
			t.Fatalf("read stderr: %v", readErr)
		}
		return string(out)
	}

	stderr := captureStderr(func() { loadServerModeFromBeadsDir(beadsDir) })

	if !serverMode {
		t.Error("expected shared-server env to win for the session (serverMode=true)")
	}
	if !strings.Contains(stderr, "dolt_mode=\"embedded\"") {
		t.Errorf("expected mismatch notice on stderr, got: %q", stderr)
	}
	after, err := os.ReadFile(configfile.ConfigPath(beadsDir))
	if err != nil {
		t.Fatalf("re-read metadata.json: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("metadata.json was rewritten on disk:\nbefore: %s\nafter: %s", before, after)
	}

	// The notice is once-per-process: a second load must stay quiet.
	stderr = captureStderr(func() { loadServerModeFromBeadsDir(beadsDir) })
	if strings.Contains(stderr, "dolt_mode") {
		t.Errorf("expected no repeat notice, got: %q", stderr)
	}
}
