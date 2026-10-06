//go:build cgo && integration

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestCreateIssueFromFormValues(t *testing.T) {
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)
	ctx := context.Background()

	t.Run("BasicIssue", func(t *testing.T) {
		fv := &createFormValues{
			Title:     "Test Form Issue",
			Priority:  1,
			IssueType: "bug",
		}

		issue, err := CreateIssueFromFormValues(ctx, s, fv, "test")
		if err != nil {
			t.Fatalf("failed to create issue: %v", err)
		}

		if issue.Title != "Test Form Issue" {
			t.Errorf("expected title 'Test Form Issue', got %q", issue.Title)
		}
		if issue.Priority != 1 {
			t.Errorf("expected priority 1, got %d", issue.Priority)
		}
		if issue.IssueType != types.TypeBug {
			t.Errorf("expected type bug, got %s", issue.IssueType)
		}
		if issue.Status != types.StatusOpen {
			t.Errorf("expected status open, got %s", issue.Status)
		}
	})

	t.Run("WithDescription", func(t *testing.T) {
		fv := &createFormValues{
			Title:       "Issue with description",
			Description: "This is a detailed description",
			Priority:    2,
			IssueType:   "task",
		}

		issue, err := CreateIssueFromFormValues(ctx, s, fv, "test")
		if err != nil {
			t.Fatalf("failed to create issue: %v", err)
		}

		if issue.Description != "This is a detailed description" {
			t.Errorf("expected description, got %q", issue.Description)
		}
	})

	t.Run("WithDesignAndAcceptance", func(t *testing.T) {
		fv := &createFormValues{
			Title:              "Feature with design",
			Design:             "Use MVC pattern",
			AcceptanceCriteria: "All tests pass",
			IssueType:          "feature",
			Priority:           2,
		}

		issue, err := CreateIssueFromFormValues(ctx, s, fv, "test")
		if err != nil {
			t.Fatalf("failed to create issue: %v", err)
		}

		if issue.Design != "Use MVC pattern" {
			t.Errorf("expected design, got %q", issue.Design)
		}
		if issue.AcceptanceCriteria != "All tests pass" {
			t.Errorf("expected acceptance criteria, got %q", issue.AcceptanceCriteria)
		}
	})

	t.Run("WithAssignee", func(t *testing.T) {
		fv := &createFormValues{
			Title:     "Assigned issue",
			Assignee:  "alice",
			Priority:  1,
			IssueType: "task",
		}

		issue, err := CreateIssueFromFormValues(ctx, s, fv, "test")
		if err != nil {
			t.Fatalf("failed to create issue: %v", err)
		}

		if issue.Assignee != "alice" {
			t.Errorf("expected assignee 'alice', got %q", issue.Assignee)
		}
	})

	t.Run("WithExternalRef", func(t *testing.T) {
		fv := &createFormValues{
			Title:       "Issue with external ref",
			ExternalRef: "gh-123",
			Priority:    2,
			IssueType:   "bug",
		}

		issue, err := CreateIssueFromFormValues(ctx, s, fv, "test")
		if err != nil {
			t.Fatalf("failed to create issue: %v", err)
		}

		if issue.ExternalRef == nil {
			t.Fatal("expected external ref to be set")
		}
		if *issue.ExternalRef != "gh-123" {
			t.Errorf("expected external ref 'gh-123', got %q", *issue.ExternalRef)
		}
	})

	t.Run("WithLabels", func(t *testing.T) {
		fv := &createFormValues{
			Title:     "Issue with labels",
			Priority:  0,
			IssueType: "bug",
			Labels:    []string{"bug", "critical"},
		}

		issue, err := CreateIssueFromFormValues(ctx, s, fv, "test")
		if err != nil {
			t.Fatalf("failed to create issue: %v", err)
		}

		labels, err := s.GetLabels(ctx, issue.ID)
		if err != nil {
			t.Fatalf("failed to get labels: %v", err)
		}

		if len(labels) != 2 {
			t.Errorf("expected 2 labels, got %d", len(labels))
		}

		labelMap := make(map[string]bool)
		for _, l := range labels {
			labelMap[l] = true
		}

		if !labelMap["bug"] || !labelMap["critical"] {
			t.Errorf("expected labels 'bug' and 'critical', got %v", labels)
		}
	})

	t.Run("WithDependencies", func(t *testing.T) {
		// Create a parent issue first
		parentFv := &createFormValues{
			Title:     "Parent issue for deps",
			Priority:  1,
			IssueType: "task",
		}
		parent, err := CreateIssueFromFormValues(ctx, s, parentFv, "test")
		if err != nil {
			t.Fatalf("failed to create parent: %v", err)
		}

		// Create child with dependency
		childFv := &createFormValues{
			Title:        "Child issue",
			Priority:     1,
			IssueType:    "task",
			Dependencies: []string{parent.ID}, // Default blocks type
		}
		child, err := CreateIssueFromFormValues(ctx, s, childFv, "test")
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}

		deps, err := s.GetDependencies(ctx, child.ID)
		if err != nil {
			t.Fatalf("failed to get dependencies: %v", err)
		}

		if len(deps) == 0 {
			t.Fatal("expected at least 1 dependency, got 0")
		}

		found := false
		for _, d := range deps {
			if d.ID == parent.ID {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("expected dependency on %s, not found", parent.ID)
		}
	})

	t.Run("WithTypedDependencies", func(t *testing.T) {
		// Create a parent issue
		parentFv := &createFormValues{
			Title:     "Related parent",
			Priority:  1,
			IssueType: "task",
		}
		parent, err := CreateIssueFromFormValues(ctx, s, parentFv, "test")
		if err != nil {
			t.Fatalf("failed to create parent: %v", err)
		}

		// Create child with typed dependency
		childFv := &createFormValues{
			Title:        "Child with typed dep",
			Priority:     1,
			IssueType:    "bug",
			Dependencies: []string{"discovered-from:" + parent.ID},
		}
		child, err := CreateIssueFromFormValues(ctx, s, childFv, "test")
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}

		deps, err := s.GetDependencies(ctx, child.ID)
		if err != nil {
			t.Fatalf("failed to get dependencies: %v", err)
		}

		if len(deps) == 0 {
			t.Fatal("expected at least 1 dependency, got 0")
		}

		found := false
		for _, d := range deps {
			if d.ID == parent.ID {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("expected dependency on %s, not found", parent.ID)
		}
	})

	t.Run("AllIssueTypes", func(t *testing.T) {
		issueTypes := []string{"bug", "feature", "task", "epic", "chore", "decision"}
		expectedTypes := []types.IssueType{
			types.TypeBug,
			types.TypeFeature,
			types.TypeTask,
			types.TypeEpic,
			types.TypeChore,
			types.TypeDecision,
		}

		for i, issueType := range issueTypes {
			fv := &createFormValues{
				Title:     "Test " + issueType,
				IssueType: issueType,
				Priority:  2,
			}

			issue, err := CreateIssueFromFormValues(ctx, s, fv, "test")
			if err != nil {
				t.Fatalf("failed to create issue type %s: %v", issueType, err)
			}

			if issue.IssueType != expectedTypes[i] {
				t.Errorf("expected type %s, got %s", expectedTypes[i], issue.IssueType)
			}
		}
	})

	t.Run("MultipleDependencies", func(t *testing.T) {
		// Create two parent issues
		parent1Fv := &createFormValues{
			Title:     "Multi-dep Parent 1",
			Priority:  1,
			IssueType: "task",
		}
		parent1, err := CreateIssueFromFormValues(ctx, s, parent1Fv, "test")
		if err != nil {
			t.Fatalf("failed to create parent1: %v", err)
		}

		parent2Fv := &createFormValues{
			Title:     "Multi-dep Parent 2",
			Priority:  1,
			IssueType: "task",
		}
		parent2, err := CreateIssueFromFormValues(ctx, s, parent2Fv, "test")
		if err != nil {
			t.Fatalf("failed to create parent2: %v", err)
		}

		// Create child with multiple dependencies
		childFv := &createFormValues{
			Title:        "Multi-dep Child",
			Priority:     1,
			IssueType:    "task",
			Dependencies: []string{"blocks:" + parent1.ID, "related:" + parent2.ID},
		}
		child, err := CreateIssueFromFormValues(ctx, s, childFv, "test")
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}

		deps, err := s.GetDependencies(ctx, child.ID)
		if err != nil {
			t.Fatalf("failed to get dependencies: %v", err)
		}

		if len(deps) < 2 {
			t.Fatalf("expected at least 2 dependencies, got %d", len(deps))
		}

		foundParents := make(map[string]bool)
		for _, d := range deps {
			if d.ID == parent1.ID || d.ID == parent2.ID {
				foundParents[d.ID] = true
			}
		}

		if len(foundParents) != 2 {
			t.Errorf("expected to find both parent dependencies, found %d", len(foundParents))
		}
	})

	t.Run("DiscoveredFromInheritsSourceRepo", func(t *testing.T) {
		// Create a parent issue with a custom source_repo
		parent := &types.Issue{
			Title:      "Parent with source repo",
			Priority:   1,
			Status:     types.StatusOpen,
			IssueType:  types.TypeTask,
			SourceRepo: "/path/to/custom/repo",
		}

		if err := s.CreateIssue(ctx, parent, "test"); err != nil {
			t.Fatalf("failed to create parent: %v", err)
		}

		// Create a discovered issue with discovered-from dependency
		childFv := &createFormValues{
			Title:        "Discovered bug",
			Priority:     1,
			IssueType:    "bug",
			Dependencies: []string{"discovered-from:" + parent.ID},
		}
		child, err := CreateIssueFromFormValues(ctx, s, childFv, "test")
		if err != nil {
			t.Fatalf("failed to create discovered issue: %v", err)
		}

		// Verify the discovered issue inherited the source_repo
		retrievedIssue, err := s.GetIssue(ctx, child.ID)
		if err != nil {
			t.Fatalf("failed to get discovered issue: %v", err)
		}

		if retrievedIssue.SourceRepo != parent.SourceRepo {
			t.Errorf("expected source_repo %q, got %q", parent.SourceRepo, retrievedIssue.SourceRepo)
		}
	})

	t.Run("AllPriorities", func(t *testing.T) {
		for priority := 0; priority <= 4; priority++ {
			fv := &createFormValues{
				Title:     "Priority test",
				IssueType: "task",
				Priority:  priority,
			}

			issue, err := CreateIssueFromFormValues(ctx, s, fv, "test")
			if err != nil {
				t.Fatalf("failed to create issue with priority %d: %v", priority, err)
			}

			if issue.Priority != priority {
				t.Errorf("expected priority %d, got %d", priority, issue.Priority)
			}
		}
	})
}

