//go:build cgo && integration

// Integration tier (be-b23): moved verbatim from the unit-tier sibling file.
// These tests run bd init against a fresh store, which migrates; the unit
// tier's BD_TEST_TIER tripwire refuses that.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/backends"
	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
	"github.com/steveyegge/beads/internal/types"
)

func TestServeAnswersFromARegisteredBackendStore(t *testing.T) {
	const name = "serve-registered-e2e"

	dir := t.TempDir()
	initGitRepoAt(t, dir)
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	seedID := seedRegisteredBackendWorkspace(t, beadsDir)

	// A hook the workspace would fire on a CLI claim. `bd serve` documents that
	// it does not, and this marker is the only evidence that survives the
	// subprocess either way.
	hookMarker := filepath.Join(dir, "on_update.ran")
	plantOnUpdateHook(t, beadsDir, hookMarker)

	// Every store this process opens for the registered name is counted, and
	// the count is an assertion at the end: bd serve creates NOTHING on this
	// arm, so the root command's one open has to be the only one in the
	// process. A serve that opened a handle of its own would leak it — nothing
	// closes it — and double the backend's pools against a workspace some
	// backends hold an exclusive lock on. Counting here catches it wherever it
	// is spelled, because every store factory in cmd/bd dispatches through this
	// same registry (newDoltStoreFromConfig, newReadOnlyStoreFromConfig).
	var opens atomic.Int64
	backends.Register(name, backends.Backend{
		Open: func(ctx context.Context, beadsDir string) (storage.DoltStorage, error) {
			opens.Add(1)
			return embeddeddolt.Open(ctx, beadsDir, serveRegisteredDatabase, "main")
		},
		OpenReadOnly: func(ctx context.Context, beadsDir string) (storage.DoltStorage, error) {
			opens.Add(1)
			return embeddeddolt.OpenReadOnly(ctx, beadsDir, serveRegisteredDatabase, "main")
		},
		WorkspaceIsBeadsDir: true,
	})
	t.Cleanup(func() { backends.Deregister(name) })
	if err := (&configfile.Config{Backend: name, DoltDatabase: serveRegisteredDatabase}).Save(beadsDir); err != nil {
		t.Fatalf("save metadata.json: %v", err)
	}

	addr, done := startServeInProcess(t, dir, beadsDir)

	base := "http://" + addr
	t.Run("the handshake answers", func(t *testing.T) {
		body := getJSON(t, base+"/v0/beads/context")
		if body["schema_version"] == nil {
			t.Errorf("GET /v0/beads/context returned no schema_version: %v", body)
		}
	})

	// GET /v0/beads/context is the one endpoint automation is told to trust for
	// this server's identity, and it was reporting backend="dolt",
	// dolt_mode="embedded", database="beads" here — a full, confident
	// description of the exact topology bd serve REFUSES to serve, while the
	// startup line beside it named the registered backend correctly.
	//
	// database is empty even though this workspace's metadata.json carries
	// dolt_database and the store behind the registered name really does open
	// it: bd does not implement this backend, its Open reads whatever it wants
	// out of the workspace, and a value bd cannot verify is the same lie made
	// quieter. Empty is what bd can assert. Both fields stay required strings —
	// the wire shape is unchanged.
	t.Run("the handshake names the registered backend", func(t *testing.T) {
		body := getJSON(t, base+"/v0/beads/context")
		if body["backend"] != name {
			t.Errorf("backend = %v, want %q: the handshake names a backend this server is not on", body["backend"], name)
		}
		if body["dolt_mode"] != "" {
			t.Errorf("dolt_mode = %v, want empty: a registered backend has no Dolt mode", body["dolt_mode"])
		}
		if body["database"] != "" {
			t.Errorf("database = %v, want empty: bd cannot know which database a registered backend opened", body["database"])
		}
	})

	t.Run("reads come from the registered store", func(t *testing.T) {
		body := getJSON(t, base+"/v0/beads/ready?limit=5")
		if !strings.Contains(string(mustMarshal(t, body)), seedID) {
			t.Errorf("GET /v0/beads/ready does not carry the seeded issue %q: %v", seedID, body)
		}
	})

	t.Run("a claim lands in the registered store", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, base+"/v0/beads/issues/"+seedID+":claim",
			strings.NewReader(`{"actor":"serve-e2e"}`))
		if err != nil {
			t.Fatalf("build claim request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			payload, _ := io.ReadAll(resp.Body)
			t.Fatalf("claim status = %d, want 200: %s", resp.StatusCode, payload)
		}
	})

	// Stop the server the way an operator does. The root command's signal
	// context is what serve rides, so canceling it here exercises the real
	// shutdown path including PersistentPostRunE.
	if rootCancel == nil {
		t.Fatal("the root command published no cancel function; nothing here stopped the server")
	}
	rootCancel()
	if err := <-done; err != nil {
		t.Fatalf("bd serve returned %v, want a clean shutdown", err)
	}

	// One open for the whole process, and it is the root command's. Serving
	// from the store bd already opened is the entire point of this arm; a
	// second handle here would be an unclosed leak with no owner.
	if n := opens.Load(); n != 1 {
		t.Errorf("the registered backend was opened %d times, want 1: bd serve created a store of its own", n)
	}

	// The root command owns the store's whole lifecycle: PersistentPostRunE
	// closed it after the server drained. A store still open would still hold
	// the embedded workspace's exclusive lock, so this reopen is the assertion.
	if store != nil {
		t.Error("PersistentPostRunE left the registered backend's store open")
	}
	reopened, err := embeddeddolt.Open(t.Context(), beadsDir, serveRegisteredDatabase, "main")
	if err != nil {
		t.Fatalf("reopen the workspace after shutdown: %v", err)
	}
	defer reopened.Close()

	claimed, err := reopened.GetIssue(t.Context(), seedID)
	if err != nil {
		t.Fatalf("read the claimed issue back: %v", err)
	}
	if claimed.Status != types.StatusInProgress {
		t.Errorf("status = %q, want %q: the HTTP claim did not land in the registered store", claimed.Status, types.StatusInProgress)
	}
	if claimed.Assignee != "serve-e2e" {
		t.Errorf("assignee = %q, want serve-e2e", claimed.Assignee)
	}

	// The contract this server publishes: a CLI claim runs on_update, an HTTP
	// claim does not. Only the peel beneath the hook decorator makes that true.
	if _, err := os.Stat(hookMarker); err == nil {
		t.Error("an HTTP claim ran this workspace's on_update hook; bd serve documents that hooks do not fire")
	} else if !os.IsNotExist(err) {
		t.Errorf("stat hook marker: %v", err)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}
