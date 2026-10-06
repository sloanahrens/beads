//go:build cgo && integration

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/testutil"
	"github.com/steveyegge/beads/internal/types"
)

// TestCloseIssueSetsClosedAt verifies the dedicated close operation owns its lifecycle metadata.
func TestCloseIssueSetsClosedAt(t *testing.T) {
	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "test.db")

	testStore := newTestStoreWithPrefix(t, dbPath, "bd")

	ctx := context.Background()

	// Step 1: Create an open issue in the database
	openIssue := &types.Issue{
		ID:          "bd-transition-1",
		Title:       "Test transition",
		Description: "This will be closed",
		Status:      types.StatusOpen,
		Priority:    1,
		IssueType:   types.TypeBug,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		ClosedAt:    nil,
	}

	if err := testStore.CreateIssue(ctx, openIssue, "test"); err != nil {
		t.Fatalf("Failed to create open issue: %v", err)
	}

	// Step 2: Lifecycle transitions use the dedicated close operation.
	if err := testStore.CloseIssue(ctx, "bd-transition-1", "", "test", ""); err != nil {
		t.Fatalf("CloseIssue failed: %v", err)
	}

	// Step 3: Verify the issue is now closed with correct closed_at
	updated, err := testStore.GetIssue(ctx, "bd-transition-1")
	if err != nil {
		t.Fatalf("Failed to get updated issue: %v", err)
	}

	if updated.Status != types.StatusClosed {
		t.Errorf("Expected status to be closed, got %s", updated.Status)
	}

	if updated.ClosedAt == nil {
		t.Fatal("Expected closed_at to be set after transition to closed")
	}
}

// TestReopenIssueClearsClosedAt verifies the dedicated reopen operation clears lifecycle metadata.
func TestReopenIssueClearsClosedAt(t *testing.T) {
	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "test.db")

	testStore := newTestStoreWithPrefix(t, dbPath, "bd")

	ctx := context.Background()

	// Step 1: Create a closed issue in the database
	closedTime := time.Now()
	closedIssue := &types.Issue{
		ID:          "bd-transition-2",
		Title:       "Test reopening",
		Description: "This will be reopened",
		Status:      types.StatusClosed,
		Priority:    1,
		IssueType:   types.TypeBug,
		CreatedAt:   time.Now(),
		UpdatedAt:   closedTime,
		ClosedAt:    &closedTime,
	}

	if err := testStore.CreateIssue(ctx, closedIssue, "test"); err != nil {
		t.Fatalf("Failed to create closed issue: %v", err)
	}

	// Step 2: Lifecycle transitions use the dedicated reopen operation.
	if err := testStore.ReopenIssue(ctx, "bd-transition-2", "", "test"); err != nil {
		t.Fatalf("ReopenIssue failed: %v", err)
	}

	// Step 3: Verify the issue is now open with null closed_at
	updated, err := testStore.GetIssue(ctx, "bd-transition-2")
	if err != nil {
		t.Fatalf("Failed to get updated issue: %v", err)
	}

	if updated.Status != types.StatusOpen {
		t.Errorf("Expected status to be open, got %s", updated.Status)
	}

	if updated.ClosedAt != nil {
		t.Errorf("Expected closed_at to be nil after reopening, got %v", updated.ClosedAt)
	}
}

func TestListUsesRepoBeadsDirWhenDoltDataDirEscapesDotBeads(t *testing.T) {
	if testDoltServerPort == 0 {
		testutil.SkipOrFailUnavailable(t, "Dolt test server not available, skipping")
	}

	initConfigForTest(t)
	ensureCleanGlobalState(t)

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	beadsDir := filepath.Join(repoDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0o755); err != nil {
		t.Fatalf("mkdir beads dir: %v", err)
	}

	relativeDoltDir := "../external-dolt"
	externalDoltDir := filepath.Join(beadsDir, relativeDoltDir)
	if err := os.MkdirAll(filepath.Dir(externalDoltDir), 0o755); err != nil {
		t.Fatalf("mkdir external dolt parent: %v", err)
	}

	database := uniqueTestDBName(t)
	cfg := &configfile.Config{
		Backend:        configfile.BackendDolt,
		DoltMode:       configfile.DoltModeServer,
		DoltServerHost: "127.0.0.1",
		DoltServerPort: testDoltServerPort,
		DoltDatabase:   database,
		DoltDataDir:    relativeDoltDir,
	}
	if err := cfg.Save(beadsDir); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "dolt-server.port"), []byte(strconv.Itoa(testDoltServerPort)), 0o600); err != nil {
		t.Fatalf("write port file: %v", err)
	}

	ctx := context.Background()
	testStore, err := dolt.New(ctx, &dolt.Config{
		Path:            externalDoltDir,
		BeadsDir:        beadsDir,
		ServerHost:      "127.0.0.1",
		ServerPort:      testDoltServerPort,
		Database:        database,
		CreateIfMissing: true,
	})
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	defer func() {
		_ = testStore.Close()
		dropTestDatabase(database, testDoltServerPort)
	}()

	if err := testStore.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("set issue_prefix: %v", err)
	}
	if err := testStore.SetConfig(ctx, "types.custom", "molecule,gate,convoy,merge-request,slot,agent,role,rig,event,message"); err != nil {
		t.Fatalf("set types.custom: %v", err)
	}

	now := time.Now()
	issue := &types.Issue{
		ID:          "test-port-proof-1",
		Title:       "Port-proof issue",
		Description: "Verifies bd list uses the repo's .beads config even with external dolt data",
		Status:      types.StatusOpen,
		Priority:    1,
		IssueType:   types.TypeBug,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := testStore.CreateIssue(ctx, issue, "test-user"); err != nil {
		t.Fatalf("create issue: %v", err)
	}

	t.Setenv("BEADS_DIR", beadsDir)
	t.Setenv("BEADS_DB", "")
	t.Setenv("BEADS_DOLT_SERVER_PORT", "")
	t.Setenv("BEADS_DOLT_PORT", "")

	// Shared once-per-process binary (honors BEADS_TEST_BD_BINARY) instead of
	// a per-test go build — the in-test link steps dominated cmd/bd's wall
	// clock (wy-4mtr0).
	binPath := buildBDForInitTests(t)

	listCmd := exec.Command(binPath, "list", "--json")
	listCmd.Dir = repoDir
	listCmd.Env = append(os.Environ(),
		"BEADS_TEST_MODE=1",
		"BEADS_DIR="+beadsDir,
		"BEADS_DB=",
		"BEADS_DOLT_SERVER_PORT=",
		"BEADS_DOLT_PORT=",
	)
	output, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd list failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "Port-proof issue") {
		t.Fatalf("expected list output to include created issue\n%s", output)
	}
}
