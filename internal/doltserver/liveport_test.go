package doltserver

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/testtier"
)

// startLiveListener opens a TCP listener on an OS-assigned localhost port and
// keeps it open for the rest of the test. The listener is a plain
// net.Listener on purpose: liveness is a TCP-accept question, so no Dolt
// server or MySQL handshake is needed (be-60t).
func startLiveListener(t *testing.T) (net.Listener, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on a free localhost port: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln, ln.Addr().(*net.TCPAddr).Port
}

// closedPort returns a localhost port that nothing is listening on: it binds
// an OS-assigned port and releases it, retrying a bounded number of times in
// case another process claims the port in between.
func closedPort(t *testing.T) int {
	t.Helper()
	for range 5 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen on a free localhost port: %v", err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		if !portAcceptsConnections("127.0.0.1", port) {
			return port
		}
	}
	t.Fatalf("could not find a closed localhost port in 5 attempts")
	return 0
}

// isolateDoltConfig clears the ambient Dolt environment and starts the
// package-global config from an empty BEADS_DIR under a temporary HOME, so
// neither the developer's ~/.config/bd/config.yaml nor a dolt.auto-start /
// BEADS_DOLT_AUTO_START from the surrounding shell can change what these
// tests resolve.
func isolateDoltConfig(t *testing.T) {
	t.Helper()
	clearAmbientDoltEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BEADS_DOLT_AUTO_START", "")
	t.Setenv("BEADS_DIR", t.TempDir())
	config.ResetForTesting()
	if err := config.Initialize(); err != nil {
		t.Fatalf("config.Initialize: %v", err)
	}
	t.Cleanup(config.ResetForTesting)
}

// writeBeadsConfigPort writes dolt.port to the BEADS_DIR config.yaml and
// re-initializes the package config so DefaultConfig reads it — the same
// layering the precedence tests in portsources_test.go use.
func writeBeadsConfigPort(t *testing.T, port int) {
	t.Helper()
	dir := os.Getenv("BEADS_DIR")
	if dir == "" {
		t.Fatal("BEADS_DIR is not set; isolateDoltConfig sets it")
	}
	body := fmt.Sprintf("dolt.port: %d\n", port)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("writing BEADS_DIR config.yaml: %v", err)
	}
	config.ResetForTesting()
	if err := config.Initialize(); err != nil {
		t.Fatalf("config.Initialize: %v", err)
	}
}

// TestDefaultConfigDeadPortFileYieldsToLiveConfiguredPort is be-60t's first
// acceptance box: after a reboot the port file still names the ephemeral port
// of a server that is gone, while dolt.port in config.yaml names the live
// server. The dead bookkeeping entry must no longer win, and the live
// configured port (with its authoritative source) must be what bd resolves.
func TestDefaultConfigDeadPortFileYieldsToLiveConfiguredPort(t *testing.T) {
	isolateDoltConfig(t)

	beadsDir := t.TempDir()
	_, livePort := startLiveListener(t)
	deadPort := closedPort(t)
	if err := writePortFile(beadsDir, deadPort); err != nil {
		t.Fatalf("writePortFile: %v", err)
	}

	// Control: with no live configured port the dead entry keeps its place —
	// the chain has nothing better to offer, and auto-start may reuse it.
	if got := DefaultConfig(beadsDir).Port; got != deadPort {
		t.Fatalf("no configured port: port = %d, want the port file's %d", got, deadPort)
	}

	writeBeadsConfigPort(t, livePort)

	cfg := DefaultConfig(beadsDir)
	if cfg.Port != livePort {
		t.Fatalf("dead port file %d + live dolt.port %d: port = %d, want the live configured port", deadPort, livePort, cfg.Port)
	}
	if cfg.PortSource != PortSourceConfigYaml {
		t.Fatalf("port source = %q, want %q (the source that supplied the live port)", cfg.PortSource, PortSourceConfigYaml)
	}
	if !cfg.PortSource.IsAuthoritative() {
		t.Fatalf("source %q must stay authoritative", cfg.PortSource)
	}
}

// TestEnsureRunningDetailedAttachesToLiveConfiguredPort is be-60t's second
// acceptance box: when a configured source names a port with a live listener,
// EnsureRunningDetailed returns that port as an adopted server and starts
// nothing. Both configured shapes are covered — metadata.json dolt_server_port
// (external server mode, the reported workspace) and dolt.port in config.yaml
// (owned mode).
func TestEnsureRunningDetailedAttachesToLiveConfiguredPort(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(t *testing.T, beadsDir string, port int)
	}{
		{
			name: "metadata.json dolt_server_port",
			configure: func(t *testing.T, beadsDir string, port int) {
				writeMetadataPort(t, beadsDir, port)
			},
		},
		{
			name: "config.yaml dolt.port",
			configure: func(t *testing.T, _ string, port int) {
				writeBeadsConfigPort(t, port)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateDoltConfig(t)
			// If the attach path were missing, EnsureRunningDetailed would
			// fall through to Start(); the unit tier refuses to spawn a Dolt
			// server, so the test goes red instead of leaking a real process.
			t.Setenv(testtier.EnvVar, "unit")

			beadsDir := t.TempDir()
			_, livePort := startLiveListener(t)
			tc.configure(t, beadsDir, livePort)

			port, startedByUs, err := EnsureRunningDetailed(beadsDir)
			if err != nil {
				t.Fatalf("EnsureRunningDetailed must adopt the live configured server, got error: %v", err)
			}
			if port != livePort {
				t.Fatalf("port = %d, want the live configured port %d", port, livePort)
			}
			if startedByUs {
				t.Fatalf("startedByUs = true, want false (the server was already listening)")
			}
		})
	}
}

// TestEnsureRunningDetailedAutoStartsWithNoConfiguredPort pins the other half
// of be-60t's second acceptance box: zero-config workspaces keep today's
// auto-start. With no configured port the call must still reach Start()
// rather than erroring out because nothing is listening yet.
func TestEnsureRunningDetailedAutoStartsWithNoConfiguredPort(t *testing.T) {
	isolateDoltConfig(t)
	t.Setenv(testtier.EnvVar, "unit")

	beadsDir := t.TempDir()
	if got := DefaultConfig(beadsDir).Port; got != 0 {
		t.Fatalf("precondition: resolved port = %d, want 0 (nothing configured)", got)
	}

	_, _, err := EnsureRunningDetailed(beadsDir)
	if !errors.Is(err, testtier.ErrUnitTier) {
		t.Fatalf("EnsureRunningDetailed with no configured port = %v, want the auto-start path reaching Start() (refused in the unit tier)", err)
	}
}
