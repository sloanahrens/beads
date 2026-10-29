package issueops

import (
	"strings"
	"testing"
)

// normalizeSQLSpace collapses every run of whitespace (including the newlines and
// tabs the query builders emit) to one space, so the assertions pin the SQL
// tokens and not the layout of the Go source.
func normalizeSQLSpace(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

// TestReverseDependentsBatchSQL pins the batched reverse-dependents query: its
// WHERE predicate is one bare indexed equality per typed target column, and the
// SELECT list and ORDER BY keep the COALESCE target expression they had before.
func TestReverseDependentsBatchSQL(t *testing.T) {
	const inClause = "?,?"
	const wantSelect = "SELECT id, issue_id, COALESCE(depends_on_issue_id, depends_on_wisp_id, depends_on_external) AS depends_on_id, type, created_at, created_by, metadata, thread_id FROM ("
	const wantOrderBy = ") AS dep ORDER BY COALESCE(depends_on_issue_id, depends_on_wisp_id, depends_on_external)"

	for _, depTable := range []string{"dependencies", "wisp_dependencies"} {
		t.Run(depTable, func(t *testing.T) {
			got := normalizeSQLSpace(reverseDependentsBatchSQL(depTable, inClause))

			if !strings.HasPrefix(got, wantSelect) {
				t.Errorf("SELECT list changed:\n got: %s\nwant prefix: %s", got, wantSelect)
			}
			if !strings.HasSuffix(got, wantOrderBy) {
				t.Errorf("ORDER BY changed:\n got: %s\nwant suffix: %s", got, wantOrderBy)
			}
			if n := strings.Count(got, "COALESCE("); n != 2 {
				t.Errorf("COALESCE occurs %d times, want 2 (the projection and the ORDER BY only; none in WHERE)", n)
			}
			for _, col := range []string{"depends_on_issue_id", "depends_on_wisp_id", "depends_on_external"} {
				want := "FROM " + depTable + " WHERE " + col + " IN (" + inClause + ")"
				if !strings.Contains(got, want) {
					t.Errorf("missing per-column WHERE branch %q in:\n%s", want, got)
				}
			}
			if n := strings.Count(got, "UNION ALL"); n != 2 {
				t.Errorf("UNION ALL occurs %d times, want 2 (one join per typed target column)", n)
			}
		})
	}
}

// TestReverseDependentsBlocksInfoSQL pins the inbound-blocking query: the target
// predicate and the type filter sit in each per-column branch, and the projection
// keeps the COALESCE target expression over the d alias. The query has no ORDER BY.
func TestReverseDependentsBlocksInfoSQL(t *testing.T) {
	const inClause = "?,?"
	const wantSelect = "SELECT COALESCE(d.depends_on_issue_id, d.depends_on_wisp_id, d.depends_on_external) AS depends_on_id, d.issue_id, d.type FROM ("
	const wantTail = ") d"

	for _, depTable := range []string{"dependencies", "wisp_dependencies"} {
		t.Run(depTable, func(t *testing.T) {
			got := normalizeSQLSpace(reverseBlocksInfoSQL(depTable, inClause))

			if !strings.HasPrefix(got, wantSelect) {
				t.Errorf("SELECT list changed:\n got: %s\nwant prefix: %s", got, wantSelect)
			}
			if !strings.HasSuffix(got, wantTail) {
				t.Errorf("query tail changed:\n got: %s\nwant suffix: %s", got, wantTail)
			}
			if n := strings.Count(got, "COALESCE("); n != 1 {
				t.Errorf("COALESCE occurs %d times, want 1 (the projection only; none in WHERE)", n)
			}
			if strings.Contains(got, "ORDER BY") {
				t.Errorf("query gained an ORDER BY:\n%s", got)
			}
			for _, col := range []string{"depends_on_issue_id", "depends_on_wisp_id", "depends_on_external"} {
				want := "FROM " + depTable + " WHERE " + col + " IN (" + inClause + ") AND type IN ('blocks', 'parent-child')"
				if !strings.Contains(got, want) {
					t.Errorf("missing per-column WHERE branch %q in:\n%s", want, got)
				}
			}
		})
	}
}
