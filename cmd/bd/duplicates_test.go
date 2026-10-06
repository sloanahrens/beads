//go:build cgo

package main

import (
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestFindDuplicateGroups(t *testing.T) {
	tests := []struct {
		name           string
		issues         []*types.Issue
		expectedGroups int
	}{
		{
			name: "no duplicates",
			issues: []*types.Issue{
				{ID: "bd-1", Title: "Task 1", Status: types.StatusOpen},
				{ID: "bd-2", Title: "Task 2", Status: types.StatusOpen},
			},
			expectedGroups: 0,
		},
		{
			name: "simple duplicate",
			issues: []*types.Issue{
				{ID: "bd-1", Title: "Task 1", Status: types.StatusOpen},
				{ID: "bd-2", Title: "Task 1", Status: types.StatusOpen},
			},
			expectedGroups: 1,
		},
		{
			name: "duplicate with different status ignored",
			issues: []*types.Issue{
				{ID: "bd-1", Title: "Task 1", Status: types.StatusOpen},
				{ID: "bd-2", Title: "Task 1", Status: types.StatusClosed},
			},
			expectedGroups: 0,
		},
		{
			name: "multiple duplicates",
			issues: []*types.Issue{
				{ID: "bd-1", Title: "Task 1", Status: types.StatusOpen},
				{ID: "bd-2", Title: "Task 1", Status: types.StatusOpen},
				{ID: "bd-3", Title: "Task 2", Status: types.StatusOpen},
				{ID: "bd-4", Title: "Task 2", Status: types.StatusOpen},
			},
			expectedGroups: 2,
		},
		{
			name: "different descriptions are duplicates if title matches",
			issues: []*types.Issue{
				{ID: "bd-1", Title: "Task 1", Description: "Desc 1", Status: types.StatusOpen},
				{ID: "bd-2", Title: "Task 1", Description: "Desc 2", Status: types.StatusOpen},
			},
			expectedGroups: 0, // Different descriptions = not duplicates
		},
		{
			name: "exact content match",
			issues: []*types.Issue{
				{ID: "bd-1", Title: "Task 1", Description: "Desc 1", Design: "Design 1", AcceptanceCriteria: "AC 1", Status: types.StatusOpen},
				{ID: "bd-2", Title: "Task 1", Description: "Desc 1", Design: "Design 1", AcceptanceCriteria: "AC 1", Status: types.StatusOpen},
			},
			expectedGroups: 1,
		},
		{
			name: "three-way duplicate",
			issues: []*types.Issue{
				{ID: "bd-1", Title: "Task 1", Status: types.StatusOpen},
				{ID: "bd-2", Title: "Task 1", Status: types.StatusOpen},
				{ID: "bd-3", Title: "Task 1", Status: types.StatusOpen},
			},
			expectedGroups: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := findDuplicateGroups(tt.issues)
			if len(groups) != tt.expectedGroups {
				t.Errorf("findDuplicateGroups() returned %d groups, want %d", len(groups), tt.expectedGroups)
			}
		})
	}
}

