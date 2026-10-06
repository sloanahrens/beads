//go:build cgo

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	storageissueops "github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
)

func TestDepCommandsInit(t *testing.T) {
	if depCmd == nil {
		t.Fatal("depCmd should be initialized")
	}

	if depCmd.Use != "dep [issue-id]" {
		t.Errorf("Expected Use='dep [issue-id]', got %q", depCmd.Use)
	}

	if depAddCmd == nil {
		t.Fatal("depAddCmd should be initialized")
	}

	if depRemoveCmd == nil {
		t.Fatal("depRemoveCmd should be initialized")
	}
}

func TestDepAddFlagAliases(t *testing.T) {
	// Test that --blocked-by flag exists on depAddCmd
	blockedByFlag := depAddCmd.Flags().Lookup("blocked-by")
	if blockedByFlag == nil {
		t.Fatal("depAddCmd should have --blocked-by flag")
	}
	if blockedByFlag.DefValue != "" {
		t.Errorf("Expected default blocked-by='', got %q", blockedByFlag.DefValue)
	}

	// Test that --depends-on flag exists on depAddCmd
	dependsOnFlag := depAddCmd.Flags().Lookup("depends-on")
	if dependsOnFlag == nil {
		t.Fatal("depAddCmd should have --depends-on flag")
	}
	if dependsOnFlag.DefValue != "" {
		t.Errorf("Expected default depends-on='', got %q", dependsOnFlag.DefValue)
	}

	// Verify the help text mentions the flags
	longDesc := depAddCmd.Long
	if !strings.Contains(longDesc, "--blocked-by") {
		t.Error("Expected Long description to mention --blocked-by flag")
	}
	if !strings.Contains(longDesc, "--depends-on") {
		t.Error("Expected Long description to mention --depends-on flag")
	}
	if fileFlag := depAddCmd.Flags().Lookup("file"); fileFlag == nil {
		t.Fatal("depAddCmd should have --file flag")
	} else if fileFlag.DefValue != "" {
		t.Errorf("Expected default file='', got %q", fileFlag.DefValue)
	}
	if !strings.Contains(longDesc, "--file") {
		t.Error("Expected Long description to mention --file flag")
	}
}

func TestDepBlocksFlag(t *testing.T) {
	// Test that the --blocks flag exists on depCmd
	flag := depCmd.Flags().Lookup("blocks")
	if flag == nil {
		t.Fatal("depCmd should have --blocks flag")
	}

	// Test shorthand is -b
	if flag.Shorthand != "b" {
		t.Errorf("Expected shorthand='b', got %q", flag.Shorthand)
	}

	// Test default value is empty string
	if flag.DefValue != "" {
		t.Errorf("Expected default blocks='', got %q", flag.DefValue)
	}

	// Test usage text
	if !strings.Contains(flag.Usage, "blocks") {
		t.Errorf("Expected flag usage to mention 'blocks', got %q", flag.Usage)
	}
}

func TestDepTreeFormatFlag(t *testing.T) {
	// Test that the --format flag exists on depTreeCmd
	flag := depTreeCmd.Flags().Lookup("format")
	if flag == nil {
		t.Fatal("depTreeCmd should have --format flag")
	}

	// Test default value is empty string
	if flag.DefValue != "" {
		t.Errorf("Expected default format='', got %q", flag.DefValue)
	}

	// Test usage text mentions mermaid
	if !strings.Contains(flag.Usage, "mermaid") {
		t.Errorf("Expected flag usage to mention 'mermaid', got %q", flag.Usage)
	}
}

