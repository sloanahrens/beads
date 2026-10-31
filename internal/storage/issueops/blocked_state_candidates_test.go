package issueops

// The candidate prefilter (be-gxe3): a mark or unmark statement is only run for
// the ids that can possibly change, so its five correlated EXISTS subqueries
// never run for a closed, pinned or already-converged row. These tests pin
// which ids reach which statement; the SQL itself executes in the dolt tier
// (TestRecomputeAllIsBlocked_FullRepairAllLegsDifferential and the
// blocked-consistency lockstep), which stays green unchanged.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// countingDBTX records every statement the recompute sends, with its
// arguments, so a test can assert which ids reached which statement.
type countingDBTX struct {
	DBTX
	statements []loggedStatement
}

type loggedStatement struct {
	kind  string // "query" or "exec"
	query string
	args  []any
}

func (c *countingDBTX) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	c.statements = append(c.statements, loggedStatement{kind: "exec", query: query, args: args})
	return c.DBTX.ExecContext(ctx, query, args...)
}

func (c *countingDBTX) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	c.statements = append(c.statements, loggedStatement{kind: "query", query: query, args: args})
	return c.DBTX.QueryContext(ctx, query, args...)
}

// execs returns the recorded statements of kind "exec" whose SQL contains match.
func (c *countingDBTX) execs(match string) []loggedStatement {
	var out []loggedStatement
	for _, s := range c.statements {
		if s.kind == "exec" && strings.Contains(s.query, match) {
			out = append(out, s)
		}
	}
	return out
}

// describe renders the recorded statements for a failure message.
func (c *countingDBTX) describe() string {
	var b strings.Builder
	for i, s := range c.statements {
		fmt.Fprintf(&b, "\n[%d] %s args=%v query=%s", i, s.kind, s.args, strings.Join(strings.Fields(s.query), " "))
	}
	return b.String()
}

func newCountingBlockedMock(t *testing.T) (*countingDBTX, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sqlmock expectations: %v", err)
		}
		_ = db.Close()
	})
	return &countingDBTX{DBTX: db}, mock
}

// idRows scripts a candidate select result for the given ids.
func idRows(ids ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	return rows
}

// expectCandidateSelect queues the candidate select for table, returning ids.
func expectCandidateSelect(mock sqlmock.Sqlmock, table, alias string, ids ...string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT " + alias + ".id FROM " + table + " " + alias)).
		WillReturnRows(idRows(ids...))
}

// TestRecomputeIsBlockedAllClosedIdsRunsNoCorrelatedStatement: a batch whose
// ids are all closed or pinned (so the candidate select finds none) must cost
// only the cheap candidate selects — no UPDATE, and therefore no correlated
// EXISTS evaluation over those ids.
func TestRecomputeIsBlockedAllClosedIdsRunsNoCorrelatedStatement(t *testing.T) {
	rec, mock := newCountingBlockedMock(t)
	expectCandidateSelect(mock, "issues", "i") // mark candidates: none can change
	expectCandidateSelect(mock, "issues", "i") // unmark candidates: none blocked

	if err := RecomputeIsBlockedForIDsInTx(context.Background(), rec, []string{"closed-1", "closed-2"}); err != nil {
		t.Fatalf("RecomputeIsBlockedForIDsInTx: %v", err)
	}

	for _, s := range rec.statements {
		if s.kind == "exec" {
			t.Fatalf("recompute ran a statement for ids that cannot change:%s", rec.describe())
		}
		if strings.Contains(s.query, "EXISTS") {
			t.Fatalf("recompute evaluated a correlated subquery for ids that cannot change:%s", rec.describe())
		}
	}
	if len(rec.statements) != 2 {
		t.Fatalf("statements = %d, want only the mark and unmark candidate selects:%s",
			len(rec.statements), rec.describe())
	}
	// The narrowing itself is what makes closed/pinned rows uncandidates, so
	// pin the predicates rather than just the statement count: the mark
	// candidate is "unblocked and open", the unmark candidate "blocked".
	mark, unmark := rec.statements[0], rec.statements[1]
	for _, want := range []string{"is_blocked = 0", "status <> 'closed'", "status <> 'pinned'", "id IN (?,?)"} {
		if !strings.Contains(mark.query, want) {
			t.Fatalf("mark candidate select lacks %q:%s", want, rec.describe())
		}
	}
	if !strings.Contains(unmark.query, "is_blocked = 1") {
		t.Fatalf("unmark candidate select lacks the blocked filter:%s", rec.describe())
	}
}

