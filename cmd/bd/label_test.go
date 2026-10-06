//go:build cgo

package main

import (
	"context"
	"testing"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
)

type labelTestHelper struct {
	s   *dolt.DoltStore
	ctx context.Context
	t   *testing.T
}

func (h *labelTestHelper) createIssue(title string, issueType types.IssueType, priority int) *types.Issue {
	issue := &types.Issue{
		Title:     title,
		Priority:  priority,
		IssueType: issueType,
		Status:    types.StatusOpen,
	}
	if err := h.s.CreateIssue(h.ctx, issue, "test-user"); err != nil {
		h.t.Fatalf("Failed to create issue: %v", err)
	}
	return issue
}

func (h *labelTestHelper) addLabel(issueID, label string) {
	if err := h.s.AddLabel(h.ctx, issueID, label, "test-user"); err != nil {
		h.t.Fatalf("Failed to add label '%s': %v", label, err)
	}
}

func (h *labelTestHelper) addLabels(issueID string, labels []string) {
	for _, label := range labels {
		h.addLabel(issueID, label)
	}
}

func (h *labelTestHelper) removeLabel(issueID, label string) {
	if err := h.s.RemoveLabel(h.ctx, issueID, label, "test-user"); err != nil {
		h.t.Fatalf("Failed to remove label '%s': %v", label, err)
	}
}

func (h *labelTestHelper) getLabels(issueID string) []string {
	labels, err := h.s.GetLabels(h.ctx, issueID)
	if err != nil {
		h.t.Fatalf("Failed to get labels: %v", err)
	}
	return labels
}

func (h *labelTestHelper) assertLabelCount(issueID string, expected int) {
	labels := h.getLabels(issueID)
	if len(labels) != expected {
		h.t.Errorf("Expected %d labels, got %d", expected, len(labels))
	}
}

func (h *labelTestHelper) assertHasLabel(issueID, expected string) {
	labels := h.getLabels(issueID)
	for _, l := range labels {
		if l == expected {
			return
		}
	}
	h.t.Errorf("Expected label '%s' not found", expected)
}

func (h *labelTestHelper) assertHasLabels(issueID string, expected []string) {
	labels := h.getLabels(issueID)
	labelMap := make(map[string]bool)
	for _, l := range labels {
		labelMap[l] = true
	}
	for _, exp := range expected {
		if !labelMap[exp] {
			h.t.Errorf("Expected label '%s' not found", exp)
		}
	}
}

func (h *labelTestHelper) assertNotHasLabel(issueID, label string) {
	labels := h.getLabels(issueID)
	for _, l := range labels {
		if l == label {
			h.t.Errorf("Did not expect label '%s' but found it", label)
		}
	}
}

func (h *labelTestHelper) assertLabelEvent(issueID string, eventType types.EventType, labelName string) {
	events, err := h.s.GetEvents(h.ctx, issueID, 100)
	if err != nil {
		h.t.Fatalf("Failed to get events: %v", err)
	}

	expectedComment := ""
	if eventType == types.EventLabelAdded {
		expectedComment = "Added label: " + labelName
	} else if eventType == types.EventLabelRemoved {
		expectedComment = "Removed label: " + labelName
	}

	for _, e := range events {
		if e.EventType == eventType && e.Comment != nil && *e.Comment == expectedComment {
			return
		}
	}
	h.t.Errorf("Expected to find event %s for label %s", eventType, labelName)
}
