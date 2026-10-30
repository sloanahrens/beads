package issueops

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/steveyegge/beads/internal/types"
)

// normalizeSQLSpace collapses every run of whitespace (including the newlines and
// tabs the query builders emit) to one space, so the assertions pin the SQL
// tokens and not the layout of the Go source.
func normalizeSQLSpace(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

// typedTargetColumns are the three typed target columns the WHERE predicate must
// name, one branch each.
var typedTargetColumns = []string{"depends_on_issue_id", "depends_on_wisp_id", "depends_on_external"}

// TestReverseDependentsBatchSQL pins the batched reverse-dependents query: its
// WHERE predicate names each typed target column with no COALESCE, and the
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
			for _, col := range typedTargetColumns {
				if want := col + " IN (" + inClause + ")"; !strings.Contains(got, want) {
					t.Errorf("WHERE does not seek %q:\n%s", want, got)
				}
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
			for _, col := range typedTargetColumns {
				want := col + " IN (" + inClause + ") AND type IN ('blocks', 'parent-child')"
				if !strings.Contains(got, want) {
					t.Errorf("WHERE does not seek %q:\n%s", want, got)
				}
			}
		})
	}
}

// TestReverseDependentsNewlyUnblockedByCloseSQL pins the close-unblocks
// candidate read: one SELECT per typed target column, UNION ALL'd, each with a
// bare equality on that column and the blocks type filter.
func TestReverseDependentsNewlyUnblockedByCloseSQL(t *testing.T) {
	for _, depTable := range []string{"dependencies", "wisp_dependencies"} {
		t.Run(depTable, func(t *testing.T) {
			got := normalizeSQLSpace(newlyUnblockedCandidateSQL(depTable))

			branches := strings.Split(got, " UNION ALL ")
			if len(branches) != len(typedTargetColumns) {
				t.Fatalf("candidate read has %d branches, want one per typed target column (%d):\n%s",
					len(branches), len(typedTargetColumns), got)
			}
			for i, col := range typedTargetColumns {
				want := "SELECT issue_id FROM " + depTable + " WHERE " + col + " = ? AND type = 'blocks'"
				if branches[i] != want {
					t.Errorf("branch %d:\n got: %s\nwant: %s", i, branches[i], want)
				}
			}
			// PLAN GUARD: Dolt plans the per-column OR of target predicates as a
			// full table scan (be-1jdi), so the branches must never collapse back
			// into one OR'd predicate.
			if strings.Contains(got, " OR ") {
				t.Errorf("candidate read ORs target predicates instead of UNION ALL branches:\n%s", got)
			}
			if strings.Contains(got, "COALESCE(") {
				t.Errorf("candidate read predicates on the COALESCE target:\n%s", got)
			}
		})
	}
}

// TestReverseDependentsNewlyUnblockedByClose drives the production close read
// and pins that it emits that per-column query and binds the closed id once per
// branch.
func TestReverseDependentsNewlyUnblockedByClose(t *testing.T) {
	const closedID = "bd-closed"

	tx, mock := newBindCheckedDB(t)
	rec := &recordingDBTX{DBTX: tx}
	for _, depTable := range []string{"dependencies", "wisp_dependencies"} {
		mock.ExpectQuery(regexp.QuoteMeta("FROM "+depTable+" WHERE")).
			WithArgs(closedID, closedID, closedID).
			WillReturnRows(sqlmock.NewRows([]string{"issue_id"}))
	}

	unblocked, err := GetNewlyUnblockedByCloseInTx(context.Background(), rec, closedID)
	if err != nil {
		t.Fatalf("GetNewlyUnblockedByCloseInTx: %v", err)
	}
	if len(unblocked) != 0 {
		t.Fatalf("unblocked = %d issues, want none when no dependency rows matched", len(unblocked))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
	if n := len(rec.queries); n != 2 {
		t.Fatalf("ran %d candidate queries, want one per dependency table", n)
	}

	for i, depTable := range []string{"dependencies", "wisp_dependencies"} {
		got := normalizeSQLSpace(rec.queries[i])
		want := normalizeSQLSpace(newlyUnblockedCandidateSQL(depTable))
		if got != want {
			t.Errorf("%s candidate read:\n got: %s\nwant: %s", depTable, got, want)
		}
	}
}

// bindCheckedDBTX fails any query whose ? placeholder count differs from the
// number of bind arguments it is given. The driver would reject that too, but
// sqlmock only compares the arguments it is told to expect, so this count check
// is what catches a query that repeats its placeholders without repeating binds.
type bindCheckedDBTX struct {
	DBTX
}

func (d bindCheckedDBTX) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if n := strings.Count(query, "?"); n != len(args) {
		return nil, fmt.Errorf("query has %d placeholders for %d bind args:\n%s", n, len(args), query)
	}
	return d.DBTX.QueryContext(ctx, query, args...)
}

// newBindCheckedDB returns a mock database whose queries must carry the expected
// bind arguments, in order, and whose placeholder counts match them.
func newBindCheckedDB(t *testing.T) (DBTX, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return bindCheckedDBTX{DBTX: db}, mock
}

// idBinds returns the bind list a batched read must send for ids: every id once
// per typed target column, in branch order.
func idBinds(ids ...string) []driver.Value {
	var out []driver.Value
	for range typedTargetColumns {
		for _, id := range ids {
			out = append(out, id)
		}
	}
	return out
}

// TestReverseDependentsBindsSQL drives the production reverse-dependents read
// and pins its bind list: the batch ids repeat once per typed target branch.
func TestReverseDependentsBindsSQL(t *testing.T) {
	ids := []string{"bd-1", "bd-2"}

	for _, depTable := range []string{"dependencies", "wisp_dependencies"} {
		t.Run(depTable, func(t *testing.T) {
			tx, mock := newBindCheckedDB(t)
			mock.ExpectQuery(regexp.QuoteMeta("FROM " + depTable + " WHERE")).
				WithArgs(idBinds(ids...)...).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))

			err := getDependentRecordsIntoFromTable(context.Background(), tx, depTable, ids,
				map[string]bool{}, map[string][]*types.Dependency{})
			if err != nil {
				t.Fatalf("getDependentRecordsIntoFromTable: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet expectations: %v", err)
			}
		})
	}
}

// TestReverseBlocksInfoBindsSQL drives the production blocks-info read and pins
// its bind lists: the status lookup binds each id once, and each dependency
// table's blocks read binds each id once per typed target branch.
func TestReverseBlocksInfoBindsSQL(t *testing.T) {
	ids := []string{"bd-1", "bd-2"}
	idArgs := []driver.Value{"bd-1", "bd-2"}

	tx, mock := newBindCheckedDB(t)
	for _, issueTable := range []string{"issues", "wisps"} {
		mock.ExpectQuery(regexp.QuoteMeta("FROM " + issueTable + " WHERE id IN")).
			WithArgs(idArgs...).
			WillReturnRows(sqlmock.NewRows([]string{"id", "status"}))
	}
	for _, depTable := range []string{"dependencies", "wisp_dependencies"} {
		mock.ExpectQuery(regexp.QuoteMeta("FROM " + depTable + " WHERE")).
			WithArgs(idBinds(ids...)...).
			WillReturnRows(sqlmock.NewRows([]string{"blocker", "blocked", "type"}))
	}

	blocksMap := map[string][]string{}
	if err := queryBlocksInfo(context.Background(), tx, ids,
		[]string{"dependencies", "wisp_dependencies"}, blocksMap); err != nil {
		t.Fatalf("queryBlocksInfo: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
