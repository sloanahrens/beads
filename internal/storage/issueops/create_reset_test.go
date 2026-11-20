package issueops

import (
	"errors"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

func TestSnapshotUnsetCreateIDs(t *testing.T) {
	tests := []struct {
		name  string
		build func() []*types.Issue
		// fill simulates the create path minting IDs into the caller's structs
		// after the snapshot was taken.
		fill func(issues []*types.Issue)
		// check runs after reset and asserts the caller's structs are back to
		// how they were passed in.
		check func(t *testing.T, issues []*types.Issue)
	}{
		{
			name: "minted issue dependency and comment ids are emptied",
			build: func() []*types.Issue {
				return []*types.Issue{{
					ID:           "",
					Dependencies: []*types.Dependency{{IssueID: ""}},
					Comments:     []*types.Comment{{ID: ""}},
				}}
			},
			fill: func(issues []*types.Issue) {
				issues[0].ID = "be-minted"
				issues[0].Dependencies[0].IssueID = "be-minted"
				issues[0].Comments[0].ID = "be-minted.1"
			},
			check: func(t *testing.T, issues []*types.Issue) {
				t.Helper()
				if got := issues[0].ID; got != "" {
					t.Errorf("issue ID = %q, want empty", got)
				}
				if got := issues[0].Dependencies[0].IssueID; got != "" {
					t.Errorf("dependency IssueID = %q, want empty", got)
				}
				if got := issues[0].Comments[0].ID; got != "" {
					t.Errorf("comment ID = %q, want empty", got)
				}
			},
		},
		{
			name: "caller supplied ids are left in place",
			build: func() []*types.Issue {
				return []*types.Issue{{
					ID:           "be-keep",
					Dependencies: []*types.Dependency{{IssueID: "be-keep"}},
					Comments:     []*types.Comment{{ID: "keep-1"}},
				}}
			},
			fill: func([]*types.Issue) {},
			check: func(t *testing.T, issues []*types.Issue) {
				t.Helper()
				if got := issues[0].ID; got != "be-keep" {
					t.Errorf("issue ID = %q, want %q", got, "be-keep")
				}
				if got := issues[0].Dependencies[0].IssueID; got != "be-keep" {
					t.Errorf("dependency IssueID = %q, want %q", got, "be-keep")
				}
				if got := issues[0].Comments[0].ID; got != "keep-1" {
					t.Errorf("comment ID = %q, want %q", got, "keep-1")
				}
			},
		},
		{
			name: "only ids empty at snapshot time are emptied",
			build: func() []*types.Issue {
				return []*types.Issue{
					nil,
					{
						ID:           "be-keep",
						Dependencies: []*types.Dependency{nil, {IssueID: ""}, {IssueID: "be-keep"}},
						Comments:     []*types.Comment{nil, {ID: ""}, {ID: "keep-1"}},
					},
				}
			},
			fill: func(issues []*types.Issue) {
				issues[1].Dependencies[1].IssueID = "be-minted"
				issues[1].Comments[1].ID = "be-minted.1"
			},
			check: func(t *testing.T, issues []*types.Issue) {
				t.Helper()
				if len(issues) != 2 || issues[0] != nil {
					t.Fatalf("nil issue entry was not preserved: %#v", issues)
				}
				issue := issues[1]
				if got := issue.ID; got != "be-keep" {
					t.Errorf("issue ID = %q, want %q", got, "be-keep")
				}
				if issue.Dependencies[0] != nil {
					t.Errorf("nil dependency entry was not preserved")
				}
				if got := issue.Dependencies[1].IssueID; got != "" {
					t.Errorf("minted dependency IssueID = %q, want empty", got)
				}
				if got := issue.Dependencies[2].IssueID; got != "be-keep" {
					t.Errorf("caller dependency IssueID = %q, want %q", got, "be-keep")
				}
				if issue.Comments[0] != nil {
					t.Errorf("nil comment entry was not preserved")
				}
				if got := issue.Comments[1].ID; got != "" {
					t.Errorf("minted comment ID = %q, want empty", got)
				}
				if got := issue.Comments[2].ID; got != "keep-1" {
					t.Errorf("caller comment ID = %q, want %q", got, "keep-1")
				}
			},
		},
		{
			name:  "empty issue slice",
			build: func() []*types.Issue { return nil },
			fill:  func([]*types.Issue) {},
			check: func(*testing.T, []*types.Issue) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := tt.build()
			reset := SnapshotUnsetCreateIDs(issues)
			if reset == nil {
				t.Fatal("SnapshotUnsetCreateIDs returned a nil reset")
			}
			tt.fill(issues)
			reset()
			tt.check(t, issues)
		})
	}
}

func TestResetCreateIDsIfRolledBack(t *testing.T) {
	otherErr := errors.New("insert failed")

	tests := []struct {
		name      string
		err       error
		nilReset  bool
		wantReset bool
	}{
		{name: "nil error keeps the filled ids", err: nil},
		{name: "plain error resets", err: otherErr, wantReset: true},
		{name: "indeterminate commit keeps the filled ids", err: storage.ErrCommitIndeterminate},
		{name: "joined indeterminate commit keeps the filled ids", err: errors.Join(storage.ErrCommitIndeterminate, otherErr)},
		{name: "nil reset does not panic", err: otherErr, nilReset: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCalled := false
			var reset func()
			if !tt.nilReset {
				reset = func() { resetCalled = true }
			}

			got := ResetCreateIDsIfRolledBack(tt.err, reset)

			if got != tt.err {
				t.Fatalf("returned error = %v, want the same error %v", got, tt.err)
			}
			if resetCalled != tt.wantReset {
				t.Fatalf("reset called = %v, want %v", resetCalled, tt.wantReset)
			}
		})
	}
}
