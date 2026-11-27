package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/issueops"
)

func shownDeps(ids ...string) []*types.IssueWithDependencyMetadata {
	deps := make([]*types.IssueWithDependencyMetadata, 0, len(ids))
	for _, id := range ids {
		deps = append(deps, &types.IssueWithDependencyMetadata{Issue: types.Issue{ID: id}})
	}
	return deps
}

// TestReadExternalDepsPicks pins that the answer holds only the edges the
// detail view did not show, each under the id whose edges they are. Mutating
// the pick to return every stored edge turns it red.
func TestReadExternalDepsPicks(t *testing.T) {
	t.Parallel()
	reader := &fakeEdgeReader{edges: map[string][]*types.Dependency{
		"bd-1": {
			{IssueID: "bd-1", DependsOnID: "bd-2", Type: types.DepBlocks},
			{IssueID: "bd-1", DependsOnID: "ext-9", Type: types.DepRelated},
		},
		"bd-2": {{IssueID: "bd-2", DependsOnID: "bd-1", Type: types.DepBlocks}},
	}}
	group := showBatchGroup{store: &edgeReaderStore{reader: reader}, ids: []string{"bd-1", "bd-2"}}
	details := map[string]*issueops.IssueDetails{
		"bd-1": {Dependencies: shownDeps("bd-2")},
		"bd-2": {Dependencies: shownDeps("bd-1")},
	}

	got, err := readExternalDeps(context.Background(), group, details)
	if err != nil {
		t.Fatalf("readExternalDeps: %v", err)
	}
	if len(reader.calls) != 1 || strings.Join(reader.calls[0], ",") != "bd-1,bd-2" {
		t.Errorf("edge read calls = %v, want one for the whole group", reader.calls)
	}
	if len(got) != 1 || len(got["bd-1"]) != 1 || got["bd-1"][0].DependsOnID != "ext-9" {
		t.Fatalf("external = %v, want only bd-1's ext-9 edge", got)
	}
	if _, ok := got["bd-2"]; ok {
		t.Errorf("bd-2 has entry %v, want none: its only edge was shown", got["bd-2"])
	}
}

// TestReadExternalDepsSkipsUnreadView pins that a view whose dependency list
// could not be read is not answered: its shown set is unknown, so every stored
// edge would be reported external.
func TestReadExternalDepsSkipsUnreadView(t *testing.T) {
	t.Parallel()
	reader := &fakeEdgeReader{edges: map[string][]*types.Dependency{
		"bd-1": {{IssueID: "bd-1", DependsOnID: "ext-9", Type: types.DepRelated}},
	}}
	group := showBatchGroup{store: &edgeReaderStore{reader: reader}, ids: []string{"bd-1"}}
	unread := true
	details := map[string]*issueops.IssueDetails{
		"bd-1": {DependenciesUnread: &unread},
	}

	got, err := readExternalDeps(context.Background(), group, details)
	if err != nil {
		t.Fatalf("readExternalDeps: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("external = %v, want nothing for an unread view", got)
	}
}

// TestReadExternalDepsError pins that neither failure is answered as "no
// external edges": a failed accessor and a failed read each return the error
// with no map.
func TestReadExternalDepsError(t *testing.T) {
	t.Parallel()
	want := errors.New("dependency table is unreachable")
	cases := map[string]*edgeReaderStore{
		"accessor": {err: want},
		"read":     {reader: &fakeEdgeReader{err: want}},
	}
	for name, store := range cases {
		group := showBatchGroup{store: store, ids: []string{"bd-1"}}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := readExternalDeps(context.Background(), group, map[string]*issueops.IssueDetails{"bd-1": {}})
			if !errors.Is(err, want) {
				t.Fatalf("error = %v, want the read failure", err)
			}
			if got != nil {
				t.Fatalf("external = %v, want none alongside the failure", got)
			}
		})
	}
}
