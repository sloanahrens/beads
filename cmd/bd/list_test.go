//go:build cgo

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
)

// listTestHelper provides test setup and assertion methods
type listTestHelper struct {
	t      *testing.T
	ctx    context.Context
	store  *dolt.DoltStore
	issues []*types.Issue
}

func newListTestHelper(t *testing.T, store *dolt.DoltStore) *listTestHelper {
	return &listTestHelper{t: t, ctx: context.Background(), store: store}
}

func (h *listTestHelper) createTestIssues() {
	now := time.Now()
	h.issues = []*types.Issue{
		{
			Title:       "Bug Issue",
			Description: "Test bug",
			Priority:    0,
			IssueType:   types.TypeBug,
			Status:      types.StatusOpen,
		},
		{
			Title:       "Feature Issue",
			Description: "Test feature",
			Priority:    1,
			IssueType:   types.TypeFeature,
			Status:      types.StatusInProgress,
			Assignee:    testUserAlice,
		},
		{
			Title:       "Task Issue",
			Description: "Test task",
			Priority:    2,
			IssueType:   types.TypeTask,
			Status:      types.StatusClosed,
			ClosedAt:    &now,
		},
	}
	for _, issue := range h.issues {
		if err := h.store.CreateIssue(h.ctx, issue, "test-user"); err != nil {
			h.t.Fatalf("Failed to create issue: %v", err)
		}
	}
}

func (h *listTestHelper) addLabel(id, label string) {
	if err := h.store.AddLabel(h.ctx, id, label, "test-user"); err != nil {
		h.t.Fatalf("Failed to add label: %v", err)
	}
}

func (h *listTestHelper) search(filter types.IssueFilter) []*types.Issue {
	results, err := h.store.SearchIssues(h.ctx, "", filter)
	if err != nil {
		h.t.Fatalf("Failed to search issues: %v", err)
	}
	return results
}

func (h *listTestHelper) assertCount(count, expected int, desc string) {
	if count != expected {
		h.t.Errorf("Expected %d %s, got %d", expected, desc, count)
	}
}

func (h *listTestHelper) assertEqual(expected, actual interface{}, field string) {
	if expected != actual {
		h.t.Errorf("Expected %s %v, got %v", field, expected, actual)
	}
}

func (h *listTestHelper) assertAtMost(count, maxCount int, desc string) {
	if count > maxCount {
		h.t.Errorf("Expected at most %d %s, got %d", maxCount, desc, count)
	}
}

// Helper function to compare string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFormatIssueLong(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		issue  *types.Issue
		labels []string
		want   string // substring to check for
	}{
		{
			name: "open issue",
			issue: &types.Issue{
				ID:        "test-123",
				Title:     "Test Issue",
				Priority:  1,
				IssueType: types.TypeBug,
				Status:    types.StatusOpen,
			},
			labels: nil,
			want:   "test-123",
		},
		{
			name: "closed issue",
			issue: &types.Issue{
				ID:        "test-456",
				Title:     "Closed Issue",
				Priority:  0,
				IssueType: types.TypeTask,
				Status:    types.StatusClosed,
			},
			labels: nil,
			want:   "test-456",
		},
		{
			name: "issue with assignee",
			issue: &types.Issue{
				ID:        "test-789",
				Title:     "Assigned Issue",
				Priority:  2,
				IssueType: types.TypeFeature,
				Status:    types.StatusInProgress,
				Assignee:  "alice",
			},
			labels: nil,
			want:   "Assignee: alice",
		},
		{
			name: "issue with labels",
			issue: &types.Issue{
				ID:        "test-abc",
				Title:     "Labeled Issue",
				Priority:  1,
				IssueType: types.TypeBug,
				Status:    types.StatusOpen,
			},
			labels: []string{"critical", "security"},
			want:   "Labels:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			formatIssueLong(&buf, tt.issue, tt.labels, false)
			result := buf.String()
			if !strings.Contains(result, tt.want) {
				t.Errorf("formatIssueLong() = %q, want to contain %q", result, tt.want)
			}
		})
	}
}

