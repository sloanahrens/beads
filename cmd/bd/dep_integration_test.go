//go:build cgo && integration

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
)

func TestDependencySuite(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)
	ctx := context.Background()

	t.Run("DepAdd", func(t *testing.T) {
		// Create test issues
		issues := []*types.Issue{
			{
				ID:        "test-1",
				Title:     "Task 1",
				Status:    types.StatusOpen,
				Priority:  1,
				IssueType: types.TypeTask,
				CreatedAt: time.Now(),
			},
			{
				ID:        "test-2",
				Title:     "Task 2",
				Status:    types.StatusOpen,
				Priority:  1,
				IssueType: types.TypeTask,
				CreatedAt: time.Now(),
			},
		}

		for _, issue := range issues {
			if err := s.CreateIssue(ctx, issue, "test"); err != nil {
				t.Fatal(err)
			}
		}

		// Add dependency
		dep := &types.Dependency{
			IssueID:     "test-1",
			DependsOnID: "test-2",
			Type:        types.DepBlocks,
			CreatedAt:   time.Now(),
		}

		if err := s.AddDependency(ctx, dep, "test"); err != nil {
			t.Fatalf("AddDependency failed: %v", err)
		}

		// Verify dependency was added
		deps, err := s.GetDependencies(ctx, "test-1")
		if err != nil {
			t.Fatalf("GetDependencies failed: %v", err)
		}

		if len(deps) != 1 {
			t.Fatalf("Expected 1 dependency, got %d", len(deps))
		}

		if deps[0].ID != "test-2" {
			t.Errorf("Expected dependency on test-2, got %s", deps[0].ID)
		}
	})

	t.Run("DepTypes", func(t *testing.T) {
		// Create test issues
		for i := 1; i <= 4; i++ {
			issue := &types.Issue{
				ID:        fmt.Sprintf("test-types-%d", i),
				Title:     fmt.Sprintf("Task %d", i),
				Status:    types.StatusOpen,
				Priority:  1,
				IssueType: types.TypeTask,
				CreatedAt: time.Now(),
			}
			if err := s.CreateIssue(ctx, issue, "test"); err != nil {
				t.Fatal(err)
			}
		}

		// Test different dependency types (without creating cycles)
		depTypes := []struct {
			depType types.DependencyType
			from    string
			to      string
		}{
			{types.DepBlocks, "test-types-2", "test-types-1"},
			{types.DepRelated, "test-types-3", "test-types-1"},
			{types.DepParentChild, "test-types-4", "test-types-1"},
			{types.DepDiscoveredFrom, "test-types-3", "test-types-2"},
		}

		for _, dt := range depTypes {
			dep := &types.Dependency{
				IssueID:     dt.from,
				DependsOnID: dt.to,
				Type:        dt.depType,
				CreatedAt:   time.Now(),
			}

			if err := s.AddDependency(ctx, dep, "test"); err != nil {
				t.Fatalf("AddDependency failed for type %s: %v", dt.depType, err)
			}
		}
	})

	t.Run("DepCycleDetection", func(t *testing.T) {
		// Create test issues
		for i := 1; i <= 3; i++ {
			issue := &types.Issue{
				ID:        fmt.Sprintf("test-cycle-%d", i),
				Title:     fmt.Sprintf("Task %d", i),
				Status:    types.StatusOpen,
				Priority:  1,
				IssueType: types.TypeTask,
				CreatedAt: time.Now(),
			}
			if err := s.CreateIssue(ctx, issue, "test"); err != nil {
				t.Fatal(err)
			}
		}

		// Create a cycle: test-cycle-1 -> test-cycle-2 -> test-cycle-3 -> test-cycle-1
		// Add first two deps successfully
		deps := []struct {
			from string
			to   string
		}{
			{"test-cycle-1", "test-cycle-2"},
			{"test-cycle-2", "test-cycle-3"},
		}

		for _, d := range deps {
			dep := &types.Dependency{
				IssueID:     d.from,
				DependsOnID: d.to,
				Type:        types.DepBlocks,
				CreatedAt:   time.Now(),
			}
			if err := s.AddDependency(ctx, dep, "test"); err != nil {
				t.Fatalf("AddDependency failed: %v", err)
			}
		}

		// Try to add the third dep which would create a cycle - should fail
		cycleDep := &types.Dependency{
			IssueID:     "test-cycle-3",
			DependsOnID: "test-cycle-1",
			Type:        types.DepBlocks,
			CreatedAt:   time.Now(),
		}
		if err := s.AddDependency(ctx, cycleDep, "test"); err == nil {
			t.Fatal("Expected AddDependency to fail when creating cycle, but it succeeded")
		}

		// Since cycle detection prevented the cycle, DetectCycles should find no cycles
		cycles, err := s.DetectCycles(ctx)
		if err != nil {
			t.Fatalf("DetectCycles failed: %v", err)
		}

		if len(cycles) != 0 {
			t.Error("Expected no cycles since cycle was prevented")
		}
	})

	t.Run("DepRemove", func(t *testing.T) {
		// Create test issues
		issues := []*types.Issue{
			{
				ID:        "test-remove-1",
				Title:     "Task 1",
				Status:    types.StatusOpen,
				Priority:  1,
				IssueType: types.TypeTask,
				CreatedAt: time.Now(),
			},
			{
				ID:        "test-remove-2",
				Title:     "Task 2",
				Status:    types.StatusOpen,
				Priority:  1,
				IssueType: types.TypeTask,
				CreatedAt: time.Now(),
			},
		}

		for _, issue := range issues {
			if err := s.CreateIssue(ctx, issue, "test"); err != nil {
				t.Fatal(err)
			}
		}

		// Add dependency
		dep := &types.Dependency{
			IssueID:     "test-remove-1",
			DependsOnID: "test-remove-2",
			Type:        types.DepBlocks,
			CreatedAt:   time.Now(),
		}

		if err := s.AddDependency(ctx, dep, "test"); err != nil {
			t.Fatal(err)
		}

		// Remove dependency
		if err := s.RemoveDependency(ctx, "test-remove-1", "test-remove-2", "test"); err != nil {
			t.Fatalf("RemoveDependency failed: %v", err)
		}

		// Verify dependency was removed
		deps, err := s.GetDependencies(ctx, "test-remove-1")
		if err != nil {
			t.Fatalf("GetDependencies failed: %v", err)
		}

		if len(deps) != 0 {
			t.Errorf("Expected 0 dependencies after removal, got %d", len(deps))
		}
	})

	// Merged from TestDepBlocksFlagFunctionality — tests --blocks flag semantics
	t.Run("BlocksFlagFunctionality", func(t *testing.T) {
		issues := []*types.Issue{
			{ID: "test-blocks-1", Title: "Blocker Issue", Status: types.StatusOpen, Priority: 1, IssueType: types.TypeTask, CreatedAt: time.Now()},
			{ID: "test-blocks-2", Title: "Blocked Issue", Status: types.StatusOpen, Priority: 1, IssueType: types.TypeTask, CreatedAt: time.Now()},
		}
		for _, issue := range issues {
			if err := s.CreateIssue(ctx, issue, "test"); err != nil {
				t.Fatal(err)
			}
		}

		// "blocker --blocks blocked" means blocked depends on blocker
		dep := &types.Dependency{
			IssueID:     "test-blocks-2",
			DependsOnID: "test-blocks-1",
			Type:        types.DepBlocks,
			CreatedAt:   time.Now(),
		}
		if err := s.AddDependency(ctx, dep, "test"); err != nil {
			t.Fatalf("AddDependency failed: %v", err)
		}

		deps, err := s.GetDependencies(ctx, "test-blocks-2")
		if err != nil {
			t.Fatalf("GetDependencies failed: %v", err)
		}
		if len(deps) != 1 {
			t.Fatalf("Expected 1 dependency, got %d", len(deps))
		}
		if deps[0].ID != "test-blocks-1" {
			t.Errorf("Expected blocked issue to depend on test-blocks-1, got %s", deps[0].ID)
		}

		dependents, err := s.GetDependents(ctx, "test-blocks-1")
		if err != nil {
			t.Fatalf("GetDependents failed: %v", err)
		}
		if len(dependents) != 1 {
			t.Fatalf("Expected 1 dependent, got %d", len(dependents))
		}
		if dependents[0].ID != "test-blocks-2" {
			t.Errorf("Expected test-blocks-1 to have dependent test-blocks-2, got %s", dependents[0].ID)
		}
	})

	// Merged from TestDepAdd_FKError* and TestDepRemove_FKError — tests
	// that FK constraint violations produce user-friendly error messages.
	t.Run("FKError_InvalidFromID", func(t *testing.T) {
		validIssue := &types.Issue{
			ID: "test-fk-valid", Title: "Valid Issue", Status: types.StatusOpen,
			Priority: 1, IssueType: types.TypeTask, CreatedAt: time.Now(),
		}
		if err := s.CreateIssue(ctx, validIssue, "test"); err != nil {
			t.Fatal(err)
		}

		dep := &types.Dependency{
			IssueID: "test-nonexistent-from", DependsOnID: "test-fk-valid",
			Type: types.DepBlocks, CreatedAt: time.Now(),
		}
		err := s.AddDependency(ctx, dep, "test")
		if err == nil {
			t.Fatal("Expected error when adding dependency with invalid from ID")
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "FOREIGN KEY constraint failed") || strings.Contains(errMsg, "foreign key constraint failed") {
			t.Errorf("Error exposes raw FK constraint: %q", errMsg)
		}
		if !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "Not Found") {
			t.Errorf("Error message should indicate issue not found: %q", errMsg)
		}
	})

	t.Run("FKError_InvalidToID", func(t *testing.T) {
		dep := &types.Dependency{
			IssueID: "test-fk-valid", DependsOnID: "test-nonexistent-to",
			Type: types.DepBlocks, CreatedAt: time.Now(),
		}
		err := s.AddDependency(ctx, dep, "test")
		if err == nil {
			t.Fatal("Expected error when adding dependency with invalid to ID")
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "FOREIGN KEY constraint failed") || strings.Contains(errMsg, "foreign key constraint failed") {
			t.Errorf("Error exposes raw FK constraint: %q", errMsg)
		}
		if !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "Not Found") {
			t.Errorf("Error message should indicate dependency target not found: %q", errMsg)
		}
	})

	t.Run("FKError_BothInvalid", func(t *testing.T) {
		dep := &types.Dependency{
			IssueID: "test-nonexistent-1", DependsOnID: "test-nonexistent-2",
			Type: types.DepBlocks, CreatedAt: time.Now(),
		}
		err := s.AddDependency(ctx, dep, "test")
		if err == nil {
			t.Fatal("Expected error when adding dependency with both invalid IDs")
		}
		errMsg := err.Error()
		if strings.Contains(errMsg, "FOREIGN KEY constraint failed") || strings.Contains(errMsg, "foreign key constraint failed") {
			t.Errorf("Error exposes raw FK constraint: %q", errMsg)
		}
		if !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "Not Found") {
			t.Errorf("Error message should indicate issue not found: %q", errMsg)
		}
	})

	t.Run("FKError_JSONMode", func(t *testing.T) {
		dep := &types.Dependency{
			IssueID: "test-fk-valid", DependsOnID: "test-json-nonexistent",
			Type: types.DepBlocks, CreatedAt: time.Now(),
		}
		err := s.AddDependency(ctx, dep, "test")
		if err == nil {
			t.Fatal("Expected error when adding dependency with invalid ID")
		}
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			t.Errorf("Error exposes raw FK constraint (JSON mode): %q", err.Error())
		}
	})

	t.Run("FKError_DaemonMode", func(t *testing.T) {
		dep := &types.Dependency{
			IssueID: "test-fk-valid", DependsOnID: "test-daemon-nonexistent",
			Type: types.DepBlocks, CreatedAt: time.Now(),
		}
		err := s.AddDependency(ctx, dep, "test")
		if err == nil {
			t.Fatal("Expected error when adding dependency with invalid ID via daemon path")
		}
		errMsg := err.Error()
		daemonError := fmt.Sprintf("failed to add dependency: %v", err)
		if strings.Contains(daemonError, "FOREIGN KEY constraint failed") || strings.Contains(daemonError, "foreign key constraint failed") {
			t.Errorf("Daemon error exposes raw FK constraint: %q", daemonError)
		}
		if !strings.Contains(errMsg, "not found") {
			t.Errorf("Storage error should indicate not found: %q", errMsg)
		}
	})

	t.Run("FKError_RemoveNonexistent", func(t *testing.T) {
		// Create issues and dep for removal test
		issue1 := &types.Issue{ID: "test-fk-remove-1", Title: "Issue 1", Status: types.StatusOpen, Priority: 1, IssueType: types.TypeTask, CreatedAt: time.Now()}
		issue2 := &types.Issue{ID: "test-fk-remove-2", Title: "Issue 2", Status: types.StatusOpen, Priority: 1, IssueType: types.TypeTask, CreatedAt: time.Now()}
		if err := s.CreateIssue(ctx, issue1, "test"); err != nil {
			t.Fatal(err)
		}
		if err := s.CreateIssue(ctx, issue2, "test"); err != nil {
			t.Fatal(err)
		}
		dep := &types.Dependency{
			IssueID: "test-fk-remove-1", DependsOnID: "test-fk-remove-2",
			Type: types.DepBlocks, CreatedAt: time.Now(),
		}
		if err := s.AddDependency(ctx, dep, "test"); err != nil {
			t.Fatal(err)
		}

		// Try removing with non-existent IDs
		err := s.RemoveDependency(ctx, "test-nonexistent-1", "test-nonexistent-2", "test")
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "FOREIGN KEY constraint failed") || strings.Contains(errMsg, "foreign key constraint failed") {
				t.Errorf("Error exposes raw FK constraint: %q", errMsg)
			}
		}

		// Remove the real dep, then try removing again
		if err := s.RemoveDependency(ctx, "test-fk-remove-1", "test-fk-remove-2", "test"); err != nil {
			t.Fatalf("Failed to remove existing dependency: %v", err)
		}
		err = s.RemoveDependency(ctx, "test-fk-remove-1", "test-fk-remove-2", "test")
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "FOREIGN KEY") {
				t.Errorf("Error exposes raw FK constraint: %q", errMsg)
			}
		}
	})
}

