package issueops

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/sqlbuild"
	"github.com/steveyegge/beads/internal/types"
)

// GetIssueDetailsBatchInTx reads the raw material of many detail views inside
// one transaction: the issue rows (labels hydrated), the outgoing edges with
// their hydrated targets, the three cardinalities, and — when asked for — the
// dependent and comment rows.
//
// IT IS THE BATCH FORM OF THE PER-ISSUE READS, not a reimplementation of their
// answers. Every statement below mirrors the statement its single-id sibling
// runs (getIssueFromTableInTx, GetDependenciesWithMetadata, CountDependents,
// CountDependencies, CountIssueComments, IterDependentsWithMetadata,
// GetIssueComments), with the id equality widened to a chunked IN list and the
// per-id aggregates folded into GROUP BY. Output equality with the per-id path
// is what `bd show --json` depends on (be-2wkz), so a statement here that
// drifts from its sibling is a bug, not an optimization.
//
// The chunk size is the package's queryBatchSize, so the statement count is
// fixed per chunk and does not grow with the number of ids.
func GetIssueDetailsBatchInTx(ctx context.Context, tx DBTX, req storage.IssueDetailsBatchRequest) (*storage.IssueDetailsBatch, error) {
	out := &storage.IssueDetailsBatch{
		Issues:           make(map[string]*types.Issue),
		Dependencies:     make(map[string][]*types.IssueWithDependencyMetadata),
		Dependents:       make(map[string][]*types.IssueWithDependencyMetadata),
		Comments:         make(map[string][]*types.Comment),
		DependencyCounts: make(map[string]int64),
		DependentCounts:  make(map[string]int64),
		CommentCounts:    make(map[string]int64),
	}
	ids := compactIDs(req.IDs)
	if len(ids) == 0 {
		return out, nil
	}

	// The wisp set decides, for every id below, which plane's edge and comment
	// tables the single-issue read would have routed to.
	wispSet, err := WispIDSetInTx(ctx, tx, ids)
	if err != nil {
		return nil, fmt.Errorf("issue details batch: %w", err)
	}

	issues, err := GetIssuesByIDsInTx(ctx, tx, ids, wispSet)
	if err != nil {
		return nil, fmt.Errorf("issue details batch: issues: %w", err)
	}
	for _, issue := range issues {
		out.Issues[issue.ID] = issue
	}

	wispIDs, permIDs := partitionByWispSet(ids, wispSet)

	if err := readBatchDependencies(ctx, tx, permIDs, wispIDs, out); err != nil {
		return nil, fmt.Errorf("issue details batch: dependencies: %w", err)
	}
	if err := readBatchCounts(ctx, tx, ids, out); err != nil {
		return nil, fmt.Errorf("issue details batch: counts: %w", err)
	}

	if req.IncludeDependents {
		if err := readBatchDependents(ctx, tx, ids, out); err != nil {
			return nil, fmt.Errorf("issue details batch: dependents: %w", err)
		}
	}
	if req.IncludeComments {
		comments, err := GetCommentsForIssuesInTx(ctx, tx, ids)
		if err != nil {
			return nil, fmt.Errorf("issue details batch: comments: %w", err)
		}
		out.Comments = comments
	}
	return out, nil
}

