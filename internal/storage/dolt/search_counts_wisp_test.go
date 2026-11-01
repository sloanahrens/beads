package dolt

import (
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/sqlbuild"
	"github.com/steveyegge/beads/internal/types"
)

// TestSearchCountsWispsConditionalAggregates pins dep_count and parent_id for
// the filtered predicate form on a wisps table after be-bbcb moved the type
// test out of the dc/pc WHERE and into COUNT(CASE ...)/MIN(CASE ...).
//
// The fixture covers the four edge shapes that make the rewrite non-trivial: a
// blocks edge, a parent-child edge, both, and neither — plus one row of an
// untyped dependency on the "neither" wisp, which is the row a COUNT(*) that
// dropped the CASE would wrongly count as a blocker. Expected values are
// literals; the same rows are also compared against the pre-rewrite
// whole-table statement (oldSearchCountsSQL) to prove the rewrite is
// result-identical.
func TestSearchCountsWispsConditionalAggregates(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	// wc-none: neither edge (only a non-counted relates-to row)
	// wc-blocks: one blocks edge
	// wc-parent: one parent-child edge
	// wc-both: two blocks edges and one parent-child edge
	// The three wc-target-* rows exist so the dependency targets resolve.
	for _, id := range []string{
		"wc-none", "wc-blocks", "wc-parent", "wc-both",
		"wc-target-a", "wc-target-b", "wc-ancestor",
	} {
		createWisp(t, ctx, store, id)
	}
	for _, dep := range []*types.Dependency{
		{IssueID: "wc-blocks", DependsOnID: "wc-target-a", Type: types.DepBlocks},
		{IssueID: "wc-both", DependsOnID: "wc-target-a", Type: types.DepBlocks},
		{IssueID: "wc-both", DependsOnID: "wc-target-b", Type: types.DepBlocks},
		{IssueID: "wc-parent", DependsOnID: "wc-ancestor", Type: types.DepParentChild},
		{IssueID: "wc-both", DependsOnID: "wc-ancestor", Type: types.DepParentChild},
		{IssueID: "wc-none", DependsOnID: "wc-target-a", Type: types.DepRelatesTo},
	} {
		if err := store.AddDependency(ctx, dep, "tester"); err != nil {
			t.Fatalf("AddDependency %s -> %s (%s): %v", dep.IssueID, dep.DependsOnID, dep.Type, err)
		}
	}

	want := map[string]struct {
		depCount int
		parent   string
	}{
		"wc-none":   {depCount: 0, parent: ""},
		"wc-blocks": {depCount: 1, parent: ""},
		"wc-parent": {depCount: 0, parent: "wc-ancestor"},
		"wc-both":   {depCount: 2, parent: "wc-ancestor"},
	}

	filter := types.IssueFilter{IDs: []string{"wc-none", "wc-blocks", "wc-parent", "wc-both"}}
	whereClauses, args, err := sqlbuild.BuildIssueFilterClauses("", filter, sqlbuild.WispsFilterTables)
	if err != nil {
		t.Fatalf("BuildIssueFilterClauses: %v", err)
	}
	if len(whereClauses) == 0 {
		t.Fatal("filter produced no WHERE clause; the test would compare two unfiltered queries")
	}
	whereSQL := "WHERE " + strings.Join(whereClauses, " AND ")

	hyd := sqlbuild.CountsHydration{}
	newSQL, genArgs := sqlbuild.SearchCountsSQL(sqlbuild.WispsFilterTables, nil, whereSQL, "", "", true, hyd)
	if genArgs != nil {
		t.Fatalf("predicate form must not generate args, got %d", len(genArgs))
	}
	oldSQL := oldSearchCountsSQL(sqlbuild.WispsFilterTables, whereSQL, "", "", true, false)

	gotNew := runCountsSQL(ctx, t, store.db, newSQL, args, hyd)
	gotOld := runCountsSQL(ctx, t, store.db, oldSQL, args, hyd)
	if len(gotNew) != len(want) || len(gotOld) != len(want) {
		t.Fatalf("row count: new=%d old=%d, want %d each", len(gotNew), len(gotOld), len(want))
	}

	byID := func(rows []countsRow) map[string]countsRow {
		out := make(map[string]countsRow, len(rows))
		for _, row := range rows {
			out[row.id] = row
		}
		return out
	}
	newByID := byID(gotNew)
	oldByID := byID(gotOld)

	for id, w := range want {
		newRow, ok := newByID[id]
		if !ok {
			t.Fatalf("new query returned no row for %s", id)
		}
		oldRow, ok := oldByID[id]
		if !ok {
			t.Fatalf("old query returned no row for %s", id)
		}
		if newRow.depCount != oldRow.depCount || newRow.parent != oldRow.parent {
			t.Errorf("%s: new (dep_count=%d parent=%q) differs from old (dep_count=%d parent=%q)",
				id, newRow.depCount, newRow.parent, oldRow.depCount, oldRow.parent)
		}
		if newRow.depCount != w.depCount || newRow.parent != w.parent {
			t.Errorf("%s: dep_count=%d parent=%q, want dep_count=%d parent=%q",
				id, newRow.depCount, newRow.parent, w.depCount, w.parent)
		}
	}
}