func TestChooseMergeTarget(t *testing.T) {
	tests := []struct {
		name             string
		group            []*types.Issue
		refCounts        map[string]int
		structuralScores map[string]*issueScore
		wantID           string
	}{
		{
			name: "choose by reference count when no structural data",
			group: []*types.Issue{
				{ID: "bd-2", Title: "Task"},
				{ID: "bd-1", Title: "Task"},
			},
			refCounts: map[string]int{
				"bd-1": 5,
				"bd-2": 0,
			},
			structuralScores: map[string]*issueScore{},
			wantID:           "bd-1",
		},
		{
			name: "choose by lexicographic order if same references",
			group: []*types.Issue{
				{ID: "bd-2", Title: "Task"},
				{ID: "bd-1", Title: "Task"},
			},
			refCounts: map[string]int{
				"bd-1": 0,
				"bd-2": 0,
			},
			structuralScores: map[string]*issueScore{},
			wantID:           "bd-1",
		},
		{
			name: "prefer higher references even with larger ID",
			group: []*types.Issue{
				{ID: "bd-1", Title: "Task"},
				{ID: "bd-100", Title: "Task"},
			},
			refCounts: map[string]int{
				"bd-1":   1,
				"bd-100": 10,
			},
			structuralScores: map[string]*issueScore{},
			wantID:           "bd-100",
		},
		{
			name: "prefer dependents over text references (GH#1022)",
			group: []*types.Issue{
				{ID: "HONEY-s2g1", Title: "P1 / Foundations"}, // Has 17 children
				{ID: "HONEY-d0mw", Title: "P1 / Foundations"}, // Empty shell
			},
			refCounts: map[string]int{
				"HONEY-s2g1": 0,
				"HONEY-d0mw": 0,
			},
			structuralScores: map[string]*issueScore{
				"HONEY-s2g1": {dependentCount: 17, dependsOnCount: 2, textRefs: 0},
				"HONEY-d0mw": {dependentCount: 0, dependsOnCount: 0, textRefs: 0},
			},
			wantID: "HONEY-s2g1", // Should keep the one with children
		},
		{
			name: "dependents beat text references",
			group: []*types.Issue{
				{ID: "bd-1", Title: "Task"}, // Has text refs but no deps
				{ID: "bd-2", Title: "Task"}, // Has deps but no text refs
			},
			refCounts: map[string]int{
				"bd-1": 100, // Lots of text references
				"bd-2": 0,
			},
			structuralScores: map[string]*issueScore{
				"bd-1": {dependentCount: 0, dependsOnCount: 0, textRefs: 100},
				"bd-2": {dependentCount: 5, dependsOnCount: 0, textRefs: 0}, // 5 children/dependents
			},
			wantID: "bd-2", // Dependents take priority
		},
		{
			name: "dependsOnCount included in weight calculation (GH#1022)",
			group: []*types.Issue{
				{ID: "bd-1", Title: "Task"}, // Has dependencies (depends on others)
				{ID: "bd-2", Title: "Task"}, // Empty shell
			},
			refCounts: map[string]int{
				"bd-1": 0,
				"bd-2": 0,
			},
			structuralScores: map[string]*issueScore{
				"bd-1": {dependentCount: 0, dependsOnCount: 3, textRefs: 0}, // Depends on 3 other issues
				"bd-2": {dependentCount: 0, dependsOnCount: 0, textRefs: 0}, // Empty shell
			},
			wantID: "bd-1", // Issue with dependencies should be kept over empty shell
		},
		{
			name: "dependents weighted 3x more than depends-on (anti-orphan)",
			group: []*types.Issue{
				{ID: "bd-1", Title: "Task"}, // Has only dependents (children)
				{ID: "bd-2", Title: "Task"}, // Has more depends-on but fewer children
			},
			refCounts: map[string]int{
				"bd-1": 0,
				"bd-2": 0,
			},
			structuralScores: map[string]*issueScore{
				"bd-1": {dependentCount: 5, dependsOnCount: 0, textRefs: 0}, // Weight = 5*3 = 15
				"bd-2": {dependentCount: 3, dependsOnCount: 4, textRefs: 0}, // Weight = 3*3+4 = 13
			},
			wantID: "bd-1", // More children wins (anti-orphan weighting)
		},
		{
			name: "depends-on still contributes when dependents are equal",
			group: []*types.Issue{
				{ID: "bd-1", Title: "Task"},
				{ID: "bd-2", Title: "Task"},
			},
			refCounts: map[string]int{
				"bd-1": 0,
				"bd-2": 0,
			},
			structuralScores: map[string]*issueScore{
				"bd-1": {dependentCount: 2, dependsOnCount: 0, textRefs: 0}, // Weight = 2*3 = 6
				"bd-2": {dependentCount: 2, dependsOnCount: 3, textRefs: 0}, // Weight = 2*3+3 = 9
			},
			wantID: "bd-2", // Equal dependents, more deps wins
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := chooseMergeTarget(tt.group, tt.refCounts, tt.structuralScores)
			if target.ID != tt.wantID {
				t.Errorf("chooseMergeTarget() = %v, want %v", target.ID, tt.wantID)
			}
		})
	}
}

func TestCountReferences(t *testing.T) {
	issues := []*types.Issue{
		{
			ID:          "bd-1",
			Description: "See bd-2 for details",
			Notes:       "Related to bd-3",
		},
		{
			ID:          "bd-2",
			Description: "Mentioned bd-1 twice: bd-1",
		},
		{
			ID:    "bd-3",
			Notes: "Nothing to see here",
		},
	}

	counts := countReferences(issues)

	expectedCounts := map[string]int{
		"bd-1": 2, // Referenced twice in bd-2
		"bd-2": 1, // Referenced once in bd-1
		"bd-3": 1, // Referenced once in bd-1
	}

	for id, expectedCount := range expectedCounts {
		if counts[id] != expectedCount {
			t.Errorf("countReferences()[%s] = %d, want %d", id, counts[id], expectedCount)
		}
	}
}

func TestDuplicateGroupsWithDifferentStatuses(t *testing.T) {
	issues := []*types.Issue{
		{ID: "bd-1", Title: "Task 1", Status: types.StatusOpen},
		{ID: "bd-2", Title: "Task 1", Status: types.StatusClosed},
		{ID: "bd-3", Title: "Task 1", Status: types.StatusOpen},
	}

	groups := findDuplicateGroups(issues)

	// Should have 1 group with bd-1 and bd-3 (both open)
	if len(groups) != 1 {
		t.Fatalf("Expected 1 group, got %d", len(groups))
	}

	if len(groups[0]) != 2 {
		t.Fatalf("Expected 2 issues in group, got %d", len(groups[0]))
	}

	// Verify bd-2 (closed) is not in the group
	for _, issue := range groups[0] {
		if issue.ID == "bd-2" {
			t.Errorf("bd-2 (closed) should not be in group with open issues")
		}
	}
}