func TestFormatIssueCompact(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		issue  *types.Issue
		labels []string
		want   string
	}{
		{
			name: "basic issue",
			issue: &types.Issue{
				ID:        "test-123",
				Title:     "Test Issue",
				Priority:  1,
				IssueType: types.TypeBug,
				Status:    types.StatusOpen,
			},
			labels: nil,
			want:   "Test Issue",
		},
		{
			name: "issue with assignee",
			issue: &types.Issue{
				ID:        "test-456",
				Title:     "Assigned Issue",
				Priority:  2,
				IssueType: types.TypeTask,
				Status:    types.StatusInProgress,
				Assignee:  "bob",
			},
			labels: nil,
			want:   "@bob",
		},
		{
			name: "issue with labels",
			issue: &types.Issue{
				ID:        "test-789",
				Title:     "Labeled Issue",
				Priority:  0,
				IssueType: types.TypeFeature,
				Status:    types.StatusOpen,
			},
			labels: []string{"urgent"},
			want:   "[urgent]",
		},
		{
			name: "closed issue",
			issue: &types.Issue{
				ID:        "test-def",
				Title:     "Closed Issue",
				Priority:  3,
				IssueType: types.TypeTask,
				Status:    types.StatusClosed,
			},
			labels: nil,
			want:   "Closed Issue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			formatIssueCompact(&buf, tt.issue, tt.labels, nil, nil, "")
			result := buf.String()
			if !strings.Contains(result, tt.want) {
				t.Errorf("formatIssueCompact() = %q, want to contain %q", result, tt.want)
			}
		})
	}
}

func TestBuildBlockingMaps(t *testing.T) {
	t.Parallel()
	// Create test dependency records
	allDeps := map[string][]*types.Dependency{
		"issue-A": {
			{IssueID: "issue-A", DependsOnID: "issue-B", Type: types.DepBlocks},
		},
		"issue-C": {
			{IssueID: "issue-C", DependsOnID: "issue-A", Type: types.DepBlocks},
			{IssueID: "issue-C", DependsOnID: "issue-B", Type: types.DepRelated}, // Should be ignored (not blocking)
		},
	}

	blockedByMap, blocksMap, _ := buildBlockingMaps(allDeps, nil)

	// issue-A is blocked by issue-B
	if len(blockedByMap["issue-A"]) != 1 || blockedByMap["issue-A"][0] != "issue-B" {
		t.Errorf("issue-A blockedBy = %v, want [issue-B]", blockedByMap["issue-A"])
	}

	// issue-B blocks issue-A
	if len(blocksMap["issue-B"]) != 1 || blocksMap["issue-B"][0] != "issue-A" {
		t.Errorf("issue-B blocks = %v, want [issue-A]", blocksMap["issue-B"])
	}

	// issue-C is blocked by issue-A (related dep is ignored)
	if len(blockedByMap["issue-C"]) != 1 || blockedByMap["issue-C"][0] != "issue-A" {
		t.Errorf("issue-C blockedBy = %v, want [issue-A]", blockedByMap["issue-C"])
	}

	// issue-A also blocks issue-C
	if len(blocksMap["issue-A"]) != 1 || blocksMap["issue-A"][0] != "issue-C" {
		t.Errorf("issue-A blocks = %v, want [issue-C]", blocksMap["issue-A"])
	}
}

