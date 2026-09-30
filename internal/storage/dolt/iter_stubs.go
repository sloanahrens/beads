// Package dolt — iter_stubs.go
//
// Slice-wrapping stubs for the Iter* methods whose fully streaming
// implementation has not landed yet. The interface ships complete now
// (be-jaavsb / be-yinl4d); each stub will be replaced by a fully
// streaming implementation in a follow-up child of be-yinl4d. The TODO
// comment names the tracking bead so reviewers can find the work item.
package dolt

import (
	"context"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// IterIssueComments streams comments on an issue.
//
// TODO(be-yinl4d-iter): replace slice-then-walk with a fully streaming
// implementation. Tracked under be-7hvi6c (or its successor child).
func (s *DoltStore) IterIssueComments(ctx context.Context, issueID string) (storage.Iter[types.Comment], error) {
	cs, err := s.GetIssueComments(ctx, issueID)
	if err != nil {
		return nil, err
	}
	return storage.NewSliceIter(cs), nil
}

// IterEvents streams the audit-trail events for an issue.
//
// TODO(be-yinl4d-iter): replace with a fully streaming implementation.
func (s *DoltStore) IterEvents(ctx context.Context, issueID string, limit int) (storage.Iter[types.Event], error) {
	ev, err := s.GetEvents(ctx, issueID, limit)
	if err != nil {
		return nil, err
	}
	return storage.NewSliceIter(ev), nil
}