func TestCreateIssueFromFormValues_WithParent(t *testing.T) {
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)
	ctx := context.Background()

	t.Run("ParentChildCreation", func(t *testing.T) {
		// Create parent (epic) first
		parentFv := &createFormValues{
			Title:     "Epic parent issue",
			Priority:  1,
			IssueType: "epic",
		}
		parent, err := CreateIssueFromFormValues(ctx, s, parentFv, "test")
		if err != nil {
			t.Fatalf("failed to create parent: %v", err)
		}

		// Create child via --parent
		childFv := &createFormValues{
			Title:     "Child bug under epic",
			Priority:  2,
			IssueType: "bug",
			ParentID:  parent.ID,
		}
		child, err := CreateIssueFromFormValues(ctx, s, childFv, "test")
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}

		// Child ID should be hierarchical (parent.1)
		if !strings.HasPrefix(child.ID, parent.ID+".") {
			t.Errorf("child ID %q should start with parent ID %q + '.'", child.ID, parent.ID)
		}

		// Verify parent-child dependency was created
		deps, err := s.GetDependencies(ctx, child.ID)
		if err != nil {
			t.Fatalf("failed to get dependencies: %v", err)
		}

		found := false
		for _, d := range deps {
			if d.ID == parent.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected parent-child dependency on %s, not found", parent.ID)
		}
	})

	t.Run("ParentNotFound", func(t *testing.T) {
		fv := &createFormValues{
			Title:     "Orphan child",
			Priority:  2,
			IssueType: "task",
			ParentID:  "nonexistent-id",
		}
		_, err := CreateIssueFromFormValues(ctx, s, fv, "test")
		if err == nil {
			t.Fatal("expected error for nonexistent parent, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' in error, got: %v", err)
		}
	})

	t.Run("ParentLabelInheritance", func(t *testing.T) {
		// Create parent with labels
		parentFv := &createFormValues{
			Title:     "Labeled parent",
			Priority:  1,
			IssueType: "epic",
			Labels:    []string{"team-a", "urgent"},
		}
		parent, err := CreateIssueFromFormValues(ctx, s, parentFv, "test")
		if err != nil {
			t.Fatalf("failed to create parent: %v", err)
		}

		// Create child with --parent and its own label
		childFv := &createFormValues{
			Title:     "Child inherits labels",
			Priority:  2,
			IssueType: "task",
			ParentID:  parent.ID,
			Labels:    []string{"child-only"},
		}
		child, err := CreateIssueFromFormValues(ctx, s, childFv, "test")
		if err != nil {
			t.Fatalf("failed to create child: %v", err)
		}

		// Child should have its own label plus inherited parent labels
		labels, err := s.GetLabels(ctx, child.ID)
		if err != nil {
			t.Fatalf("failed to get child labels: %v", err)
		}

		labelMap := make(map[string]bool)
		for _, l := range labels {
			labelMap[l] = true
		}

		if !labelMap["child-only"] {
			t.Error("expected child's own label 'child-only'")
		}
		if !labelMap["team-a"] {
			t.Error("expected inherited label 'team-a'")
		}
		if !labelMap["urgent"] {
			t.Error("expected inherited label 'urgent'")
		}
		if len(labels) != 3 {
			t.Errorf("expected 3 labels (1 own + 2 inherited), got %d: %v", len(labels), labels)
		}
	})

	t.Run("MultipleChildrenUnderSameParent", func(t *testing.T) {
		// Create parent
		parentFv := &createFormValues{
			Title:     "Multi-child parent",
			Priority:  1,
			IssueType: "epic",
		}
		parent, err := CreateIssueFromFormValues(ctx, s, parentFv, "test")
		if err != nil {
			t.Fatalf("failed to create parent: %v", err)
		}

		// Create two children
		child1Fv := &createFormValues{
			Title:     "First child",
			Priority:  2,
			IssueType: "task",
			ParentID:  parent.ID,
		}
		child1, err := CreateIssueFromFormValues(ctx, s, child1Fv, "test")
		if err != nil {
			t.Fatalf("failed to create child1: %v", err)
		}

		child2Fv := &createFormValues{
			Title:     "Second child",
			Priority:  2,
			IssueType: "task",
			ParentID:  parent.ID,
		}
		child2, err := CreateIssueFromFormValues(ctx, s, child2Fv, "test")
		if err != nil {
			t.Fatalf("failed to create child2: %v", err)
		}

		// Children should have distinct IDs
		if child1.ID == child2.ID {
			t.Errorf("children should have distinct IDs, both got %q", child1.ID)
		}

		// Both should be under the parent
		if !strings.HasPrefix(child1.ID, parent.ID+".") {
			t.Errorf("child1 ID %q should start with %q", child1.ID, parent.ID+".")
		}
		if !strings.HasPrefix(child2.ID, parent.ID+".") {
			t.Errorf("child2 ID %q should start with %q", child2.ID, parent.ID+".")
		}
	})
}

