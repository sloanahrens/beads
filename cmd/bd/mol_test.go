//go:build cgo

package main

import (
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestParseDistillVar(t *testing.T) {
	tests := []struct {
		name           string
		varFlag        string
		searchableText string
		wantFind       string
		wantVar        string
		wantErr        bool
	}{
		{
			name:           "spawn-style: variable=value",
			varFlag:        "branch=feature-auth",
			searchableText: "Implement feature-auth login flow",
			wantFind:       "feature-auth",
			wantVar:        "branch",
			wantErr:        false,
		},
		{
			name:           "substitution-style: value=variable",
			varFlag:        "feature-auth=branch",
			searchableText: "Implement feature-auth login flow",
			wantFind:       "feature-auth",
			wantVar:        "branch",
			wantErr:        false,
		},
		{
			name:           "spawn-style with version number",
			varFlag:        "version=1.2.3",
			searchableText: "Release version 1.2.3 to production",
			wantFind:       "1.2.3",
			wantVar:        "version",
			wantErr:        false,
		},
		{
			name:           "both found - prefers spawn-style",
			varFlag:        "api=api",
			searchableText: "The api endpoint uses api keys",
			wantFind:       "api",
			wantVar:        "api",
			wantErr:        false,
		},
		{
			name:           "neither found - error",
			varFlag:        "foo=bar",
			searchableText: "Nothing matches here",
			wantFind:       "",
			wantVar:        "",
			wantErr:        true,
		},
		{
			name:           "empty left side - error",
			varFlag:        "=value",
			searchableText: "Some text with value",
			wantFind:       "",
			wantVar:        "",
			wantErr:        true,
		},
		{
			name:           "empty right side - error",
			varFlag:        "value=",
			searchableText: "Some text with value",
			wantFind:       "",
			wantVar:        "",
			wantErr:        true,
		},
		{
			name:           "no equals sign - error",
			varFlag:        "noequals",
			searchableText: "Some text",
			wantFind:       "",
			wantVar:        "",
			wantErr:        true,
		},
		{
			name:           "value with equals sign",
			varFlag:        "env=KEY=VALUE",
			searchableText: "Set KEY=VALUE in config",
			wantFind:       "KEY=VALUE",
			wantVar:        "env",
			wantErr:        false,
		},
		{
			name:           "partial match in longer word - finds it",
			varFlag:        "name=auth",
			searchableText: "authentication module",
			wantFind:       "auth",
			wantVar:        "name",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFind, gotVar, err := parseDistillVar(tt.varFlag, tt.searchableText)

			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDistillVar() expected error, got none")
				}
				return
			}

			if err != nil {
				t.Errorf("parseDistillVar() unexpected error: %v", err)
				return
			}

			if gotFind != tt.wantFind {
				t.Errorf("parseDistillVar() find = %q, want %q", gotFind, tt.wantFind)
			}
			if gotVar != tt.wantVar {
				t.Errorf("parseDistillVar() var = %q, want %q", gotVar, tt.wantVar)
			}
		})
	}
}

func TestCollectSubgraphText(t *testing.T) {
	// Create a simple subgraph for testing
	subgraph := &MoleculeSubgraph{
		Issues: []*types.Issue{
			{
				Title:       "Epic: Feature Auth",
				Description: "Implement authentication",
				Design:      "Use OAuth2",
			},
			{
				Title: "Add login endpoint",
				Notes: "See RFC 6749",
			},
		},
	}

	text := collectSubgraphText(subgraph)

	// Verify all fields are included
	expected := []string{
		"Epic: Feature Auth",
		"Implement authentication",
		"Use OAuth2",
		"Add login endpoint",
		"See RFC 6749",
	}

	for _, exp := range expected {
		if !strings.Contains(text, exp) {
			t.Errorf("collectSubgraphText() missing %q", exp)
		}
	}
}

