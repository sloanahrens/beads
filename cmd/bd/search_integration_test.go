//go:build cgo && integration

package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// TestSearchWithDateAndPriorityFilters tests bd search with date range and priority filters
func TestSearchWithDateAndPriorityFilters(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)
	ctx := context.Background()

	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)

	// Create test issues with search-relevant content
	issue1 := &types.Issue{
		Title:       "Critical security bug in auth",
		Description: "Authentication bypass vulnerability",
		Priority:    0,
		IssueType:   types.TypeBug,
		Status:      types.StatusOpen,
	}
	issue2 := &types.Issue{
		Title:       "Add security scanning feature",
		Description: "Implement automated security checks",
		Priority:    2,
		IssueType:   types.TypeFeature,
		Status:      types.StatusInProgress,
	}
	issue3 := &types.Issue{
		Title:       "Security audit task",
		Description: "Review all security practices",
		Priority:    3,
		IssueType:   types.TypeTask,
		Status:      types.StatusOpen,
	}

	for _, issue := range []*types.Issue{issue1, issue2, issue3} {
		if err := s.CreateIssue(ctx, issue, "test-user"); err != nil {
			t.Fatalf("Failed to create issue: %v", err)
		}
	}

	// Close issue3 to set closed_at timestamp
	if err := s.CloseIssue(ctx, issue3.ID, "test-user", "Testing", ""); err != nil {
		t.Fatalf("Failed to close issue3: %v", err)
	}

	t.Run("search with priority range - min", func(t *testing.T) {
		minPrio := 2
		results, err := s.SearchIssues(ctx, "security", types.IssueFilter{
			PriorityMin: &minPrio,
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("Expected 2 issues matching 'security' with priority >= 2, got %d", len(results))
		}
	})

	t.Run("search with priority range - max", func(t *testing.T) {
		maxPrio := 1
		results, err := s.SearchIssues(ctx, "security", types.IssueFilter{
			PriorityMax: &maxPrio,
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 issue matching 'security' with priority <= 1, got %d", len(results))
		}
		if len(results) > 0 && results[0].ID != issue1.ID {
			t.Errorf("Expected issue1, got %s", results[0].ID)
		}
	})

	t.Run("search with priority range - min and max", func(t *testing.T) {
		minPrio := 1
		maxPrio := 2
		results, err := s.SearchIssues(ctx, "security", types.IssueFilter{
			PriorityMin: &minPrio,
			PriorityMax: &maxPrio,
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 issue matching 'security' with priority 1-2, got %d", len(results))
		}
		if len(results) > 0 && results[0].ID != issue2.ID {
			t.Errorf("Expected issue2, got %s", results[0].ID)
		}
	})

	t.Run("search with created after", func(t *testing.T) {
		results, err := s.SearchIssues(ctx, "security", types.IssueFilter{
			CreatedAfter: &twoDaysAgo,
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 issues matching 'security' created after two days ago, got %d", len(results))
		}
	})

	t.Run("search with updated before", func(t *testing.T) {
		futureTime := now.Add(24 * time.Hour)
		results, err := s.SearchIssues(ctx, "security", types.IssueFilter{
			UpdatedBefore: &futureTime,
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("Expected 3 issues matching 'security', got %d", len(results))
		}
	})

	t.Run("search with closed after", func(t *testing.T) {
		results, err := s.SearchIssues(ctx, "security", types.IssueFilter{
			ClosedAfter: &yesterday,
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("Expected 1 closed issue matching 'security', got %d", len(results))
		}
		if len(results) > 0 && results[0].ID != issue3.ID {
			t.Errorf("Expected issue3, got %s", results[0].ID)
		}
	})

	t.Run("search with combined filters", func(t *testing.T) {
		minPrio := 0
		maxPrio := 2
		results, err := s.SearchIssues(ctx, "auth", types.IssueFilter{
			PriorityMin:  &minPrio,
			PriorityMax:  &maxPrio,
			CreatedAfter: &twoDaysAgo,
		})
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		// Should match issue1 (has "auth" in title, priority 0)
		// and issue2 (has "auth" in description via "automated", priority 2)
		// Note: "auth" is a substring match, so it matches "authentication" and "automated"
		if len(results) < 1 {
			t.Errorf("Expected at least 1 result matching combined filters, got %d", len(results))
		}
	})
}