func TestBuildBlockingMaps_ParentChildSeparation(t *testing.T) {
	t.Parallel()
	allDeps := map[string][]*types.Dependency{
		"child-1": {
			{IssueID: "child-1", DependsOnID: "parent-1", Type: types.DepParentChild},
		},
		"child-2": {
			{IssueID: "child-2", DependsOnID: "parent-1", Type: types.DepParentChild},
		},
		"issue-X": {
			{IssueID: "issue-X", DependsOnID: "parent-1", Type: types.DepBlocks},
		},
	}

	_, blocksMap, childrenMap := buildBlockingMaps(allDeps, nil)

	// parent-1 should have children, not blocks, for parent-child deps
	if len(childrenMap["parent-1"]) != 2 {
		t.Errorf("parent-1 children = %v, want 2 children", childrenMap["parent-1"])
	}
	// parent-1 should only block issue-X (not child-1 or child-2)
	if len(blocksMap["parent-1"]) != 1 || blocksMap["parent-1"][0] != "issue-X" {
		t.Errorf("parent-1 blocks = %v, want [issue-X]", blocksMap["parent-1"])
	}
}

func TestBuildBlockingMaps_ClosedBlockersFiltered(t *testing.T) {
	t.Parallel()
	// issue-A is blocked by issue-B (open) and issue-C (closed)
	// issue-D is blocked by issue-C (closed) only
	allDeps := map[string][]*types.Dependency{
		"issue-A": {
			{IssueID: "issue-A", DependsOnID: "issue-B", Type: types.DepBlocks},
			{IssueID: "issue-A", DependsOnID: "issue-C", Type: types.DepBlocks},
		},
		"issue-D": {
			{IssueID: "issue-D", DependsOnID: "issue-C", Type: types.DepBlocks},
		},
	}

	closedIDs := map[string]bool{"issue-C": true}
	blockedByMap, blocksMap, _ := buildBlockingMaps(allDeps, closedIDs)

	// issue-A should only show issue-B as blocker (issue-C is closed)
	if len(blockedByMap["issue-A"]) != 1 || blockedByMap["issue-A"][0] != "issue-B" {
		t.Errorf("issue-A blockedBy = %v, want [issue-B]", blockedByMap["issue-A"])
	}

	// issue-D should have no blockers (issue-C is closed)
	if len(blockedByMap["issue-D"]) != 0 {
		t.Errorf("issue-D blockedBy = %v, want []", blockedByMap["issue-D"])
	}

	// issue-B should still show as blocking issue-A
	if len(blocksMap["issue-B"]) != 1 || blocksMap["issue-B"][0] != "issue-A" {
		t.Errorf("issue-B blocks = %v, want [issue-A]", blocksMap["issue-B"])
	}

	// issue-C should NOT show as blocking anything (it's closed)
	if len(blocksMap["issue-C"]) != 0 {
		t.Errorf("issue-C blocks = %v, want [] (closed blocker)", blocksMap["issue-C"])
	}
}

func TestBuildBlockingMaps_NilClosedIDs(t *testing.T) {
	t.Parallel()
	// When closedIDs is nil, all blockers should be included (backward compat)
	allDeps := map[string][]*types.Dependency{
		"issue-A": {
			{IssueID: "issue-A", DependsOnID: "issue-B", Type: types.DepBlocks},
		},
	}

	blockedByMap, blocksMap, _ := buildBlockingMaps(allDeps, nil)

	if len(blockedByMap["issue-A"]) != 1 || blockedByMap["issue-A"][0] != "issue-B" {
		t.Errorf("issue-A blockedBy = %v, want [issue-B]", blockedByMap["issue-A"])
	}
	if len(blocksMap["issue-B"]) != 1 || blocksMap["issue-B"][0] != "issue-A" {
		t.Errorf("issue-B blocks = %v, want [issue-A]", blocksMap["issue-B"])
	}
}

