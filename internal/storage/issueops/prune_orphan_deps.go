package issueops

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/steveyegge/beads/internal/storage"
)

// orphanDependencyPredicate selects a dependency row of table whose source
// row or typed target row exists in NEITHER plane. External targets
// (depends_on_external) are never orphans. Each id is checked against both
// issues and wisps, so a row is kept whenever either plane still holds the id:
// pruning is for rows that point at nothing, never for rows that point across
// planes. table is one of two hardcoded names.
func orphanDependencyPredicate(table string) string {
	return fmt.Sprintf(`(
	(NOT EXISTS (SELECT 1 FROM issues i WHERE i.id = %[1]s.issue_id)
	 AND NOT EXISTS (SELECT 1 FROM wisps w WHERE w.id = %[1]s.issue_id))
	OR (%[1]s.depends_on_issue_id IS NOT NULL
	 AND NOT EXISTS (SELECT 1 FROM issues i WHERE i.id = %[1]s.depends_on_issue_id)
	 AND NOT EXISTS (SELECT 1 FROM wisps w WHERE w.id = %[1]s.depends_on_issue_id))
	OR (%[1]s.depends_on_wisp_id IS NOT NULL
	 AND NOT EXISTS (SELECT 1 FROM wisps w WHERE w.id = %[1]s.depends_on_wisp_id)
	 AND NOT EXISTS (SELECT 1 FROM issues i WHERE i.id = %[1]s.depends_on_wisp_id))
)`, table)
}

// PruneOrphanDependenciesInTx removes (or, under dryRun, counts) dependency
// rows in `dependencies` and `wisp_dependencies` whose issue or target no
// longer exists (be-cgr). The sources of pruned edges that still exist get
// their blocked state recomputed in the same transaction, so a row that was
// held blocked by a vanished target is released with the edge.
//
// It returns the durable tables it changed; wisp tables are dolt-ignored and
// never reported.
func PruneOrphanDependenciesInTx(ctx context.Context, tx *sql.Tx, dryRun bool) (storage.PruneOrphanDependenciesResult, ChangedTables, error) {
	result := storage.PruneOrphanDependenciesResult{DryRun: dryRun}
	tables := ChangedTables{}
	var issueSources, wispSources []string

	for _, table := range []string{"dependencies", "wisp_dependencies"} {
		edges, err := orphanDependencyEdges(ctx, tx, table)
		if err != nil {
			return storage.PruneOrphanDependenciesResult{}, nil, err
		}
		sources := make([]string, 0, len(edges))
		for _, edge := range edges {
			sources = append(sources, edge.source)
		}
		n := len(edges)
		if !dryRun && n > 0 {
			// Journal each edge before it goes, like every other bulk edge
			// delete: a consumer tailing events sees the removal.
			if err := recordDependencyRemovalsInTx(ctx, tx, edges); err != nil {
				return storage.PruneOrphanDependenciesResult{}, nil, fmt.Errorf("prune orphan %s: journal: %w", table, err)
			}
			//nolint:gosec // G201: table is one of two hardcoded names
			res, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s", table, orphanDependencyPredicate(table)))
			if err != nil {
				return storage.PruneOrphanDependenciesResult{}, nil, fmt.Errorf("prune orphan %s: %w", table, err)
			}
			affected, err := res.RowsAffected()
			if err != nil {
				return storage.PruneOrphanDependenciesResult{}, nil, fmt.Errorf("prune orphan %s: rows affected: %w", table, err)
			}
			n = int(affected)
			tables.Add(table)
		}
		if table == "dependencies" {
			result.Dependencies = n
			issueSources = append(issueSources, sources...)
		} else {
			result.WispDependencies = n
			wispSources = append(wispSources, sources...)
		}
	}
	result.Total = result.Dependencies + result.WispDependencies

	if !dryRun && result.Total > 0 {
		recomputed, err := RecomputeIsBlockedInTxWithResult(ctx, tx, issueSources, wispSources)
		if err != nil {
			return storage.PruneOrphanDependenciesResult{}, nil, fmt.Errorf("prune orphan dependencies: recompute blocked state: %w", err)
		}
		if recomputed.IssueRowsChanged {
			tables.Add("issues")
		}
	}
	return result, tables, nil
}

// orphanDependencyEdges returns every orphan row in table as a journal edge,
// sorted, so the prune can journal a dep_remove for each before deleting it.
func orphanDependencyEdges(ctx context.Context, tx *sql.Tx, table string) ([]journalDependencyEdge, error) {
	//nolint:gosec // G201: table is one of two hardcoded names
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT %[1]s.issue_id, %[2]s AS target, %[1]s.type, %[1]s.metadata FROM %[1]s WHERE %[3]s",
		table, DepTargetExpr, orphanDependencyPredicate(table)))
	if err != nil {
		return nil, fmt.Errorf("find orphan %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	byKey := make(map[string]journalDependencyEdge)
	for rows.Next() {
		var edge journalDependencyEdge
		if err := rows.Scan(&edge.source, &edge.target, &edge.kind, &edge.metadata); err != nil {
			return nil, fmt.Errorf("find orphan %s: %w", table, err)
		}
		byKey[dependencyEdgeKey(edge)] = edge
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("find orphan %s: %w", table, err)
	}
	return sortedDependencyEdges(byKey), nil
}
