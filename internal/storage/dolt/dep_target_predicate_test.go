package dolt

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/sqlbuild"
	"github.com/steveyegge/beads/internal/types"
)

// The reverse-dependency lookups used to predicate on the COALESCE target
// expression, which no index can serve. They now decompose it into one indexed
// equality per typed target column (sqlbuild.DepTargetSelectSQL). These tests
// pin the two things that decomposition has to be: RESULT-IDENTICAL to the
// COALESCE predicate over rows in all three target columns, and INDEXED on
// both dependency tables.
//
// Measured on the reference Dolt before the change, on a live rig database:
//
//	WHERE COALESCE(depends_on_issue_id, depends_on_wisp_id, depends_on_external) = 'X'
//	  -> Filter -> Table(name: wisp_dependencies)                    (full scan)
//
// and after:
//
//	SELECT count(*) FROM (SELECT 1 FROM wisp_dependencies WHERE depends_on_issue_id = 'X'
//	  UNION ALL ... depends_on_wisp_id = 'X' UNION ALL ... depends_on_external = 'X') t
//	  -> three IndexedTableAccess, one per target column              (index seeks)
//
// The equivalent per-column OR (`a = ? OR b = ? OR c = ?`) was measured too and
// plans as the same full Table scan — Dolt does not index-merge an OR of
// distinct columns — which is why the decomposition is a UNION ALL.

var depTargetPredicateFixture = struct {
	sourceIssue string
	sourceWisp  string
	targets     []struct{ kind, id string }
}{
	sourceIssue: "rdp-src-issue",
	sourceWisp:  "rdp-src-wisp",
	targets: []struct{ kind, id string }{
		{"issue", "rdp-tgt-issue"},
		{"wisp", "rdp-tgt-wisp"},
		{"external", "external:rdp-ext"},
	},
}