func TestGetStatusEmoji(t *testing.T) {
	tests := []struct {
		status types.Status
		want   string
	}{
		{types.StatusOpen, "☐"},
		{types.StatusInProgress, "◧"},
		{types.StatusBlocked, "⚠"},
		{types.StatusClosed, "☑"},
		{types.Status("unknown"), "?"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := getStatusEmoji(tt.status)
			if got != tt.want {
				t.Errorf("getStatusEmoji(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestOutputMermaidTree(t *testing.T) {
	tests := []struct {
		name   string
		tree   []*types.TreeNode
		rootID string
		want   []string // Lines that must appear in output
	}{
		{
			name:   "empty tree",
			tree:   []*types.TreeNode{},
			rootID: "test-1",
			want: []string{
				"flowchart TD",
				`test-1["No dependencies"]`,
			},
		},
		{
			name: "single dependency",
			tree: []*types.TreeNode{
				{
					Issue:    types.Issue{ID: "test-1", Title: "Task 1", Status: types.StatusInProgress},
					Depth:    0,
					ParentID: "",
				},
				{
					Issue:    types.Issue{ID: "test-2", Title: "Task 2", Status: types.StatusClosed},
					Depth:    1,
					ParentID: "test-1",
				},
			},
			rootID: "test-1",
			want: []string{
				"flowchart TD",
				`test-1["◧ test-1: Task 1"]`,
				`test-2["☑ test-2: Task 2"]`,
				"test-1 --> test-2",
			},
		},
		{
			name: "multiple dependencies",
			tree: []*types.TreeNode{
				{
					Issue:    types.Issue{ID: "test-1", Title: "Main", Status: types.StatusOpen},
					Depth:    0,
					ParentID: "",
				},
				{
					Issue:    types.Issue{ID: "test-2", Title: "Sub 1", Status: types.StatusClosed},
					Depth:    1,
					ParentID: "test-1",
				},
				{
					Issue:    types.Issue{ID: "test-3", Title: "Sub 2", Status: types.StatusBlocked},
					Depth:    1,
					ParentID: "test-1",
				},
			},
			rootID: "test-1",
			want: []string{
				"flowchart TD",
				`test-1["☐ test-1: Main"]`,
				`test-2["☑ test-2: Sub 1"]`,
				`test-3["⚠ test-3: Sub 2"]`,
				"test-1 --> test-2",
				"test-1 --> test-3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			defer func() { os.Stdout = old }()

			outputMermaidTree(tt.tree, tt.rootID)

			w.Close()

			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			// Verify all expected lines appear
			for _, line := range tt.want {
				if !strings.Contains(output, line) {
					t.Errorf("expected output to contain %q, got:\n%s", line, output)
				}
			}
		})
	}
}

func TestOutputMermaidTree_Siblings(t *testing.T) {
	// Test case: Siblings with children (reproduces issue with wrong parent inference)
	// Structure:
	//   BD-1 (root)
	//   ├── BD-2 (sibling 1)
	//   │   └── BD-4 (child of BD-2)
	//   └── BD-3 (sibling 2)
	//       └── BD-5 (child of BD-3)
	tree := []*types.TreeNode{
		{
			Issue:    types.Issue{ID: "BD-1", Title: "Parent", Status: types.StatusOpen},
			Depth:    0,
			ParentID: "",
		},
		{
			Issue:    types.Issue{ID: "BD-2", Title: "Sibling 1", Status: types.StatusOpen},
			Depth:    1,
			ParentID: "BD-1",
		},
		{
			Issue:    types.Issue{ID: "BD-3", Title: "Sibling 2", Status: types.StatusOpen},
			Depth:    1,
			ParentID: "BD-1",
		},
		{
			Issue:    types.Issue{ID: "BD-4", Title: "Child of Sibling 1", Status: types.StatusOpen},
			Depth:    2,
			ParentID: "BD-2",
		},
		{
			Issue:    types.Issue{ID: "BD-5", Title: "Child of Sibling 2", Status: types.StatusOpen},
			Depth:    2,
			ParentID: "BD-3",
		},
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	outputMermaidTree(tree, "BD-1")

	w.Close()

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify correct edges exist
	correctEdges := []string{
		"BD-1 --> BD-2",
		"BD-1 --> BD-3",
		"BD-2 --> BD-4",
		"BD-3 --> BD-5",
	}

	for _, edge := range correctEdges {
		if !strings.Contains(output, edge) {
			t.Errorf("expected edge %q to be present, got:\n%s", edge, output)
		}
	}

	// Verify incorrect edges do NOT exist (siblings shouldn't be connected)
	incorrectEdges := []string{
		"BD-2 --> BD-3", // Siblings shouldn't be connected
		"BD-3 --> BD-4", // BD-4's parent is BD-2, not BD-3
		"BD-4 --> BD-3", // Wrong direction
		"BD-4 --> BD-5", // These are cousins, not parent-child
	}

	for _, edge := range incorrectEdges {
		if strings.Contains(output, edge) {
			t.Errorf("incorrect edge %q should NOT be present, got:\n%s", edge, output)
		}
	}
}

func TestDepTreeDirectionFlag(t *testing.T) {
	// Test that the --direction flag exists on depTreeCmd
	flag := depTreeCmd.Flags().Lookup("direction")
	if flag == nil {
		t.Fatal("depTreeCmd should have --direction flag")
	}

	// Test default value is empty string (will default to "down")
	if flag.DefValue != "" {
		t.Errorf("Expected default direction='', got %q", flag.DefValue)
	}

	// Test usage text mentions valid options
	usage := flag.Usage
	if !strings.Contains(usage, "down") || !strings.Contains(usage, "up") || !strings.Contains(usage, "both") {
		t.Errorf("Expected flag usage to mention 'down', 'up', 'both', got %q", usage)
	}
}

func TestDepTreeStatusFlag(t *testing.T) {
	// Test that the --status flag exists on depTreeCmd
	flag := depTreeCmd.Flags().Lookup("status")
	if flag == nil {
		t.Fatal("depTreeCmd should have --status flag")
	}

	// Test default value is empty string
	if flag.DefValue != "" {
		t.Errorf("Expected default status='', got %q", flag.DefValue)
	}
}

func TestFormatTreeNode(t *testing.T) {
	tests := []struct {
		name     string
		node     *types.TreeNode
		contains []string
	}{
		{
			name: "open issue at depth 0 shows READY",
			node: &types.TreeNode{
				Issue: types.Issue{
					ID:       "BD-1",
					Title:    "Test Issue",
					Status:   types.StatusOpen,
					Priority: 2,
				},
				Depth: 0,
			},
			contains: []string{"BD-1", "Test Issue", "P2", "open", "[READY]"},
		},
		{
			name: "open issue at depth 1 does not show READY",
			node: &types.TreeNode{
				Issue: types.Issue{
					ID:       "BD-2",
					Title:    "Child Issue",
					Status:   types.StatusOpen,
					Priority: 1,
				},
				Depth: 1,
			},
			contains: []string{"BD-2", "Child Issue", "P1", "open"},
		},
		{
			name: "closed issue",
			node: &types.TreeNode{
				Issue: types.Issue{
					ID:       "BD-3",
					Title:    "Done Issue",
					Status:   types.StatusClosed,
					Priority: 3,
				},
				Depth: 0,
			},
			contains: []string{"BD-3", "Done Issue", "P3", "closed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTreeNode(tt.node, false)
			for _, want := range tt.contains {
				if !strings.Contains(result, want) {
					t.Errorf("formatTreeNode() = %q, want to contain %q", result, want)
				}
			}

			// For non-root open issues, verify READY is NOT shown
			if tt.node.Status == types.StatusOpen && tt.node.Depth > 0 {
				if strings.Contains(result, "[READY]") {
					t.Errorf("formatTreeNode() = %q, should NOT contain [READY] for depth > 0", result)
				}
			}
		})
	}

	// Test that blocked root shows [BLOCKED] instead of [READY]
	t.Run("blocked root shows BLOCKED not READY", func(t *testing.T) {
		node := &types.TreeNode{
			Issue: types.Issue{
				ID:       "BD-10",
				Title:    "Blocked Root",
				Status:   types.StatusOpen,
				Priority: 1,
			},
			Depth: 0,
		}
		result := formatTreeNode(node, true)
		if strings.Contains(result, "[READY]") {
			t.Errorf("blocked root should not show [READY], got: %q", result)
		}
		if !strings.Contains(result, "[BLOCKED]") {
			t.Errorf("blocked root should show [BLOCKED], got: %q", result)
		}
	})
}

func TestFormatTreeNodeShowsDependencyType(t *testing.T) {
	tests := []struct {
		name string
		node *types.TreeNode
		want string
	}{
		{
			name: "blocks edge",
			node: &types.TreeNode{
				Issue:          types.Issue{ID: "BD-2", Title: "Blocked task", Status: types.StatusOpen, Priority: 1},
				Depth:          1,
				ParentID:       "BD-1",
				EdgeFromParent: types.DepBlocks,
			},
			want: "[blocks]",
		},
		{
			name: "parent-child edge",
			node: &types.TreeNode{
				Issue:          types.Issue{ID: "BD-3", Title: "Child task", Status: types.StatusOpen, Priority: 2},
				Depth:          1,
				ParentID:       "BD-1",
				EdgeFromParent: types.DepParentChild,
			},
			want: "[parent-child]",
		},
		{
			name: "root has no edge label",
			node: &types.TreeNode{
				Issue:          types.Issue{ID: "BD-1", Title: "Root", Status: types.StatusOpen, Priority: 0},
				Depth:          0,
				EdgeFromParent: types.DepBlocks,
			},
			want: "[blocks]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTreeNode(tt.node, false)
			if tt.node.Depth == 0 {
				if strings.Contains(got, tt.want) {
					t.Fatalf("root node should not show dependency label %q: %s", tt.want, got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("formatTreeNode() = %q, want dependency label %q", got, tt.want)
			}
		})
	}
}

func TestRenderTreeOutput(t *testing.T) {
	// Test tree with proper connectors
	tree := []*types.TreeNode{
		{
			Issue:    types.Issue{ID: "BD-1", Title: "Root", Status: types.StatusOpen, Priority: 1},
			Depth:    0,
			ParentID: "",
		},
		{
			Issue:    types.Issue{ID: "BD-2", Title: "Child 1", Status: types.StatusOpen, Priority: 2},
			Depth:    1,
			ParentID: "BD-1",
		},
		{
			Issue:    types.Issue{ID: "BD-3", Title: "Child 2", Status: types.StatusClosed, Priority: 2},
			Depth:    1,
			ParentID: "BD-1",
		},
		{
			Issue:    types.Issue{ID: "BD-4", Title: "Grandchild", Status: types.StatusOpen, Priority: 3},
			Depth:    2,
			ParentID: "BD-2",
		},
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	renderTree(tree, 50, "down")

	w.Close()

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Check for tree connectors
	if !strings.Contains(output, "├──") && !strings.Contains(output, "└──") {
		t.Errorf("Expected tree connectors (├── or └──) in output, got:\n%s", output)
	}

	// Check that all nodes are present
	for _, node := range tree {
		if !strings.Contains(output, node.ID) {
			t.Errorf("Expected node %s in output, got:\n%s", node.ID, output)
		}
	}
}

func TestRenderTreeExternalBlockerMarksRootBlocked(t *testing.T) {
	tree := []*types.TreeNode{
		{
			Issue: types.Issue{ID: "BD-root", Title: "Root", Status: types.StatusOpen, Priority: 1},
		},
		{
			Issue:          types.Issue{ID: "external:remote:payments", Title: "○ payments", Status: types.StatusOpen},
			Depth:          1,
			ParentID:       "BD-root",
			EdgeFromParent: types.DepBlocks,
		},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	renderTree(tree, 50, "down")
	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()
	if !strings.Contains(output, "[BLOCKED]") || strings.Contains(output, "[READY]") {
		t.Fatalf("external blocker should mark root blocked, got:\n%s", output)
	}
}

func TestRenderTreeOutputShowsDependencyTypeLabelsInMixedGraph(t *testing.T) {
	downTree := []*types.TreeNode{
		{
			Issue:    types.Issue{ID: "BD-root", Title: "Root", Status: types.StatusOpen, Priority: 1},
			Depth:    0,
			ParentID: "",
		},
		{
			Issue:          types.Issue{ID: "BD-child", Title: "Child", Status: types.StatusOpen, Priority: 2},
			Depth:          1,
			ParentID:       "BD-root",
			EdgeFromParent: types.DepParentChild,
		},
	}
	upTree := []*types.TreeNode{
		{
			Issue:    types.Issue{ID: "BD-root", Title: "Root", Status: types.StatusOpen, Priority: 1},
			Depth:    0,
			ParentID: "",
		},
		{
			Issue:          types.Issue{ID: "BD-dependent", Title: "Dependent", Status: types.StatusOpen, Priority: 3},
			Depth:          1,
			ParentID:       "BD-root",
			EdgeFromParent: types.DepBlocks,
		},
	}
	tree := storageissueops.MergeBidirectionalTree(downTree, upTree, "BD-root")

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	renderTree(tree, 3, "both")

	w.Close()

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	for _, want := range []string{"BD-dependent", "[blocks]", "BD-child", "[parent-child]"} {
		if !strings.Contains(output, want) {
			t.Fatalf("expected mixed graph output to contain %q, got:\n%s", want, output)
		}
	}
}

func TestTreeNodeJSONIncludesEdgeFromParent(t *testing.T) {
	node := types.TreeNode{
		Issue:          types.Issue{ID: "BD-child", Title: "Child", Status: types.StatusOpen, Priority: 2},
		Depth:          1,
		ParentID:       "BD-root",
		EdgeFromParent: types.DepParentChild,
	}

	got, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("json.Marshal(TreeNode): %v", err)
	}

	if !strings.Contains(string(got), `"edge_from_parent":"parent-child"`) {
		t.Fatalf("TreeNode JSON missing edge_from_parent: %s", got)
	}
}

// Tests for child→parent dependency detection (bd-nim5)
// ============================================================================
// Foreign Key Error Tests (GH#952 Issue 4)
// ============================================================================
//
// These tests verify that foreign key constraint violations produce
// user-friendly error messages instead of raw database errors.
//
// Expected behavior:
//   - Error should say "issue X or Y not found" (user-friendly)
//   - Error should NOT say "FOREIGN KEY constraint failed" (raw database error)
//
// TRACER BULLET FINDING (Phase 1):
//   The storage layer (dependencies.go) already validates issue existence
//   BEFORE inserting into the database, so FK constraint errors don't occur
//   at the storage layer. Tests PASS because AddDependency returns proper
//   "not found" errors.
//
// If bugs exist, they would be in:
//   1. CLI layer (dep.go) - when ResolvePartialID has edge cases
//   2. Daemon RPC layer - if ID resolution behaves differently
//   3. Race conditions - issue deleted between resolve and add
//
// These tests serve as regression tests ensuring the storage layer
// continues to provide user-friendly error messages.

func TestIsChildOf(t *testing.T) {
	tests := []struct {
		name     string
		childID  string
		parentID string
		want     bool
	}{
		// Positive cases: should be detected as child
		{
			name:     "direct child",
			childID:  "bd-abc.1",
			parentID: "bd-abc",
			want:     true,
		},
		{
			name:     "grandchild",
			childID:  "bd-abc.1.2",
			parentID: "bd-abc",
			want:     true,
		},
		{
			name:     "nested grandchild direct parent",
			childID:  "bd-abc.1.2",
			parentID: "bd-abc.1",
			want:     true,
		},
		{
			name:     "deeply nested child",
			childID:  "bd-abc.1.2.3",
			parentID: "bd-abc",
			want:     true,
		},

		// Negative cases: should NOT be detected as child
		{
			name:     "same ID",
			childID:  "bd-abc",
			parentID: "bd-abc",
			want:     false,
		},
		{
			name:     "not a child - unrelated IDs",
			childID:  "bd-xyz",
			parentID: "bd-abc",
			want:     false,
		},
		{
			name:     "not a child - sibling",
			childID:  "bd-abc.2",
			parentID: "bd-abc.1",
			want:     false,
		},
		{
			name:     "reversed - parent is not child of child",
			childID:  "bd-abc",
			parentID: "bd-abc.1",
			want:     false,
		},
		{
			name:     "prefix but not hierarchical",
			childID:  "bd-abcd",
			parentID: "bd-abc",
			want:     false,
		},
		{
			name:     "not hierarchical ID",
			childID:  "bd-abc",
			parentID: "bd-xyz",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isChildOf(tt.childID, tt.parentID)
			if got != tt.want {
				t.Errorf("isChildOf(%q, %q) = %v, want %v", tt.childID, tt.parentID, got, tt.want)
			}
		})
	}
}
