//go:build cgo

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/configfile"
)

// deprecatedMetadataPortNotice is the three-line warning the metadata.json
// dolt_server_port arm of doltserver's port precedence chain writes to stderr.
// It is spelled out here rather than read from the package so a reworded
// notice fails these tests instead of silently changing what they assert.
const deprecatedMetadataPortNotice = "" +
	"Warning: dolt_server_port in metadata.json is deprecated (can cause cross-project data leakage).\n" +
	"  The port file (.beads/dolt-server.port) is now the primary source.\n" +
	"  Remove dolt_server_port from .beads/metadata.json to silence this warning.\n"

// deprecatedMetadataPortWorkspace creates a git repo whose only configured
// port is metadata.json's deprecated dolt_server_port (3307).
//
// That is what puts `bd context` on the metadata arm: dolt_mode=server makes it
// resolve the server identity, and with no BEADS_DOLT_SERVER_PORT, no
// .beads/dolt-server.port file and no config.yaml anywhere, the metadata arm is
// the first source in the chain that resolves. Nothing listens on the port —
// `bd context` reads config files and never dials — so no Dolt server is
// needed.
func deprecatedMetadataPortWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	initGitRepo(t, dir)

	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o700); err != nil {
		t.Fatalf("mkdir beads dir: %v", err)
	}
	cfg := &configfile.Config{
		Backend:        configfile.BackendDolt,
		DoltMode:       configfile.DoltModeServer,
		DoltServerPort: 3307,
		DoltDatabase:   "testdb",
	}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	return dir
}

// runBDMerged runs the bd under test in dir and returns stdout and stderr
// MERGED into one stream, the way a caller that writes `2>&1` (or a test
// capture helper that assigns both to one buffer) sees the process. The
// environment is built from scratch: an inherited BEADS_* or GT_TOWN_ROOT
// would point the command at the developer's own town instead of the fixture.
func runBDMerged(t *testing.T, binPath, dir string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Dir = dir
	home := t.TempDir()
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"BEADS_TEST_IGNORE_REPO_CONFIG=1",
		"BEADS_DIR=",
		"BEADS_DB=",
	}
	var merged bytes.Buffer
	cmd.Stdout = &merged
	cmd.Stderr = &merged
	err := cmd.Run()
	return merged.String(), err
}

// TestJSONMergedOutputStaysPureWithDeprecatedMetadataPort is the reported bug
// (be-g9x7): with a deprecated metadata.json dolt_server_port set, the
// deprecation notice used to follow stdout into the merged stream, so a
// consumer that reads `bd context --json` with stderr merged could not parse
// the result. The whole merged stream must be one --json envelope, and its data
// must carry the metadata port, which is what proves the run reached the
// deprecated arm rather than passing by answering from another source.
func TestJSONMergedOutputStaysPureWithDeprecatedMetadataPort(t *testing.T) {
	binPath := buildBDUnderTest(t)
	dir := deprecatedMetadataPortWorkspace(t)

	merged, err := runBDMerged(t, binPath, dir, "context", "--json")
	if err != nil {
		t.Fatalf("bd context --json: %v\nmerged output: %s", err, merged)
	}

	var env struct {
		SchemaVersion *int            `json:"schema_version"`
		Data          json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(merged), &env); err != nil {
		t.Fatalf("merged --json output is not one JSON document: %v\nmerged output: %s", err, merged)
	}
	if env.SchemaVersion == nil || len(env.Data) == 0 {
		t.Fatalf("merged output is not a --json envelope (want schema_version and data): %s", merged)
	}
	var data struct {
		ServerPort int `json:"server_port"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode envelope data: %v\nmerged output: %s", err, merged)
	}
	if data.ServerPort != 3307 {
		t.Fatalf("server_port = %d, want 3307 from metadata.json: the run did not resolve the deprecated metadata arm, so it says nothing about the notice\nmerged output: %s", data.ServerPort, merged)
	}
	if strings.Contains(merged, deprecatedMetadataPortNotice) {
		t.Errorf("deprecated dolt_server_port notice landed in --json output:\n%s", merged)
	}
}

// TestTextOutputKeepsDeprecatedMetadataPortWarning is the twin that keeps the
// suppression honest: only the machine-readable path is quiet. A text-mode
// caller — a script, an agent, the daemon — still gets the deprecation notice
// on the same fixture. Without this test, gating the notice on anything that
// silences it in JSON mode (an isatty probe, a global sweep) would pass the
// JSON test while dropping the warning for every scripted text run.
func TestTextOutputKeepsDeprecatedMetadataPortWarning(t *testing.T) {
	binPath := buildBDUnderTest(t)
	dir := deprecatedMetadataPortWorkspace(t)

	merged, err := runBDMerged(t, binPath, dir, "context")
	if err != nil {
		t.Fatalf("bd context: %v\nmerged output: %s", err, merged)
	}
	if !strings.Contains(merged, deprecatedMetadataPortNotice) {
		t.Errorf("text-mode output dropped the deprecated dolt_server_port notice (want it present in text mode):\n%s", merged)
	}
}
