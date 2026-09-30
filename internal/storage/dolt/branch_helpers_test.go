package dolt

import (
	"context"

	"github.com/steveyegge/beads/internal/storage/versioncontrolops"
)

// DeleteBranch deletes a branch. Test fixtures that create branches use it to
// clean up; no production path deletes branches.
func (s *DoltStore) DeleteBranch(ctx context.Context, branch string) error {
	return versioncontrolops.DeleteBranch(ctx, s.db, branch)
}
