package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

// TestShowExternalJSONApply pins fillExternalDeps: a found edge lands in that
// view's external_dependencies, an edge-less view marshals as before, and a
// failed read marks the group's views unread and returns the error.
func TestShowExternalJSONApply(t *testing.T) {
	t.Parallel()
	decode := func(t *testing.T, details *issueops.IssueDetails) map[string]json.RawMessage {
		t.Helper()
		raw, err := json.Marshal(details)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var view map[string]json.RawMessage
		if err := json.Unmarshal(raw, &view); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return view
	}
	details := map[string]*issueops.IssueDetails{"bd-1": {Issue: types.Issue{ID: "bd-1"}}, "bd-2": {Issue: types.Issue{ID: "bd-2"}}}
	before := decode(t, details["bd-2"])
	reader := &fakeEdgeReader{edges: map[string][]*types.Dependency{
		"bd-1": {{IssueID: "bd-1", DependsOnID: "ext-9", Type: types.DepRelated}},
	}}
	group := showBatchGroup{store: &edgeReaderStore{reader: reader}, ids: []string{"bd-1", "bd-2"}}
	if err := fillExternalDeps(context.Background(), group, details); err != nil {
		t.Fatalf("fillExternalDeps: %v", err)
	}
	if got := string(decode(t, details["bd-1"])["external_dependencies"]); got != `[{"id":"ext-9","type":"related"}]` {
		t.Errorf("external_dependencies = %s, want the ext-9 edge", got)
	}
	if after := decode(t, details["bd-2"]); !reflect.DeepEqual(after, before) {
		t.Errorf("edge-less view = %v, want unchanged %v", after, before)
	}
	// A failed read: every view of the group is marked and the error returned.
	want := errors.New("edge read is unreachable")
	failed := map[string]*issueops.IssueDetails{
		"bd-1": {Issue: types.Issue{ID: "bd-1"}, Dependencies: shownDeps("bd-2")},
		"bd-2": {Issue: types.Issue{ID: "bd-2"}},
	}
	group = showBatchGroup{store: &edgeReaderStore{reader: &fakeEdgeReader{err: want}}, ids: []string{"bd-1", "bd-2"}}
	if err := fillExternalDeps(context.Background(), group, failed); !errors.Is(err, want) {
		t.Fatalf("error = %v, want the read failure", err)
	}
	for _, id := range []string{"bd-1", "bd-2"} {
		view := decode(t, failed[id])
		if got := string(view["external_dependencies_unread"]); got != "true" {
			t.Errorf("%s external_dependencies_unread = %s, want true", id, got)
		}
		if _, ok := view["dependencies_unread"]; ok {
			t.Errorf("%s has dependencies_unread, want it untouched", id)
		}
	}
}

// TestShowExternalJSONGroups pins fillShowExternalDeps across a batch: a failed
// group does not stop the later ones, and each failure is warned about once, on
// one line, naming only the views the group could not answer.
func TestShowExternalJSONGroups(t *testing.T) {
	t.Parallel()

	t.Run("failed group does not stop later ones", func(t *testing.T) {
		t.Parallel()
		details := map[string]*issueops.IssueDetails{"bd-1": {Issue: types.Issue{ID: "bd-1"}}, "bd-3": {Issue: types.Issue{ID: "bd-3"}}}
		groups := []showBatchGroup{
			{store: &edgeReaderStore{reader: &fakeEdgeReader{err: errors.New("edge read failed")}}, ids: []string{"bd-1"}},
			{store: &edgeReaderStore{reader: &fakeEdgeReader{edges: map[string][]*types.Dependency{
				"bd-3": {{IssueID: "bd-3", DependsOnID: "ext-9", Type: types.DepRelated}},
			}}}, ids: []string{"bd-3"}},
		}
		var warnings []string
		fillShowExternalDeps(context.Background(), groups, details, func(format string, args ...interface{}) {
			warnings = append(warnings, fmt.Sprintf(format, args...))
		})

		if got := details["bd-1"].ExternalDependenciesUnread; got == nil || !*got {
			t.Errorf("failed group's view is not marked external_dependencies_unread")
		}
		later := details["bd-3"]
		if len(later.ExternalDependencies) != 1 || later.ExternalDependencies[0].ID != "ext-9" {
			t.Errorf("later group's external_dependencies = %+v, want the ext-9 edge", later.ExternalDependencies)
		}
		if later.ExternalDependenciesUnread != nil {
			t.Errorf("later group's view is marked unread, want it untouched")
		}
		if len(warnings) != 1 {
			t.Fatalf("warnings = %q, want one for the failed group only", warnings)
		}
	})

	t.Run("warning counts views and folds a two-line error", func(t *testing.T) {
		t.Parallel()
		details := map[string]*issueops.IssueDetails{"bd-1": {Issue: types.Issue{ID: "bd-1"}}}
		group := showBatchGroup{store: &edgeReaderStore{reader: &fakeEdgeReader{err: errors.New("edge read failed\nand more")}}, ids: []string{"bd-1", "bd-2"}}
		var warnings []string
		fillShowExternalDeps(context.Background(), []showBatchGroup{group}, details, func(format string, args ...interface{}) {
			warnings = append(warnings, fmt.Sprintf(format, args...))
		})

		want := "external dependencies unread for 1 issue(s): edge read failed and more"
		if len(warnings) != 1 || warnings[0] != want {
			t.Fatalf("warnings = %q, want %q", warnings, want)
		}
	})
}
