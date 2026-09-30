//go:build integration

// Integration tier (be-b23): starts a dolt sql-server and migrates a fresh
// schema, which the unit tier's BD_TEST_TIER tripwire refuses.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage/doltutil"
	"github.com/steveyegge/beads/internal/storage/schema"
	"github.com/steveyegge/beads/internal/testutil"
)

func TestSeedProductionShapeFullSmallIssueCountRealSchema(t *testing.T) {
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not installed, skipping real-schema smoke")
	}
	skipOldDoltForCurrentSchema(t)

	ctx := context.Background()
	baseDir := t.TempDir()
	dbName := "testdb"
	dbDir := filepath.Join(baseDir, dbName)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestCmd(t, dbDir, "dolt", "init", "--name", "test", "--email", "test@example.com")

	port, err := testutil.FindFreePort()
	if err != nil {
		t.Fatal(err)
	}
	serverCmd := exec.Command("dolt", "sql-server",
		"-H", "127.0.0.1",
		"-P", fmt.Sprintf("%d", port),
	)
	serverCmd.Dir = baseDir
	if err := serverCmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = serverCmd.Process.Kill()
		_ = serverCmd.Wait()
	})
	if !testutil.WaitForServer(port, 15*time.Second) {
		t.Fatal("dolt sql-server did not become ready")
	}

	dsn := doltutil.ServerDSN{Host: "127.0.0.1", Port: port, User: "root", Database: dbName, Timeout: 10 * time.Second}.String()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if _, err := schema.MigrateUp(ctx, db); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}

	cfg := config{SeedMode: "full", IssueCount: 50, DepCount: 50, Ops: 5, ChainDepth: 2}
	if err := seedProductionShape(ctx, db, cfg); err != nil {
		t.Fatal(err)
	}

	var depRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM dependencies").Scan(&depRows); err != nil {
		t.Fatal(err)
	}
	wantDeps := cfg.DepCount + cfg.Ops*cfg.ChainDepth
	if depRows != wantDeps {
		t.Fatalf("dependency rows = %d, want %d", depRows, wantDeps)
	}

	sourceID, targetID := dependencyEndpoints(0, cfg.IssueCount, 0, 200)
	if err := cycleCheckCurrentSQL(ctx, db, perfIssueID(1), sourceID, 0); err != nil {
		t.Fatalf("cycleCheckCurrentSQL: %v", err)
	}
	targets, err := fetchBlockingTargets(ctx, db, []string{sourceID})
	if err != nil {
		t.Fatalf("fetchBlockingTargets: %v", err)
	}
	if !slices.Contains(targets, targetID) {
		t.Fatalf("fetchBlockingTargets(%q) = %v, want %q", sourceID, targets, targetID)
	}
}

func skipOldDoltForCurrentSchema(t *testing.T) {
	t.Helper()
	output, err := exec.Command("dolt", "version").CombinedOutput()
	if err != nil {
		t.Skipf("dolt version unavailable, skipping real-schema smoke: %v", err)
	}
	if regexp.MustCompile(`\bdolt version 1\.`).Match(output) {
		t.Skipf("dolt 1.x cannot initialize the current migration set: %s", strings.TrimSpace(string(output)))
	}
}

func runTestCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed in %s: %v\nOutput: %s", name, args, dir, err, output)
	}
}
