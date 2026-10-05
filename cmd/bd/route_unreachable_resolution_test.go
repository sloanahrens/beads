// Unit-tier cover for be-sut (deep review B1-04..06, be-qm8.2 residual b).
//
// A matched prefix route whose target database cannot be asked is route
// unreachable — an UNKNOWN — and must never be folded into the definite "no
// issue found matching" answer. bd close, bd defer and bd undefer each used to
// do exactly that; these tests drive the commands through the resolution seam
// where the typed routeUnreachableError has to survive.
//
// No Dolt store is involved: the local store is a stub that misses every id, so
// resolution reaches exactly the prefix-route lookup the fix touches and never
// opens a database. That keeps this file in the unit tier, which is the tier
// `make gate` runs. The end-to-end contract with a real store (exit codes,
// batch reporting, stderr prose) is covered by
// route_unreachable_write_verbs_integration_test.go.

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// writeUnreachableRouteFixture lays out a town whose routes.jsonl maps "zz-"
// to a rig directory, and whose rig .beads/metadata.json names no
// dolt_database. The route therefore matches but its store cannot be opened.
//
// townRoot is the directory the route path is relative to; beadsDir is the
// .beads directory the running command resolves as its own (it must be the
// one that carries the routes table).
func writeUnreachableRouteFixture(t *testing.T, townRoot, beadsDir string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"),
		[]byte(`{"prefix":"zz-","path":"rig"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	rigBeads := filepath.Join(townRoot, "rig", ".beads")
	if err := os.MkdirAll(rigBeads, 0o755); err != nil {
		t.Fatalf("create rig beads dir: %v", err)
	}
	// No dolt_database: the route matches but the target cannot be opened.
	if err := os.WriteFile(filepath.Join(rigBeads, "metadata.json"), []byte(`{"backend":"dolt"}`), 0o644); err != nil {
		t.Fatalf("write rig metadata.json: %v", err)
	}
}

// missingIssueStore is the minimum storage.DoltStorage that resolves no ids:
// every search misses and the configured prefix is unrelated to "zz-".
// Embedding the interface means any other method call panics, so a test that
// reaches one is a test exercising a path it did not mean to.
type missingIssueStore struct {
	storage.DoltStorage
}

func (missingIssueStore) SearchIssues(context.Context, string, types.IssueFilter) ([]*types.Issue, error) {
	return nil, nil
}

func (missingIssueStore) SearchIssueIDs(context.Context, string, types.IssueFilter) ([]string, error) {
	return nil, nil
}

func (missingIssueStore) GetConfig(context.Context, string) (string, error) {
	return "test", nil
}

// GetAllConfig keeps the contributor auto-routing probe on the "not
// configured" path instead of reaching the nil embedded interface.
func (missingIssueStore) GetAllConfig(context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (missingIssueStore) GetIssue(context.Context, string) (*types.Issue, error) {
	return nil, storage.ErrNotFound
}

// unreachableRouteTown builds the fixture against a temp town root, points the
// package's dbPath at its .beads directory, and returns the store to resolve
// against.
func unreachableRouteTown(t *testing.T) storage.DoltStorage {
	t.Helper()
	saveAndRestoreGlobals(t)

	townRoot := t.TempDir()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("create beads dir: %v", err)
	}
	// A real .beads dir carries a metadata.json pointing at its database; only
	// its presence matters here, since the local store is a stub.
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(`{"backend":"dolt"}`), 0o644); err != nil {
		t.Fatalf("write town metadata.json: %v", err)
	}
	writeUnreachableRouteFixture(t, townRoot, beadsDir)

	dbPath = filepath.Join(beadsDir, "beads.db")
	return missingIssueStore{}
}

// runWriteVerb drives a write verb's RunE against the stub store and returns
// the error it would hand main.go, plus whatever it wrote to stderr. The
// globals the RunE functions read are saved and restored.
func runWriteVerb(t *testing.T, st storage.DoltStorage, cmd *cobra.Command, args ...string) (error, string) {
	t.Helper()
	stdioMutex.Lock()
	defer stdioMutex.Unlock()

	savedCtx, savedStore, savedActor := rootCtx, store, actor
	savedJSON, savedReadonly, savedQuiet := jsonOutput, readonlyMode, quietFlag
	t.Cleanup(func() {
		rootCtx, store, actor = savedCtx, savedStore, savedActor
		jsonOutput, readonlyMode, quietFlag = savedJSON, savedReadonly, savedQuiet
		resetCommandFlagsToDefaults(cmd)
	})

	rootCtx = context.Background()
	store = st
	actor = "be-sut-test"
	jsonOutput = false
	readonlyMode = false
	quietFlag = true

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, r); close(done) }()
	os.Stderr = w
	runErr := cmd.RunE(cmd, args)
	_ = w.Close()
	os.Stderr = oldStderr
	<-done
	_ = r.Close()

	return runErr, buf.String()
}

// TestDeferReportsUnreachableRoute pins the defer half at the command level:
// bd defer used to resolve every id against the local store and answer
// "no issue found matching" for a route it could not reach.
func TestDeferReportsUnreachableRoute(t *testing.T) {
	st := unreachableRouteTown(t)

	runErr, stderr := runWriteVerb(t, st, deferCmd, "zz-abc")
	if runErr == nil {
		t.Fatal("bd defer reported success for an unreachable route")
	}
	if got := errorKindOf(runErr); got != kindRouteUnreachable {
		t.Errorf("kind = %q, want %q\nstderr:\n%s", got, kindRouteUnreachable, stderr)
	}
	if !strings.Contains(stderr, "zz-") || !strings.Contains(stderr, "rig") {
		t.Errorf("stderr does not name the prefix and route:\n%s", stderr)
	}
	if strings.Contains(stderr, "not found") {
		t.Errorf("unreachable route reported as a definite negative:\n%s", stderr)
	}
}

// TestUndeferReportsUnreachableRoute is defer's counterpart for bd undefer.
func TestUndeferReportsUnreachableRoute(t *testing.T) {
	st := unreachableRouteTown(t)

	runErr, stderr := runWriteVerb(t, st, undeferCmd, "zz-abc")
	if runErr == nil {
		t.Fatal("bd undefer reported success for an unreachable route")
	}
	if got := errorKindOf(runErr); got != kindRouteUnreachable {
		t.Errorf("kind = %q, want %q\nstderr:\n%s", got, kindRouteUnreachable, stderr)
	}
	if strings.Contains(stderr, "not found") {
		t.Errorf("unreachable route reported as a definite negative:\n%s", stderr)
	}
}

// TestUnreachableRouteIsStillDistinctFromAMiss guards the boundary: an id
// whose prefix has no route at all stays an ordinary not-found, so the fix
// cannot have widened route_unreachable over every unknown id.
func TestUnreachableRouteIsStillDistinctFromAMiss(t *testing.T) {
	st := unreachableRouteTown(t)

	runErr, stderr := runWriteVerb(t, st, deferCmd, "qq-abc")
	if runErr == nil {
		t.Fatal("bd defer reported success for an id with no route")
	}
	if got := errorKindOf(runErr); got != kindNotFound {
		t.Errorf("kind = %q, want %q\nstderr:\n%s", got, kindNotFound, stderr)
	}
	if strings.Contains(stderr, "could not reach") {
		t.Errorf("routeless id reported as unreachable:\n%s", stderr)
	}
}

// TestCloseKeepsUnreachableRoutePerID pins the close half: an id whose prefix
// route cannot be opened is reported unresolved, carrying the typed route
// error, rather than being turned into "no issue found matching" (or aborting
// the batch for the ids that did resolve).
func TestCloseKeepsUnreachableRoutePerID(t *testing.T) {
	store := unreachableRouteTown(t)
	ctx := context.Background()

	results, unresolved, cleanup, err := resolveCloseTargets(ctx, store, []string{"zz-abc"})
	if err != nil {
		t.Fatalf("resolveCloseTargets: %v", err)
	}
	defer cleanup()

	if len(results) != 0 {
		t.Fatalf("resolved %d ids, want 0: %+v", len(results), results)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved = %d entries, want 1: %+v", len(unresolved), unresolved)
	}
	if unresolved[0].ID != "zz-abc" {
		t.Errorf("unresolved id = %q, want %q", unresolved[0].ID, "zz-abc")
	}
	var re *routeUnreachableError
	if !errors.As(unresolved[0].Err, &re) {
		t.Fatalf("unresolved err = %v (%T), want *routeUnreachableError", unresolved[0].Err, unresolved[0].Err)
	}
	if re.Prefix != "zz-" {
		t.Errorf("route error prefix = %q, want %q", re.Prefix, "zz-")
	}
	if got := errorKindOf(unresolved[0].Err); got != kindRouteUnreachable {
		t.Errorf("kind = %q, want %q", got, kindRouteUnreachable)
	}
	if isNotFoundErr(unresolved[0].Err) {
		t.Error("unreachable route misclassified as not found")
	}
}

// TestDeferAndUndeferKeepUnreachableRoute pins the resolution the two commands
// share: resolveAndGetIssueForMutation must surface the typed route error
// rather than the local store's miss.
func TestDeferAndUndeferKeepUnreachableRoute(t *testing.T) {
	st := unreachableRouteTown(t)
	ctx := context.Background()

	result, err := resolveAndGetIssueForMutation(ctx, st, "zz-abc")
	if result != nil {
		result.Close()
	}
	if err == nil {
		t.Fatal("resolveAndGetIssueForMutation returned no error for an unreachable route")
	}
	var re *routeUnreachableError
	if !errors.As(err, &re) {
		t.Fatalf("err = %v (%T), want *routeUnreachableError", err, err)
	}
	if re.Prefix != "zz-" {
		t.Errorf("route error prefix = %q, want %q", re.Prefix, "zz-")
	}
	if got := errorKindOf(err); got != kindRouteUnreachable {
		t.Errorf("kind = %q, want %q", got, kindRouteUnreachable)
	}
	if isNotFoundErr(err) {
		t.Error("unreachable route misclassified as not found")
	}
}
