package doltserver

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
)

const metadataPortNoticeText = "Warning: dolt_server_port in metadata.json is deprecated (can cause cross-project data leakage).\n" +
	"  The port file (.beads/dolt-server.port) is now the primary source.\n" +
	"  Remove dolt_server_port from .beads/metadata.json to silence this warning.\n"

// rearmMetadataPortNotice gives the test a fresh once-state, so the notice
// fires for this test whatever an earlier test in the process already printed.
func rearmMetadataPortNotice(t *testing.T) {
	t.Helper()
	orig := metadataPortNotice
	metadataPortNotice = &sync.Once{}
	t.Cleanup(func() { metadataPortNotice = orig })
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns what fn
// wrote there. The notice is written to the process's stderr from inside
// DefaultConfig, so the redirect is the only way to observe it from a caller.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = orig })

	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	fn()
	os.Stderr = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	s := <-out
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestDeprecatedMetadataPortNoticeOncePerProcess pins that resolving the port
// twice through a metadata.json carrying dolt_server_port writes the notice
// once. DefaultConfig walks the port chain more than once per call, and the
// notice used to repeat on every walk.
func TestDeprecatedMetadataPortNoticeOncePerProcess(t *testing.T) {
	isolateDoltConfig(t)
	rearmMetadataPortNotice(t)
	beadsDir := t.TempDir()
	writeMetadataPort(t, beadsDir, 5001)

	var first, second *Config
	got := captureStderr(t, func() {
		first = DefaultConfig(beadsDir)
		second = DefaultConfig(beadsDir)
	})

	for i, cfg := range []*Config{first, second} {
		if cfg.Port != 5001 || cfg.PortSource != PortSourceMetadataJSON {
			t.Fatalf("resolution %d: port=%d source=%q, want 5001/%q", i+1, cfg.Port, cfg.PortSource, PortSourceMetadataJSON)
		}
	}
	if got != metadataPortNoticeText {
		t.Fatalf("stderr across two resolutions = %q, want the notice once: %q", got, metadataPortNoticeText)
	}
}

// TestDeprecatedMetadataPortSilentWithPortFile pins that a workspace whose port
// file is present stays silent even when metadata.json still carries the key.
// The port file is not listening here, so the be-60t reconciliation also reads
// the metadata arm; it must not print the notice on that read either.
func TestDeprecatedMetadataPortSilentWithPortFile(t *testing.T) {
	isolateDoltConfig(t)
	rearmMetadataPortNotice(t)
	beadsDir := t.TempDir()
	writeMetadataPort(t, beadsDir, 5001)
	if err := writePortFile(beadsDir, 5002); err != nil {
		t.Fatal(err)
	}

	var cfg *Config
	got := captureStderr(t, func() {
		cfg = DefaultConfig(beadsDir)
	})

	if cfg.PortSource != PortSourcePortFile {
		t.Fatalf("source=%q, want the port file %q", cfg.PortSource, PortSourcePortFile)
	}
	if got != "" {
		t.Fatalf("stderr with a port file present = %q, want nothing", got)
	}
}

// TestEmitDeprecatedMetadataPortWarning pins the two behaviours the metadata
// dolt_server_port notice has: it is the documented three-line warning in text
// mode, and it writes nothing when the process's output belongs to a machine
// consumer (be-g9x7 — the notice used to follow stdout into a merged --json
// capture and make it unparseable).
//
// The writer and the quiet flag are parameters, which is why this runs against
// a buffer instead of redirecting os.Stderr: the emitter's contract is what it
// writes to the writer it is handed, not which stream the process has.
func TestEmitDeprecatedMetadataPortWarning(t *testing.T) {
	tests := []struct {
		name  string
		quiet bool
		want  string
	}{
		{
			name:  "text mode writes the three-line notice",
			quiet: false,
			want: "Warning: dolt_server_port in metadata.json is deprecated (can cause cross-project data leakage).\n" +
				"  The port file (.beads/dolt-server.port) is now the primary source.\n" +
				"  Remove dolt_server_port from .beads/metadata.json to silence this warning.\n",
		},
		{
			name:  "quiet mode writes nothing",
			quiet: true,
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			emitDeprecatedMetadataPortWarning(&buf, tt.quiet)
			if got := buf.String(); got != tt.want {
				t.Errorf("emitDeprecatedMetadataPortWarning(quiet=%v) wrote %q, want %q", tt.quiet, got, tt.want)
			}
			if tt.quiet && strings.Contains(buf.String(), "deprecated") {
				t.Errorf("quiet mode leaked the notice: %q", buf.String())
			}
		})
	}
}
