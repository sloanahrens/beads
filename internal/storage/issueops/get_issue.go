package issueops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/dberrors"
	"github.com/steveyegge/beads/internal/storage/sqlbuild"
	"github.com/steveyegge/beads/internal/types"
)

// GetIssueInTx retrieves a single issue by ID within an existing transaction,
// including its labels. Automatically routes to the wisps/wisp_labels tables
// if the ID is an active wisp. Returns storage.ErrNotFound (wrapped) if the
// issue does not exist in either table.
func GetIssueInTx(ctx context.Context, tx DBTX, id string) (*types.Issue, error) {
	issue, err := getIssueFromTableInTx(ctx, tx, "issues", "labels", id)
	if err == nil {
		return issue, nil
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return nil, err
	}

	issue, err = getIssueFromTableInTx(ctx, tx, "wisps", "wisp_labels", id)
	if err == nil {
		return issue, nil
	}
	if errors.Is(err, storage.ErrNotFound) {
		return nil, fmt.Errorf("%w: issue %s", storage.ErrNotFound, id)
	}
	return nil, err
}

// missingOptionalIssueTable reports whether err is the absence of the optional
// issue plane the hydration query just read. The hydration FROM clause also
// carries sqlbuild.LeaseJoin, so a blanket table-not-exist check here folds a
// missing leases table into "row absent" — a wrong answer, not an empty one.
func missingOptionalIssueTable(err error, issueTable string) bool {
	return optionalBlockedTable(issueTable) && dberrors.IsMissingTable(err, issueTable)
}

func getIssueFromTableInTx(ctx context.Context, tx DBTX, issueTable, labelTable, id string) (*types.Issue, error) {
	join := sqlbuild.LeaseJoin(issueTable)
	//nolint:gosec // G201: issueTable is a hardcoded literal supplied by GetIssueInTx ("issues" or "wisps")
	querySQL := fmt.Sprintf(`SELECT %s FROM %s %s WHERE id = ?`, IssueSelectColumns, issueTable, join)
	row := tx.QueryRowContext(ctx, querySQL, id)
	issue, err := ScanIssueFrom(row)
	// No degradeLeaseSQL retry here, unlike the listing reads that use it
	// (search.go, search_counts.go, dependencies.go, stale.go). Those read
	// many rows into a list, where one row's lease overlay is a display
	// detail. This query hydrates a single row into the caller's model of
	// that issue: `bd show` renders the lease line from it, and the mutation
	// verbs read their pre-update row through it (update.go, claim.go,
	// unclaim.go, promote.go, release_role.go, execution.go), so a stripped
	// overlay is an answer they act on — "no live lease" for a row on a
	// database whose leases table is gone, indistinguishable from a healthy
	// unleased issue, with nothing telling the caller the table is missing.
	//
	// The missing table is refused instead, classified the way the write
	// paths classify it so a caller never has to pattern-match a raw MySQL
	// 1146 to learn the database needs migration 0055.
	// Pins: TestGetIssueInTxMissingLeasesTableIsAnError and
	// TestUpdateIssueInTxMissingLeasesTableIsAnError here, and (through the
	// store) TestGetIssue/missing_leases_table_is_an_error_not_absent (be-bz4).
	if err == sql.ErrNoRows || missingOptionalIssueTable(err, issueTable) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		if leasesTableMissing(err) {
			return nil, fmt.Errorf("get issue %s: %w", id, ErrLeasesTableMissing)
		}
		return nil, fmt.Errorf("get issue: %w", err)
	}

	// Fetch labels in the same transaction to avoid MaxOpenConns=1 deadlock.
	labels, err := GetLabelsInTx(ctx, tx, labelTable, id)
	if err != nil {
		return nil, fmt.Errorf("get issue labels: %w", err)
	}
	issue.Labels = labels

	return issue, nil
}
