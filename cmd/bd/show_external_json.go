package main

import (
	"context"
	"strings"

	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/issueops"
)

// readExternalDeps answers, for the ids of one store group, which of each
// view's STORED dependency edges the detail read left out because its target
// has no row in that store — a cross-repo or `external:` target. It makes one
// EdgeReader read for the whole group and keys the answer by id, so an id the
// detail read answered keeps its own edges.
//
// A view whose dependency list could not be read (DependenciesUnread) is
// skipped: its shown set is unknown, and picking against an empty slice would
// report every stored edge as external. The caller already marks that view
// unread, so nothing is lost by saying nothing here.
func readExternalDeps(ctx context.Context, group showBatchGroup, detailsByID map[string]*issueops.IssueDetails) (map[string][]*types.Dependency, error) {
	reader, err := group.store.EdgeReader()
	if err != nil {
		return nil, err
	}
	result, err := reader.ReadEdges(ctx, issueops.EdgeReadRequest{IDs: group.ids})
	if err != nil {
		return nil, err
	}
	external := make(map[string][]*types.Dependency)
	for _, anchor := range result.Anchors {
		details, ok := detailsByID[anchor.ID]
		if !ok || (details.DependenciesUnread != nil && *details.DependenciesUnread) {
			continue
		}
		shown := make([]string, 0, len(details.Dependencies))
		for _, dep := range details.Dependencies {
			shown = append(shown, dep.ID)
		}
		if dropped := droppedDepEdges(anchor.Edges, shown); len(dropped) > 0 {
			external[anchor.ID] = dropped
		}
	}
	return external, nil
}

// fillExternalDeps applies readExternalDeps' answer to one store group's
// detail views. A failed read marks every view of this group
// ExternalDependenciesUnread and returns the error, so a failure never
// marshals as "no external edges"; DependenciesUnread answers another read.
func fillExternalDeps(ctx context.Context, group showBatchGroup, detailsByID map[string]*issueops.IssueDetails) error {
	external, err := readExternalDeps(ctx, group, detailsByID)
	if err != nil {
		unread := true
		for _, id := range group.ids {
			if details, ok := detailsByID[id]; ok {
				details.ExternalDependenciesUnread = &unread
			}
		}
		return err
	}
	for id, deps := range external {
		details, ok := detailsByID[id]
		if !ok {
			continue
		}
		for _, dep := range deps {
			details.ExternalDependencies = append(details.ExternalDependencies,
				types.ExternalDependency{ID: dep.DependsOnID, Type: dep.Type})
		}
	}
	return nil
}

// fillShowExternalDeps fills every store group in order. A failed group's views
// are marked unread and warned about once, on one line; later groups still fill.
func fillShowExternalDeps(ctx context.Context, groups []showBatchGroup, detailsByID map[string]*issueops.IssueDetails, warn func(string, ...interface{})) {
	for _, group := range groups {
		if err := fillExternalDeps(ctx, group, detailsByID); err != nil {
			marked := make(map[string]bool, len(group.ids))
			for _, id := range group.ids {
				if _, ok := detailsByID[id]; ok {
					marked[id] = true
				}
			}
			warn("external dependencies unread for %d issue(s): %s", len(marked), strings.ReplaceAll(err.Error(), "\n", " "))
		}
	}
}
