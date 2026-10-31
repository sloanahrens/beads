package dolt

import (
	"context"
	"database/sql"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/issueops"
)

var _ storage.ExactIDStore = (*DoltStore)(nil)

// ExactIssueIDs answers which of ids name a row exactly, in ONE query set, on
// the same read transaction and retry policy every other read here gets.
//
// The statement list is issueops.ExactIDSetInTx's; this method owns only the
// transaction, exactly as GetIssueDetailsBatch does.
func (s *DoltStore) ExactIssueIDs(ctx context.Context, ids []string) (map[string]struct{}, error) {
	var out map[string]struct{}
	err := s.withReadTx(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = issueops.ExactIDSetInTx(ctx, tx, ids)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
