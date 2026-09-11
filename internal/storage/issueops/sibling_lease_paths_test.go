package issueops

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/steveyegge/beads/internal/storage/sqlbuild"
	"github.com/steveyegge/beads/internal/types"
)

// These tests cover the be-2ex follow-up to be-cm3: GetIssuesByIDsInTx,
// searchTableInTxT (both its direct query and Pattern B's hydration fetch),
// GetStaleIssuesInTx, and GetReadyWorkInTx's wisps leg all still joined
// leases unconditionally after be-cm3 degraded the main read path
// (get_issue.go, search_counts.go). Same fixture DB as
// lease_table_compat_test.go: no leases table at all.

// TestGetIssuesByIDsInTxDegradesOnMissingLeases covers the batched
// WHERE id IN (...) fetch that backs GetStaleIssuesInTx's hydration and
// dependency/tree lookups.
func TestGetIssuesByIDsInTxDegradesOnMissingLeases(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM wisps LIMIT 1")).
		WillReturnError(sql.ErrNoRows)

	join := sqlbuild.LeaseJoin("issues")
	primary := "SELECT " + IssueSelectColumns + " FROM issues " + join + " WHERE id IN (?)"
	mock.ExpectQuery(regexp.QuoteMeta(primary)).
		WithArgs("bd-1").
		WillReturnError(tableNotFound("leases"))

	degraded := degradeLeaseSQL(primary, join)
	rows := issueRows()
	rows.AddRow(issueRowValues("bd-1", "Title")...)
	mock.ExpectQuery(regexp.QuoteMeta(degraded)).
		WithArgs("bd-1").
		WillReturnRows(rows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT issue_id, label FROM labels WHERE issue_id IN (?) ORDER BY issue_id, label")).
		WithArgs("bd-1").
		WillReturnRows(sqlmock.NewRows([]string{"issue_id", "label"}))

	issues, err := GetIssuesByIDsInTx(context.Background(), tx, []string{"bd-1"}, nil)
	if err != nil {
		t.Fatalf("GetIssuesByIDsInTx on a database with no leases table: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "bd-1" {
		t.Fatalf("GetIssuesByIDsInTx returned %+v, want [bd-1]", issues)
	}
	if issues[0].LeaseExpiresAt != nil || issues[0].HeartbeatAt != nil || issues[0].LeaseGrantedNode != "" {
		t.Fatalf("degraded issue carries lease data: %+v", issues[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestGetIssuesByIDsInTxUnrelatedErrorPropagates is the control: the retry
// must be specific to the leases table.
func TestGetIssuesByIDsInTxUnrelatedErrorPropagates(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM wisps LIMIT 1")).
		WillReturnError(sql.ErrNoRows)
	boom := errors.New("connection refused")
	mock.ExpectQuery("FROM issues").WillReturnError(boom)

	if _, err := GetIssuesByIDsInTx(context.Background(), tx, []string{"bd-1"}, nil); !errors.Is(err, boom) {
		t.Fatalf("GetIssuesByIDsInTx did not propagate an unrelated failure: %v", err)
	}
}

// TestSearchTableInTxDegradesOnMissingLeases covers the direct (non-Pattern-B)
// query searchTableInTxT issues when a caller runs SearchIssuesInTx with no
// Limit — the shape `bd search` uses without --limit.
func TestSearchTableInTxDegradesOnMissingLeases(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(`(?s)FROM issues`).WillReturnError(tableNotFound("leases"))
	rows := issueRows()
	rows.AddRow(issueRowValues("bd-1", "Title")...)
	mock.ExpectQuery(`(?s)FROM issues`).WillReturnRows(rows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT issue_id, label FROM labels WHERE issue_id IN (?) ORDER BY issue_id, label")).
		WithArgs("bd-1").
		WillReturnRows(sqlmock.NewRows([]string{"issue_id", "label"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM wisps LIMIT 1")).
		WillReturnError(sql.ErrNoRows)

	out, err := SearchIssuesInTx(context.Background(), tx, "", types.IssueFilter{})
	if err != nil {
		t.Fatalf("SearchIssuesInTx on a database with no leases table: %v", err)
	}
	if len(out) != 1 || out[0].ID != "bd-1" {
		t.Fatalf("SearchIssuesInTx returned %+v, want [bd-1]", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestSearchTablePatternBDegradesOnMissingLeases covers Pattern B: a wide
// projection with a Limit>0 (e.g. `bd list --limit 5`) runs the cheap id-only
// scan first (no lease join, idProjection.joinLeases is false) and then
// batch-fetches the full rows, which does join leases and must degrade the
// same way as the direct path.
func TestSearchTablePatternBDegradesOnMissingLeases(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(`FROM issues`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("bd-1"))
	mock.ExpectQuery(`(?s)FROM issues`).WillReturnError(tableNotFound("leases"))
	rows := issueRows()
	rows.AddRow(issueRowValues("bd-1", "Title")...)
	mock.ExpectQuery(`(?s)FROM issues`).WillReturnRows(rows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT issue_id, label FROM labels WHERE issue_id IN (?) ORDER BY issue_id, label")).
		WithArgs("bd-1").
		WillReturnRows(sqlmock.NewRows([]string{"issue_id", "label"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM wisps LIMIT 1")).
		WillReturnError(sql.ErrNoRows)

	out, err := SearchIssuesInTx(context.Background(), tx, "", types.IssueFilter{Limit: 5})
	if err != nil {
		t.Fatalf("SearchIssuesInTx (pattern B) on a database with no leases table: %v", err)
	}
	if len(out) != 1 || out[0].ID != "bd-1" {
		t.Fatalf("SearchIssuesInTx (pattern B) returned %+v, want [bd-1]", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestSearchTableInTxUnrelatedErrorPropagates is the control for both
// searchTableInTxT paths.
func TestSearchTableInTxUnrelatedErrorPropagates(t *testing.T) {
	_, mock, tx := beginMockTx(t)
	boom := errors.New("connection refused")
	mock.ExpectQuery(`FROM issues`).WillReturnError(boom)

	_, err := SearchIssuesInTx(context.Background(), tx, "", types.IssueFilter{})
	if !errors.Is(err, boom) {
		t.Fatalf("SearchIssuesInTx did not propagate an unrelated failure: %v", err)
	}
}

// TestGetStaleIssuesInTxDegradesOnMissingLeases covers `bd stale`: the
// NOT EXISTS heartbeat guard reads the leases table directly (not via
// sqlbuild.LeaseJoin), so it needs its own retry that drops the guard
// entirely instead of reusing degradeLeaseSQL.
func TestGetStaleIssuesInTxDegradesOnMissingLeases(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(`(?s)SELECT id FROM issues.*NOT EXISTS`).WillReturnError(tableNotFound("leases"))
	mock.ExpectQuery(`(?s)SELECT id FROM issues`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("bd-1"))

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM wisps LIMIT 1")).
		WillReturnError(sql.ErrNoRows)
	rows := issueRows()
	rows.AddRow(issueRowValues("bd-1", "Title")...)
	mock.ExpectQuery(`(?s)FROM issues`).WillReturnRows(rows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT issue_id, label FROM labels WHERE issue_id IN (?) ORDER BY issue_id, label")).
		WithArgs("bd-1").
		WillReturnRows(sqlmock.NewRows([]string{"issue_id", "label"}))

	out, err := GetStaleIssuesInTx(context.Background(), tx, types.StaleFilter{Days: 30})
	if err != nil {
		t.Fatalf("GetStaleIssuesInTx on a database with no leases table: %v", err)
	}
	if len(out) != 1 || out[0].ID != "bd-1" {
		t.Fatalf("GetStaleIssuesInTx returned %+v, want [bd-1]", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestGetStaleIssuesInTxUnrelatedErrorPropagates is the control.
func TestGetStaleIssuesInTxUnrelatedErrorPropagates(t *testing.T) {
	_, mock, tx := beginMockTx(t)
	boom := errors.New("connection refused")
	mock.ExpectQuery(`SELECT id FROM issues`).WillReturnError(boom)

	_, err := GetStaleIssuesInTx(context.Background(), tx, types.StaleFilter{Days: 30})
	if !errors.Is(err, boom) {
		t.Fatalf("GetStaleIssuesInTx did not propagate an unrelated failure: %v", err)
	}
}

// TestGetReadyWorkInTxDegradesOnMissingLeases covers `bd ready`'s wisps leg
// (getReadyWispsInTx's unbounded call into searchTableInTxT): the leases case
// used to be classified alongside wisp_labels as "broken wisp plane" in
// counts_missing_table_test.go, but now degrades instead of erroring, same as
// the mega-query (be-2ex).
func TestGetReadyWorkInTxDegradesOnMissingLeases(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(`SELECT id FROM issues`).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM wisps LIMIT 1")).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectQuery(`(?s)FROM wisps`).WillReturnError(tableNotFound("leases"))
	rows := issueRows()
	rows.AddRow(issueRowValues("bd-w1", "Wisp Title")...)
	mock.ExpectQuery(`(?s)FROM wisps`).WillReturnRows(rows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT issue_id, label FROM wisp_labels WHERE issue_id IN (?) ORDER BY issue_id, label")).
		WithArgs("bd-w1").
		WillReturnRows(sqlmock.NewRows([]string{"issue_id", "label"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM wisps WHERE id IN (?) AND is_blocked = 1")).
		WithArgs("bd-w1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	out, err := GetReadyWorkInTx(context.Background(), tx, readyFilter())
	if err != nil {
		t.Fatalf("GetReadyWorkInTx on a database with no leases table: %v", err)
	}
	if len(out) != 1 || out[0].ID != "bd-w1" {
		t.Fatalf("GetReadyWorkInTx returned %+v, want [bd-w1]", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
