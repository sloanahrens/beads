package doltserver

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const metadataPortNoticeText = "Warning: dolt_server_port in metadata.json is deprecated (can cause cross-project data leakage).\n" +
	"  The port file (.beads/dolt-server.port) is now the primary source.\n" +
	"  Remove dolt_server_port from .beads/metadata.json to silence this warning.\n"

// newTestNotice returns a notice of its own, with its own once-state, writing to
// a buffer the test can read. Each test gets one, so no test shares state with
// another. isolateDoltConfig uses t.Setenv, which bars t.Parallel, so these
// tests run one after another.
func newTestNotice() (*metadataPortNotice, *bytes.Buffer) {
	var buf bytes.Buffer
	return &metadataPortNotice{w: &buf, quiet: &atomic.Bool{}}, &buf
}

// TestDeprecatedMetadataPortNoticeOncePerProcess pins that resolving the port
// twice through a metadata.json carrying dolt_server_port, with no other source,
// writes the notice once. DefaultConfig walks the port chain more than once per
// call, and the notice used to repeat on every walk.
func TestDeprecatedMetadataPortNoticeOncePerProcess(t *testing.T) {
	isolateDoltConfig(t)
	beadsDir := t.TempDir()
	writeMetadataPort(t, beadsDir, 5001)
	notice, buf := newTestNotice()

	first := defaultConfig(beadsDir, notice)
	second := defaultConfig(beadsDir, notice)

	for i, cfg := range []*Config{first, second} {
		if cfg.Port != 5001 || cfg.PortSource != PortSourceMetadataJSON {
			t.Fatalf("resolution %d: port=%d source=%q, want 5001/%q", i+1, cfg.Port, cfg.PortSource, PortSourceMetadataJSON)
		}
	}
	if got := buf.String(); got != metadataPortNoticeText {
		t.Fatalf("output across two resolutions = %q, want the notice once: %q", got, metadataPortNoticeText)
	}
}

// TestDeprecatedMetadataPortSilentWithPortFile pins that a workspace whose port
// file is present stays silent even when metadata.json still carries the key.
// The port file is not listening here, so the be-60t reconciliation also looks
// at the metadata arm; the notice must stay silent on that look too.
func TestDeprecatedMetadataPortSilentWithPortFile(t *testing.T) {
	isolateDoltConfig(t)
	beadsDir := t.TempDir()
	writeMetadataPort(t, beadsDir, 5001)
	if err := writePortFile(beadsDir, 5002); err != nil {
		t.Fatal(err)
	}
	notice, buf := newTestNotice()

	cfg := defaultConfig(beadsDir, notice)

	if cfg.PortSource != PortSourcePortFile {
		t.Fatalf("source=%q, want the port file %q", cfg.PortSource, PortSourcePortFile)
	}
	if got := buf.String(); got != "" {
		t.Fatalf("output with a port file present = %q, want nothing", got)
	}
}

// TestDeprecatedMetadataPortNoticeBySource pins which source decides the port
// and whether the notice follows. The notice prints only when the metadata.json
// key decided the port; a port file or the environment variable that wins keeps
// it silent.
func TestDeprecatedMetadataPortNoticeBySource(t *testing.T) {
	tests := []struct {
		name       string
		env        string
		portFile   bool
		metaKey    bool
		wantPort   int
		wantSource PortSource
		wantNotice bool
	}{
		{
			name:       "port file present, metadata key also set",
			portFile:   true,
			metaKey:    true,
			wantPort:   5002,
			wantSource: PortSourcePortFile,
		},
		{
			name:       "BEADS_DOLT_SERVER_PORT set, metadata key also set",
			env:        "5003",
			metaKey:    true,
			wantPort:   5003,
			wantSource: PortSourceEnv,
		},
		{
			name:       "metadata key is the only source",
			metaKey:    true,
			wantPort:   5001,
			wantSource: PortSourceMetadataJSON,
			wantNotice: true,
		},
		{
			name:       "no metadata key",
			wantPort:   0,
			wantSource: PortSourceUnset,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateDoltConfig(t)
			beadsDir := t.TempDir()
			if tt.env != "" {
				t.Setenv("BEADS_DOLT_SERVER_PORT", tt.env)
			}
			if tt.metaKey {
				writeMetadataPort(t, beadsDir, 5001)
			} else if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tt.portFile {
				if err := writePortFile(beadsDir, 5002); err != nil {
					t.Fatal(err)
				}
			}
			notice, buf := newTestNotice()

			cfg := defaultConfig(beadsDir, notice)

			if cfg.Port != tt.wantPort || cfg.PortSource != tt.wantSource {
				t.Fatalf("port=%d source=%q, want %d/%q", cfg.Port, cfg.PortSource, tt.wantPort, tt.wantSource)
			}
			want := ""
			if tt.wantNotice {
				want = metadataPortNoticeText
			}
			if got := buf.String(); got != want {
				t.Fatalf("output = %q, want %q", got, want)
			}
		})
	}
}

// TestDeprecatedMetadataPortNoticeWhenLiveMetadataWinsOverDeadPortFile pins the
// be-60t path: the port file names a dead server, and the metadata.json port is
// live, so reconciliation replaces the port file with the metadata port. The key
// now sets the port, so the notice must print.
func TestDeprecatedMetadataPortNoticeWhenLiveMetadataWinsOverDeadPortFile(t *testing.T) {
	isolateDoltConfig(t)
	beadsDir := t.TempDir()
	_, livePort := startLiveListener(t)
	writeMetadataPort(t, beadsDir, livePort)
	if err := writePortFile(beadsDir, closedPort(t)); err != nil {
		t.Fatal(err)
	}
	notice, buf := newTestNotice()

	cfg := defaultConfig(beadsDir, notice)

	if cfg.Port != livePort || cfg.PortSource != PortSourceMetadataJSON {
		t.Fatalf("port=%d source=%q, want the live metadata port %d/%q", cfg.Port, cfg.PortSource, livePort, PortSourceMetadataJSON)
	}
	if got := buf.String(); got != metadataPortNoticeText {
		t.Fatalf("output = %q, want the notice once: %q", got, metadataPortNoticeText)
	}
}

// TestDeprecatedMetadataPortNoticeQuiet pins that the notice honours the quiet
// flag (be-g9x7) when metadata.json decides the port: the port is still resolved
// from the key, and nothing is written.
func TestDeprecatedMetadataPortNoticeQuiet(t *testing.T) {
	isolateDoltConfig(t)
	beadsDir := t.TempDir()
	writeMetadataPort(t, beadsDir, 5001)
	notice, buf := newTestNotice()
	notice.quiet.Store(true)

	cfg := defaultConfig(beadsDir, notice)

	if cfg.Port != 5001 || cfg.PortSource != PortSourceMetadataJSON {
		t.Fatalf("port=%d source=%q, want 5001/%q", cfg.Port, cfg.PortSource, PortSourceMetadataJSON)
	}
	if got := buf.String(); got != "" {
		t.Fatalf("quiet output = %q, want nothing", got)
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
