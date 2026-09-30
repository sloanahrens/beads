package dolt

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/steveyegge/beads/internal/storage"
	storageissueops "github.com/steveyegge/beads/internal/storage/issueops"
)

// PruneOrphanDependencies removes dependency rows whose issue or target no
// longer exists (be-cgr), in one transaction with one version commit when a
// durable row went. A dry run counts inside a read transaction.
func (s *DoltStore) PruneOrphanDependencies(ctx context.Context, dryRun bool) (storage.PruneOrphanDependenciesResult, error) {
	var result storage.PruneOrphanDependenciesResult
	if dryRun {
		// A preview writes nothing, so it takes a read transaction: the store
		// is opened read-only for a --dry-run command.
		run := func(tx *sql.Tx) error {
			var err error
			result, _, err = storageissueops.PruneOrphanDependenciesInTx(ctx, tx, true)
			return err
		}
		if err := s.withReadTx(ctx, run); err != nil {
			return storage.PruneOrphanDependenciesResult{}, err
		}
		return result, nil
	}
	err := s.runIssueOperationTxWithMessage(ctx, func(tx *sql.Tx) (storageissueops.ChangedTables, string, error) {
		attempt, tables, err := storageissueops.PruneOrphanDependenciesInTx(ctx, tx, false)
		if err != nil {
			return nil, "", err
		}
		result = attempt
		if len(tables) == 0 {
			return nil, "", nil
		}
		return tables, fmt.Sprintf("bd: prune %d orphan dependency row(s)", attempt.Total), nil
	})
	if err != nil {
		return storage.PruneOrphanDependenciesResult{}, err
	}
	return result, nil
}

var _ storage.OrphanDependencyPruner = (*DoltStore)(nil)
