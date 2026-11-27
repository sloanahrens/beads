package main

import (
	"slices"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// TestDroppedDepEdges pins the selection warnDroppedDepEdges uses: the stored
// edges whose target ID was not shown, in their stored order.
func TestDroppedDepEdges(t *testing.T) {
	t.Parallel()
	dep := func(target string) *types.Dependency { return &types.Dependency{IssueID: "anchor", DependsOnID: target} }
	tests := []struct {
		name  string
		edges []*types.Dependency
		shown []string
		want  []string
	}{
		{"every target shown gives none", []*types.Dependency{dep("be-a"), dep("be-b")}, []string{"be-a", "be-b"}, nil},
		{"an unshown target is returned", []*types.Dependency{dep("be-a"), dep("external:rig/be-b")}, []string{"be-a"}, []string{"external:rig/be-b"}},
		{"two unshown targets keep their order", []*types.Dependency{dep("be-a"), dep("be-z"), dep("be-b"), dep("be-y")}, []string{"be-a", "be-b"}, []string{"be-z", "be-y"}},
		{"no edges gives none", nil, []string{"be-a"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, d := range droppedDepEdges(tt.edges, tt.shown) {
				got = append(got, d.DependsOnID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("droppedDepEdges targets = %v, want %v", got, tt.want)
			}
		})
	}
}