// TestRecomputeIsBlockedMarkReceivesOnlyOpenCandidateIds: a batch of 50 closed
// wisps and 2 open ones must reach the mark UPDATE as those 2 ids only, and the
// UPDATE's own rows-affected is what counts both of them as now blocked.
func TestRecomputeIsBlockedMarkReceivesOnlyOpenCandidateIds(t *testing.T) {
	var ids []string
	for i := 0; i < 50; i++ {
		ids = append(ids, fmt.Sprintf("w-closed-%02d", i))
	}
	ids = append(ids, "w-open-1", "w-open-2")

	rec, mock := newCountingBlockedMock(t)
	// Pass 1: the two open wisps are mark candidates; nothing is blocked yet.
	expectCandidateSelect(mock, "wisps", "w", "w-open-1", "w-open-2")
	mock.ExpectExec(regexp.QuoteMeta("UPDATE wisps w SET w.is_blocked = 1")).
		WithArgs("w-open-1", "w-open-2").
		WillReturnResult(sqlmock.NewResult(0, 2))
	expectCandidateSelect(mock, "wisps", "w")
	// Pass 2: nothing can change, so the fixpoint ends.
	expectCandidateSelect(mock, "wisps", "w")
	expectCandidateSelect(mock, "wisps", "w")

	result, err := RecomputeIsBlockedInTxWithResult(context.Background(), rec, nil, ids)
	if err != nil {
		t.Fatalf("RecomputeIsBlockedInTxWithResult: %v", err)
	}
	if !result.WispRowsChanged {
		t.Fatalf("result = %+v, want the wisp rows reported changed", result)
	}

	markExecs := rec.execs("SET w.is_blocked = 1")
	if len(markExecs) != 1 {
		t.Fatalf("mark statements = %d, want 1:%s", len(markExecs), rec.describe())
	}
	if got := fmt.Sprint(markExecs[0].args); got != "[w-open-1 w-open-2]" {
		t.Fatalf("mark UPDATE args = %s, want only the two open candidate ids:%s", got, rec.describe())
	}
	if n := strings.Count(markExecs[0].query, "?"); n != 2 {
		t.Fatalf("mark UPDATE bound %d ids, want 2:%s", n, rec.describe())
	}
	if unmarkExecs := rec.execs("SET w.is_blocked = 0"); len(unmarkExecs) != 0 {
		t.Fatalf("unmark ran %d time(s), want 0:%s", len(unmarkExecs), rec.describe())
	}
}

// TestRecomputeIsBlockedUnmarkReceivesOnlyBlockedIds: a blocked wisp whose
// blocker closed is unblocked, and the unmark statement sees only the ids that
// are currently blocked — never the converged rest of the batch.
func TestRecomputeIsBlockedUnmarkReceivesOnlyBlockedIds(t *testing.T) {
	ids := []string{"w-blocked", "w-open", "w-closed"}

	rec, mock := newCountingBlockedMock(t)
	// Pass 1: nothing is a mark candidate; the blocked wisp is the only unmark
	// candidate.
	expectCandidateSelect(mock, "wisps", "w")
	expectCandidateSelect(mock, "wisps", "w", "w-blocked")
	mock.ExpectExec(regexp.QuoteMeta("UPDATE wisps w SET w.is_blocked = 0")).
		WithArgs("w-blocked").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Pass 2: converged.
	expectCandidateSelect(mock, "wisps", "w")
	expectCandidateSelect(mock, "wisps", "w")

	result, err := RecomputeIsBlockedInTxWithResult(context.Background(), rec, nil, ids)
	if err != nil {
		t.Fatalf("RecomputeIsBlockedInTxWithResult: %v", err)
	}
	if !result.WispRowsChanged {
		t.Fatalf("result = %+v, want the unblocked wisp reported changed", result)
	}

	unmarkExecs := rec.execs("SET w.is_blocked = 0")
	if len(unmarkExecs) != 1 {
		t.Fatalf("unmark statements = %d, want 1:%s", len(unmarkExecs), rec.describe())
	}
	if got := fmt.Sprint(unmarkExecs[0].args); got != "[w-blocked]" {
		t.Fatalf("unmark UPDATE args = %s, want only the blocked candidate:%s", got, rec.describe())
	}
	if markExecs := rec.execs("SET w.is_blocked = 1"); len(markExecs) != 0 {
		t.Fatalf("mark ran %d time(s), want 0:%s", len(markExecs), rec.describe())
	}
}

