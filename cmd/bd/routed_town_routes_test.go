package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRoutesFile creates <beadsDir>/routes.jsonl with content and returns the
// file's path.
func writeRoutesFile(t *testing.T, beadsDir, content string) string {
	t.Helper()
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	routesPath := filepath.Join(beadsDir, "routes.jsonl")
	if err := os.WriteFile(routesPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return routesPath
}

// be-xkj: a malformed line in routes.jsonl must not be skipped silently. It is
// warned about (file + 1-based line number) and skipped, while the valid routes
// on other lines still load and the read never fails. Blank lines and comments
// stay quiet — they are not typos.
func TestLoadPrefixRoutes_WarnsOnMalformedLines(t *testing.T) {
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	routesPath := writeRoutesFile(t, beadsDir, strings.Join([]string{
		`# comment`,                       // 1: comment, silent
		``,                                // 2: blank, silent
		`{"prefix":"hq-","path":"."}`,     // 3: valid
		`{"prefix":"om-","path":}`,        // 4: invalid JSON
		`{"prefix":"","path":"om/rig"}`,   // 5: empty prefix
		`{"prefix":"zz-","path":""}`,      // 6: empty path
		`   `,                             // 7: whitespace-only, silent
		`{"prefix":"be-","path":"beads"}`, // 8: valid
	}, "\n")+"\n")

	var routes []prefixRoute
	var err error
	stderr := captureStderr(t, func() { routes, err = loadPrefixRoutes(beadsDir) })
	if err != nil {
		t.Fatalf("a bad line must not fail the read: %v", err)
	}
	if len(routes) != 2 || routes[0].Prefix != "hq-" || routes[1].Prefix != "be-" {
		t.Fatalf("valid routes must still load, got %+v", routes)
	}

	if got := strings.Count(stderr, "Warning:"); got != 3 {
		t.Fatalf("want exactly 3 warnings (one per bad line), got %d: %q", got, stderr)
	}
	for _, want := range []string{routesPath, "line 4", "line 5", "line 6"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("warning must name %q, got %q", want, stderr)
		}
	}
	for _, unwanted := range []string{"line 3", "line 8"} {
		if strings.Contains(stderr, unwanted) {
			t.Errorf("valid route on %q must not warn, got %q", unwanted, stderr)
		}
	}
}

// be-xkj: the warning is advisory. Machine mode (BD_MACHINE / --json) implies a
// machine-readable stdout, so a malformed routes.jsonl must add nothing there;
// the warning belongs on stderr.
func TestLoadPrefixRoutes_MachineModeKeepsStdoutClean(t *testing.T) {
	beadsDir := filepath.Join(t.TempDir(), ".beads")
	writeRoutesFile(t, beadsDir, "{\"prefix\":\"om-\",\"path\":}\n{\"prefix\":\"hq-\",\"path\":\".\"}\n")

	oldMachine := machineMode
	machineMode = true
	defer func() { machineMode = oldMachine }()

	stdout := captureStdout(t, func() error {
		if _, err := loadPrefixRoutes(beadsDir); err != nil {
			return err
		}
		return nil
	})
	if stdout != "" {
		t.Errorf("machine mode: stdout must stay empty, got %q", stdout)
	}

	stderr := captureStderr(t, func() { _, _ = loadPrefixRoutes(beadsDir) })
	if !strings.Contains(stderr, "line 1") {
		t.Errorf("the warning must still reach stderr in machine mode, got %q", stderr)
	}
}

// be-v1o: a rig-scoped agent (refinery, witness, polecat) runs bd from a rig
// directory whose .beads redirects to <rig>/mayor/rig/.beads. That directory
// has no routes.jsonl — the prefix routes live in the TOWN's .beads — so
// prefix routing must find the town table by walking up from the resolved
// beads dir instead of declaring "no routes available".
func TestLocatePrefixRoutes_WalksUpToTownTable(t *testing.T) {
	town := t.TempDir()
	townBeads := filepath.Join(town, ".beads")
	if err := os.MkdirAll(townBeads, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"),
		[]byte("{\"prefix\":\"hq-\",\"path\":\".\"}\n{\"prefix\":\"om-\",\"path\":\"om/mayor/rig\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rigBeads := filepath.Join(town, "om", "mayor", "rig", ".beads")
	if err := os.MkdirAll(rigBeads, 0o755); err != nil {
		t.Fatal(err)
	}

	routes, tableDir, err := locatePrefixRoutes(rigBeads)
	if err != nil {
		t.Fatalf("locatePrefixRoutes from rig beads dir: %v", err)
	}
	if tableDir != townBeads {
		t.Fatalf("routes table dir = %q, want the town's %q", tableDir, townBeads)
	}
	if len(routes) != 2 || routes[0].Prefix != "hq-" || routes[1].Path != "om/mayor/rig" {
		t.Fatalf("routes = %+v, want the town table", routes)
	}
}

// A beads dir that carries its own routes.jsonl (the town root itself) is
// answered directly, without walking past it.
func TestLocatePrefixRoutes_LocalTableWins(t *testing.T) {
	town := t.TempDir()
	townBeads := filepath.Join(town, ".beads")
	if err := os.MkdirAll(townBeads, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(townBeads, "routes.jsonl"), []byte("{\"prefix\":\"hq-\",\"path\":\".\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	routes, tableDir, err := locatePrefixRoutes(townBeads)
	if err != nil {
		t.Fatal(err)
	}
	if tableDir != townBeads || len(routes) != 1 {
		t.Fatalf("got dir %q routes %+v", tableDir, routes)
	}
}

// No table anywhere up the tree is an error, not a silent empty answer.
func TestLocatePrefixRoutes_NoTableIsAnError(t *testing.T) {
	beadsDir := filepath.Join(t.TempDir(), "a", "b", ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := locatePrefixRoutes(beadsDir); err == nil {
		t.Fatal("expected an error when no routes.jsonl exists up the tree")
	}
}

// The "." route (the town database) must be followed from a rig: only a route
// whose target IS the current beads dir is skipped.
func TestPrefixRouteTarget_DotRouteFromRigResolvesToTown(t *testing.T) {
	town := t.TempDir()
	townBeads := filepath.Join(town, ".beads")
	rigBeads := filepath.Join(town, "om", "mayor", "rig", ".beads")
	for _, d := range []string{townBeads, rigBeads} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	target, skip := prefixRouteTarget(prefixRoute{Prefix: "hq-", Path: "."}, townBeads, rigBeads)
	if skip {
		t.Fatalf("the town route was skipped from a rig beads dir")
	}
	if target != townBeads {
		t.Fatalf("target = %q, want %q", target, townBeads)
	}
	if _, skip := prefixRouteTarget(prefixRoute{Prefix: "hq-", Path: "."}, townBeads, townBeads); !skip {
		t.Fatalf("a route back to the current beads dir must be skipped")
	}
}
