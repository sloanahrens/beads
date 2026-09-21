package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