func TestFormatIssueCompactWithDependencies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		issue     *types.Issue
		blockedBy []string
		blocks    []string
		want      string
	}{
		{
			name: "issue with blocked by",
			issue: &types.Issue{
				ID:        "test-123",
				Title:     "Blocked Issue",
				Priority:  1,
				IssueType: types.TypeTask,
				Status:    types.StatusOpen,
			},
			blockedBy: []string{"test-100"},
			blocks:    nil,
			want:      "(blocked by: test-100)",
		},
		{
			name: "issue with blocks",
			issue: &types.Issue{
				ID:        "test-456",
				Title:     "Blocking Issue",
				Priority:  1,
				IssueType: types.TypeTask,
				Status:    types.StatusOpen,
			},
			blockedBy: nil,
			blocks:    []string{"test-200", "test-300"},
			want:      "(blocks: test-200, test-300)",
		},
		{
			name: "issue with both",
			issue: &types.Issue{
				ID:        "test-789",
				Title:     "Middle Issue",
				Priority:  1,
				IssueType: types.TypeTask,
				Status:    types.StatusOpen,
			},
			blockedBy: []string{"test-100"},
			blocks:    []string{"test-200"},
			want:      "(blocked by: test-100, blocks: test-200)",
		},
		{
			name: "issue with no dependencies",
			issue: &types.Issue{
				ID:        "test-abc",
				Title:     "Independent Issue",
				Priority:  1,
				IssueType: types.TypeTask,
				Status:    types.StatusOpen,
			},
			blockedBy: nil,
			blocks:    nil,
			want:      "Independent Issue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			formatIssueCompact(&buf, tt.issue, nil, tt.blockedBy, tt.blocks, "")
			result := buf.String()
			if !strings.Contains(result, tt.want) {
				t.Errorf("formatIssueCompact() = %q, want to contain %q", result, tt.want)
			}
		})
	}
}

// TestFormatIssueCompactBlockedIcon verifies that dependency-blocked open issues
// show the blocked icon (●) instead of the open icon (○) in compact list output. (GH#2858)
func TestFormatIssueCompactBlockedIcon(t *testing.T) {
	t.Parallel()

	t.Run("open issue with blockers shows blocked icon", func(t *testing.T) {
		issue := &types.Issue{
			ID:        "test-blocked",
			Title:     "Blocked by dependency",
			Priority:  2,
			IssueType: types.TypeTask,
			Status:    types.StatusOpen,
		}
		var buf strings.Builder
		formatIssueCompact(&buf, issue, nil, []string{"blocker-1"}, nil, "")
		result := buf.String()
		// Should show blocked icon ● not open icon ○
		if strings.Contains(result, ui.StatusIconOpen) {
			t.Errorf("dependency-blocked issue should not show open icon ○, got: %q", result)
		}
		if !strings.Contains(result, ui.StatusIconBlocked) {
			t.Errorf("dependency-blocked issue should show blocked icon ●, got: %q", result)
		}
	})

	t.Run("open issue without blockers shows open icon", func(t *testing.T) {
		issue := &types.Issue{
			ID:        "test-open",
			Title:     "Normal open issue",
			Priority:  2,
			IssueType: types.TypeTask,
			Status:    types.StatusOpen,
		}
		var buf strings.Builder
		formatIssueCompact(&buf, issue, nil, nil, nil, "")
		result := buf.String()
		if !strings.Contains(result, ui.StatusIconOpen) {
			t.Errorf("open issue without blockers should show open icon ○, got: %q", result)
		}
	})

	t.Run("in_progress issue with blockers keeps in_progress icon", func(t *testing.T) {
		issue := &types.Issue{
			ID:        "test-wip",
			Title:     "In progress with blocker",
			Priority:  2,
			IssueType: types.TypeTask,
			Status:    types.StatusInProgress,
		}
		var buf strings.Builder
		formatIssueCompact(&buf, issue, nil, []string{"blocker-1"}, nil, "")
		result := buf.String()
		// Should keep in_progress icon, not override to blocked
		if !strings.Contains(result, ui.StatusIconInProgress) {
			t.Errorf("in_progress issue should keep its icon even with blockers, got: %q", result)
		}
	})
}

