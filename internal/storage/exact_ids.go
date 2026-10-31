package storage

import "context"

// ExactIDStore answers which of a list of ids name a row EXACTLY, in one query
// set, across both planes (issues and wisps).
//
// It is a resolution aid, not a read: the answer decides only whether a caller
// that would otherwise resolve each id through a per-id lookup can skip that
// lookup and take the id as its own canonical id. Nothing about the rows is
// returned, so it cannot answer a detail view or a listing.
//
// It is its own narrow interface rather than a method on Storage because a
// decorated store passes it through unchanged by embedding DoltStorage.
type ExactIDStore interface {
	ExactIssueIDs(ctx context.Context, ids []string) (map[string]struct{}, error)
}