// TestBlockedCandidateSelectErrorsFailTheRecompute: a candidate select that
// cannot be read must fail the write, never fall through to the statement (or
// to a silent no-op) as if the batch had no candidates.
func TestBlockedCandidateSelectErrorsFailTheRecompute(t *testing.T) {
	sentinel := errors.New("candidate select failed")

	t.Run("mark candidates", func(t *testing.T) {
		rec, mock := newCountingBlockedMock(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT i.id FROM issues i")).WillReturnError(sentinel)

		err := RecomputeIsBlockedForIDsInTx(context.Background(), rec, []string{"issue-1"})
		if !errors.Is(err, sentinel) {
			t.Fatalf("err = %v, want the candidate select error", err)
		}
		if !strings.Contains(err.Error(), "mark candidates") {
			t.Fatalf("err = %v, want it to name the mark candidate read", err)
		}
	})

	t.Run("unmark candidates", func(t *testing.T) {
		rec, mock := newCountingBlockedMock(t)
		expectCandidateSelect(mock, "issues", "i", "issue-1")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE issues i SET i.is_blocked = 1")).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery(regexp.QuoteMeta("SELECT i.id FROM issues i")).WillReturnError(sentinel)

		err := RecomputeIsBlockedForIDsInTx(context.Background(), rec, []string{"issue-1"})
		if !errors.Is(err, sentinel) {
			t.Fatalf("err = %v, want the candidate select error", err)
		}
		if !strings.Contains(err.Error(), "unmark candidates") {
			t.Fatalf("err = %v, want it to name the unmark candidate read", err)
		}
	})

	t.Run("mark-only pass", func(t *testing.T) {
		rec, mock := newCountingBlockedMock(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT i.id FROM issues i")).WillReturnError(sentinel)

		err := MarkIsBlockedInTx(context.Background(), rec, []string{"issue-1"}, nil)
		if !errors.Is(err, sentinel) {
			t.Fatalf("err = %v, want the candidate select error", err)
		}
		if execs := rec.execs("UPDATE"); len(execs) != 0 {
			t.Fatalf("mark ran after the candidate read failed:%s", rec.describe())
		}
	})
}

// TestMarkIsBlockedPassReceivesOnlyOpenCandidateIds covers the mark-only path
// (MarkIsBlockedInTx, used when a blocking edge is added): it narrows the same
// way and skips the statement entirely when nothing can be marked.
func TestMarkIsBlockedPassReceivesOnlyOpenCandidateIds(t *testing.T) {
	t.Run("narrows to the open ids", func(t *testing.T) {
		rec, mock := newCountingBlockedMock(t)
		expectCandidateSelect(mock, "issues", "i", "i-open")
		mock.ExpectExec(regexp.QuoteMeta("UPDATE issues i SET i.is_blocked = 1")).
			WithArgs("i-open").
			WillReturnResult(sqlmock.NewResult(0, 1))
		// The mark changed a row, so the fixpoint runs a second, converged pass.
		expectCandidateSelect(mock, "issues", "i")

		if err := MarkIsBlockedInTx(context.Background(), rec, []string{"i-closed", "i-open"}, nil); err != nil {
			t.Fatalf("MarkIsBlockedInTx: %v", err)
		}
		execs := rec.execs("UPDATE issues i SET i.is_blocked = 1")
		if len(execs) != 1 || fmt.Sprint(execs[0].args) != "[i-open]" {
			t.Fatalf("mark statements = %v, want one for [i-open]:%s", execs, rec.describe())
		}
	})

	t.Run("skips the statement when no id can be marked", func(t *testing.T) {
		// The pass returns no change, so MarkIsBlockedInTx's fixpoint ends
		// after one narrowing pass.
		rec, mock := newCountingBlockedMock(t)
		expectCandidateSelect(mock, "issues", "i")

		if err := MarkIsBlockedInTx(context.Background(), rec, []string{"i-closed"}, nil); err != nil {
			t.Fatalf("MarkIsBlockedInTx: %v", err)
		}
		if execs := rec.execs("UPDATE"); len(execs) != 0 {
			t.Fatalf("mark ran a statement for an id that cannot change:%s", rec.describe())
		}
	})
}

// The candidate select's own cost is bounded by the primary-key IN list; keep
// it off the dependency graph tables entirely (no JOIN, no EXISTS).
func TestBlockedCandidateSQLSelectsOnlyCheapPredicates(t *testing.T) {
	for _, target := range []blockedStatementTarget{issuesBlockedTarget, wispsBlockedTarget} {
		for name, query := range map[string]string{
			"mark":   target.markCandidatesSQL(),
			"unmark": target.unmarkCandidatesSQL(),
		} {
			for _, banned := range []string{"JOIN", "EXISTS", "dependencies"} {
				if strings.Contains(query, banned) {
					t.Errorf("%s/%s candidate select references %s: %s", target.table, name, banned, query)
				}
			}
			if !strings.Contains(query, target.alias+".id IN (%s)") {
				t.Errorf("%s/%s candidate select does not filter by id: %s", target.table, name, query)
			}
		}
	}
}
