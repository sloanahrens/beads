//go:build cgo && integration

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/storage/doltutil"
)

func TestConfigValidateReadOnlyIsHermetic(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("BEADS_DOLT_PORT"))
	if err != nil || port <= 0 {
		t.Skip("shared Dolt test server is unavailable")
	}
	circuitDir := os.Getenv("BEADS_TEST_CIRCUIT_DIR")
	if !filepath.IsAbs(circuitDir) {
		t.Fatalf("suite circuit directory is not isolated: %q", circuitDir)
	}

	repoDir := t.TempDir()
	beadsDir := filepath.Join(repoDir, ".beads")
	doltPath := filepath.Join(beadsDir, "dolt")
	if err := os.MkdirAll(doltPath, 0o755); err != nil {
		t.Fatalf("create isolated Dolt path: %v", err)
	}
	database := fmt.Sprintf("readonly_canary_%d_%d", os.Getpid(), time.Now().UnixNano())
	cfg := &configfile.Config{
		Database:       "dolt",
		Backend:        configfile.BackendDolt,
		DoltMode:       configfile.DoltModeServer,
		DoltDatabase:   database,
		DoltServerHost: "127.0.0.1",
		DoltServerPort: port,
	}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("save isolated metadata: %v", err)
	}
	store, err := dolt.New(context.Background(), &dolt.Config{
		Path:            doltPath,
		BeadsDir:        beadsDir,
		ServerHost:      "127.0.0.1",
		ServerPort:      port,
		Database:        database,
		CreateIfMissing: true,
	})
	if err != nil {
		t.Fatalf("create isolated database: %v", err)
	}
	if err := store.SetConfig(context.Background(), "issue_prefix", "readonly"); err != nil {
		_ = store.Close()
		t.Fatalf("seed isolated database config: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close isolated database: %v", err)
	}
	t.Cleanup(func() {
		db, openErr := sql.Open("mysql", doltutil.ServerDSN{Host: "127.0.0.1", Port: port, User: "root"}.String())
		if openErr == nil {
			defer db.Close()
			_, _ = db.ExecContext(context.Background(), fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", database)) //nolint:gosec // generated test name
		}
	})
	// federation.remote is required for `bd config validate` to pass since
	// the JSONL-removal change; without it the canary fails on validation
	// rather than exercising the hermeticity contract.
	if err := os.WriteFile(filepath.Join(beadsDir, "config.yaml"), []byte("issue-prefix: readonly\ndolt.auto-start: false\nfederation:\n  remote: https://github.com/example/beads-remote\n"), 0o644); err != nil {
		t.Fatalf("write isolated config: %v", err)
	}

	beforeBeads := snapshotReadonlyTree(t, beadsDir)
	beforeCircuit := snapshotReadonlyTree(t, circuitDir)
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "xdg"), 0o755); err != nil {
		t.Fatalf("create isolated XDG home: %v", err)
	}
	// The canary must execute the current worktree source, never a caller-provided
	// prebuilt binary that may predate this candidate.
	t.Setenv("BEADS_TEST_BD_BINARY", "")
	bd := buildBDForTest(t)
	cmd := exec.Command(bd, "config", "validate", "--readonly")
	cmd.Dir = repoDir
	cmd.Env = readonlyCanaryEnv(home, beadsDir, circuitDir, port)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd config validate --readonly: %v\n%s", err, output)
	}

	afterBeads := snapshotReadonlyTree(t, beadsDir)
	if !reflect.DeepEqual(afterBeads, beforeBeads) {
		t.Fatalf("target .beads changed\nbefore: %#v\nafter:  %#v\noutput: %s", beforeBeads, afterBeads, output)
	}
	afterCircuit := snapshotReadonlyTree(t, circuitDir)
	if !reflect.DeepEqual(afterCircuit, beforeCircuit) {
		t.Fatalf("test circuit state changed\nbefore: %#v\nafter:  %#v", beforeCircuit, afterCircuit)
	}
	for _, artifact := range []string{".local_version", "dolt-server.port", "dolt-server.pid", "dolt-server.log"} {
		if _, err := os.Lstat(filepath.Join(beadsDir, artifact)); !os.IsNotExist(err) {
			t.Fatalf("strict readonly created server/version artifact %s (stat error: %v)", artifact, err)
		}
	}
}