// seedDepTargetFixture creates a fixture with one inbound edge per target kind
// in EACH dependency table: the durable source contributes to `dependencies`,
// the wisp source to `wisp_dependencies`, and the targets exercise
// depends_on_issue_id, depends_on_wisp_id and depends_on_external. Rows are
// inserted directly so each typed column is populated by construction rather
// than through the classifier under test.
func seedDepTargetFixture(ctx context.Context, t *testing.T, store *DoltStore) {
	t.Helper()

	mkIssue := func(id string, ephemeral bool) {
		iss := &types.Issue{ID: id, Title: id, Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask, Ephemeral: ephemeral}
		if err := store.CreateIssue(ctx, iss, "tester"); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	f := depTargetPredicateFixture
	mkIssue(f.sourceIssue, false)
	mkIssue(f.targets[0].id, false) // durable issue target
	mkIssue(f.sourceWisp, true)
	mkIssue(f.targets[1].id, true) // wisp target

	insert := func(table, source string) {
		t.Helper()
		for _, tgt := range f.targets {
			var issueTarget, wispTarget, externalTarget any
			switch tgt.kind {
			case "issue":
				issueTarget = tgt.id
			case "wisp":
				wispTarget = tgt.id
			case "external":
				externalTarget = tgt.id
			}
			if _, err := store.db.ExecContext(ctx,
				"INSERT INTO "+table+" (id, issue_id, depends_on_issue_id, depends_on_wisp_id, depends_on_external, type, created_at, created_by) "+
					"VALUES (UUID(), ?, ?, ?, ?, 'blocks', NOW(), 'tester')",
				source, issueTarget, wispTarget, externalTarget); err != nil {
				t.Fatalf("seed %s edge %s->%s: %v", table, source, tgt.id, err)
			}
		}
	}
	insert("dependencies", f.sourceIssue)
	insert("wisp_dependencies", f.sourceWisp)
}

// TestReverseDependencyTargetPredicateParity proves the decomposed predicate
// returns exactly the rows (sources) and count the COALESCE predicate does, for
// every typed target column, on both tables — and that the production
// CountDependents path agrees with the reference.
func TestReverseDependencyTargetPredicateParity(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	seedDepTargetFixture(ctx, t, store)

	f := depTargetPredicateFixture
	for _, table := range []string{"dependencies", "wisp_dependencies"} {
		table := table
		for _, tgt := range f.targets {
			tgt := tgt
			t.Run(table+"/"+tgt.kind, func(t *testing.T) {
				refRows := queryStrings(ctx, t, store.db,
					"SELECT issue_id FROM "+table+" WHERE "+sqlbuild.DepTargetExpr+" = ? ORDER BY issue_id",
					tgt.id)
				gotRows := queryStrings(ctx, t, store.db,
					"SELECT issue_id FROM ("+sqlbuild.DepTargetSelectSQL(table, "issue_id", "%s = ?")+") z ORDER BY issue_id",
					sqlbuild.DepTargetBranchArgs(tgt.id)...)
				if len(refRows) != len(gotRows) {
					t.Fatalf("row count: COALESCE=%v decomposed=%v", refRows, gotRows)
				}
				for i := range refRows {
					if refRows[i] != gotRows[i] {
						t.Fatalf("row %d: COALESCE=%q decomposed=%q", i, refRows[i], gotRows[i])
					}
				}
			})
		}
	}

	// The production count spans both tables: one durable-source edge plus one
	// wisp-source edge per target.
	for _, tgt := range f.targets {
		n, err := store.CountDependents(ctx, tgt.id)
		if err != nil {
			t.Fatalf("CountDependents(%s): %v", tgt.id, err)
		}
		if n != 2 {
			t.Fatalf("CountDependents(%s) = %d, want 2 (one edge per dependency table)", tgt.id, n)
		}
	}
	// Status-filtered path exercises the same decomposition with a join.
	if n, err := store.CountDependentsByStatus(ctx, f.targets[0].id, types.StatusOpen); err != nil {
		t.Fatalf("CountDependentsByStatus: %v", err)
	} else if n != 2 {
		t.Fatalf("CountDependentsByStatus = %d, want 2", n)
	}
}

// TestReverseDependencyCountPlanIsIndexed is the sargability guard: the
// production reverse-dependency count must reach an index on every typed target
// column of both tables, never a bare table scan. It EXPLAINs the exact
// production SQL (literalized) and skips rather than fails if the EXPLAIN
// format is unrecognizable.
func TestReverseDependencyCountPlanIsIndexed(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx, cancel := testContext(t)
	defer cancel()

	seedDepTargetFixture(ctx, t, store)

	target := depTargetPredicateFixture.targets[0].id
	for _, table := range []string{"dependencies", "wisp_dependencies"} {
		table := table
		t.Run(table, func(t *testing.T) {
			// Single-source the guarded SQL from production: three placeholders,
			// one per typed target column.
			plan := explainPlan(t, ctx, store.db, literalizeParams(
				sqlbuild.DepTargetCountSQL(table, "%s = ?"), "'"+target+"'", "'"+target+"'", "'"+target+"'"))
			if !looksLikeDoltPlan(plan) {
				t.Skipf("EXPLAIN output not in a recognized Dolt plan format, skipping sargability assertion; plan=\n%s", plan)
			}
			for _, col := range sqlbuild.DepTargetColumns {
				if !strings.Contains(plan, table+"."+col) {
					t.Fatalf("count on %s does not seek %s.%s — the reverse-dependency count regressed to a scan.\nplan:\n%s",
						table, table, col, plan)
				}
			}
			if strings.Contains(plan, "name: "+table) {
				t.Fatalf("count on %s still full-scans the table (bare Table node).\nplan:\n%s", table, plan)
			}
		})
	}
}

// queryStrings runs a query returning one text column and collects its rows.
func queryStrings(ctx context.Context, t *testing.T, db *sql.DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("query: %v\nSQL:\n%s", err, query)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	return out
}
