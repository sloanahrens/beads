//go:build cgo && integration

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/formula"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/testutil"
	"github.com/steveyegge/beads/internal/types"
)

func TestBondProtoProto(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	store, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer store.Close()
	if err := store.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create two protos
	protoA := &types.Issue{
		Title:     "Proto A",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	protoB := &types.Issue{
		Title:     "Proto B",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}

	if err := store.CreateIssue(ctx, protoA, "test"); err != nil {
		t.Fatalf("Failed to create protoA: %v", err)
	}
	if err := store.CreateIssue(ctx, protoB, "test"); err != nil {
		t.Fatalf("Failed to create protoB: %v", err)
	}

	// Test sequential bond
	result, err := bondProtoProto(ctx, store, protoA, protoB, types.BondTypeSequential, "", "test")
	if err != nil {
		t.Fatalf("bondProtoProto failed: %v", err)
	}

	if result.ResultType != "compound_proto" {
		t.Errorf("ResultType = %q, want %q", result.ResultType, "compound_proto")
	}
	if result.BondType != types.BondTypeSequential {
		t.Errorf("BondType = %q, want %q", result.BondType, types.BondTypeSequential)
	}

	// Verify compound was created
	compound, err := store.GetIssue(ctx, result.ResultID)
	if err != nil {
		t.Fatalf("Failed to get compound: %v", err)
	}
	if !isProto(compound) {
		t.Errorf("Compound should be a proto (have template label), got labels: %v", compound.Labels)
	}
	if compound.Priority != 1 {
		t.Errorf("Compound priority = %d, want %d (min of 1,2)", compound.Priority, 1)
	}

	// Verify dependencies exist (protoA depends on compound via parent-child)
	deps, err := store.GetDependenciesWithMetadata(ctx, protoA.ID)
	if err != nil {
		t.Fatalf("Failed to get deps for protoA: %v", err)
	}
	foundParentChild := false
	for _, dep := range deps {
		if dep.ID == compound.ID && dep.DependencyType == types.DepParentChild {
			foundParentChild = true
		}
	}
	if !foundParentChild {
		t.Error("Expected parent-child dependency from protoA to compound")
	}
}

func TestBondProtoMol(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	store, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer store.Close()
	if err := store.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a proto with a child issue
	proto := &types.Issue{
		Title:     "Proto: {{name}}",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	if err := store.CreateIssue(ctx, proto, "test"); err != nil {
		t.Fatalf("Failed to create proto: %v", err)
	}

	protoChild := &types.Issue{
		Title:     "Step 1 for {{name}}",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
		Labels:    []string{MoleculeLabel},
	}
	if err := store.CreateIssue(ctx, protoChild, "test"); err != nil {
		t.Fatalf("Failed to create proto child: %v", err)
	}

	// Add parent-child dependency
	if err := store.AddDependency(ctx, &types.Dependency{
		IssueID:     protoChild.ID,
		DependsOnID: proto.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add dependency: %v", err)
	}

	// Create a molecule (existing epic)
	mol := &types.Issue{
		Title:     "Existing Work",
		Status:    types.StatusInProgress,
		Priority:  1,
		IssueType: types.TypeEpic,
	}
	if err := store.CreateIssue(ctx, mol, "test"); err != nil {
		t.Fatalf("Failed to create molecule: %v", err)
	}

	// Bond proto to molecule
	vars := map[string]string{"name": "auth-feature"}
	result, err := bondProtoMol(ctx, store, proto, mol, types.BondTypeSequential, vars, "", "test", false, false)
	if err != nil {
		t.Fatalf("bondProtoMol failed: %v", err)
	}

	if result.ResultType != "compound_molecule" {
		t.Errorf("ResultType = %q, want %q", result.ResultType, "compound_molecule")
	}
	if result.Spawned != 2 {
		t.Errorf("Spawned = %d, want 2", result.Spawned)
	}
	if result.ResultID != mol.ID {
		t.Errorf("ResultID = %q, want %q (original molecule)", result.ResultID, mol.ID)
	}
}

func TestBondMolMol(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	store, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer store.Close()
	if err := store.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create two molecules
	molA := &types.Issue{
		Title:     "Molecule A",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
	}
	molB := &types.Issue{
		Title:     "Molecule B",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeEpic,
	}

	if err := store.CreateIssue(ctx, molA, "test"); err != nil {
		t.Fatalf("Failed to create molA: %v", err)
	}
	if err := store.CreateIssue(ctx, molB, "test"); err != nil {
		t.Fatalf("Failed to create molB: %v", err)
	}

	// Test sequential bond
	result, err := bondMolMol(ctx, store, molA, molB, types.BondTypeSequential, "test")
	if err != nil {
		t.Fatalf("bondMolMol failed: %v", err)
	}

	if result.ResultType != "compound_molecule" {
		t.Errorf("ResultType = %q, want %q", result.ResultType, "compound_molecule")
	}
	if result.ResultID != molA.ID {
		t.Errorf("ResultID = %q, want %q", result.ResultID, molA.ID)
	}

	// Verify dependency: B blocks on A
	deps, err := store.GetDependenciesWithMetadata(ctx, molB.ID)
	if err != nil {
		t.Fatalf("Failed to get deps for molB: %v", err)
	}
	foundBlocks := false
	for _, dep := range deps {
		if dep.ID == molA.ID && dep.DependencyType == types.DepBlocks {
			foundBlocks = true
		}
	}
	if !foundBlocks {
		t.Error("Expected blocks dependency from molB to molA for sequential bond")
	}

	// Test parallel bond (create new molecules)
	molC := &types.Issue{
		Title:     "Molecule C",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
	}
	molD := &types.Issue{
		Title:     "Molecule D",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
	}
	if err := store.CreateIssue(ctx, molC, "test"); err != nil {
		t.Fatalf("Failed to create molC: %v", err)
	}
	if err := store.CreateIssue(ctx, molD, "test"); err != nil {
		t.Fatalf("Failed to create molD: %v", err)
	}

	result2, err := bondMolMol(ctx, store, molC, molD, types.BondTypeParallel, "test")
	if err != nil {
		t.Fatalf("bondMolMol parallel failed: %v", err)
	}

	// Verify parent-child dependency for parallel
	deps2, err := store.GetDependenciesWithMetadata(ctx, molD.ID)
	if err != nil {
		t.Fatalf("Failed to get deps for molD: %v", err)
	}
	foundParentChild := false
	for _, dep := range deps2 {
		if dep.ID == molC.ID && dep.DependencyType == types.DepParentChild {
			foundParentChild = true
		}
	}
	if !foundParentChild {
		t.Errorf("Expected parent-child dependency for parallel bond, result: %+v", result2)
	}
}

func TestSquashMolecule(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a molecule (root issue)
	root := &types.Issue{
		Title:     "Test Molecule",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	// Create ephemeral children
	child1 := &types.Issue{
		Title:       "Step 1: Design",
		Description: "Design the architecture",
		Status:      types.StatusClosed,
		Priority:    2,
		IssueType:   types.TypeTask,
		Ephemeral:   true,
		CloseReason: "Completed design",
	}
	child2 := &types.Issue{
		Title:       "Step 2: Implement",
		Description: "Build the feature",
		Status:      types.StatusClosed,
		Priority:    2,
		IssueType:   types.TypeTask,
		Ephemeral:   true,
		CloseReason: "Code merged",
	}

	if err := s.CreateIssue(ctx, child1, "test"); err != nil {
		t.Fatalf("Failed to create child1: %v", err)
	}
	if err := s.CreateIssue(ctx, child2, "test"); err != nil {
		t.Fatalf("Failed to create child2: %v", err)
	}

	// Add parent-child dependencies
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     child1.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add child1 dependency: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     child2.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add child2 dependency: %v", err)
	}

	// Test squash with keep-children
	children := []*types.Issue{child1, child2}
	result, err := squashMolecule(ctx, s, root, children, true, "", "test")
	if err != nil {
		t.Fatalf("squashMolecule failed: %v", err)
	}

	if result.SquashedCount != 2 {
		t.Errorf("SquashedCount = %d, want 2", result.SquashedCount)
	}
	if result.DeletedCount != 0 {
		t.Errorf("DeletedCount = %d, want 0 (keep-children)", result.DeletedCount)
	}
	if !result.KeptChildren {
		t.Error("KeptChildren should be true")
	}

	// Verify digest was created
	digest, err := s.GetIssue(ctx, result.DigestID)
	if err != nil {
		t.Fatalf("Failed to get digest: %v", err)
	}
	if digest.Ephemeral {
		t.Error("Digest should NOT be ephemeral")
	}
	if digest.Status != types.StatusClosed {
		t.Errorf("Digest status = %v, want closed", digest.Status)
	}
	if !strings.Contains(digest.Description, "Step 1: Design") {
		t.Error("Digest should contain child titles")
	}
	if !strings.Contains(digest.Description, "Completed design") {
		t.Error("Digest should contain close reasons")
	}

	// Children should still exist
	c1, err := s.GetIssue(ctx, child1.ID)
	if err != nil || c1 == nil {
		t.Error("Child1 should still exist with keep-children")
	}
}

func TestSquashMoleculeWithDelete(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a molecule with ephemeral children
	root := &types.Issue{
		Title:     "Delete Test Molecule",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	child := &types.Issue{
		Title:     "Wisp Step",
		Status:    types.StatusClosed,
		Priority:  2,
		IssueType: types.TypeTask,
		Ephemeral: true,
	}
	if err := s.CreateIssue(ctx, child, "test"); err != nil {
		t.Fatalf("Failed to create child: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     child.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add dependency: %v", err)
	}

	// Squash with delete (keepChildren=false)
	result, err := squashMolecule(ctx, s, root, []*types.Issue{child}, false, "", "test")
	if err != nil {
		t.Fatalf("squashMolecule failed: %v", err)
	}

	if result.DeletedCount != 1 {
		t.Errorf("DeletedCount = %d, want 1", result.DeletedCount)
	}

	// Child should be deleted
	c, err := s.GetIssue(ctx, child.ID)
	if err == nil && c != nil {
		t.Error("Child should have been deleted")
	}

	// Digest should exist
	digest, err := s.GetIssue(ctx, result.DigestID)
	if err != nil || digest == nil {
		t.Error("Digest should exist after squash")
	}
}

// TestSquashMoleculeWithAgentSummary verifies that agent-provided summaries are used
func TestSquashMoleculeWithAgentSummary(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a molecule with ephemeral child
	root := &types.Issue{
		Title:     "Agent Summary Test",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	child := &types.Issue{
		Title:       "Wisp Step",
		Description: "This should NOT appear in digest",
		Status:      types.StatusClosed,
		Priority:    2,
		IssueType:   types.TypeTask,
		Ephemeral:   true,
		CloseReason: "Done",
	}
	if err := s.CreateIssue(ctx, child, "test"); err != nil {
		t.Fatalf("Failed to create child: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     child.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add dependency: %v", err)
	}

	// Squash with agent-provided summary
	agentSummary := "## AI-Generated Summary\n\nThe agent completed the task successfully."
	result, err := squashMolecule(ctx, s, root, []*types.Issue{child}, true, agentSummary, "test")
	if err != nil {
		t.Fatalf("squashMolecule failed: %v", err)
	}

	// Verify digest uses agent summary, not auto-generated content
	digest, err := s.GetIssue(ctx, result.DigestID)
	if err != nil {
		t.Fatalf("Failed to get digest: %v", err)
	}

	if digest.Description != agentSummary {
		t.Errorf("Digest should use agent summary.\nGot: %s\nWant: %s", digest.Description, agentSummary)
	}

	// Verify auto-generated content is NOT present
	if strings.Contains(digest.Description, "Wisp Step") {
		t.Error("Digest should NOT contain auto-generated content when agent summary provided")
	}
}

// TestSquashWispMoleculeClearsRootEphemeral verifies that squashing a wisp
// molecule (root with Ephemeral=true) clears the root's Ephemeral flag in
// addition to auto-closing it. Without this, auto-export keeps re-emitting
// the closed root as ephemeral=true on every export cycle, accumulating
// duplicate JSONL rows.
func TestSquashWispMoleculeClearsRootEphemeral(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Wisp molecule: root is ephemeral too (created via `bd mol wisp`).
	root := &types.Issue{
		Title:     "Wisp Molecule Root",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Ephemeral: true,
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create ephemeral root: %v", err)
	}

	child := &types.Issue{
		Title:     "Wisp Step",
		Status:    types.StatusClosed,
		Priority:  2,
		IssueType: types.TypeTask,
		Ephemeral: true,
	}
	if err := s.CreateIssue(ctx, child, "test"); err != nil {
		t.Fatalf("Failed to create child: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     child.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add dependency: %v", err)
	}

	result, err := squashMolecule(ctx, s, root, []*types.Issue{child}, false, "", "test")
	if err != nil {
		t.Fatalf("squashMolecule failed: %v", err)
	}
	if !result.WispSquash {
		t.Error("expected WispSquash=true for ephemeral root")
	}

	closed, err := s.GetIssue(ctx, root.ID)
	if err != nil {
		t.Fatalf("Failed to get root after squash: %v", err)
	}
	if closed.Status != types.StatusClosed {
		t.Errorf("root status = %v, want closed", closed.Status)
	}
	if closed.Ephemeral {
		t.Errorf("root Ephemeral should be false after squash, got true (issue: closed wisp roots leak as duplicate JSONL rows on every export)")
	}
}

// =============================================================================
// Spawn --attach Tests (bd-f7p1)
// =============================================================================

// TestSpawnWithBasicAttach tests spawning a proto with one --attach flag
func TestSpawnWithBasicAttach(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create primary proto with a child
	primaryProto := &types.Issue{
		Title:     "Primary: {{feature}}",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, primaryProto, "test"); err != nil {
		t.Fatalf("Failed to create primary proto: %v", err)
	}

	primaryChild := &types.Issue{
		Title:     "Step 1 for {{feature}}",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, primaryChild, "test"); err != nil {
		t.Fatalf("Failed to create primary child: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     primaryChild.ID,
		DependsOnID: primaryProto.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add primary child dependency: %v", err)
	}

	// Create attachment proto with a child
	attachProto := &types.Issue{
		Title:     "Attachment: {{feature}} docs",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, attachProto, "test"); err != nil {
		t.Fatalf("Failed to create attach proto: %v", err)
	}

	attachChild := &types.Issue{
		Title:     "Write docs for {{feature}}",
		Status:    types.StatusOpen,
		Priority:  3,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, attachChild, "test"); err != nil {
		t.Fatalf("Failed to create attach child: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     attachChild.ID,
		DependsOnID: attachProto.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add attach child dependency: %v", err)
	}

	// Spawn primary proto
	primarySubgraph, err := loadTemplateSubgraph(ctx, s, primaryProto.ID)
	if err != nil {
		t.Fatalf("Failed to load primary subgraph: %v", err)
	}

	vars := map[string]string{"feature": "auth"}
	spawnResult, err := spawnMolecule(ctx, s, primarySubgraph, vars, "", "test", true, "")
	if err != nil {
		t.Fatalf("Failed to spawn primary: %v", err)
	}

	if spawnResult.Created != 2 {
		t.Errorf("Spawn created = %d, want 2", spawnResult.Created)
	}

	// Get the spawned molecule
	spawnedMol, err := s.GetIssue(ctx, spawnResult.NewEpicID)
	if err != nil {
		t.Fatalf("Failed to get spawned molecule: %v", err)
	}

	// Attach the second proto (simulating --attach flag behavior)
	bondResult, err := bondProtoMol(ctx, s, attachProto, spawnedMol, types.BondTypeSequential, vars, "", "test", false, false)
	if err != nil {
		t.Fatalf("Failed to bond attachment: %v", err)
	}

	if bondResult.Spawned != 2 {
		t.Errorf("Bond spawned = %d, want 2", bondResult.Spawned)
	}
	if bondResult.ResultType != "compound_molecule" {
		t.Errorf("ResultType = %q, want %q", bondResult.ResultType, "compound_molecule")
	}

	// Verify the spawned attachment root has dependency on the primary molecule
	attachedRootID := bondResult.IDMapping[attachProto.ID]
	deps, err := s.GetDependenciesWithMetadata(ctx, attachedRootID)
	if err != nil {
		t.Fatalf("Failed to get deps: %v", err)
	}

	foundBlocks := false
	for _, dep := range deps {
		if dep.ID == spawnedMol.ID && dep.DependencyType == types.DepBlocks {
			foundBlocks = true
		}
	}
	if !foundBlocks {
		t.Error("Expected blocks dependency from attached proto to spawned molecule for sequential bond")
	}

	// Verify variable substitution worked in attached issues
	attachedRoot, err := s.GetIssue(ctx, attachedRootID)
	if err != nil {
		t.Fatalf("Failed to get attached root: %v", err)
	}
	if !strings.Contains(attachedRoot.Title, "auth") {
		t.Errorf("Attached root title %q should contain 'auth' from variable substitution", attachedRoot.Title)
	}
}

// TestSpawnWithMultipleAttachments tests spawning with --attach A --attach B
func TestSpawnWithMultipleAttachments(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create primary proto
	primaryProto := &types.Issue{
		Title:     "Primary Feature",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, primaryProto, "test"); err != nil {
		t.Fatalf("Failed to create primary proto: %v", err)
	}

	// Create first attachment proto
	attachA := &types.Issue{
		Title:     "Attachment A: Testing",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, attachA, "test"); err != nil {
		t.Fatalf("Failed to create attachA: %v", err)
	}

	// Create second attachment proto
	attachB := &types.Issue{
		Title:     "Attachment B: Documentation",
		Status:    types.StatusOpen,
		Priority:  3,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, attachB, "test"); err != nil {
		t.Fatalf("Failed to create attachB: %v", err)
	}

	// Spawn primary
	primarySubgraph, err := loadTemplateSubgraph(ctx, s, primaryProto.ID)
	if err != nil {
		t.Fatalf("Failed to load primary subgraph: %v", err)
	}

	spawnResult, err := spawnMolecule(ctx, s, primarySubgraph, nil, "", "test", true, "")
	if err != nil {
		t.Fatalf("Failed to spawn primary: %v", err)
	}

	spawnedMol, err := s.GetIssue(ctx, spawnResult.NewEpicID)
	if err != nil {
		t.Fatalf("Failed to get spawned molecule: %v", err)
	}

	// Attach both protos (simulating --attach A --attach B)
	bondResultA, err := bondProtoMol(ctx, s, attachA, spawnedMol, types.BondTypeSequential, nil, "", "test", false, false)
	if err != nil {
		t.Fatalf("Failed to bond attachA: %v", err)
	}

	bondResultB, err := bondProtoMol(ctx, s, attachB, spawnedMol, types.BondTypeSequential, nil, "", "test", false, false)
	if err != nil {
		t.Fatalf("Failed to bond attachB: %v", err)
	}

	// Both should have spawned their protos
	if bondResultA.Spawned != 1 {
		t.Errorf("bondResultA.Spawned = %d, want 1", bondResultA.Spawned)
	}
	if bondResultB.Spawned != 1 {
		t.Errorf("bondResultB.Spawned = %d, want 1", bondResultB.Spawned)
	}

	// Both should depend on the primary molecule
	attachedAID := bondResultA.IDMapping[attachA.ID]
	attachedBID := bondResultB.IDMapping[attachB.ID]

	depsA, err := s.GetDependenciesWithMetadata(ctx, attachedAID)
	if err != nil {
		t.Fatalf("Failed to get deps for A: %v", err)
	}
	depsB, err := s.GetDependenciesWithMetadata(ctx, attachedBID)
	if err != nil {
		t.Fatalf("Failed to get deps for B: %v", err)
	}

	foundABlocks := false
	for _, dep := range depsA {
		if dep.ID == spawnedMol.ID && dep.DependencyType == types.DepBlocks {
			foundABlocks = true
		}
	}
	foundBBlocks := false
	for _, dep := range depsB {
		if dep.ID == spawnedMol.ID && dep.DependencyType == types.DepBlocks {
			foundBBlocks = true
		}
	}

	if !foundABlocks {
		t.Error("Expected A to block on spawned molecule")
	}
	if !foundBBlocks {
		t.Error("Expected B to block on spawned molecule")
	}
}

// TestSpawnAttachTypes verifies sequential vs parallel bonding behavior
func TestSpawnAttachTypes(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create primary proto
	primaryProto := &types.Issue{
		Title:     "Primary",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, primaryProto, "test"); err != nil {
		t.Fatalf("Failed to create primary: %v", err)
	}

	// Create attachment proto
	attachProto := &types.Issue{
		Title:     "Attachment",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, attachProto, "test"); err != nil {
		t.Fatalf("Failed to create attachment: %v", err)
	}

	tests := []struct {
		name       string
		bondType   string
		expectType types.DependencyType
	}{
		{"sequential uses blocks", types.BondTypeSequential, types.DepBlocks},
		{"parallel uses parent-child", types.BondTypeParallel, types.DepParentChild},
		{"conditional uses conditional-blocks", types.BondTypeConditional, types.DepConditionalBlocks},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Spawn fresh primary for each test
			primarySubgraph, err := loadTemplateSubgraph(ctx, s, primaryProto.ID)
			if err != nil {
				t.Fatalf("Failed to load primary subgraph: %v", err)
			}

			spawnResult, err := spawnMolecule(ctx, s, primarySubgraph, nil, "", "test", true, "")
			if err != nil {
				t.Fatalf("Failed to spawn primary: %v", err)
			}

			spawnedMol, err := s.GetIssue(ctx, spawnResult.NewEpicID)
			if err != nil {
				t.Fatalf("Failed to get spawned molecule: %v", err)
			}

			// Bond with specified type
			bondResult, err := bondProtoMol(ctx, s, attachProto, spawnedMol, tt.bondType, nil, "", "test", false, false)
			if err != nil {
				t.Fatalf("Failed to bond: %v", err)
			}

			// Check dependency type
			attachedID := bondResult.IDMapping[attachProto.ID]
			deps, err := s.GetDependenciesWithMetadata(ctx, attachedID)
			if err != nil {
				t.Fatalf("Failed to get deps: %v", err)
			}

			foundExpected := false
			for _, dep := range deps {
				if dep.ID == spawnedMol.ID && dep.DependencyType == tt.expectType {
					foundExpected = true
				}
			}

			if !foundExpected {
				t.Errorf("Expected %s dependency from attached to spawned molecule", tt.expectType)
			}
		})
	}
}

// TestSpawnVariableAggregation tests that variables from primary + attachments are combined
func TestSpawnVariableAggregation(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create primary proto with one variable
	primaryProto := &types.Issue{
		Title:       "Feature: {{feature_name}}",
		Description: "Implement the {{feature_name}} feature",
		Status:      types.StatusOpen,
		Priority:    1,
		IssueType:   types.TypeEpic,
		Labels:      []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, primaryProto, "test"); err != nil {
		t.Fatalf("Failed to create primary: %v", err)
	}

	// Create attachment proto with a different variable
	attachProto := &types.Issue{
		Title:       "Docs for {{doc_version}}",
		Description: "Document version {{doc_version}}",
		Status:      types.StatusOpen,
		Priority:    2,
		IssueType:   types.TypeEpic,
		Labels:      []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, attachProto, "test"); err != nil {
		t.Fatalf("Failed to create attachment: %v", err)
	}

	// Load subgraphs and extract variables
	primarySubgraph, err := loadTemplateSubgraph(ctx, s, primaryProto.ID)
	if err != nil {
		t.Fatalf("Failed to load primary subgraph: %v", err)
	}
	attachSubgraph, err := loadTemplateSubgraph(ctx, s, attachProto.ID)
	if err != nil {
		t.Fatalf("Failed to load attach subgraph: %v", err)
	}

	// Aggregate variables (simulating runMolSpawn logic)
	requiredVars := extractAllVariables(primarySubgraph)
	attachVars := extractAllVariables(attachSubgraph)
	for _, v := range attachVars {
		found := false
		for _, rv := range requiredVars {
			if rv == v {
				found = true
				break
			}
		}
		if !found {
			requiredVars = append(requiredVars, v)
		}
	}

	// Should have both variables
	if len(requiredVars) != 2 {
		t.Errorf("Expected 2 required vars, got %d: %v", len(requiredVars), requiredVars)
	}

	hasFeatureName := false
	hasDocVersion := false
	for _, v := range requiredVars {
		if v == "feature_name" {
			hasFeatureName = true
		}
		if v == "doc_version" {
			hasDocVersion = true
		}
	}

	if !hasFeatureName {
		t.Error("Missing feature_name variable from primary proto")
	}
	if !hasDocVersion {
		t.Error("Missing doc_version variable from attachment proto")
	}

	// Provide both variables and verify substitution
	vars := map[string]string{
		"feature_name": "authentication",
		"doc_version":  "2.0",
	}

	// Spawn primary with variables
	spawnResult, err := spawnMolecule(ctx, s, primarySubgraph, vars, "", "test", true, "")
	if err != nil {
		t.Fatalf("Failed to spawn primary: %v", err)
	}

	// Verify primary variable was substituted
	spawnedPrimary, err := s.GetIssue(ctx, spawnResult.NewEpicID)
	if err != nil {
		t.Fatalf("Failed to get spawned primary: %v", err)
	}
	if !strings.Contains(spawnedPrimary.Title, "authentication") {
		t.Errorf("Primary title %q should contain 'authentication'", spawnedPrimary.Title)
	}

	// Bond attachment with same variables
	spawnedMol, _ := s.GetIssue(ctx, spawnResult.NewEpicID)
	bondResult, err := bondProtoMol(ctx, s, attachProto, spawnedMol, types.BondTypeSequential, vars, "", "test", false, false)
	if err != nil {
		t.Fatalf("Failed to bond: %v", err)
	}

	// Verify attachment variable was substituted
	attachedID := bondResult.IDMapping[attachProto.ID]
	attachedIssue, err := s.GetIssue(ctx, attachedID)
	if err != nil {
		t.Fatalf("Failed to get attached issue: %v", err)
	}
	if !strings.Contains(attachedIssue.Title, "2.0") {
		t.Errorf("Attached title %q should contain '2.0'", attachedIssue.Title)
	}
}

// TestWispFilteringFromExport verifies that wisp issues are filtered
// from sync (bd-687g). Wisp issues are stored in the dolt_ignore table,
// excluded from push/pull, to prevent "zombie" resurrection after mol squash.
func TestWispFilteringFromExport(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a mix of wisp and non-wisp issues
	normalIssue := &types.Issue{
		Title:     "Normal Issue",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeTask,
		Ephemeral: false,
	}
	wispIssue := &types.Issue{
		Title:     "Wisp Issue",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
		Ephemeral: true,
	}

	if err := s.CreateIssue(ctx, normalIssue, "test"); err != nil {
		t.Fatalf("Failed to create normal issue: %v", err)
	}
	if err := s.CreateIssue(ctx, wispIssue, "test"); err != nil {
		t.Fatalf("Failed to create wisp issue: %v", err)
	}

	// Get all issues from DB - should include both
	allIssues, err := s.SearchIssues(ctx, "", types.IssueFilter{})
	if err != nil {
		t.Fatalf("Failed to search issues: %v", err)
	}
	if len(allIssues) != 2 {
		t.Fatalf("Expected 2 issues in DB, got %d", len(allIssues))
	}

	// Filter wisp issues (simulating export behavior)
	exportableIssues := make([]*types.Issue, 0)
	for _, issue := range allIssues {
		if !issue.Ephemeral {
			exportableIssues = append(exportableIssues, issue)
		}
	}

	// Should only have the non-wisp issue
	if len(exportableIssues) != 1 {
		t.Errorf("Expected 1 exportable issue, got %d", len(exportableIssues))
	}
	if exportableIssues[0].ID != normalIssue.ID {
		t.Errorf("Expected normal issue %s, got %s", normalIssue.ID, exportableIssues[0].ID)
	}
}

// =============================================================================
// Mol Current Tests (bd-nurq)
// =============================================================================

// TestGetMoleculeProgress tests loading a molecule and computing progress
func TestGetMoleculeProgress(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a molecule (epic with template label)
	root := &types.Issue{
		Title:     "Test Molecule",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{BeadsTemplateLabel},
		Assignee:  "test-agent",
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	// Create steps with different statuses
	step1 := &types.Issue{
		Title:     "Step 1: Done",
		Status:    types.StatusClosed,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	step2 := &types.Issue{
		Title:     "Step 2: Current",
		Status:    types.StatusInProgress,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	step3 := &types.Issue{
		Title:     "Step 3: Pending",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}

	for _, step := range []*types.Issue{step1, step2, step3} {
		if err := s.CreateIssue(ctx, step, "test"); err != nil {
			t.Fatalf("Failed to create step: %v", err)
		}
		// Add parent-child dependency
		if err := s.AddDependency(ctx, &types.Dependency{
			IssueID:     step.ID,
			DependsOnID: root.ID,
			Type:        types.DepParentChild,
		}, "test"); err != nil {
			t.Fatalf("Failed to add dependency: %v", err)
		}
	}

	// Add blocking dependency: step3 blocks on step2
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     step3.ID,
		DependsOnID: step2.ID,
		Type:        types.DepBlocks,
	}, "test"); err != nil {
		t.Fatalf("Failed to add blocking dependency: %v", err)
	}

	// Get progress
	progress, err := getMoleculeProgress(ctx, s, root.ID)
	if err != nil {
		t.Fatalf("getMoleculeProgress failed: %v", err)
	}

	// Verify progress
	if progress.MoleculeID != root.ID {
		t.Errorf("MoleculeID = %q, want %q", progress.MoleculeID, root.ID)
	}
	if progress.MoleculeTitle != root.Title {
		t.Errorf("MoleculeTitle = %q, want %q", progress.MoleculeTitle, root.Title)
	}
	if progress.Assignee != "test-agent" {
		t.Errorf("Assignee = %q, want %q", progress.Assignee, "test-agent")
	}
	if progress.Total != 3 {
		t.Errorf("Total = %d, want 3", progress.Total)
	}
	if progress.Completed != 1 {
		t.Errorf("Completed = %d, want 1", progress.Completed)
	}
	if progress.CurrentStep == nil {
		t.Error("CurrentStep should not be nil")
	} else if progress.CurrentStep.ID != step2.ID {
		t.Errorf("CurrentStep.ID = %q, want %q", progress.CurrentStep.ID, step2.ID)
	}
	if len(progress.Steps) != 3 {
		t.Errorf("Steps count = %d, want 3", len(progress.Steps))
	}
}

// TestFindParentMolecule tests walking up parent-child chain
func TestFindParentMolecule(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create molecule root (epic with template label)
	root := &types.Issue{
		Title:     "Molecule Root",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{BeadsTemplateLabel},
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	// Create child step
	child := &types.Issue{
		Title:     "Child Step",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, child, "test"); err != nil {
		t.Fatalf("Failed to create child: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     child.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add parent-child: %v", err)
	}

	// Create grandchild
	grandchild := &types.Issue{
		Title:     "Grandchild Step",
		Status:    types.StatusOpen,
		Priority:  3,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, grandchild, "test"); err != nil {
		t.Fatalf("Failed to create grandchild: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     grandchild.ID,
		DependsOnID: child.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add grandchild parent-child: %v", err)
	}

	// Find parent molecule from grandchild
	moleculeID := findParentMolecule(ctx, s, grandchild.ID)
	if moleculeID != root.ID {
		t.Errorf("findParentMolecule(grandchild) = %q, want %q", moleculeID, root.ID)
	}

	// Find parent molecule from child
	moleculeID = findParentMolecule(ctx, s, child.ID)
	if moleculeID != root.ID {
		t.Errorf("findParentMolecule(child) = %q, want %q", moleculeID, root.ID)
	}

	// Find parent molecule from root
	moleculeID = findParentMolecule(ctx, s, root.ID)
	if moleculeID != root.ID {
		t.Errorf("findParentMolecule(root) = %q, want %q", moleculeID, root.ID)
	}

	// Create orphan issue (not part of any molecule)
	orphan := &types.Issue{
		Title:     "Orphan Issue",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, orphan, "test"); err != nil {
		t.Fatalf("Failed to create orphan: %v", err)
	}

	// Should return empty for orphan
	moleculeID = findParentMolecule(ctx, s, orphan.ID)
	if moleculeID != "" {
		t.Errorf("findParentMolecule(orphan) = %q, want empty", moleculeID)
	}
}

// TestFindParentMoleculesBatch tests batch-finding molecule roots (bd-hn4q)
func TestFindParentMoleculesBatch(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create molecule root (epic with template label)
	root := &types.Issue{
		Title:     "Molecule Root",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{BeadsTemplateLabel},
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	// Create child step
	child := &types.Issue{
		Title:     "Child Step",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, child, "test"); err != nil {
		t.Fatalf("Failed to create child: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     child.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add parent-child: %v", err)
	}

	// Create grandchild
	grandchild := &types.Issue{
		Title:     "Grandchild Step",
		Status:    types.StatusOpen,
		Priority:  3,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, grandchild, "test"); err != nil {
		t.Fatalf("Failed to create grandchild: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     grandchild.ID,
		DependsOnID: child.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add grandchild parent-child: %v", err)
	}

	// Create orphan issue (not part of any molecule)
	orphan := &types.Issue{
		Title:     "Orphan Issue",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, orphan, "test"); err != nil {
		t.Fatalf("Failed to create orphan: %v", err)
	}

	// Batch-find molecule roots for all issues at once
	roots := findParentMolecules(ctx, s, []string{grandchild.ID, child.ID, root.ID, orphan.ID})

	if got := roots[grandchild.ID]; got != root.ID {
		t.Errorf("findParentMolecules[grandchild] = %q, want %q", got, root.ID)
	}
	if got := roots[child.ID]; got != root.ID {
		t.Errorf("findParentMolecules[child] = %q, want %q", got, root.ID)
	}
	if got := roots[root.ID]; got != root.ID {
		t.Errorf("findParentMolecules[root] = %q, want %q", got, root.ID)
	}
	if got := roots[orphan.ID]; got != "" {
		t.Errorf("findParentMolecules[orphan] = %q, want empty", got)
	}

	// Empty input should return nil
	if result := findParentMolecules(ctx, s, nil); result != nil {
		t.Errorf("findParentMolecules(nil) = %v, want nil", result)
	}
}

// TestFindParentMolecule_RootShapes covers every shape of molecule root that
// findParentMolecule(s) needs to recognize:
//   - TypeEpic + BeadsTemplateLabel  (distilled / older formula path)
//   - TypeMolecule with no label     (`bd mol pour` path — gastownhall/beads#3719)
//   - TypeTask + BeadsTemplateLabel  (template-label-only fallback)
//   - TypeTask, no label             (not a molecule)
//
// Before the fix for #3719 the TypeMolecule case returned "", which caused
// `bd close --continue` to silently no-op for any poured molecule.
func TestFindParentMolecule_RootShapes(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	tests := []struct {
		name       string
		rootType   types.IssueType
		rootLabels []string
		isMolecule bool
	}{
		{"epic with template label", types.TypeEpic, []string{BeadsTemplateLabel}, true},
		{"poured molecule (no label)", types.TypeMolecule, nil, true},
		{"task with template label", types.TypeTask, []string{BeadsTemplateLabel}, true},
		{"plain task is not a molecule", types.TypeTask, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := &types.Issue{
				Title:     tt.name + " root",
				Status:    types.StatusOpen,
				Priority:  1,
				IssueType: tt.rootType,
				Labels:    tt.rootLabels,
			}
			if err := s.CreateIssue(ctx, root, "test"); err != nil {
				t.Fatalf("Failed to create root: %v", err)
			}

			child := &types.Issue{
				Title:     tt.name + " child",
				Status:    types.StatusOpen,
				Priority:  2,
				IssueType: types.TypeTask,
			}
			if err := s.CreateIssue(ctx, child, "test"); err != nil {
				t.Fatalf("Failed to create child: %v", err)
			}
			if err := s.AddDependency(ctx, &types.Dependency{
				IssueID:     child.ID,
				DependsOnID: root.ID,
				Type:        types.DepParentChild,
			}, "test"); err != nil {
				t.Fatalf("Failed to add parent-child: %v", err)
			}

			wantSingle := ""
			if tt.isMolecule {
				wantSingle = root.ID
			}
			if got := findParentMolecule(ctx, s, child.ID); got != wantSingle {
				t.Errorf("findParentMolecule(child) = %q, want %q", got, wantSingle)
			}

			wantBatch := map[string]string{child.ID: wantSingle}
			gotBatch := findParentMolecules(ctx, s, []string{child.ID})
			if got := gotBatch[child.ID]; got != wantBatch[child.ID] {
				t.Errorf("findParentMolecules(child) = %q, want %q", got, wantBatch[child.ID])
			}
		})
	}
}

// TestAdvanceToNextStep_PouredMolecule reproduces gastownhall/beads#3719: when a
// step lives under a TypeMolecule root (as `bd mol pour` creates), AdvanceToNextStep
// must find the parent molecule and (with autoClaim=true) claim the next ready step.
// Before the fix it silently returned (nil, nil).
func TestAdvanceToNextStep_PouredMolecule(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Root shaped like `bd mol pour` output: TypeMolecule, no template label.
	root := &types.Issue{
		Title:     "Poured Molecule",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeMolecule,
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	step1 := &types.Issue{
		Title:     "Step 1",
		Status:    types.StatusClosed,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	step2 := &types.Issue{
		Title:     "Step 2",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	for _, step := range []*types.Issue{step1, step2} {
		if err := s.CreateIssue(ctx, step, "test"); err != nil {
			t.Fatalf("Failed to create step: %v", err)
		}
		if err := s.AddDependency(ctx, &types.Dependency{
			IssueID:     step.ID,
			DependsOnID: root.ID,
			Type:        types.DepParentChild,
		}, "test"); err != nil {
			t.Fatalf("Failed to add parent-child: %v", err)
		}
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     step2.ID,
		DependsOnID: step1.ID,
		Type:        types.DepBlocks,
	}, "test"); err != nil {
		t.Fatalf("Failed to add blocking dep: %v", err)
	}

	result, err := AdvanceToNextStep(ctx, newStandaloneStoreMolWriter(s), step1.ID, true, "test")
	if err != nil {
		t.Fatalf("AdvanceToNextStep failed: %v", err)
	}
	if result == nil {
		t.Fatal("AdvanceToNextStep returned nil result; expected molecule advance for TypeMolecule root")
	}
	if result.MoleculeID != root.ID {
		t.Errorf("MoleculeID = %q, want %q", result.MoleculeID, root.ID)
	}
	if result.NextStep == nil || result.NextStep.ID != step2.ID {
		t.Errorf("NextStep = %v, want step2 (%s)", result.NextStep, step2.ID)
	}
	if !result.AutoAdvanced {
		t.Errorf("AutoAdvanced = false; want true (autoClaim was on)")
	}

	// Confirm step2 is in_progress now.
	got, err := s.GetIssue(ctx, step2.ID)
	if err != nil {
		t.Fatalf("GetIssue(step2) failed: %v", err)
	}
	if got.Status != types.StatusInProgress {
		t.Errorf("step2 status = %q, want %q", got.Status, types.StatusInProgress)
	}
}

// TestFindHookedMolecules tests finding molecules bonded to hooked issues
func TestFindHookedMolecules(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create molecule root (epic)
	molecule := &types.Issue{
		Title:     "Test Molecule",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
	}
	if err := s.CreateIssue(ctx, molecule, "test"); err != nil {
		t.Fatalf("Failed to create molecule: %v", err)
	}

	// Create step as child of molecule
	step := &types.Issue{
		Title:     "Step 1",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, step, "test"); err != nil {
		t.Fatalf("Failed to create step: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     step.ID,
		DependsOnID: molecule.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add parent-child: %v", err)
	}

	// Create hooked issue with blocks dependency on molecule
	hookedIssue := &types.Issue{
		Title:     "Hooked Work",
		Status:    types.StatusHooked,
		Priority:  2,
		IssueType: types.TypeTask,
		Assignee:  "test-agent",
	}
	if err := s.CreateIssue(ctx, hookedIssue, "test"); err != nil {
		t.Fatalf("Failed to create hooked issue: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     hookedIssue.ID,
		DependsOnID: molecule.ID,
		Type:        types.DepBlocks,
	}, "test"); err != nil {
		t.Fatalf("Failed to add blocks dependency: %v", err)
	}

	// Test: findHookedMolecules should find the molecule for this agent
	molecules := findHookedMolecules(ctx, s, "test-agent")
	if len(molecules) != 1 {
		t.Fatalf("findHookedMolecules() got %d molecules, want 1", len(molecules))
	}
	if molecules[0].MoleculeID != molecule.ID {
		t.Errorf("findHookedMolecules() got molecule %q, want %q", molecules[0].MoleculeID, molecule.ID)
	}

	// Test: different agent should not find the molecule
	molecules = findHookedMolecules(ctx, s, "other-agent")
	if len(molecules) != 0 {
		t.Errorf("findHookedMolecules(other-agent) got %d molecules, want 0", len(molecules))
	}

	// Test: no agent filter should find the molecule
	molecules = findHookedMolecules(ctx, s, "")
	if len(molecules) != 1 {
		t.Errorf("findHookedMolecules('') got %d molecules, want 1", len(molecules))
	}
}

// TestAdvanceToNextStep tests auto-advancing to next step
func TestAdvanceToNextStep(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create molecule with sequential steps
	root := &types.Issue{
		Title:     "Advance Test Molecule",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{BeadsTemplateLabel},
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	step1 := &types.Issue{
		Title:     "Step 1",
		Status:    types.StatusClosed,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	step2 := &types.Issue{
		Title:     "Step 2",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}

	for _, step := range []*types.Issue{step1, step2} {
		if err := s.CreateIssue(ctx, step, "test"); err != nil {
			t.Fatalf("Failed to create step: %v", err)
		}
		if err := s.AddDependency(ctx, &types.Dependency{
			IssueID:     step.ID,
			DependsOnID: root.ID,
			Type:        types.DepParentChild,
		}, "test"); err != nil {
			t.Fatalf("Failed to add dependency: %v", err)
		}
	}

	// step2 blocks on step1
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     step2.ID,
		DependsOnID: step1.ID,
		Type:        types.DepBlocks,
	}, "test"); err != nil {
		t.Fatalf("Failed to add blocking dependency: %v", err)
	}

	// Advance from step1 (just closed) without auto-claim
	result, err := AdvanceToNextStep(ctx, newStandaloneStoreMolWriter(s), step1.ID, false, "test")
	if err != nil {
		t.Fatalf("AdvanceToNextStep failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.MoleculeID != root.ID {
		t.Errorf("MoleculeID = %q, want %q", result.MoleculeID, root.ID)
	}
	if result.NextStep == nil {
		t.Error("NextStep should not be nil")
	} else if result.NextStep.ID != step2.ID {
		t.Errorf("NextStep.ID = %q, want %q", result.NextStep.ID, step2.ID)
	}
	if result.AutoAdvanced {
		t.Error("AutoAdvanced should be false when not requested")
	}

	// Verify step2 is still open
	step2Updated, _ := s.GetIssue(ctx, step2.ID)
	if step2Updated.Status != types.StatusOpen {
		t.Errorf("Step2 status = %v, want open (no auto-claim)", step2Updated.Status)
	}

	// Now test with auto-claim
	result, err = AdvanceToNextStep(ctx, newStandaloneStoreMolWriter(s), step1.ID, true, "test")
	if err != nil {
		t.Fatalf("AdvanceToNextStep with auto-claim failed: %v", err)
	}
	if !result.AutoAdvanced {
		t.Error("AutoAdvanced should be true when requested")
	}

	// Verify step2 is now in_progress
	step2Updated, _ = s.GetIssue(ctx, step2.ID)
	if step2Updated.Status != types.StatusInProgress {
		t.Errorf("Step2 status = %v, want in_progress (auto-claim)", step2Updated.Status)
	}
}

// TestAdvanceToNextStepMoleculeComplete tests behavior when molecule is complete
func TestAdvanceToNextStepMoleculeComplete(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create molecule with single step
	root := &types.Issue{
		Title:     "Complete Test Molecule",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{BeadsTemplateLabel},
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	step1 := &types.Issue{
		Title:     "Only Step",
		Status:    types.StatusClosed,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, step1, "test"); err != nil {
		t.Fatalf("Failed to create step: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     step1.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add dependency: %v", err)
	}

	// Advance from the only step (molecule should be complete)
	result, err := AdvanceToNextStep(ctx, newStandaloneStoreMolWriter(s), step1.ID, false, "test")
	if err != nil {
		t.Fatalf("AdvanceToNextStep failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if !result.MolComplete {
		t.Error("MolComplete should be true when all steps are done")
	}
	if result.NextStep != nil {
		t.Error("NextStep should be nil when molecule is complete")
	}
}

// TestAdvanceToNextStepOrphanIssue tests behavior for non-molecule issues
func TestAdvanceToNextStepOrphanIssue(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create standalone issue (not part of molecule)
	orphan := &types.Issue{
		Title:     "Standalone Issue",
		Status:    types.StatusClosed,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	if err := s.CreateIssue(ctx, orphan, "test"); err != nil {
		t.Fatalf("Failed to create orphan: %v", err)
	}

	// Advance should return nil (not part of molecule)
	result, err := AdvanceToNextStep(ctx, newStandaloneStoreMolWriter(s), orphan.ID, false, "test")
	if err != nil {
		t.Fatalf("AdvanceToNextStep failed: %v", err)
	}
	if result != nil {
		t.Error("result should be nil for orphan issue")
	}
}

// TestAdvanceToNextStepConcurrentClaim tests optimistic concurrency: when the
// first ready step has already been claimed by another agent, AdvanceToNextStep
// falls back to the next available ready step.
func TestAdvanceToNextStepConcurrentClaim(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create molecule: root -> step1 (closed), step2 (open), step3 (open)
	// step2 and step3 both depend on step1, so both become ready when step1 closes.
	root := &types.Issue{
		Title:     "Concurrent Claim Test Molecule",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{BeadsTemplateLabel},
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	step1 := &types.Issue{
		Title:     "Step 1 (closed)",
		Status:    types.StatusClosed,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	step2 := &types.Issue{
		Title:     "Step 2 (will be pre-claimed)",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	step3 := &types.Issue{
		Title:     "Step 3 (fallback target)",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}

	for _, step := range []*types.Issue{step1, step2, step3} {
		if err := s.CreateIssue(ctx, step, "test"); err != nil {
			t.Fatalf("Failed to create step: %v", err)
		}
		if err := s.AddDependency(ctx, &types.Dependency{
			IssueID:     step.ID,
			DependsOnID: root.ID,
			Type:        types.DepParentChild,
		}, "test"); err != nil {
			t.Fatalf("Failed to add parent dependency: %v", err)
		}
	}

	// step2 and step3 both depend on step1
	for _, step := range []*types.Issue{step2, step3} {
		if err := s.AddDependency(ctx, &types.Dependency{
			IssueID:     step.ID,
			DependsOnID: step1.ID,
			Type:        types.DepBlocks,
		}, "test"); err != nil {
			t.Fatalf("Failed to add blocking dependency: %v", err)
		}
	}

	// Simulate another agent claiming step2 (TOCTOU race scenario)
	if err := s.UpdateIssue(ctx, step2.ID, map[string]interface{}{
		"status": types.StatusInProgress,
	}, "other-agent"); err != nil {
		t.Fatalf("Failed to pre-claim step2: %v", err)
	}

	// Now our agent tries to advance with autoClaim — step2 was identified as
	// ready during the read phase, but it's already in_progress. The function
	// should fall back to step3.
	result, err := AdvanceToNextStep(ctx, newStandaloneStoreMolWriter(s), step1.ID, true, "our-agent")
	if err != nil {
		t.Fatalf("AdvanceToNextStep failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if !result.AutoAdvanced {
		t.Error("AutoAdvanced should be true (should have claimed step3)")
	}
	if result.NextStep == nil {
		t.Fatal("NextStep should not be nil")
	}
	if result.NextStep.ID != step3.ID {
		t.Errorf("NextStep.ID = %q, want %q (should fall back to step3)", result.NextStep.ID, step3.ID)
	}

	// Verify step3 is now in_progress
	step3Updated, _ := s.GetIssue(ctx, step3.ID)
	if step3Updated.Status != types.StatusInProgress {
		t.Errorf("Step3 status = %v, want in_progress", step3Updated.Status)
	}
}

// TestAdvanceToNextStepAllClaimed tests that when all ready steps are already
// claimed by other agents, AdvanceToNextStep reports no auto-advance.
func TestAdvanceToNextStepAllClaimed(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create molecule: root -> step1 (closed), step2 (open, will be pre-claimed)
	root := &types.Issue{
		Title:     "All Claimed Test Molecule",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeEpic,
		Labels:    []string{BeadsTemplateLabel},
	}
	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create root: %v", err)
	}

	step1 := &types.Issue{
		Title:     "Step 1 (closed)",
		Status:    types.StatusClosed,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	step2 := &types.Issue{
		Title:     "Step 2 (only ready, will be pre-claimed)",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}

	for _, step := range []*types.Issue{step1, step2} {
		if err := s.CreateIssue(ctx, step, "test"); err != nil {
			t.Fatalf("Failed to create step: %v", err)
		}
		if err := s.AddDependency(ctx, &types.Dependency{
			IssueID:     step.ID,
			DependsOnID: root.ID,
			Type:        types.DepParentChild,
		}, "test"); err != nil {
			t.Fatalf("Failed to add parent dependency: %v", err)
		}
	}

	// step2 depends on step1
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     step2.ID,
		DependsOnID: step1.ID,
		Type:        types.DepBlocks,
	}, "test"); err != nil {
		t.Fatalf("Failed to add blocking dependency: %v", err)
	}

	// Pre-claim the only ready step
	if err := s.UpdateIssue(ctx, step2.ID, map[string]interface{}{
		"status": types.StatusInProgress,
	}, "other-agent"); err != nil {
		t.Fatalf("Failed to pre-claim step2: %v", err)
	}

	// Advance with autoClaim — should fail gracefully (no steps to claim)
	result, err := AdvanceToNextStep(ctx, newStandaloneStoreMolWriter(s), step1.ID, true, "our-agent")
	if err != nil {
		t.Fatalf("AdvanceToNextStep failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.AutoAdvanced {
		t.Error("AutoAdvanced should be false when all ready steps are claimed")
	}
}

// =============================================================================
// Dynamic Bonding Tests (bd-xo1o.1)
// =============================================================================

// TestBondProtoMolWithRef tests dynamic bonding with custom child references
func TestBondProtoMolWithRef(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "patrol"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a proto with child steps (mol-polecat-arm template)
	protoRoot := &types.Issue{
		Title:     "Polecat Arm: {{polecat_name}}",
		IssueType: types.TypeEpic,
		Status:    types.StatusOpen,
		Priority:  2,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, protoRoot, "test"); err != nil {
		t.Fatalf("Failed to create proto root: %v", err)
	}

	// Add proto steps
	protoCapture := &types.Issue{
		Title:     "Capture {{polecat_name}}",
		IssueType: types.TypeTask,
		Status:    types.StatusOpen,
		Priority:  2,
	}
	if err := s.CreateIssue(ctx, protoCapture, "test"); err != nil {
		t.Fatalf("Failed to create proto capture: %v", err)
	}
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     protoCapture.ID,
		DependsOnID: protoRoot.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add proto dependency: %v", err)
	}

	// Create target molecule (patrol-xxx)
	patrol := &types.Issue{
		Title:     "Witness Patrol",
		IssueType: types.TypeEpic,
		Status:    types.StatusInProgress,
		Priority:  1,
	}
	if err := s.CreateIssue(ctx, patrol, "test"); err != nil {
		t.Fatalf("Failed to create patrol: %v", err)
	}

	// Bond proto to patrol with custom child ref
	vars := map[string]string{"polecat_name": "ace"}
	childRef := "arm-{{polecat_name}}"
	result, err := bondProtoMol(ctx, s, protoRoot, patrol, types.BondTypeSequential, vars, childRef, "test", false, false)
	if err != nil {
		t.Fatalf("bondProtoMol failed: %v", err)
	}

	// Verify spawned count
	if result.Spawned != 2 {
		t.Errorf("Spawned = %d, want 2", result.Spawned)
	}

	// Verify root ID follows pattern: patrol.arm-ace
	expectedRootID := patrol.ID + ".arm-ace"
	if result.IDMapping[protoRoot.ID] != expectedRootID {
		t.Errorf("Root ID = %q, want %q", result.IDMapping[protoRoot.ID], expectedRootID)
	}

	// Verify child ID follows pattern: patrol.arm-ace.relative
	// The child's ID should be patrol.arm-ace.capture (but relative part depends on proto structure)
	childID := result.IDMapping[protoCapture.ID]
	if !strings.HasPrefix(childID, expectedRootID+".") {
		t.Errorf("Child ID %q should start with %q", childID, expectedRootID+".")
	}

	// Verify the spawned issues exist and have correct titles
	spawnedRoot, err := s.GetIssue(ctx, expectedRootID)
	if err != nil {
		t.Fatalf("Failed to get spawned root: %v", err)
	}
	if !strings.Contains(spawnedRoot.Title, "ace") {
		t.Errorf("Spawned root title %q should contain 'ace'", spawnedRoot.Title)
	}
}

// TestBondProtoMolMultipleArms tests bonding multiple arms to the same parent
func TestBondProtoMolMultipleArms(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "patrol"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create simple proto
	proto := &types.Issue{
		Title:     "Arm: {{name}}",
		IssueType: types.TypeTask,
		Status:    types.StatusOpen,
		Priority:  2,
		Labels:    []string{MoleculeLabel},
	}
	if err := s.CreateIssue(ctx, proto, "test"); err != nil {
		t.Fatalf("Failed to create proto: %v", err)
	}

	// Create parent patrol
	patrol := &types.Issue{
		Title:     "Patrol",
		IssueType: types.TypeEpic,
		Status:    types.StatusOpen,
		Priority:  1,
	}
	if err := s.CreateIssue(ctx, patrol, "test"); err != nil {
		t.Fatalf("Failed to create patrol: %v", err)
	}

	// Bond arm-ace
	varsAce := map[string]string{"name": "ace"}
	resultAce, err := bondProtoMol(ctx, s, proto, patrol, types.BondTypeParallel, varsAce, "arm-{{name}}", "test", false, false)
	if err != nil {
		t.Fatalf("bondProtoMol (ace) failed: %v", err)
	}

	// Bond arm-nux
	varsNux := map[string]string{"name": "nux"}
	resultNux, err := bondProtoMol(ctx, s, proto, patrol, types.BondTypeParallel, varsNux, "arm-{{name}}", "test", false, false)
	if err != nil {
		t.Fatalf("bondProtoMol (nux) failed: %v", err)
	}

	// Verify IDs are correct and distinct
	aceID := resultAce.IDMapping[proto.ID]
	nuxID := resultNux.IDMapping[proto.ID]

	expectedAceID := patrol.ID + ".arm-ace"
	expectedNuxID := patrol.ID + ".arm-nux"

	if aceID != expectedAceID {
		t.Errorf("Ace ID = %q, want %q", aceID, expectedAceID)
	}
	if nuxID != expectedNuxID {
		t.Errorf("Nux ID = %q, want %q", nuxID, expectedNuxID)
	}

	// Verify both exist
	aceIssue, err := s.GetIssue(ctx, aceID)
	if err != nil || aceIssue == nil {
		t.Errorf("Ace issue not found: %v", err)
	}
	nuxIssue, err := s.GetIssue(ctx, nuxID)
	if err != nil || nuxIssue == nil {
		t.Errorf("Nux issue not found: %v", err)
	}
}

// =============================================================================
// Parallel Detection Tests (bd-xo1o.4)
// =============================================================================

// TestSpawnMoleculeEphemeralFlag verifies that spawnMolecule with ephemeral=true
// creates issues with the Ephemeral flag set (bd-phin)
func TestSpawnMoleculeEphemeralFlag(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a template with a child (IDs will be auto-generated)
	root := &types.Issue{
		Title:     "Template Epic",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeEpic,
		Labels:    []string{MoleculeLabel}, // Required for loadTemplateSubgraph
	}
	child := &types.Issue{
		Title:     "Template Task",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}

	if err := s.CreateIssue(ctx, root, "test"); err != nil {
		t.Fatalf("Failed to create template root: %v", err)
	}
	if err := s.CreateIssue(ctx, child, "test"); err != nil {
		t.Fatalf("Failed to create template child: %v", err)
	}

	// Add parent-child dependency
	if err := s.AddDependency(ctx, &types.Dependency{
		IssueID:     child.ID,
		DependsOnID: root.ID,
		Type:        types.DepParentChild,
	}, "test"); err != nil {
		t.Fatalf("Failed to add parent-child dependency: %v", err)
	}

	// Load subgraph
	subgraph, err := loadTemplateSubgraph(ctx, s, root.ID)
	if err != nil {
		t.Fatalf("Failed to load subgraph: %v", err)
	}

	// Spawn with ephemeral=true
	result, err := spawnMolecule(ctx, s, subgraph, nil, "", "test", true, "wisp")
	if err != nil {
		t.Fatalf("spawnMolecule failed: %v", err)
	}

	// Verify all spawned issues have Ephemeral=true
	for oldID, newID := range result.IDMapping {
		spawned, err := s.GetIssue(ctx, newID)
		if err != nil {
			t.Fatalf("Failed to get spawned issue %s: %v", newID, err)
		}
		if !spawned.Ephemeral {
			t.Errorf("Spawned issue %s (from %s) should have Ephemeral=true, got false", newID, oldID)
		}
	}

	// Verify spawned issues have the correct prefix
	for _, newID := range result.IDMapping {
		if !strings.HasPrefix(newID, "test-wisp-") {
			t.Errorf("Spawned issue ID %s should have prefix 'test-wisp-'", newID)
		}
	}
}

// TestSpawnMoleculeFromFormulaEphemeral verifies that spawning from a cooked formula
// with ephemeral=true creates issues with the Ephemeral flag set (bd-phin)
func TestSpawnMoleculeFromFormulaEphemeral(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a minimal in-memory subgraph (simulating cookFormulaToSubgraph output)
	root := &types.Issue{
		ID:         "test-formula",
		Title:      "Test Formula",
		Status:     types.StatusOpen,
		Priority:   2,
		IssueType:  types.TypeEpic,
		IsTemplate: true,
	}
	step := &types.Issue{
		ID:         "test-formula.step1",
		Title:      "Step 1",
		Status:     types.StatusOpen,
		Priority:   2,
		IssueType:  types.TypeTask,
		IsTemplate: true,
	}

	subgraph := &TemplateSubgraph{
		Root:   root,
		Issues: []*types.Issue{root, step},
		Dependencies: []*types.Dependency{
			{
				IssueID:     step.ID,
				DependsOnID: root.ID,
				Type:        types.DepParentChild,
			},
		},
		IssueMap: map[string]*types.Issue{
			root.ID: root,
			step.ID: step,
		},
	}

	// Spawn with ephemeral=true (simulating bd mol wisp <formula>)
	result, err := spawnMolecule(ctx, s, subgraph, nil, "", "test", true, "wisp")
	if err != nil {
		t.Fatalf("spawnMolecule failed: %v", err)
	}

	// Verify all spawned issues have Ephemeral=true
	for oldID, newID := range result.IDMapping {
		spawned, err := s.GetIssue(ctx, newID)
		if err != nil {
			t.Fatalf("Failed to get spawned issue %s: %v", newID, err)
		}
		if !spawned.Ephemeral {
			t.Errorf("Spawned issue %s (from %s) should have Ephemeral=true, got false", newID, oldID)
		}
		t.Logf("Issue %s: Ephemeral=%v", newID, spawned.Ephemeral)
	}

	// Verify they have the correct prefix
	for _, newID := range result.IDMapping {
		if !strings.HasPrefix(newID, "test-wisp-") {
			t.Errorf("Spawned issue ID %s should have prefix 'test-wisp-'", newID)
		}
	}

	// Verify ephemeral issues are excluded from ready work
	readyWork, err := s.GetReadyWork(ctx, types.WorkFilter{})
	if err != nil {
		t.Fatalf("GetReadyWork failed: %v", err)
	}
	for _, issue := range readyWork {
		for _, spawnedID := range result.IDMapping {
			if issue.ID == spawnedID {
				t.Errorf("Ephemeral issue %s should not appear in ready work", spawnedID)
			}
		}
	}
}

// TestSpawnMolecule_PreservesStepMetadata verifies that a cooked formula with
// per-step metadata results in spawned issues whose Metadata field carries the
// declared keys through to the database. Regression for gastownhall/beads#3341.
func TestSpawnMolecule_PreservesStepMetadata(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "test"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	f := &formula.Formula{
		Formula: "meta-test",
		Version: 1,
		Type:    formula.TypeWorkflow,
		Steps: []*formula.Step{
			{
				ID:    "work",
				Title: "Do the work",
				Metadata: map[string]interface{}{
					"reminder_list": "brewery",
					"origin":        "repro",
				},
			},
		},
	}

	subgraph, err := cookFormulaToSubgraph(f, "meta-test")
	if err != nil {
		t.Fatalf("cookFormulaToSubgraph failed: %v", err)
	}

	result, err := spawnMolecule(ctx, s, subgraph, nil, "", "test", false, types.IDPrefixMol)
	if err != nil {
		t.Fatalf("spawnMolecule failed: %v", err)
	}

	newWorkID, ok := result.IDMapping["meta-test.work"]
	if !ok {
		t.Fatalf("result.IDMapping missing entry for meta-test.work; got %v", result.IDMapping)
	}
	spawned, err := s.GetIssue(ctx, newWorkID)
	if err != nil {
		t.Fatalf("GetIssue(%s) failed: %v", newWorkID, err)
	}
	if len(spawned.Metadata) == 0 {
		t.Fatalf("spawned issue %s has empty Metadata; want step metadata carried through", newWorkID)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(spawned.Metadata, &decoded); err != nil {
		t.Fatalf("spawned.Metadata is not valid JSON: %v (raw: %s)", err, string(spawned.Metadata))
	}
	if got := decoded["reminder_list"]; got != "brewery" {
		t.Errorf("spawned.Metadata[reminder_list] = %v, want \"brewery\"", got)
	}
	if got := decoded["origin"]; got != "repro" {
		t.Errorf("spawned.Metadata[origin] = %v, want \"repro\"", got)
	}
}

// TestPourRootTitleDescSubstitution verifies that the root molecule's title and description
// are substituted with {{title}} and {{desc}} variables when pouring a formula.
// This is a tracer bullet test for GitHub issue #852:
// https://github.com/steveyegge/beads/issues/852
func TestPourRootTitleDescSubstitution(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "mol"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a formula that has title and desc variables
	f := &formula.Formula{
		Formula:     "mol-task",
		Description: "Standard task workflow for 2-8 hour work...",
		Version:     1,
		Type:        formula.TypeWorkflow,
		Vars: map[string]*formula.VarDef{
			"title": {
				Description: "Task title",
				Required:    true,
			},
			"desc": {
				Description: "Task description",
				Required:    false,
				Default:     formula.StringPtr("No description provided"),
			},
		},
		Steps: []*formula.Step{
			{ID: "plan", Title: "Plan: {{title}}", Type: "task"},
			{ID: "implement", Title: "Implement: {{title}}", Type: "task", DependsOn: []string{"plan"}},
			{ID: "verify", Title: "Verify: {{title}}", Type: "task", DependsOn: []string{"implement"}},
			{ID: "review", Title: "Review: {{title}}", Type: "task", DependsOn: []string{"verify"}},
		},
	}

	// Cook the formula to a subgraph (in-memory, no DB)
	subgraph, err := cookFormulaToSubgraphWithVars(f, f.Formula, f.Vars)
	if err != nil {
		t.Fatalf("Failed to cook formula: %v", err)
	}

	// Spawn with title and desc variables
	vars := map[string]string{
		"title": "My Task",
		"desc":  "My description",
	}

	result, err := spawnMolecule(ctx, s, subgraph, vars, "", "test", false, types.IDPrefixMol)
	if err != nil {
		t.Fatalf("spawnMolecule failed: %v", err)
	}

	// Get the spawned root issue
	spawnedRoot, err := s.GetIssue(ctx, result.NewEpicID)
	if err != nil {
		t.Fatalf("Failed to get spawned root: %v", err)
	}

	// BUG: The root title should contain "My Task" but currently contains "mol-task"
	// because cookFormulaToSubgraph sets root.Title = f.Formula instead of using
	// a template that includes {{title}}.
	if !strings.Contains(spawnedRoot.Title, "My Task") {
		t.Errorf("Root title should contain 'My Task' from variable substitution, got: %q", spawnedRoot.Title)
	}

	// BUG: The root description should contain "My description" but currently
	// contains the formula's static description.
	if !strings.Contains(spawnedRoot.Description, "My description") {
		t.Errorf("Root description should contain 'My description' from variable substitution, got: %q", spawnedRoot.Description)
	}

	// Verify child beads DO have correct substitution (this should pass)
	for oldID, newID := range result.IDMapping {
		if oldID == f.Formula {
			continue // Skip root
		}
		spawned, err := s.GetIssue(ctx, newID)
		if err != nil {
			t.Fatalf("Failed to get spawned issue %s: %v", newID, err)
		}
		if !strings.Contains(spawned.Title, "My Task") {
			t.Errorf("Child issue %s (from %s) title should contain 'My Task', got: %q", newID, oldID, spawned.Title)
		}
	}
}

// TestPourRootTitleOnly verifies edge case: only title var defined, no desc.
// Root should use {{title}} for title, but keep formula description.
func TestPourRootTitleOnly(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "mol"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Formula with only title var (no desc)
	f := &formula.Formula{
		Formula:     "mol-simple",
		Description: "Static description that should be preserved",
		Version:     1,
		Type:        formula.TypeWorkflow,
		Vars: map[string]*formula.VarDef{
			"title": {Description: "Task title", Required: true},
		},
		Steps: []*formula.Step{
			{ID: "work", Title: "Do: {{title}}", Type: "task"},
		},
	}

	subgraph, err := cookFormulaToSubgraphWithVars(f, f.Formula, f.Vars)
	if err != nil {
		t.Fatalf("Failed to cook formula: %v", err)
	}

	vars := map[string]string{"title": "Custom Title"}
	result, err := spawnMolecule(ctx, s, subgraph, vars, "", "test", false, types.IDPrefixMol)
	if err != nil {
		t.Fatalf("spawnMolecule failed: %v", err)
	}

	spawnedRoot, err := s.GetIssue(ctx, result.NewEpicID)
	if err != nil {
		t.Fatalf("Failed to get spawned root: %v", err)
	}

	// Title should be substituted
	if !strings.Contains(spawnedRoot.Title, "Custom Title") {
		t.Errorf("Root title should contain 'Custom Title', got: %q", spawnedRoot.Title)
	}

	// Description should be the static formula description (no desc var)
	if spawnedRoot.Description != "Static description that should be preserved" {
		t.Errorf("Root description should be static formula desc, got: %q", spawnedRoot.Description)
	}
}

// TestPourRootNoVars verifies backward compatibility: no title/desc vars defined.
// Root should use formula name and formula description (original behavior).
func TestPourRootNoVars(t *testing.T) {
	ctx := context.Background()
	dbPath := t.TempDir() + "/test.db"
	s, err := dolt.New(ctx, &dolt.Config{Path: dbPath})
	if err != nil {
		testutil.SkipOrFailUnavailable(t, "skipping: Dolt server not available: %v", err)
	}
	defer s.Close()
	if err := s.SetConfig(ctx, "issue_prefix", "mol"); err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Formula with no title/desc vars (uses different var names)
	f := &formula.Formula{
		Formula:     "mol-release",
		Description: "Release workflow for version bumps",
		Version:     1,
		Type:        formula.TypeWorkflow,
		Vars: map[string]*formula.VarDef{
			"version": {Description: "Version number", Required: true},
		},
		Steps: []*formula.Step{
			{ID: "bump", Title: "Bump to {{version}}", Type: "task"},
			{ID: "tag", Title: "Tag {{version}}", Type: "task", DependsOn: []string{"bump"}},
		},
	}

	subgraph, err := cookFormulaToSubgraphWithVars(f, f.Formula, f.Vars)
	if err != nil {
		t.Fatalf("Failed to cook formula: %v", err)
	}

	vars := map[string]string{"version": "1.2.3"}
	result, err := spawnMolecule(ctx, s, subgraph, vars, "", "test", false, types.IDPrefixMol)
	if err != nil {
		t.Fatalf("spawnMolecule failed: %v", err)
	}

	spawnedRoot, err := s.GetIssue(ctx, result.NewEpicID)
	if err != nil {
		t.Fatalf("Failed to get spawned root: %v", err)
	}

	// Title should be formula name (no title var defined)
	if spawnedRoot.Title != "mol-release" {
		t.Errorf("Root title should be formula name 'mol-release', got: %q", spawnedRoot.Title)
	}

	// Description should be formula description (no desc var defined)
	if spawnedRoot.Description != "Release workflow for version bumps" {
		t.Errorf("Root description should be formula desc, got: %q", spawnedRoot.Description)
	}
}