// compactIDs drops repeats (an IN list answers each id once) while keeping the
// caller's order, so the maps below are keyed by the caller's ids and nothing
// downstream has to know a repeat was present.
func compactIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// readBatchDependencies fills out.Dependencies with each id's OUTGOING edges,
// mirroring GetDependenciesWithMetadata per plane: a durable source reads
// `dependencies`, a wisp source reads `wisp_dependencies`, and the target rows
// are hydrated through the same batched issue read the per-issue path uses for
// them (GetIssuesByIDs). An edge whose target is absent is dropped.
//
// There is deliberately NO ORDER BY, exactly as the per-issue read has none:
// both are index range scans on the source id, so the rows for one issue come
// back in the same index order from either. Adding an order here alone would
// make the batch answer differ from the per-issue one it must match.
func readBatchDependencies(ctx context.Context, tx DBTX, permIDs, wispIDs []string, out *storage.IssueDetailsBatch) error {
	type edge struct {
		sourceID string
		targetID string
		depType  string
	}
	var edges []edge

	for _, pair := range []struct {
		table string
		ids   []string
	}{
		{"dependencies", permIDs},
		{"wisp_dependencies", wispIDs},
	} {
		for start := 0; start < len(pair.ids); start += queryBatchSize {
			end := min(start+queryBatchSize, len(pair.ids))
			batch := pair.ids[start:end]
			placeholders, args := buildSQLInClause(batch)
			//nolint:gosec // G201: pair.table is a hardcoded constant; placeholders are ? only.
			querySQL := fmt.Sprintf(`
				SELECT issue_id, %s AS depends_on_id, type FROM %s
				WHERE issue_id IN (%s)
			`, DepTargetExpr, pair.table, placeholders)
			rows, err := tx.QueryContext(ctx, querySQL, args...)
			if err != nil {
				if optionalBlockedTable(pair.table) && isTableNotExistError(err) {
					continue
				}
				return fmt.Errorf("edges from %s: %w", pair.table, err)
			}
			for rows.Next() {
				var e edge
				if scanErr := rows.Scan(&e.sourceID, &e.targetID, &e.depType); scanErr != nil {
					_ = rows.Close()
					return fmt.Errorf("edges from %s: scan: %w", pair.table, scanErr)
				}
				edges = append(edges, e)
			}
			_ = rows.Close()
			if err := rows.Err(); err != nil {
				return fmt.Errorf("edges from %s: rows: %w", pair.table, err)
			}
		}
	}
	if len(edges) == 0 {
		return nil
	}

	targetIDs := make([]string, 0, len(edges))
	seen := make(map[string]struct{}, len(edges))
	for _, e := range edges {
		if _, ok := seen[e.targetID]; ok {
			continue
		}
		seen[e.targetID] = struct{}{}
		targetIDs = append(targetIDs, e.targetID)
	}
	targets, err := GetIssuesByIDsInTx(ctx, tx, targetIDs, nil)
	if err != nil {
		return fmt.Errorf("edge targets: %w", err)
	}
	byID := make(map[string]*types.Issue, len(targets))
	for _, issue := range targets {
		byID[issue.ID] = issue
	}
	for _, e := range edges {
		target, ok := byID[e.targetID]
		if !ok {
			continue
		}
		out.Dependencies[e.sourceID] = append(out.Dependencies[e.sourceID], &types.IssueWithDependencyMetadata{
			Issue:          *target,
			DependencyType: types.DependencyType(e.depType),
		})
	}
	return nil
}

// readBatchCounts fills the three cardinalities for every id, each with the
// scope its single-id sibling uses.
func readBatchCounts(ctx context.Context, tx DBTX, ids []string, out *storage.IssueDetailsBatch) error {
	// CountDependencies sums `dependencies` and `wisp_dependencies` for every
	// id, whichever plane the id lives in.
	for _, table := range []string{"dependencies", "wisp_dependencies"} {
		if err := addGroupedCounts(ctx, tx, table, "issue_id", ids, out.DependencyCounts); err != nil {
			return err
		}
	}
	// CountDependents is the same sum over the reverse direction.
	for _, table := range []string{"dependencies", "wisp_dependencies"} {
		if err := addDependentCounts(ctx, tx, table, ids, out.DependentCounts); err != nil {
			return err
		}
	}
	// CountIssueComments counts `comments` only — including for a wisp id,
	// which is the single-id read's scope and therefore the one this must
	// reproduce (be-2wkz: the batch answer is byte-identical to the per-id
	// one, quirks included).
	return addGroupedCounts(ctx, tx, "comments", "issue_id", ids, out.CommentCounts)
}

// addGroupedCounts adds one table's per-id counts into into, summing with any
// count already there.
func addGroupedCounts(ctx context.Context, tx DBTX, table, column string, ids []string, into map[string]int64) error {
	for start := 0; start < len(ids); start += queryBatchSize {
		end := min(start+queryBatchSize, len(ids))
		placeholders, args := buildSQLInClause(ids[start:end])
		//nolint:gosec // G201: table and column are hardcoded constants; placeholders are ? only.
		querySQL := fmt.Sprintf(`SELECT %s, count(*) FROM %s WHERE %s IN (%s) GROUP BY %s`,
			column, table, column, placeholders, column)
		rows, err := tx.QueryContext(ctx, querySQL, args...)
		if err != nil {
			if optionalBlockedTable(table) && isTableNotExistError(err) {
				return nil
			}
			return fmt.Errorf("counts from %s: %w", table, err)
		}
		for rows.Next() {
			var id string
			var n int64
			if scanErr := rows.Scan(&id, &n); scanErr != nil {
				_ = rows.Close()
				return fmt.Errorf("counts from %s: scan: %w", table, scanErr)
			}
			into[id] += n
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("counts from %s: rows: %w", table, err)
		}
	}
	return nil
}