// TestDepRoutedTargetOpensReadOnly is the regression guard for the dep/link
// target-resolution invariant: a cross-rig dependency target is resolved by ID
// only, so resolveIDWithRouting must open the routed foreign store read-only,
// while resolveIDForMutation (used for the mutated source issue) opens it
// writable. Opening a dep/link target writable re-exposes GH#3231 open-time
// mutations against a foreign project.
//
// NOTE: This test uses os.Chdir and cannot run in parallel with other tests.
func TestDepRoutedTargetOpensReadOnly(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	townBeadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("create town beads dir: %v", err)
	}
	rigBeadsDir := filepath.Join(tmpDir, "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatalf("create rig beads dir: %v", err)
	}

	townDBPath := filepath.Join(townBeadsDir, "dolt")
	townStore := newTestStoreIsolatedDB(t, townDBPath, "hq")

	rigDBPath := filepath.Join(rigBeadsDir, "dolt")
	rigStore := newTestStoreIsolatedDB(t, rigDBPath, "gt")
	if err := rigStore.CreateIssue(ctx, &types.Issue{
		ID:        "gt-target1",
		Title:     "Routed dep target",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}, "test"); err != nil {
		t.Fatalf("create rig issue: %v", err)
	}
	// Release the rig store before routing reopens it.
	rigStore.Close()

	routesPath := filepath.Join(townBeadsDir, "routes.jsonl")
	if err := os.WriteFile(routesPath, []byte(`{"prefix":"gt-","path":"rig"}`), 0644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	oldDbPath := dbPath
	dbPath = townDBPath
	t.Cleanup(func() { dbPath = oldDbPath })

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	// Target-only resolution (the dep/link target) must open the routed store
	// read-only.
	roID, roStore, roCleanup, err := resolveIDWithRouting(ctx, townStore, "gt-target1")
	if err != nil {
		t.Fatalf("resolveIDWithRouting (target) failed: %v", err)
	}
	if roID != "gt-target1" {
		t.Errorf("resolved target ID = %q, want gt-target1", roID)
	}
	roDolt, ok := roStore.(*dolt.DoltStore)
	if !ok {
		roCleanup()
		t.Fatalf("routed target store is %T, want *dolt.DoltStore", roStore)
	}
	if !roDolt.IsReadOnly() {
		roCleanup()
		t.Fatal("dep/link target must be resolved read-only, but routed store is writable (GH#3231)")
	}
	roCleanup()

	// Source resolution (the mutated issue's store) must open the routed store
	// writable so the dependency write commits on the target head (#4141).
	rwID, rwStore, rwCleanup, err := resolveIDForMutation(ctx, townStore, "gt-target1")
	if err != nil {
		t.Fatalf("resolveIDForMutation (source) failed: %v", err)
	}
	defer rwCleanup()
	if rwID != "gt-target1" {
		t.Errorf("resolved source ID = %q, want gt-target1", rwID)
	}
	rwDolt, ok := rwStore.(*dolt.DoltStore)
	if !ok {
		t.Fatalf("routed source store is %T, want *dolt.DoltStore", rwStore)
	}
	if rwDolt.IsReadOnly() {
		t.Fatal("source resolution must open the routed store writable, but it is read-only")
	}
}

