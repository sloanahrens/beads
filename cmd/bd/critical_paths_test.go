//go:build cgo

package main

import (
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// TestBuildBlockingMap tests the helper that builds the blocking relationship map.
func TestBuildBlockingMap(t *testing.T) {
	t.Parallel()

	t.Run("EmptyInput", func(t *testing.T) {
		result := buildBlockingMap(nil)
		if len(result) != 0 {
			t.Errorf("Expected empty map, got %d entries", len(result))
		}
	})

	t.Run("SingleBlocker", func(t *testing.T) {
		blocked := []*types.BlockedIssue{
			{
				Issue:          types.Issue{ID: "issue-1"},
				BlockedByCount: 1,
				BlockedBy:      []string{"blocker-1"},
			},
		}
		result := buildBlockingMap(blocked)
		if issues, ok := result["blocker-1"]; !ok {
			t.Error("Expected blocker-1 in map")
		} else if len(issues) != 1 || issues[0] != "issue-1" {
			t.Errorf("blocker-1 blocks %v, want [issue-1]", issues)
		}
	})

	t.Run("MultipleBlockers", func(t *testing.T) {
		blocked := []*types.BlockedIssue{
			{
				Issue:          types.Issue{ID: "issue-1"},
				BlockedByCount: 2,
				BlockedBy:      []string{"blocker-a", "blocker-b"},
			},
			{
				Issue:          types.Issue{ID: "issue-2"},
				BlockedByCount: 1,
				BlockedBy:      []string{"blocker-a"},
			},
		}
		result := buildBlockingMap(blocked)

		// blocker-a blocks both issue-1 and issue-2
		if issues, ok := result["blocker-a"]; !ok {
			t.Error("Expected blocker-a in map")
		} else if len(issues) != 2 {
			t.Errorf("blocker-a blocks %d issues, want 2", len(issues))
		}

		// blocker-b blocks only issue-1
		if issues, ok := result["blocker-b"]; !ok {
			t.Error("Expected blocker-b in map")
		} else if len(issues) != 1 {
			t.Errorf("blocker-b blocks %d issues, want 1", len(issues))
		}
	})
}
