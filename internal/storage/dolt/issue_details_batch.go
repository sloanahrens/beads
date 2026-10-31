package dolt

import (
	"context"
	"database/sql"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/issueops"
)

var _ storage.IssueDetailsBatchStore = (*DoltStore)(nil)

// GetIssueDetailsBatch reads the detail view's rows for many ids inside one
// read transaction, so the answer describes one instant of the store and the
// whole list costs one query set per chunk rather than one per id (be-2wkz).
//
// The statement list is issueops.GetIssueDetailsBatchInTx's; this method owns
// only the transaction and the retry policy every other read on this store
// gets from withReadTx.
func (s *DoltStore) GetIssueDetailsBatch(ctx context.Context, req storage.IssueDetailsBatchRequest) (*storage.IssueDetailsBatch, error) {
	var out *storage.IssueDetailsBatch
	err := s.withReadTx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = issueops.GetIssueDetailsBatchInTx(ctx, tx, req)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
