package dolt

import (
	"strings"
	"testing"
)

// dependentsArms splits dependentsIterSQL into its UNION ALL branches.
func dependentsArms(t *testing.T, sql string) []string {
	t.Helper()
	arms := strings.Split(sql, "UNION ALL")
	for i := range arms {
		arms[i] = strings.TrimSpace(arms[i])
	}
	return arms
}

// TestDependentsIterSQLSeeksTypedTargetColumns pins the dependents iterator's
// WHERE shape: one bare equality per typed target column per dependency table,
// with no predicate on the COALESCE target expression, which Dolt cannot
// index. An edge carries exactly one typed target column, so the six arms
// select the same rows the two COALESCE-predicated arms did.
func TestDependentsIterSQLSeeksTypedTargetColumns(t *testing.T) {
	t.Parallel()

	sql := dependentsIterSQL()
	if strings.Contains(sql, "COALESCE") {
		t.Errorf("dependents iterator must not predicate on the unindexable COALESCE target:\n%s", sql)
	}

	// Branch order is the bind order: `dependencies` issues, wisp, external,
	// then `wisp_dependencies` in the same order (DepTargetColumns order).
	type arm struct{ table, alias, column string }
	want := []arm{
		{"dependencies", "i", "depends_on_issue_id"},
		{"dependencies", "i", "depends_on_wisp_id"},
		{"dependencies", "i", "depends_on_external"},
		{"wisp_dependencies", "w", "depends_on_issue_id"},
		{"wisp_dependencies", "w", "depends_on_wisp_id"},
		{"wisp_dependencies", "w", "depends_on_external"},
	}
	arms := dependentsArms(t, sql)
	if len(arms) != len(want) {
		t.Fatalf("dependents iterator has %d branches, want %d (one per table per typed target column):\n%s",
			len(arms), len(want), sql)
	}
	for i, w := range want {
		got := arms[i]
		if !strings.Contains(got, "d."+w.column+" = ?") {
			t.Errorf("branch %d must seek %s.%s:\n%s", i, w.alias, w.column, got)
		}
		if !strings.Contains(got, "JOIN "+w.table+" d ON d.issue_id = "+w.alias+".id") {
			t.Errorf("branch %d must join %s to %s:\n%s", i, w.table, w.alias, got)
		}
		if n := strings.Count(got, "?"); n != 1 {
			t.Errorf("branch %d has %d placeholders, want 1 (the typed target):\n%s", i, n, got)
		}
	}
}

// TestDependentsIterSQLProjectionAndOrderPinned pins what callers receive from
// the iterator: every branch projects the full issue row of its own plane plus
// the edge's type, and the compound keeps one ORDER BY created_at ASC, which
// row order for `bd show --include-dependents` depends on.
func TestDependentsIterSQLProjectionAndOrderPinned(t *testing.T) {
	t.Parallel()

	sql := dependentsIterSQL()
	arms := dependentsArms(t, sql)

	// The projection is the one the two-arm form selected, unchanged.
	for i, arm := range arms {
		alias := "i"
		if i >= 3 {
			alias = "w"
		}
		wantProjection := prefixedIssueColumns(alias) + ", d.type"
		start := strings.Index(arm, "SELECT ")
		end := strings.Index(arm, " FROM ")
		if start < 0 || end < start {
			t.Fatalf("branch %d is not a plain SELECT ... FROM shape:\n%s", i, arm)
		}
		if got := arm[start+len("SELECT ") : end]; got != wantProjection {
			t.Errorf("branch %d projection changed:\n got: %q\nwant: %q", i, got, wantProjection)
		}
		// Anchors that do not come from the shared column list: each arm
		// projects its own plane's row, with the edge type last.
		for _, col := range []string{alias + ".id,", alias + ".created_at,", ", d.type"} {
			if !strings.Contains(arm, col) {
				t.Errorf("branch %d projection lost %q:\n%s", i, col, arm)
			}
		}
	}

	if n := strings.Count(sql, "ORDER BY"); n != 1 {
		t.Fatalf("dependents iterator has %d ORDER BY clauses, want exactly 1 (on the compound):\n%s", n, sql)
	}
	if !strings.HasSuffix(strings.TrimSpace(sql), "ORDER BY created_at ASC") {
		t.Errorf("row order must stay ORDER BY created_at ASC at the end of the compound:\n%s", sql)
	}
}

