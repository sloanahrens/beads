package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/issueops"
)

// fixedResultEdgeReader answers every read with one fixed result, so a test can
// drive an answer the id-keyed fakeEdgeReader cannot build: no anchor at all.
type fixedResultEdgeReader struct{ result issueops.EdgeReadResult }

func (r fixedResultEdgeReader) ReadEdges(context.Context, issueops.EdgeReadRequest) (issueops.EdgeReadResult, error) {
	return r.result, nil
}

// TestShowExternalDependenciesSection: of the stored edges, only the target
// with no row (not in the hydrated dependency list) appears, with its type; an
// issue with no such edge prints no section at all.
func TestShowExternalDependenciesSection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	shown := []*types.IssueWithDependencyMetadata{{Issue: types.Issue{ID: "bd-2"}, DependencyType: types.DepBlocks}}

	t.Run("lists only the target with no row", func(t *testing.T) {
		t.Parallel()
		store := &edgeReaderStore{reader: &fakeEdgeReader{edges: map[string][]*types.Dependency{
			"bd-1": {
				{IssueID: "bd-1", DependsOnID: "bd-2", Type: types.DepBlocks},
				{IssueID: "bd-1", DependsOnID: "external:other:cap", Type: types.DepRelated},
			},
		}}}

		got := showExternalDependenciesSection(ctx, store, "bd-1", shown)
		if !strings.Contains(got, "EXTERNAL DEPENDENCIES") ||
			!strings.Contains(got, "external:other:cap via related") ||
			strings.Contains(got, "bd-2") {
			t.Fatalf("section = %q, want only external:other:cap via related", got)
		}
	})

	t.Run("no unrowed target, no section", func(t *testing.T) {
		t.Parallel()
		store := &edgeReaderStore{reader: &fakeEdgeReader{edges: map[string][]*types.Dependency{
			"bd-1": {{IssueID: "bd-1", DependsOnID: "bd-2", Type: types.DepBlocks}},
		}}}

		if got := showExternalDependenciesSection(ctx, store, "bd-1", shown); got != "" {
			t.Fatalf("section = %q, want none", got)
		}
	})
}

// TestShowExternalDependenciesUnread: a read that failed is reported as unread,
// never as an empty section — "could not be read" and "no external
// dependencies" are different answers, and only one is established here.
func TestShowExternalDependenciesUnread(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cases := []struct {
		name  string
		store *edgeReaderStore
	}{
		{"accessor fails", &edgeReaderStore{err: errors.New("edge accessor unavailable")}},
		{"read fails", &edgeReaderStore{reader: &fakeEdgeReader{err: errors.New("dependency table is unreachable")}}},
		{"answer carries no anchor", &edgeReaderStore{reader: fixedResultEdgeReader{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := showExternalDependenciesSection(ctx, tc.store, "bd-1", nil); !strings.Contains(got, "could not be read") {
				t.Fatalf("section = %q, want it to say the external dependencies could not be read", got)
			}
		})
	}
}
