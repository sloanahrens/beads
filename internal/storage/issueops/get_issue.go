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
	// Deliberately NOT degraded here, unlike the counts/search read paths that
	// call degradeLeaseSQL (be-bz4). Those project the lease overlay into a
	// listing, where a degraded "no live lease" is a tolerable answer. This is
	// the single-row hydration behind `bd show`, which renders the lease line
	// from it (cmd/bd/show_format.go), and the pre-image the mutation paths
	// read (claim.go, update.go): a degraded answer is indistinguishable from
	// a healthy unleased row, so nobody is told the table this row depends on
	// is gone. The missing table reaches the caller and names itself.
	// See TestGetIssueInTxMissingLeasesTableIsAnError.
	if err == sql.ErrNoRows || missingOptionalIssueTable(err, issueTable) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
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