// TestDependentsIterArgsSQLBindOrder pins the argument list to the placeholder
// count and order dependentsIterSQL emits: the target id once per typed target
// column per dependency table.
func TestDependentsIterArgsSQLBindOrder(t *testing.T) {
	t.Parallel()

	const id = "be-target"
	args := dependentsIterArgs(id)
	if n := strings.Count(dependentsIterSQL(), "?"); n != len(args) {
		t.Fatalf("dependentsIterSQL has %d placeholders but dependentsIterArgs binds %d", n, len(args))
	}
	if len(args) != 6 {
		t.Fatalf("dependentsIterArgs bound %d args, want 6 (three typed targets in each of two tables)", len(args))
	}
	for i, a := range args {
		if a != id {
			t.Errorf("arg %d = %v, want the bound target %q in every branch", i, a, id)
		}
	}
}

// TestCountsDependentsSQLSeeksTypedTargetColumns pins the two builders behind
// CountDependents and CountDependentsByStatus: the count(*) aggregate keeps its
// shape, and the target predicate is three bare equalities — one per typed
// target column, in COALESCE precedence order — instead of the COALESCE
// expression, which no plan can seek on Dolt.
func TestCountsDependentsSQLSeeksTypedTargetColumns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		sql       string
		perBranch int // placeholders in one branch: the target, plus the status when filtered
		want      []string
	}{
		{
			name:      "dependencies count",
			sql:       reverseDependentsSQL("dependencies"),
			perBranch: 1,
			want:      []string{"SELECT 1 FROM dependencies WHERE depends_on_issue_id = ?"},
		},
		{
			name:      "wisp_dependencies count",
			sql:       reverseDependentsSQL("wisp_dependencies"),
			perBranch: 1,
			want:      []string{"SELECT 1 FROM wisp_dependencies WHERE depends_on_issue_id = ?"},
		},
		{
			name:      "dependencies by status",
			sql:       dependentsByStatusSQL("dependencies d JOIN issues i ON i.id = d.issue_id", "i.status = ?"),
			perBranch: 2,
			want: []string{
				"SELECT 1 FROM dependencies d JOIN issues i ON i.id = d.issue_id WHERE d.depends_on_issue_id = ? AND i.status = ?",
			},
		},
		{
			name:      "wisp_dependencies by status",
			sql:       dependentsByStatusSQL("wisp_dependencies d JOIN wisps w ON w.id = d.issue_id", "w.status = ?"),
			perBranch: 2,
			want: []string{
				"SELECT 1 FROM wisp_dependencies d JOIN wisps w ON w.id = d.issue_id WHERE d.depends_on_issue_id = ? AND w.status = ?",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if strings.Contains(tc.sql, "COALESCE") {
				t.Errorf("count must not predicate on the unindexable COALESCE target:\n%s", tc.sql)
			}
			// The aggregate and its wrapping subquery keep the shape the count
			// callers scan.
			if !strings.HasPrefix(tc.sql, "SELECT count(*) FROM (") || !strings.HasSuffix(tc.sql, ") dep_targets") {
				t.Errorf("count query shape changed:\n%s", tc.sql)
			}
			for _, want := range tc.want {
				if !strings.Contains(tc.sql, want) {
					t.Errorf("missing branch %q:\n%s", want, tc.sql)
				}
			}

			// One branch per typed target column, in DepTargetColumns order:
			// the bind order callers' args follow.
			last := -1
			for _, col := range []string{"depends_on_issue_id", "depends_on_wisp_id", "depends_on_external"} {
				at := strings.Index(tc.sql, col+" = ?")
				if at < 0 {
					t.Errorf("branch for %s missing its bare equality:\n%s", col, tc.sql)
					continue
				}
				if at <= last {
					t.Errorf("branch for %s is out of DepTargetColumns order:\n%s", col, tc.sql)
				}
				last = at
			}
			if n, want := strings.Count(tc.sql, "?"), 3*tc.perBranch; n != want {
				t.Errorf("placeholders = %d, want %d (three branches x %d):\n%s", n, want, tc.perBranch, tc.sql)
			}
		})
	}
}
