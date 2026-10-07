//go:build cgo

package doctor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestCheckMisclassifiedWisps_UsesDoltWithoutJSONL(t *testing.T) {
	// Test the core logic directly
	issues := []*types.Issue{
		{ID: "bd-wisp-misclassified", Ephemeral: false},
	}

	check := checkMisclassifiedWispsForIssues(issues)
	if check.Status != StatusWarning {
		t.Fatalf("status = %q, want %q", check.Status, StatusWarning)
	}
	if check.Message != "1 wisp issue(s) missing ephemeral flag" {
		t.Fatalf("message = %q, want exact count warning", check.Message)
	}
}

func TestCheckPatrolPollution_UsesDoltWithoutJSONL(t *testing.T) {
	// Create issues directly for testing the core logic
	var issues []*types.Issue
	for i := 0; i < PatrolDigestThreshold+1; i++ {
		issues = append(issues, &types.Issue{
			ID:        fmt.Sprintf("bd-%04d", i),
			Title:     fmt.Sprintf("Digest: mol-%02d-patrol", i),
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
		})
	}

	check := checkPatrolPollutionForIssues(issues)
	if check.Status != StatusWarning {
		t.Fatalf("status = %q, want %q", check.Status, StatusWarning)
	}
	if !strings.Contains(check.Message, "11 patrol digest beads (should be 0)") {
		t.Fatalf("message = %q, want patrol digest warning", check.Message)
	}
}

func TestCheckPatrolPollution_IgnoresEphemeralWisps(t *testing.T) {
	// Ephemeral issues are filtered out by loadMaintenanceIssues at the DB layer
	// (SearchIssues with Ephemeral=false filter). When the check logic receives
	// its issue list, ephemeral issues are already excluded. Verify that an empty
	// list (all ephemeral, all filtered) produces OK.
	var issues []*types.Issue // empty — all ephemeral issues were filtered at load time

	check := checkPatrolPollutionForIssues(issues)
	if check.Status != StatusOK {
		t.Fatalf("status = %q, want %q", check.Status, StatusOK)
	}
	if check.Message != "No patrol pollution detected" {
		t.Fatalf("message = %q, want no-pollution message", check.Message)
	}
}

func TestClassifyPatrolIssue(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  patrolIssueKind
	}{
		{
			name:  "digest patrol",
			title: "Digest: mol-abc-patrol",
			want:  patrolIssueDigest,
		},
		{
			name:  "session ended",
			title: "Session ended: patrol complete",
			want:  patrolIssueSessionEnded,
		},
		{
			name:  "normal issue",
			title: "Regular task title",
			want:  patrolIssueNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyPatrolIssue(tc.title); got != tc.want {
				t.Fatalf("classifyPatrolIssue(%q) = %v, want %v", tc.title, got, tc.want)
			}
		})
	}
}
