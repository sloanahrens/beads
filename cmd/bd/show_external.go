package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
	"github.com/steveyegge/beads/issueops"
)

// showExternalDependenciesSection renders the stored dependency edges of
// issueID whose target has no row in this database — an `external:` reference
// or another repository's id. Relations answers with the issue on the far end
// of each edge, so it drops these; deps is the hydrated list the caller already
// printed, and every other raw edge is one Relations dropped (droppedDepEdges).
//
// It returns "" when nothing was dropped. A failed read returns a section that
// says it could not be read: nothing would read as "no external dependencies",
// which a failure does not establish.
func showExternalDependenciesSection(ctx context.Context, st storage.DoltStorage, issueID string, deps []*types.IssueWithDependencyMetadata) string {
	unread := func(reason error) string {
		return fmt.Sprintf("\n%s\n  (could not be read: %v)\n", ui.RenderBold("EXTERNAL DEPENDENCIES"), reason)
	}

	reader, err := st.EdgeReader()
	if err != nil {
		return unread(err)
	}
	result, err := reader.ReadEdges(ctx, issueops.EdgeReadRequest{IDs: []string{issueID}})
	if err != nil {
		return unread(err)
	}
	// Without this issue's anchor, nothing establishes that no edge was dropped.
	if len(result.Anchors) != 1 || result.Anchors[0].ID != issueID {
		return unread(fmt.Errorf("no edge answer for %s", issueID))
	}

	shown := make([]string, 0, len(deps))
	for _, dep := range deps {
		shown = append(shown, dep.ID)
	}
	dropped := droppedDepEdges(result.Anchors[0].Edges, shown)
	if len(dropped) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\n%s\n", ui.RenderBold("EXTERNAL DEPENDENCIES"))
	for _, dep := range dropped {
		fmt.Fprintf(&b, "  %s via %s\n", dep.DependsOnID, dep.Type)
	}
	return b.String()
}