func TestIsProto(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   bool
	}{
		{"with template label", []string{"template", "other"}, true},
		{"template only", []string{"template"}, true},
		{"no template label", []string{"bug", "feature"}, false},
		{"empty labels", []string{}, false},
		{"nil labels", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &types.Issue{Labels: tt.labels}
			got := isProto(issue)
			if got != tt.want {
				t.Errorf("isProto() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperandType(t *testing.T) {
	if got := operandType(true); got != "proto" {
		t.Errorf("operandType(true) = %q, want %q", got, "proto")
	}
	if got := operandType(false); got != "molecule" {
		t.Errorf("operandType(false) = %q, want %q", got, "molecule")
	}
}

func TestMinPriority(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{0, 3, 0},
		{3, 3, 3},
	}
	for _, tt := range tests {
		got := minPriority(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("minPriority(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestGenerateDigest(t *testing.T) {
	root := &types.Issue{
		Title: "Test Molecule",
	}
	children := []*types.Issue{
		{
			Title:       "Step 1",
			Description: "First step description",
			Status:      types.StatusClosed,
			CloseReason: "Done",
		},
		{
			Title:       "Step 2",
			Description: "Second step description that is longer",
			Status:      types.StatusInProgress,
		},
	}

	digest := generateDigest(root, children)

	// Verify structure
	if !strings.Contains(digest, "## Molecule Execution Summary") {
		t.Error("Digest should have summary header")
	}
	if !strings.Contains(digest, "Test Molecule") {
		t.Error("Digest should contain molecule title")
	}
	if !strings.Contains(digest, "**Steps**: 2") {
		t.Error("Digest should show step count")
	}
	if !strings.Contains(digest, "**Completed**: 1/2") {
		t.Error("Digest should show completion stats")
	}
	if !strings.Contains(digest, "**In Progress**: 1") {
		t.Error("Digest should show in-progress count")
	}
	if !strings.Contains(digest, "Step 1") {
		t.Error("Digest should list step titles")
	}
	if !strings.Contains(digest, "*Outcome: Done*") {
		t.Error("Digest should include close reasons")
	}
}

// TestSpawnAttachNonProtoError tests that attaching a non-proto fails validation
func TestSpawnAttachNonProtoError(t *testing.T) {
	// The isProto function is tested separately in TestIsProto
	// This test verifies the validation logic that would be used in runMolSpawn

	// Create a non-proto issue (no template label)
	issue := &types.Issue{
		Title:  "Not a proto",
		Status: types.StatusOpen,
		Labels: []string{"bug"}, // Not MoleculeLabel
	}

	if isProto(issue) {
		t.Error("isProto should return false for issue without template label")
	}

	// Issue with template label should pass
	protoIssue := &types.Issue{
		Title:  "A proto",
		Status: types.StatusOpen,
		Labels: []string{MoleculeLabel},
	}

	if !isProto(protoIssue) {
		t.Error("isProto should return true for issue with template label")
	}
}

// TestSpawnAttachDryRunOutput tests that dry-run includes attachment info
// This is a lighter test since dry-run is mainly a CLI output concern
func TestSpawnAttachDryRunOutput(t *testing.T) {
	// The dry-run logic in runMolSpawn outputs attachment info when len(attachments) > 0
	// We verify the data structures that would be used in dry-run

	type attachmentInfo struct {
		id       string
		title    string
		subgraph *MoleculeSubgraph
	}

	// Simulate the attachment info collection
	attachments := []attachmentInfo{
		{id: "test-1", title: "Attachment 1", subgraph: &MoleculeSubgraph{
			Issues: []*types.Issue{{Title: "Issue A"}, {Title: "Issue B"}},
		}},
		{id: "test-2", title: "Attachment 2", subgraph: &MoleculeSubgraph{
			Issues: []*types.Issue{{Title: "Issue C"}},
		}},
	}

	// Verify attachment count calculation (used in dry-run output)
	totalAttachmentIssues := 0
	for _, attach := range attachments {
		totalAttachmentIssues += len(attach.subgraph.Issues)
	}

	if totalAttachmentIssues != 3 {
		t.Errorf("Expected 3 total attachment issues, got %d", totalAttachmentIssues)
	}

	// Verify bond type would be included (sequential is default)
	attachType := types.BondTypeSequential
	if attachType != "sequential" {
		t.Errorf("Expected default attach type 'sequential', got %q", attachType)
	}
}

// TestGenerateBondedID tests the custom ID generation for dynamic bonding
func TestGenerateBondedID(t *testing.T) {
	tests := []struct {
		name     string
		oldID    string
		rootID   string
		opts     CloneOptions
		wantID   string
		wantErr  bool
		errMatch string
	}{
		{
			name:   "root issue with simple childRef",
			oldID:  "mol-arm",
			rootID: "mol-arm",
			opts: CloneOptions{
				ParentID: "patrol-x7k",
				ChildRef: "arm-ace",
			},
			wantID: "patrol-x7k.arm-ace",
		},
		{
			name:   "root issue with variable substitution",
			oldID:  "mol-arm",
			rootID: "mol-arm",
			opts: CloneOptions{
				ParentID: "patrol-x7k",
				ChildRef: "arm-{{polecat_name}}",
				Vars:     map[string]string{"polecat_name": "ace"},
			},
			wantID: "patrol-x7k.arm-ace",
		},
		{
			name:   "child issue with relative ID",
			oldID:  "mol-arm.capture",
			rootID: "mol-arm",
			opts: CloneOptions{
				ParentID: "patrol-x7k",
				ChildRef: "arm-ace",
			},
			wantID: "patrol-x7k.arm-ace.capture",
		},
		{
			name:   "nested child issue",
			oldID:  "mol-arm.capture.sub",
			rootID: "mol-arm",
			opts: CloneOptions{
				ParentID: "patrol-x7k",
				ChildRef: "arm-ace",
			},
			wantID: "patrol-x7k.arm-ace.capture.sub",
		},
		{
			name:   "no parent ID returns empty (not a bonded operation)",
			oldID:  "mol-arm",
			rootID: "mol-arm",
			opts:   CloneOptions{},
			wantID: "",
		},
		{
			name:   "empty childRef after substitution is error",
			oldID:  "mol-arm",
			rootID: "mol-arm",
			opts: CloneOptions{
				ParentID: "patrol-x7k",
				ChildRef: "{{missing_var}}",
			},
			wantErr:  true,
			errMatch: "invalid childRef",
		},
		{
			name:   "childRef with special chars is error",
			oldID:  "mol-arm",
			rootID: "mol-arm",
			opts: CloneOptions{
				ParentID: "patrol-x7k",
				ChildRef: "arm/ace",
			},
			wantErr:  true,
			errMatch: "invalid childRef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := generateBondedID(tt.oldID, tt.rootID, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("generateBondedID() expected error containing %q, got nil", tt.errMatch)
				} else if !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("generateBondedID() error = %q, want error containing %q", err.Error(), tt.errMatch)
				}
				return
			}

			if err != nil {
				t.Errorf("generateBondedID() unexpected error: %v", err)
				return
			}

			if gotID != tt.wantID {
				t.Errorf("generateBondedID() = %q, want %q", gotID, tt.wantID)
			}
		})
	}
}

// TestGetRelativeID tests extracting relative portion from child IDs
func TestGetRelativeID(t *testing.T) {
	tests := []struct {
		name   string
		oldID  string
		rootID string
		want   string
	}{
		{
			name:   "same ID returns empty",
			oldID:  "mol-arm",
			rootID: "mol-arm",
			want:   "",
		},
		{
			name:   "child with single step",
			oldID:  "mol-arm.capture",
			rootID: "mol-arm",
			want:   "capture",
		},
		{
			name:   "child with nested steps",
			oldID:  "mol-arm.capture.sub.deep",
			rootID: "mol-arm",
			want:   "capture.sub.deep",
		},
		{
			name:   "unrelated IDs returns empty",
			oldID:  "other-123",
			rootID: "mol-arm",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRelativeID(tt.oldID, tt.rootID)
			if got != tt.want {
				t.Errorf("getRelativeID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAnalyzeMoleculeParallelNoBlocking tests parallel detection with no blocking deps
func TestAnalyzeMoleculeParallelNoBlocking(t *testing.T) {
	// Create a simple molecule with parallel children (no blocking deps between them)
	root := &types.Issue{
		ID:        "mol-test",
		Title:     "Test Molecule",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
	}
	child1 := &types.Issue{
		ID:        "mol-test.step1",
		Title:     "Step 1",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}
	child2 := &types.Issue{
		ID:        "mol-test.step2",
		Title:     "Step 2",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}

	subgraph := &MoleculeSubgraph{
		Root:   root,
		Issues: []*types.Issue{root, child1, child2},
		IssueMap: map[string]*types.Issue{
			root.ID:   root,
			child1.ID: child1,
			child2.ID: child2,
		},
		Dependencies: []*types.Dependency{
			{IssueID: child1.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			{IssueID: child2.ID, DependsOnID: root.ID, Type: types.DepParentChild},
		},
	}

	analysis := analyzeMoleculeParallel(subgraph)

	// All 3 should be ready (root + 2 children with no blocking deps)
	if analysis.ReadySteps != 3 {
		t.Errorf("ReadySteps = %d, want 3", analysis.ReadySteps)
	}

	// Children should be in the same parallel group
	step1Info := analysis.Steps[child1.ID]
	step2Info := analysis.Steps[child2.ID]

	if step1Info.ParallelGroup == "" {
		t.Error("Step1 should be in a parallel group")
	}
	if step1Info.ParallelGroup != step2Info.ParallelGroup {
		t.Errorf("Step1 and Step2 should be in same parallel group: %s vs %s",
			step1Info.ParallelGroup, step2Info.ParallelGroup)
	}

	// Check can_parallel
	found := false
	for _, id := range step1Info.CanParallel {
		if id == child2.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Step1.CanParallel should contain Step2.ID")
	}
}

// TestAnalyzeMoleculeParallelWithBlocking tests parallel detection with blocking deps
func TestAnalyzeMoleculeParallelWithBlocking(t *testing.T) {
	// Create a sequential molecule: step1 blocks step2
	root := &types.Issue{
		ID:        "mol-seq",
		Title:     "Sequential Molecule",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
	}
	step1 := &types.Issue{
		ID:        "mol-seq.step1",
		Title:     "Step 1",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}
	step2 := &types.Issue{
		ID:        "mol-seq.step2",
		Title:     "Step 2 (blocked by Step 1)",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}

	subgraph := &MoleculeSubgraph{
		Root:   root,
		Issues: []*types.Issue{root, step1, step2},
		IssueMap: map[string]*types.Issue{
			root.ID:  root,
			step1.ID: step1,
			step2.ID: step2,
		},
		Dependencies: []*types.Dependency{
			{IssueID: step1.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			{IssueID: step2.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			{IssueID: step2.ID, DependsOnID: step1.ID, Type: types.DepBlocks}, // step2 blocked by step1
		},
	}

	analysis := analyzeMoleculeParallel(subgraph)

	// Only root and step1 should be ready (step2 is blocked)
	if analysis.ReadySteps != 2 {
		t.Errorf("ReadySteps = %d, want 2 (step2 blocked)", analysis.ReadySteps)
	}

	step1Info := analysis.Steps[step1.ID]
	step2Info := analysis.Steps[step2.ID]

	if !step1Info.IsReady {
		t.Error("Step1 should be ready")
	}
	if step2Info.IsReady {
		t.Error("Step2 should NOT be ready (blocked by step1)")
	}
	if len(step2Info.BlockedBy) != 1 || step2Info.BlockedBy[0] != step1.ID {
		t.Errorf("Step2.BlockedBy = %v, want [%s]", step2Info.BlockedBy, step1.ID)
	}

	// Step1 and Step2 should NOT be in the same parallel group
	if step1Info.ParallelGroup != "" && step1Info.ParallelGroup == step2Info.ParallelGroup {
		t.Error("Blocking steps should NOT be in the same parallel group")
	}
}

// TestAnalyzeMoleculeParallelCompletedBlockers tests that completed steps don't block
func TestAnalyzeMoleculeParallelCompletedBlockers(t *testing.T) {
	// Create molecule where step1 is completed, so step2 should be ready
	root := &types.Issue{
		ID:        "mol-done",
		Title:     "Molecule with completed step",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
	}
	step1 := &types.Issue{
		ID:        "mol-done.step1",
		Title:     "Step 1 (completed)",
		Status:    types.StatusClosed, // Completed!
		IssueType: types.TypeTask,
	}
	step2 := &types.Issue{
		ID:        "mol-done.step2",
		Title:     "Step 2 (depends on step1)",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}

	subgraph := &MoleculeSubgraph{
		Root:   root,
		Issues: []*types.Issue{root, step1, step2},
		IssueMap: map[string]*types.Issue{
			root.ID:  root,
			step1.ID: step1,
			step2.ID: step2,
		},
		Dependencies: []*types.Dependency{
			{IssueID: step1.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			{IssueID: step2.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			{IssueID: step2.ID, DependsOnID: step1.ID, Type: types.DepBlocks},
		},
	}

	analysis := analyzeMoleculeParallel(subgraph)

	step2Info := analysis.Steps[step2.ID]

	// Step2 should be ready since step1 is closed
	if !step2Info.IsReady {
		t.Error("Step2 should be ready (step1 is completed)")
	}
	if len(step2Info.BlockedBy) != 0 {
		t.Errorf("Step2.BlockedBy = %v, want empty (step1 completed)", step2Info.BlockedBy)
	}
}

func TestAnalyzeMoleculeParallelWaitsForChildrenOfSpawner(t *testing.T) {
	root := &types.Issue{
		ID:        "mol-fanout",
		Title:     "Fanout Molecule",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
	}
	implement := &types.Issue{
		ID:        "mol-fanout.implement",
		Title:     "Implement",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}
	otherSpawner := &types.Issue{
		ID:        "mol-fanout.other",
		Title:     "Other spawner",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}
	review := &types.Issue{
		ID:        "mol-fanout.review",
		Title:     "Review",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}
	implChild := &types.Issue{
		ID:        "mol-fanout.implement.arm-1",
		Title:     "Implement child",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}
	otherChild := &types.Issue{
		ID:        "mol-fanout.other.arm-1",
		Title:     "Other child",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}

	subgraph := &MoleculeSubgraph{
		Root:   root,
		Issues: []*types.Issue{root, implement, otherSpawner, review, implChild, otherChild},
		IssueMap: map[string]*types.Issue{
			root.ID:         root,
			implement.ID:    implement,
			otherSpawner.ID: otherSpawner,
			review.ID:       review,
			implChild.ID:    implChild,
			otherChild.ID:   otherChild,
		},
		Dependencies: []*types.Dependency{
			{IssueID: implement.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			{IssueID: otherSpawner.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			{IssueID: review.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			{IssueID: implChild.ID, DependsOnID: implement.ID, Type: types.DepParentChild},
			{IssueID: otherChild.ID, DependsOnID: otherSpawner.ID, Type: types.DepParentChild},
			{
				IssueID:     review.ID,
				DependsOnID: implement.ID,
				Type:        types.DepWaitsFor,
				Metadata:    `{"gate":"all-children"}`,
			},
		},
	}

	t.Run("blocked-before-child-close", func(t *testing.T) {
		analysis := analyzeMoleculeParallel(subgraph)
		reviewInfo := analysis.Steps[review.ID]
		if reviewInfo.IsReady {
			t.Fatalf("review should be blocked while %s is open", implChild.ID)
		}

		hasImplChildBlocker := false
		for _, blocker := range reviewInfo.BlockedBy {
			if blocker == implChild.ID {
				hasImplChildBlocker = true
			}
			if blocker == otherChild.ID {
				t.Fatalf("review should not be blocked by unrelated child %s", otherChild.ID)
			}
		}
		if !hasImplChildBlocker {
			t.Fatalf("expected review to be blocked by child of implement spawner")
		}
	})

	t.Run("ready-after-child-close", func(t *testing.T) {
		implChild.Status = types.StatusClosed
		analysisAfterClose := analyzeMoleculeParallel(subgraph)
		if !analysisAfterClose.Steps[review.ID].IsReady {
			t.Fatalf("review should become ready after %s closes", implChild.ID)
		}
	})
}

// TestAnalyzeMoleculeParallelMultipleArms tests parallel detection across bonded arms
func TestAnalyzeMoleculeParallelMultipleArms(t *testing.T) {
	// Create molecule with two arms that can run in parallel
	root := &types.Issue{
		ID:        "patrol",
		Title:     "Patrol",
		Status:    types.StatusOpen,
		IssueType: types.TypeEpic,
	}
	armAce := &types.Issue{
		ID:        "patrol.arm-ace",
		Title:     "Arm: ace",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}
	armNux := &types.Issue{
		ID:        "patrol.arm-nux",
		Title:     "Arm: nux",
		Status:    types.StatusOpen,
		IssueType: types.TypeTask,
	}

	subgraph := &MoleculeSubgraph{
		Root:   root,
		Issues: []*types.Issue{root, armAce, armNux},
		IssueMap: map[string]*types.Issue{
			root.ID:   root,
			armAce.ID: armAce,
			armNux.ID: armNux,
		},
		Dependencies: []*types.Dependency{
			{IssueID: armAce.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			{IssueID: armNux.ID, DependsOnID: root.ID, Type: types.DepParentChild},
			// No blocking deps between arms
		},
	}

	analysis := analyzeMoleculeParallel(subgraph)

	// All 3 should be ready
	if analysis.ReadySteps != 3 {
		t.Errorf("ReadySteps = %d, want 3", analysis.ReadySteps)
	}

	// Arms should be in the same parallel group
	aceInfo := analysis.Steps[armAce.ID]
	nuxInfo := analysis.Steps[armNux.ID]

	if aceInfo.ParallelGroup == "" {
		t.Error("arm-ace should be in a parallel group")
	}
	if aceInfo.ParallelGroup != nuxInfo.ParallelGroup {
		t.Errorf("Arms should be in same parallel group: %s vs %s",
			aceInfo.ParallelGroup, nuxInfo.ParallelGroup)
	}

	// Should have at least one parallel group with both arms
	foundGroup := false
	for _, members := range analysis.ParallelGroups {
		hasAce := false
		hasNux := false
		for _, id := range members {
			if id == armAce.ID {
				hasAce = true
			}
			if id == armNux.ID {
				hasNux = true
			}
		}
		if hasAce && hasNux {
			foundGroup = true
			break
		}
	}
	if !foundGroup {
		t.Error("Should have a parallel group containing both arms")
	}
}

// TestCalculateBlockingDepths tests the depth calculation
func TestCalculateBlockingDepths(t *testing.T) {
	// Create chain: root -> step1 -> step2 -> step3
	root := &types.Issue{ID: "root", Status: types.StatusOpen}
	step1 := &types.Issue{ID: "step1", Status: types.StatusOpen}
	step2 := &types.Issue{ID: "step2", Status: types.StatusOpen}
	step3 := &types.Issue{ID: "step3", Status: types.StatusOpen}

	subgraph := &MoleculeSubgraph{
		Root:     root,
		Issues:   []*types.Issue{root, step1, step2, step3},
		IssueMap: map[string]*types.Issue{"root": root, "step1": step1, "step2": step2, "step3": step3},
	}

	blockedBy := map[string]map[string]bool{
		"root":  {},
		"step1": {"root": true},
		"step2": {"step1": true},
		"step3": {"step2": true},
	}

	depths := calculateBlockingDepths(subgraph, blockedBy)

	if depths["root"] != 0 {
		t.Errorf("root depth = %d, want 0", depths["root"])
	}
	if depths["step1"] != 1 {
		t.Errorf("step1 depth = %d, want 1", depths["step1"])
	}
	if depths["step2"] != 2 {
		t.Errorf("step2 depth = %d, want 2", depths["step2"])
	}
	if depths["step3"] != 3 {
		t.Errorf("step3 depth = %d, want 3", depths["step3"])
	}
}

// TestCompoundMoleculeVisualization tests the compound molecule display in mol show
func TestCompoundMoleculeVisualization(t *testing.T) {
	// Test IsCompound() and GetConstituents()
	tests := []struct {
		name          string
		bondedFrom    []types.BondRef
		isCompound    bool
		expectedCount int
	}{
		{
			name:          "not a compound - no BondedFrom",
			bondedFrom:    nil,
			isCompound:    false,
			expectedCount: 0,
		},
		{
			name:          "not a compound - empty BondedFrom",
			bondedFrom:    []types.BondRef{},
			isCompound:    false,
			expectedCount: 0,
		},
		{
			name: "compound with one constituent",
			bondedFrom: []types.BondRef{
				{SourceID: "proto-a", BondType: types.BondTypeSequential},
			},
			isCompound:    true,
			expectedCount: 1,
		},
		{
			name: "compound with two constituents - sequential bond",
			bondedFrom: []types.BondRef{
				{SourceID: "proto-a", BondType: types.BondTypeSequential},
				{SourceID: "proto-b", BondType: types.BondTypeSequential},
			},
			isCompound:    true,
			expectedCount: 2,
		},
		{
			name: "compound with parallel bond",
			bondedFrom: []types.BondRef{
				{SourceID: "proto-a", BondType: types.BondTypeParallel},
				{SourceID: "proto-b", BondType: types.BondTypeParallel},
			},
			isCompound:    true,
			expectedCount: 2,
		},
		{
			name: "compound with bond point",
			bondedFrom: []types.BondRef{
				{SourceID: "proto-a", BondType: types.BondTypeSequential, BondPoint: "step-2"},
			},
			isCompound:    true,
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &types.Issue{
				ID:         "test-compound",
				Title:      "Test Compound Molecule",
				BondedFrom: tt.bondedFrom,
			}

			if got := issue.IsCompound(); got != tt.isCompound {
				t.Errorf("IsCompound() = %v, want %v", got, tt.isCompound)
			}

			constituents := issue.GetConstituents()
			if len(constituents) != tt.expectedCount {
				t.Errorf("GetConstituents() returned %d items, want %d", len(constituents), tt.expectedCount)
			}
		})
	}
}

// TestFormatBondType tests the formatBondType helper function
func TestFormatBondType(t *testing.T) {
	tests := []struct {
		bondType string
		expected string
	}{
		{types.BondTypeSequential, "sequential"},
		{types.BondTypeParallel, "parallel"},
		{types.BondTypeConditional, "on-failure"},
		{types.BondTypeRoot, "root"},
		{"", "default"},
		{"custom-type", "custom-type"},
	}

	for _, tt := range tests {
		t.Run(tt.bondType, func(t *testing.T) {
			if got := formatBondType(tt.bondType); got != tt.expected {
				t.Errorf("formatBondType(%q) = %q, want %q", tt.bondType, got, tt.expected)
			}
		})
	}
}