// addDependentCounts adds one dependency table's inbound counts into into.
//
// Each target branch is its own equality over the typed target column — the
// decomposition sqlbuild.DepTargetCountSQL renders for the single-id count —
// so the IN list stays indexable on all three columns instead of forcing a
// scan of the COALESCE expression.
func addDependentCounts(ctx context.Context, tx DBTX, table string, ids []string, into map[string]int64) error {
	for start := 0; start < len(ids); start += queryBatchSize {
		end := min(start+queryBatchSize, len(ids))
		placeholders, args := buildSQLInClause(ids[start:end])
		//nolint:gosec // G201: table is a hardcoded constant; placeholders are ? only.
		inner := sqlbuild.DepTargetSelectSQL(table, "%s AS target_id", "%s IN ("+placeholders+")")
		querySQL := "SELECT target_id, count(*) FROM (" + inner + ") dep_targets GROUP BY target_id"
		rows, err := tx.QueryContext(ctx, querySQL, sqlbuild.DepTargetBranchArgs(args...)...)
		if err != nil {
			if optionalBlockedTable(table) && isTableNotExistError(err) {
				return nil
			}
			return fmt.Errorf("dependent counts from %s: %w", table, err)
		}
		for rows.Next() {
			var id string
			var n int64
			if scanErr := rows.Scan(&id, &n); scanErr != nil {
				_ = rows.Close()
				return fmt.Errorf("dependent counts from %s: scan: %w", table, scanErr)
			}
			into[id] += n
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("dependent counts from %s: rows: %w", table, err)
		}
	}
	return nil
}

// readBatchDependents fills out.Dependents with each id's INCOMING edges,
// already reduced to the fields the detail view publishes for a dependent.
//
// The row set, the join target and the order mirror IterDependentsWithMetadata,
// which is what the per-issue path streams: both dependency tables, joined to
// their own source plane, ordered by the dependent's created_at with the id as
// a tiebreaker so the two reads agree on equal timestamps. The lease overlay
// the iterator selects is omitted rather than dropped: none of the five fields
// this projection keeps comes from it.
func readBatchDependents(ctx context.Context, tx DBTX, ids []string, out *storage.IssueDetailsBatch) error {
	arms := []struct {
		source string // the joined source, aliased i or w
		alias  string
	}{
		{"issues i JOIN dependencies d ON d.issue_id = i.id", "i"},
		{"wisps w JOIN wisp_dependencies d ON d.issue_id = w.id", "w"},
	}

	// Both planes share one UNION and one ORDER BY, as dependentsIterSQL does:
	// the per-issue read interleaves the two arms by created_at, so a batch
	// that ordered each arm separately would answer in a different order.
	for start := 0; start < len(ids); start += queryBatchSize {
		end := min(start+queryBatchSize, len(ids))
		placeholders, args := buildSQLInClause(ids[start:end])

		selects := make([]string, 0, len(arms)*len(sqlbuild.DepTargetColumns))
		branchArgs := make([]any, 0, len(arms)*len(sqlbuild.DepTargetColumns)*len(args))
		for _, a := range arms {
			for _, col := range sqlbuild.DepTargetColumns {
				selects = append(selects, fmt.Sprintf(
					`SELECT d.%[1]s AS target_id, %[2]s.id AS id, %[2]s.status AS status, %[2]s.issue_type AS issue_type, %[2]s.priority AS priority, %[2]s.title AS title, d.type AS dep_type, %[2]s.created_at AS created_at
					 FROM %[3]s WHERE d.%[1]s IN (%[4]s)`,
					col, a.alias, a.source, placeholders))
				branchArgs = append(branchArgs, args...)
			}
		}
		//nolint:gosec // G201: table, alias and column fragments are hardcoded constants; placeholders are ? only.
		querySQL := strings.Join(selects, " UNION ALL ") + " ORDER BY created_at ASC, id ASC"
		queryRows, err := tx.QueryContext(ctx, querySQL, branchArgs...)
		if err != nil {
			return fmt.Errorf("dependents: %w", err)
		}
		for queryRows.Next() {
			var targetID, id, status, issueType, title, depType string
			var priority int
			var createdAt sql.NullString // ORDER BY only; the detail view publishes none of it
			if scanErr := queryRows.Scan(&targetID, &id, &status, &issueType, &priority, &title, &depType, &createdAt); scanErr != nil {
				_ = queryRows.Close()
				return fmt.Errorf("dependents: scan: %w", scanErr)
			}
			out.Dependents[targetID] = append(out.Dependents[targetID], &types.IssueWithDependencyMetadata{
				Issue: types.Issue{
					ID:        id,
					Status:    types.Status(status),
					IssueType: types.IssueType(issueType),
					Priority:  priority,
					Title:     title,
				},
				DependencyType: types.DependencyType(depType),
			})
		}
		_ = queryRows.Close()
		if err := queryRows.Err(); err != nil {
			return fmt.Errorf("dependents: rows: %w", err)
		}
	}
	return nil
}
