package issueops

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/steveyegge/beads/internal/storage/sqlbuild"
	"github.com/steveyegge/beads/internal/types"
)

// These tests exercise the be-cm3 degrade paths against a fixture DB that
// has no leases table at all — the steady state of every rig DB in this
// town until the coordinated migration 0055 runs (#4259 forbids in-place
// remote migration, so this cannot be fixed by running `bd migrate` here).

// TestGetIssueInTxDegradesOnMissingLeases covers `bd show`: the classic
// single-row hydration query joins leases unconditionally, so on a
// pre-0055 database it must retry with the overlay stripped instead of
// failing the whole lookup.
func TestGetIssueInTxDegradesOnMissingLeases(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	primary := "SELECT " + IssueSelectColumns + " FROM issues " + sqlbuild.LeaseJoin("issues") + " WHERE id = ?"
	mock.ExpectQuery(regexp.QuoteMeta(primary)).
		WithArgs("bd-1").
		WillReturnError(tableNotFound("leases"))

	degraded := degradeLeaseSQL(primary, sqlbuild.LeaseJoin("issues"))
	rows := issueRows()
	rows.AddRow(issueRowValues("bd-1", "Title")...)
	mock.ExpectQuery(regexp.QuoteMeta(degraded)).
		WithArgs("bd-1").
		WillReturnRows(rows)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT label FROM labels WHERE issue_id = ? ORDER BY label")).
		WithArgs("bd-1").
		WillReturnRows(sqlmock.NewRows([]string{"label"}))

	issue, err := GetIssueInTx(context.Background(), tx, "bd-1")
	if err != nil {
		t.Fatalf("GetIssueInTx on a database with no leases table: %v", err)
	}
	if issue == nil || issue.ID != "bd-1" {
		t.Fatalf("GetIssueInTx returned %+v, want issue bd-1", issue)
	}
	if issue.LeaseExpiresAt != nil || issue.HeartbeatAt != nil || issue.LeaseGrantedNode != "" {
		t.Fatalf("degraded issue carries lease data: %+v", issue)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestGetIssueInTxUnrelatedErrorPropagates is the control: the degrade retry
// must be specific to the leases table, not a blanket retry-on-any-error.
func TestGetIssueInTxUnrelatedErrorPropagates(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	boom := errors.New("connection refused")
	mock.ExpectQuery("FROM issues").WillReturnError(boom)
	// getIssueFromTableInTx falls through to the wisps table on ErrNotFound
	// only; a genuine connection error must stop here and never reach wisps.
	if _, err := GetIssueInTx(context.Background(), tx, "bd-1"); !errors.Is(err, boom) {
		t.Fatalf("GetIssueInTx swallowed an unrelated error: %v", err)
	}
}

// TestScanCountsRowsInTxDegradesOnMissingLeases covers `bd list` / `bd
// query`: sqlbuild.SearchCountsSQL always embeds LeaseJoin("i") in the FROM
// clause, so a pre-0055 database must make scanCountsRowsInTx retry instead
// of failing the mega-query outright (the exact bd-cm3 repro: `bd list
// --limit 1` against a v53-schema fixture).
func TestScanCountsRowsInTxDegradesOnMissingLeases(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(`SELECT 1 FROM wisp_dependencies LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectQuery(`(?s)FROM issues i`).WillReturnError(tableNotFound("leases"))
	// The retried query still reads "FROM issues i" (only the lease overlay
	// changed), so the same pattern matches the degraded attempt too.
	mock.ExpectQuery(`(?s)FROM issues i`).WillReturnRows(emptyCountsRows())
	mock.ExpectQuery(`SELECT 1 FROM wisps LIMIT 1`).WillReturnError(sql.ErrNoRows)

	out, err := SearchIssuesWithCountsInTx(context.Background(), tx, "", types.IssueFilter{})
	if err != nil {
		t.Fatalf("SearchIssuesWithCountsInTx on a database with no leases table: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("got %d rows, want 0 (mock returns no rows past the retry)", len(out))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// TestScanCountsRowsInTxUnrelatedErrorPropagates is the control for the
// mega-query retry: a non-leases failure (e.g. wisp_labels, or a genuine
// connection error) must still reach the caller unchanged.
func TestScanCountsRowsInTxUnrelatedErrorPropagates(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(`SELECT 1 FROM wisp_dependencies LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	boom := errors.New("connection refused")
	mock.ExpectQuery(`(?s)FROM issues i`).WillReturnError(boom)

	_, err := SearchIssuesWithCountsInTx(context.Background(), tx, "", types.IssueFilter{})
	if !errors.Is(err, boom) {
		t.Fatalf("SearchIssuesWithCountsInTx did not propagate an unrelated failure: %v", err)
	}
}

// TestGetReadyWorkWithCountsInTxDegradesOnMissingLeases covers `bd ready`,
// whose unbounded (Limit<=0) form drives the same mega-query as `bd list`.
func TestGetReadyWorkWithCountsInTxDegradesOnMissingLeases(t *testing.T) {
	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(`SELECT 1 FROM wisp_dependencies LIMIT 1`).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectQuery(`(?s)FROM issues i`).WillReturnError(tableNotFound("leases"))
	mock.ExpectQuery(`(?s)FROM issues i`).WillReturnRows(emptyCountsRows())
	mock.ExpectQuery(`SELECT 1 FROM wisps LIMIT 1`).WillReturnError(sql.ErrNoRows)

	out, err := GetReadyWorkWithCountsInTx(context.Background(), tx, readyFilter())
	if err != nil {
		t.Fatalf("GetReadyWorkWithCountsInTx on a database with no leases table: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("got %d rows, want 0", len(out))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

// writeLeaseEntryPoint is one leases-table write path: an operation that
// genuinely requires the table (as opposed to the read paths above, which
// tolerate its absence). Each must surface ErrLeasesTableMissing instead of
// the raw MySQL "table not found" it wraps (be-cm3 deliverable 2).
type writeLeaseEntryPoint struct {
	name  string
	prime func(mock sqlmock.Sqlmock)
	run   func(ctx context.Context, tx *sql.Tx) error
}

var writeLeaseEntryPoints = []writeLeaseEntryPoint{
	{
		name: "UpsertLeaseInTx",
		prime: func(mock sqlmock.Sqlmock) {
			mock.ExpectExec(`INSERT INTO leases`).WillReturnError(tableNotFound("leases"))
		},
		run: func(ctx context.Context, tx *sql.Tx) error {
			return UpsertLeaseInTx(ctx, tx, "bd-1", "actor", time.Now(), DefaultLeaseTTL)
		},
	},
	{
		name: "DeleteLeaseInTx",
		prime: func(mock sqlmock.Sqlmock) {
			mock.ExpectExec(`DELETE FROM leases`).WillReturnError(tableNotFound("leases"))
		},
		run: func(ctx context.Context, tx *sql.Tx) error {
			return DeleteLeaseInTx(ctx, tx, "bd-1")
		},
	},
	{
		name: "HeartbeatIssueInTx",
		prime: func(mock sqlmock.Sqlmock) {
			mock.ExpectExec(`UPDATE leases SET`).WillReturnError(tableNotFound("leases"))
		},
		run: func(ctx context.Context, tx *sql.Tx) error {
			return HeartbeatIssueInTx(ctx, tx, "bd-1", "actor")
		},
	},
	{
		name: "ReclaimExpiredLeasesInTx",
		prime: func(mock sqlmock.Sqlmock) {
			mock.ExpectQuery(`(?s)FROM leases l`).WillReturnError(tableNotFound("leases"))
		},
		run: func(ctx context.Context, tx *sql.Tx) error {
			_, err := ReclaimExpiredLeasesInTx(ctx, tx, time.Now(), types.ReclaimFilter{}, "actor")
			return err
		},
	},
}

func TestLeaseWritesFailExplicitlyOnMissingLeasesTable(t *testing.T) {
	for _, tc := range writeLeaseEntryPoints {
		t.Run(tc.name, func(t *testing.T) {
			_, mock, tx := beginMockTx(t)
			tc.prime(mock)

			err := tc.run(context.Background(), tx)
			if err == nil {
				t.Fatalf("%s succeeded against a database with no leases table", tc.name)
			}
			if !errors.Is(err, ErrLeasesTableMissing) {
				t.Fatalf("%s: error is not ErrLeasesTableMissing (raw driver error leaked through): %v", tc.name, err)
			}
		})
	}
}

// TestLeaseWritesPropagateUnrelatedErrors is the control: the wrap must be
// specific to the leases-table-not-found case.
func TestLeaseWritesPropagateUnrelatedErrors(t *testing.T) {
	for _, tc := range writeLeaseEntryPoints {
		t.Run(tc.name, func(t *testing.T) {
			_, mock, tx := beginMockTx(t)
			boom := errors.New("connection refused")
			switch tc.name {
			case "UpsertLeaseInTx":
				mock.ExpectExec(`INSERT INTO leases`).WillReturnError(boom)
			case "DeleteLeaseInTx":
				mock.ExpectExec(`DELETE FROM leases`).WillReturnError(boom)
			case "HeartbeatIssueInTx":
				mock.ExpectExec(`UPDATE leases SET`).WillReturnError(boom)
			case "ReclaimExpiredLeasesInTx":
				mock.ExpectQuery(`(?s)FROM leases l`).WillReturnError(boom)
			}

			err := tc.run(context.Background(), tx)
			if !errors.Is(err, boom) {
				t.Fatalf("%s: unrelated error did not propagate: %v", tc.name, err)
			}
			if errors.Is(err, ErrLeasesTableMissing) {
				t.Fatalf("%s: unrelated error was misclassified as ErrLeasesTableMissing", tc.name)
			}
		})
	}
}
