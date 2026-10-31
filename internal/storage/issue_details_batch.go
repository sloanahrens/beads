package storage

import (
	"context"

	"github.com/steveyegge/beads/internal/types"
)

// IssueDetailsBatchRequest describes one batch detail read: the raw material of
// the issue-detail view, for many ids, read as ONE query set.
//
// The ids are exact and canonical, under the same rules as a single detail
// lookup — no fuzzy, prefix or substring resolution — and the issue-to-wisp
// fallback happens inside the read. Repeats are allowed and are answered once;
// the caller keeps its own order.
type IssueDetailsBatchRequest struct {
	IDs               []string
	IncludeDependents bool
	IncludeComments   bool
}

// IssueDetailsBatch is the raw material of many detail views, read inside one
// transaction so every map below describes the same instant of the store.
//
// It deliberately carries ROWS rather than assembled views: what a detail view
// CONTAINS is workapi.BuildIssueDetails's definition, and a store-shaped
// aggregate that assembled views itself would be a second definition of it.
// Every map is keyed by the row's own id and holds only rows for the ids
// requested; an id present in no map was not found.
type IssueDetailsBatch struct {
	// Issues holds one row per found id, with its labels hydrated into
	// Issue.Labels (ascending, as the single-issue read returns them).
	Issues map[string]*types.Issue
	// Dependencies holds each issue's OUTGOING edges, in a fixed order, with
	// the target issue row hydrated — and, like the single-issue read, an edge
	// whose target row is absent is dropped rather than reported empty.
	Dependencies map[string][]*types.IssueWithDependencyMetadata
	// Dependents holds each issue's INCOMING edges when the request asked for
	// them, already reduced to the identity-and-shape fields the detail view
	// publishes for a dependent. Empty otherwise.
	Dependents map[string][]*types.IssueWithDependencyMetadata
	// Comments holds each issue's comment rows when the request asked for
	// them, oldest first. Empty otherwise.
	Comments map[string][]*types.Comment

	// The three cardinalities, computed as aggregates rather than by counting
	// rows in Go. An id with no rows is absent from the map and reads as 0.
	//
	// They keep the single-issue read's scopes exactly, odd as one of them is:
	// DependencyCount and DependentCount span BOTH dependency tables, while
	// CommentCount counts the durable `comments` table only.
	DependencyCounts map[string]int64
	DependentCounts  map[string]int64
	CommentCounts    map[string]int64
}

// IssueDetailsBatchStore is the store-side batch detail read: the bulk form of
// the reads behind one issue's detail view (GetIssue, GetLabels,
// GetDependenciesWithMetadata and the three counts), answered for a whole id
// list in one query set instead of one query set per id.
//
// It is its own narrow interface rather than three more methods on Storage
// because it is one capability — "the detail view's rows, for a list" — and
// because a decorator that embeds DoltStorage passes it through unchanged.
type IssueDetailsBatchStore interface {
	GetIssueDetailsBatch(ctx context.Context, req IssueDetailsBatchRequest) (*IssueDetailsBatch, error)
}
