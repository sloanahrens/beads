package main

import (
	"context"
	"errors"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// exportSource abstracts the storage reads `bd export` performs, so the one
// export body (runExportFromSource) serves both storage stacks:
//
//   - storeExportSource reads through the classic `store` global
//     (embedded / direct / server modes), preserving the exact pre-seam
//     call pattern.
//   - uowExportSource reads through a proxied-server unit of work, whose
//     domain use cases are plane-pinned (issues vs wisps tables) where the
//     classic bulk loaders partition internally — the impl queries both
//     planes with the full ID set and merges, so both modes read the same
//     rows regardless of which table an issue lives in.
//
// Everything downstream of these reads is shared code; the acceptance bar for
// the seam is byte-identical JSONL output across modes.
type exportSource interface {
	// GetInfraTypes returns the resolved infra-type set (config, YAML, or
	// hardcoded defaults). A nil/empty result makes the caller fall back to
	// domain.DefaultInfraTypes(), mirroring the pre-seam behavior.
	GetInfraTypes(ctx context.Context) map[string]bool
	// SearchIssues returns the full, untruncated result set for the export
	// filter (Limit=0, MaxRows=0). Implementations MUST fail rather than
	// return a truncated page — export is a data-integrity path.
	SearchIssues(ctx context.Context, query string, filter types.IssueFilter) ([]*types.Issue, error)
	// GetConfig reads one config key from the database (used for the
	// export.exclude_owners owner-filter keys).
	GetConfig(ctx context.Context, key string) (string, error)
	// GetAllConfig reads the whole config table (used to extract kv.memory.*
	// rows when memories are exported).
	GetAllConfig(ctx context.Context) (map[string]string, error)
	// LoadExportRelations bulk-loads labels, dependency records, comments,
	// comment counts, and dependency counts for the searched issues.
	LoadExportRelations(ctx context.Context, issues []*types.Issue) (exportRelations, error)
	// WispPlaneIDs reports which of ids currently live in the WISPS table.
	// Export uses it to stamp the explicit "wisp_plane" marker on records
	// whose row flags are ambiguous: a no_history=true row is either an
	// unpromoted no-history wisp (wisps table) or a promoted one (durable
	// issues-table row that may still carry the stray flag), and only table
	// membership can tell them apart — import routes by the marker, so
	// mis-stamping a durable row would re-plane it and drop its relations
	// (bd-r9uce). Implementations must classify by table membership, never
	// by flags, and both modes must agree (byte-identity oracle).
	WispPlaneIDs(ctx context.Context, ids []string) (map[string]bool, error)
}

// storeWispPartitioner is the optional store capability WispPlaneIDs uses in
// classic mode. Both real stores (DoltStore, EmbeddedDoltStore) implement it;
// a store that does not simply gets no plane markers, which degrades to the
// data-safe side (import then routes bare no_history rows to the durable
// plane, where nothing is ever excluded from export).
type storeWispPartitioner interface {
	PartitionWispIDs(ctx context.Context, ids []string) (wispIDs, permIDs []string, err error)
}

// exportRelations carries the bulk-loaded relational data for the export set,
// keyed by issue ID.
type exportRelations struct {
	labels        map[string][]string
	deps          map[string][]*types.Dependency
	comments      map[string][]*types.Comment
	commentCounts map[string]int
	depCounts     map[string]*types.DependencyCounts
}

// errExportNoStore reports a classic-source read attempted with no store
// open. Callers of exportSource.GetConfig treat any error as "key unset",
// which reproduces the pre-seam `if store == nil` early return.
var errExportNoStore = errors.New("no store available")

// storeExportSource is the classic-mode exportSource over the `store` global.
type storeExportSource struct{}

func (storeExportSource) GetInfraTypes(ctx context.Context) map[string]bool {
	if store == nil {
		return nil
	}
	return store.GetInfraTypes(ctx)
}

func (storeExportSource) SearchIssues(ctx context.Context, query string, filter types.IssueFilter) ([]*types.Issue, error) {
	return store.SearchIssues(ctx, query, filter)
}

func (storeExportSource) GetConfig(ctx context.Context, key string) (string, error) {
	if store == nil {
		return "", errExportNoStore
	}
	return store.GetConfig(ctx, key)
}

func (storeExportSource) GetAllConfig(ctx context.Context) (map[string]string, error) {
	return store.GetAllConfig(ctx)
}

func (storeExportSource) LoadExportRelations(ctx context.Context, issues []*types.Issue) (exportRelations, error) {
	issueIDs := make([]string, len(issues))
	for i, issue := range issues {
		issueIDs[i] = issue.ID
	}

	// Individual bulk-load failures deliberately degrade to empty maps rather
	// than aborting the export — unchanged from the pre-seam classic behavior.
	labelsMap, _ := store.GetLabelsForIssues(ctx, issueIDs)
	allDeps, _ := store.GetDependencyRecordsForIssues(ctx, issueIDs)
	commentsMap, _ := store.GetCommentsForIssues(ctx, issueIDs)
	commentCounts, _ := store.GetCommentCounts(ctx, issueIDs)
	depCounts, _ := store.GetDependencyCounts(ctx, issueIDs)

	return exportRelations{
		labels:        labelsMap,
		deps:          allDeps,
		comments:      commentsMap,
		commentCounts: commentCounts,
		depCounts:     depCounts,
	}, nil
}

func (storeExportSource) WispPlaneIDs(ctx context.Context, ids []string) (map[string]bool, error) {
	if len(ids) == 0 || store == nil {
		return nil, nil
	}
	// The store global is decorator-wrapped (telemetry, hook firing); walk
	// Unwrap() down to the store that carries the partition capability.
	s := store
	var p storeWispPartitioner
	for {
		if partitioner, ok := s.(storeWispPartitioner); ok {
			p = partitioner
			break
		}
		u, ok := s.(interface{ Unwrap() storage.DoltStorage })
		if !ok {
			return nil, nil
		}
		s = u.Unwrap()
		if s == nil {
			return nil, nil
		}
	}
	wispIDs, _, err := p.PartitionWispIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(wispIDs))
	for _, id := range wispIDs {
		set[id] = true
	}
	return set, nil
}
