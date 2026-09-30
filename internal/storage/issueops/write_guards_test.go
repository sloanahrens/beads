package issueops

import (
	"errors"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
	publicops "github.com/steveyegge/beads/issueops"
)

func strPtr(s string) *string { return &s }

func TestExpectedFieldsMismatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name               string
		assignee, status   string
		wantAssignee, want *string
		wantErr            error
	}{
		{"no guards", "a", "open", nil, nil, nil},
		{"status holds", "a", "open", nil, strPtr("open"), nil},
		{"status mismatch", "a", "pinned", nil, strPtr("open"), storage.ErrStatusMismatch},
		{"unassigned holds", "", "open", strPtr(""), nil, nil},
		{"unassigned mismatch", "a", "open", strPtr(""), nil, storage.ErrAssigneeMismatch},
		{"assignee mismatch", "a", "open", strPtr("b"), nil, storage.ErrAssigneeMismatch},
	}
	for _, tc := range cases {
		err := ExpectedFieldsMismatch("x-1", tc.assignee, tc.status, tc.wantAssignee, tc.want)
		if tc.wantErr == nil && err != nil {
			t.Errorf("%s: unexpected %v", tc.name, err)
		}
		if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestCheckDeleteGuards(t *testing.T) {
	t.Parallel()
	rows := []*types.Issue{
		{ID: "x-1", Status: types.StatusOpen},
		{ID: "x-2", Status: types.StatusInProgress, Assignee: "w"},
		{ID: "x-3", Status: types.StatusPinned},
	}
	if err := CheckDeleteGuards(publicops.DeleteRequest{IDs: []string{"x-1", "x-2"}}, rows); err != nil {
		t.Fatalf("no guards: %v", err)
	}
	err := CheckDeleteGuards(publicops.DeleteRequest{IDs: []string{"x-3", "x-1", "x-2"}, ExpectedStatus: strPtr("open")}, rows)
	var ge *publicops.DeleteGuardError
	if !errors.As(err, &ge) {
		t.Fatalf("err = %v, want *DeleteGuardError", err)
	}
	if len(ge.IDs) != 2 || ge.IDs[0] != "x-3" || ge.IDs[1] != "x-2" {
		t.Fatalf("ids = %v, want request order [x-3 x-2]", ge.IDs)
	}
	if !errors.Is(err, storage.ErrStatusMismatch) {
		t.Fatalf("err does not match ErrStatusMismatch: %v", err)
	}
	// A row the probe did not return is the existence probe's to refuse.
	if err := CheckDeleteGuards(publicops.DeleteRequest{IDs: []string{"x-9"}, ExpectedStatus: strPtr("open")}, rows); err != nil {
		t.Fatalf("missing row: %v", err)
	}
}
