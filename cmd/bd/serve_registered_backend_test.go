//go:build cgo

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/backends"
)

// The registered-backend serve path end to end, in-process.
//
// In-process because it has to be: registration is init-time Go wiring and OSS
// registers no alternate backend, so a spawned `bd` — the shape every other
// serve integration test takes — has nothing to register and could not
// reproduce this case at all. bd bootstrap's registered-backend test reaches
// the same conclusion for the same reason.
//
// The store behind the registered NAME is a real embedded Dolt store. Embedded
// Dolt is refused as a WORKSPACE (serveDatabaseSource), and that refusal is
// about its commit protocol in production, not about wiring: behind a
// registered name it is a real store with real SQL, real claim arbitration and
// the real decorator chain, which is exactly the double this path needs.

// TestServeRefusesStrictReadonlyOnARegisteredBackend drives the strict-readonly
// refusal in the workspace where it was measured, and where the alternative was
// worst.
//
// Under `--readonly` the root command opens this workspace through
// backend.OpenReadOnly, and serve takes its claimer off that store. The server
// bound anyway, kept advertising `issues.claim` on GET /v0/beads/context — the
// capability set comes off the route table and cannot see a CLI flag — and
// answered every claim with an opaque 500, leaving the issue open and
// unassigned. That is a server lying about what it can do, which is worse than
// no server.
//
// The workspace here would serve: the same registration and the same store
// answer reads over HTTP when the flag is absent
// (TestServeAnswersFromTheStoreTheRootCommandOpened). What stops it is the
// refusal, not a workspace that could not have answered anyway.
// serveRefusalBudget is how long a refusal is given to be a refusal. Only a
// failing run ever waits it out.
const serveRefusalBudget = 30 * time.Second

func TestServeRefusesStrictReadonlyOnARegisteredBackend(t *testing.T) {
	const name = "serve-readonly-registered"
	backends.Register(name, backends.Backend{
		// Open is required by the registry and never reached: strict readonly
		// is what sends the root command down OpenReadOnly, and that is the
		// posture under test.
		Open: func(context.Context, string) (storage.DoltStorage, error) {
			return &serveIdentityDoltStore{serveIdentityStore: &serveIdentityStore{id: "read-write"}}, nil
		},
		OpenReadOnly: func(context.Context, string) (storage.DoltStorage, error) {
			return &serveIdentityDoltStore{serveIdentityStore: &serveIdentityStore{id: "read-only"}}, nil
		},
		WorkspaceIsBeadsDir: true,
	})
	t.Cleanup(func() { backends.Deregister(name) })

	dir := t.TempDir()
	initGitRepoAt(t, dir)
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := (&configfile.Config{Backend: name}).Save(beadsDir); err != nil {
		t.Fatalf("save metadata.json: %v", err)
	}

	useStorageModeGlobals(t)
	restoreServeGlobals(t)
	t.Chdir(dir)
	t.Setenv("BEADS_DIR", beadsDir)
	beads.ResetCaches()
	t.Cleanup(beads.ResetCaches)

	origReadonly := readonlyMode
	readonlyMode = true
	t.Cleanup(func() { readonlyMode = origReadonly })

	// What PersistentPreRunE leaves behind under --readonly: the read-only open
	// of this workspace, which is the store serve would have taken a claimer
	// off.
	backend, ok := backends.Lookup(name)
	if !ok {
		t.Fatalf("backend %q is not registered", name)
	}
	opened, err := backend.OpenReadOnly(t.Context(), beadsDir)
	if err != nil {
		t.Fatalf("read-only open: %v", err)
	}
	store = opened

	serveAddr, serveAllowNonLoopback = "127.0.0.1:0", false
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	setRootContext(ctx, cancel)

	// runServe is run on its own goroutine and bounded, because the failure
	// this test is here to catch does not return: a serve that binds blocks
	// until the root context is canceled, and an unbounded wait would turn the
	// regression into a package-wide timeout instead of one named failure.
	lines, stopCapture := captureStdoutLines(t)
	var (
		runErr error
		bound  bool
	)
	done := make(chan error, 1)
	stderr := captureBootstrapStderr(t, func() {
		go func() { done <- runServe() }()
		select {
		case runErr = <-done:
		case <-time.After(serveRefusalBudget):
			bound = true
			cancel()
			runErr = <-done
		}
	})
	stopCapture()

	if bound {
		t.Fatalf("bd --readonly serve bound a server over a read-only store and had to be stopped\nstderr:\n%s", stderr)
	}
	if runErr == nil {
		t.Fatalf("bd --readonly serve returned no error\nstderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--readonly") {
		t.Errorf("the refusal does not name the flag that caused it: %q", stderr)
	}
	for line := range lines {
		if strings.Contains(line, "listening on") {
			t.Errorf("bd serve bound before refusing: %q", line)
		}
	}
}