func TestParseTimeFlag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Absolute formats
		{"RFC3339", "2023-01-15T10:30:00Z", false},
		{"Date only", "2023-01-15", false},
		// Compact duration formats (GH#820)
		{"Compact hours", "+6h", false},
		{"Compact days", "+1d", false},
		{"Compact weeks", "+2w", false},
		{"Compact negative", "-3d", false},
		// Natural language (GH#820)
		{"Natural tomorrow", "tomorrow", false},
		{"Natural next monday", "next monday", false},
		// Invalid formats
		{"Invalid format", "not-a-date", true},
		{"Empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTimeFlag(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTimeFlag(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// TestFormatDependencyInfoWithParent tests that parent-child deps render as "parent: X" (bd-hcxu)
func TestFormatDependencyInfoWithParent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		blockedBy []string
		blocks    []string
		parent    string
		want      string
	}{
		{
			name:   "parent only",
			parent: "epic-1",
			want:   "(parent: epic-1)",
		},
		{
			name:      "parent and blocked by",
			parent:    "epic-1",
			blockedBy: []string{"blocker-1"},
			want:      "(parent: epic-1, blocked by: blocker-1)",
		},
		{
			name:   "parent and blocks",
			parent: "epic-1",
			blocks: []string{"child-1"},
			want:   "(parent: epic-1, blocks: child-1)",
		},
		{
			name:      "parent, blocked by, and blocks",
			parent:    "epic-1",
			blockedBy: []string{"blocker-1"},
			blocks:    []string{"child-1"},
			want:      "(parent: epic-1, blocked by: blocker-1, blocks: child-1)",
		},
		{
			name: "no parent, no deps",
			want: "",
		},
		{
			name:      "blocked by only (no parent)",
			blockedBy: []string{"blocker-1"},
			want:      "(blocked by: blocker-1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDependencyInfo(tt.blockedBy, tt.blocks, tt.parent)
			if result != tt.want {
				t.Errorf("formatDependencyInfo() = %q, want %q", result, tt.want)
			}
		})
	}
}

// TestFormatIssueCompactWithParent tests compact format renders parent correctly (bd-hcxu)
func TestFormatIssueCompactWithParent(t *testing.T) {
	t.Parallel()
	issue := &types.Issue{
		ID:        "test-child",
		Title:     "Child Task",
		Priority:  2,
		IssueType: types.TypeTask,
		Status:    types.StatusOpen,
	}

	t.Run("shows parent annotation", func(t *testing.T) {
		var buf strings.Builder
		formatIssueCompact(&buf, issue, nil, nil, nil, "test-parent")
		result := buf.String()
		if !strings.Contains(result, "(parent: test-parent)") {
			t.Errorf("Expected '(parent: test-parent)' in output, got %q", result)
		}
	})

	t.Run("does not show blocked by for parent", func(t *testing.T) {
		var buf strings.Builder
		formatIssueCompact(&buf, issue, nil, nil, nil, "test-parent")
		result := buf.String()
		if strings.Contains(result, "blocked by") {
			t.Errorf("Should not contain 'blocked by' for parent-child dep, got %q", result)
		}
	})

	t.Run("shows parent and blocked by together", func(t *testing.T) {
		var buf strings.Builder
		formatIssueCompact(&buf, issue, nil, []string{"blocker-1"}, nil, "test-parent")
		result := buf.String()
		if !strings.Contains(result, "(parent: test-parent, blocked by: blocker-1)") {
			t.Errorf("Expected '(parent: test-parent, blocked by: blocker-1)' in output, got %q", result)
		}
	})
}

func TestListCommandInit(t *testing.T) {
	t.Parallel()
	if listCmd == nil {
		t.Fatal("listCmd should be initialized")
	}

	// Verify --exclude-label flag exists and defaults to empty slice
	excludeLabelFlag := listCmd.Flags().Lookup("exclude-label")
	if excludeLabelFlag == nil {
		t.Fatal("--exclude-label flag should exist on bd list")
	}
	if excludeLabelFlag.DefValue != "[]" {
		t.Errorf("--exclude-label default should be '[]', got %q", excludeLabelFlag.DefValue)
	}
}