// TestDepListCrossRigRouting tests that bd dep list resolves issues via routing
// when run from the town root for rig-level issues. This is the regression test
// for bd-ciouf: "bd dep list cross-rig routing broken from town root".
//
// NOTE: This test uses os.Chdir and cannot run in parallel with other tests.
func TestDepListCrossRigRouting(t *testing.T) {
	ctx := context.Background()

	// Create temp directory structure:
	// tmpDir/
	//   .beads/
	//     dolt/ (town database, prefix "hq")
	//     routes.jsonl (routing config)
	//   rig/
	//     .beads/
	//       dolt/ (rig database, prefix "gt")
	tmpDir := t.TempDir()

	// Create town .beads directory
	townBeadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(townBeadsDir, 0755); err != nil {
		t.Fatalf("Failed to create town beads dir: %v", err)
	}

	// Create rig .beads directory
	rigBeadsDir := filepath.Join(tmpDir, "rig", ".beads")
	if err := os.MkdirAll(rigBeadsDir, 0755); err != nil {
		t.Fatalf("Failed to create rig beads dir: %v", err)
	}

	// Initialize town database
	townDBPath := filepath.Join(townBeadsDir, "dolt")
	townStore := newTestStoreIsolatedDB(t, townDBPath, "hq")

	// Initialize rig database
	rigDBPath := filepath.Join(rigBeadsDir, "dolt")
	rigStore := newTestStoreIsolatedDB(t, rigDBPath, "gt")

	// Create test issues in rig database with a dependency
	parent := &types.Issue{
		ID:        "gt-parent1",
		Title:     "Parent Issue",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	child := &types.Issue{
		ID:        "gt-child1",
		Title:     "Child Issue",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := rigStore.CreateIssue(ctx, parent, "test"); err != nil {
		t.Fatalf("Failed to create parent issue: %v", err)
	}
	if err := rigStore.CreateIssue(ctx, child, "test"); err != nil {
		t.Fatalf("Failed to create child issue: %v", err)
	}

	// gt-child1 depends on gt-parent1 (blocks relationship)
	dep := &types.Dependency{
		IssueID:     "gt-child1",
		DependsOnID: "gt-parent1",
		Type:        types.DepBlocks,
	}
	if err := rigStore.AddDependency(ctx, dep, "test"); err != nil {
		t.Fatalf("Failed to add dependency: %v", err)
	}

	// Close rig store to release Dolt lock before routing opens it
	rigStore.Close()

	// Create routes.jsonl in town .beads directory
	routesContent := `{"prefix":"gt-","path":"rig"}`
	routesPath := filepath.Join(townBeadsDir, "routes.jsonl")
	if err := os.WriteFile(routesPath, []byte(routesContent), 0644); err != nil {
		t.Fatalf("Failed to write routes.jsonl: %v", err)
	}

	// Set up global state for routing to work
	oldDbPath := dbPath
	dbPath = townDBPath
	t.Cleanup(func() { dbPath = oldDbPath })

	// Change to tmpDir so routing can find town root via CWD
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	// Test 1: Verify routing resolution works for the rig issue
	result, err := resolveAndGetIssueWithRouting(ctx, townStore, "gt-child1")
	if err != nil {
		t.Fatalf("resolveAndGetIssueWithRouting failed: %v", err)
	}
	if result == nil || result.Issue == nil {
		t.Fatal("resolveAndGetIssueWithRouting returned nil")
	}
	defer result.Close()

	if !result.Routed {
		t.Error("Expected result.Routed to be true for cross-rig lookup")
	}
	if result.ResolvedID != "gt-child1" {
		t.Errorf("Expected resolved ID %q, got %q", "gt-child1", result.ResolvedID)
	}

	// Test 2: Verify dependencies can be queried from the routed store
	deps, err := result.Store.GetDependenciesWithMetadata(ctx, result.ResolvedID)
	if err != nil {
		t.Fatalf("GetDependenciesWithMetadata on routed store failed: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("Expected 1 dependency, got %d", len(deps))
	}
	if deps[0].ID != "gt-parent1" {
		t.Errorf("Expected dependency on gt-parent1, got %s", deps[0].ID)
	}

	// Test 3: Verify dependents (up direction) also work from routed store
	dependents, err := result.Store.GetDependentsWithMetadata(ctx, "gt-parent1")
	if err != nil {
		t.Fatalf("GetDependentsWithMetadata on routed store failed: %v", err)
	}
	if len(dependents) != 1 {
		t.Fatalf("Expected 1 dependent, got %d", len(dependents))
	}
	if dependents[0].ID != "gt-child1" {
		t.Errorf("Expected dependent gt-child1, got %s", dependents[0].ID)
	}

	t.Log("Successfully resolved cross-rig dependencies via routing")
}
