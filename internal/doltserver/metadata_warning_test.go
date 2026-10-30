package doltserver

import (
	"bytes"
	"strings"
	"testing"
)

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