func TestFormValuesIntegration(t *testing.T) {
	// Test the full flow: parseCreateFormInput -> CreateIssueFromFormValues
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)
	ctx := context.Background()

	t.Run("FullFlow", func(t *testing.T) {
		// Simulate form input
		fv := parseCreateFormInput(&createFormRawInput{
			Title:       "Integration Test Issue",
			Description: "Testing the full flow from form to storage",
			IssueType:   "feature",
			Priority:    "1",
			Assignee:    "test-user",
			Labels:      "integration, test",
			Design:      "Design notes here",
			Acceptance:  "Should work end to end",
			ExternalRef: "gh-999",
		})

		issue, err := CreateIssueFromFormValues(ctx, s, fv, "test")
		if err != nil {
			t.Fatalf("failed to create issue: %v", err)
		}

		// Verify issue was stored
		retrieved, err := s.GetIssue(ctx, issue.ID)
		if err != nil {
			t.Fatalf("failed to retrieve issue: %v", err)
		}

		if retrieved.Title != "Integration Test Issue" {
			t.Errorf("unexpected title: %q", retrieved.Title)
		}
		if retrieved.Description != "Testing the full flow from form to storage" {
			t.Errorf("unexpected description: %q", retrieved.Description)
		}
		if retrieved.IssueType != types.TypeFeature {
			t.Errorf("unexpected type: %s", retrieved.IssueType)
		}
		if retrieved.Priority != 1 {
			t.Errorf("unexpected priority: %d", retrieved.Priority)
		}
		if retrieved.Assignee != "test-user" {
			t.Errorf("unexpected assignee: %q", retrieved.Assignee)
		}
		if retrieved.Design != "Design notes here" {
			t.Errorf("unexpected design: %q", retrieved.Design)
		}
		if retrieved.AcceptanceCriteria != "Should work end to end" {
			t.Errorf("unexpected acceptance criteria: %q", retrieved.AcceptanceCriteria)
		}
		if retrieved.ExternalRef == nil || *retrieved.ExternalRef != "gh-999" {
			t.Errorf("unexpected external ref: %v", retrieved.ExternalRef)
		}

		// Check labels
		labels, err := s.GetLabels(ctx, issue.ID)
		if err != nil {
			t.Fatalf("failed to get labels: %v", err)
		}
		if len(labels) != 2 {
			t.Errorf("expected 2 labels, got %d", len(labels))
		}
	})
}
