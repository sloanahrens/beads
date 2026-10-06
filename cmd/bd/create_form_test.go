//go:build cgo

package main

import (
	"testing"
)

func TestParseFormInput(t *testing.T) {
	t.Run("BasicParsing", func(t *testing.T) {
		fv := parseCreateFormInput(&createFormRawInput{
			Title:       "Test Title",
			Description: "Test Description",
			IssueType:   "bug",
			Priority:    "1",
			Assignee:    "alice",
		})

		if fv.Title != "Test Title" {
			t.Errorf("expected title 'Test Title', got %q", fv.Title)
		}
		if fv.Description != "Test Description" {
			t.Errorf("expected description 'Test Description', got %q", fv.Description)
		}
		if fv.IssueType != "bug" {
			t.Errorf("expected issue type 'bug', got %q", fv.IssueType)
		}
		if fv.Priority != 1 {
			t.Errorf("expected priority 1, got %d", fv.Priority)
		}
		if fv.Assignee != "alice" {
			t.Errorf("expected assignee 'alice', got %q", fv.Assignee)
		}
	})

	t.Run("PriorityParsing", func(t *testing.T) {
		// Valid priority
		fv := parseCreateFormInput(&createFormRawInput{Title: "Title", IssueType: "task", Priority: "0"})
		if fv.Priority != 0 {
			t.Errorf("expected priority 0, got %d", fv.Priority)
		}

		// Invalid priority defaults to 2
		fv = parseCreateFormInput(&createFormRawInput{Title: "Title", IssueType: "task", Priority: "invalid"})
		if fv.Priority != 2 {
			t.Errorf("expected default priority 2 for invalid input, got %d", fv.Priority)
		}

		// Empty priority defaults to 2
		fv = parseCreateFormInput(&createFormRawInput{Title: "Title", IssueType: "task", Priority: ""})
		if fv.Priority != 2 {
			t.Errorf("expected default priority 2 for empty input, got %d", fv.Priority)
		}
	})

	t.Run("LabelsParsing", func(t *testing.T) {
		fv := parseCreateFormInput(&createFormRawInput{
			Title:     "Title",
			IssueType: "task",
			Priority:  "2",
			Labels:    "bug, critical, needs-review",
		})

		if len(fv.Labels) != 3 {
			t.Fatalf("expected 3 labels, got %d", len(fv.Labels))
		}

		expected := []string{"bug", "critical", "needs-review"}
		for i, label := range expected {
			if fv.Labels[i] != label {
				t.Errorf("expected label %q at index %d, got %q", label, i, fv.Labels[i])
			}
		}
	})

	t.Run("LabelsWithEmptyValues", func(t *testing.T) {
		fv := parseCreateFormInput(&createFormRawInput{
			Title:     "Title",
			IssueType: "task",
			Priority:  "2",
			Labels:    "bug, , critical, ",
		})

		if len(fv.Labels) != 2 {
			t.Fatalf("expected 2 non-empty labels, got %d: %v", len(fv.Labels), fv.Labels)
		}
	})

	t.Run("DependenciesParsing", func(t *testing.T) {
		fv := parseCreateFormInput(&createFormRawInput{
			Title:     "Title",
			IssueType: "task",
			Priority:  "2",
			Deps:      "discovered-from:bd-20, blocks:bd-15",
		})

		if len(fv.Dependencies) != 2 {
			t.Fatalf("expected 2 dependencies, got %d", len(fv.Dependencies))
		}

		expected := []string{"discovered-from:bd-20", "blocks:bd-15"}
		for i, dep := range expected {
			if fv.Dependencies[i] != dep {
				t.Errorf("expected dependency %q at index %d, got %q", dep, i, fv.Dependencies[i])
			}
		}
	})

	t.Run("AllFields", func(t *testing.T) {
		fv := parseCreateFormInput(&createFormRawInput{
			Title:       "Full Issue",
			Description: "Detailed description",
			IssueType:   "feature",
			Priority:    "1",
			Assignee:    "bob",
			Labels:      "frontend, urgent",
			Design:      "Use React hooks",
			Acceptance:  "Tests pass, UI works",
			ExternalRef: "gh-123",
			Deps:        "blocks:bd-1",
		})

		if fv.Title != "Full Issue" {
			t.Errorf("unexpected title: %q", fv.Title)
		}
		if fv.Description != "Detailed description" {
			t.Errorf("unexpected description: %q", fv.Description)
		}
		if fv.IssueType != "feature" {
			t.Errorf("unexpected issue type: %q", fv.IssueType)
		}
		if fv.Priority != 1 {
			t.Errorf("unexpected priority: %d", fv.Priority)
		}
		if fv.Assignee != "bob" {
			t.Errorf("unexpected assignee: %q", fv.Assignee)
		}
		if len(fv.Labels) != 2 {
			t.Errorf("unexpected labels count: %d", len(fv.Labels))
		}
		if fv.Design != "Use React hooks" {
			t.Errorf("unexpected design: %q", fv.Design)
		}
		if fv.AcceptanceCriteria != "Tests pass, UI works" {
			t.Errorf("unexpected acceptance criteria: %q", fv.AcceptanceCriteria)
		}
		if fv.ExternalRef != "gh-123" {
			t.Errorf("unexpected external ref: %q", fv.ExternalRef)
		}
		if len(fv.Dependencies) != 1 {
			t.Errorf("unexpected dependencies count: %d", len(fv.Dependencies))
		}
	})
}
