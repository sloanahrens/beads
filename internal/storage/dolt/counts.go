package dolt

import (
	"context"
	"database/sql"

	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/storage/sqlbuild"
	"github.com/steveyegge/beads/internal/types"
)

// CountIssues returns the number of issues matching query and filter.
// Filter.Limit and Filter.Offset are ignored; all other fields apply.
// Wisps-merge semantics follow SearchIssues: SkipWisps=true counts the
// durable issues table only, otherwise the wisps tier is merged in (GH#4387).
func (s *DoltStore) CountIssues(ctx context.Context, query string, filter types.IssueFilter) (int64, error) {
	var n int64
	err := s.withReadTx(ctx, func(tx *sql.Tx) error {
		count, err := issueops.CountIssuesInTx(ctx, tx, query, filter)
		if err != nil {
			return err
		}
		n = int64(count)
		return nil
	})
	return n, err
}

// CountIssuesByGroup returns per-group issue counts. groupBy is one of:
// status, priority, type, assignee, label.
func (s *DoltStore) CountIssuesByGroup(ctx context.Context, filter types.IssueFilter, groupBy string) (map[string]int, error) {
	var result map[string]int
	err := s.withReadTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.CountIssuesByGroupInTx(ctx, tx, filter, groupBy)
		return err
	})
	return result, err
}

// CountDependents returns the number of issues that depend on issueID.
// Counts both dependency tables so the total matches GetDependentsWithMetadata:
// a dependent may be a permanent issue (edge in `dependencies`) or a wisp
// (edge in `wisp_dependencies`, routed there by WispTableRouting on the source).
//
// Both tables' targets are resolved via the split physical columns rather than
// the STORED generated depends_on_id, which a count(*) can fail to resolve
// under the pure-Go GMS analyzer, and each count brackets the three typed
// target columns as independent indexed equalities
// (sqlbuild.DepTargetCountSQL) instead of predication on the COALESCE
// expression, which no plan can seek.
func (s *DoltStore) CountDependents(ctx context.Context, issueID string) (int64, error) {
	var n int64
	err := s.withReadTx(ctx, func(tx *sql.Tx) error {
		var perm, wisp int64
		if err := tx.QueryRowContext(ctx, reverseDependentsSQL("dependencies"),
			sqlbuild.DepTargetBranchArgs(issueID)...).Scan(&perm); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, reverseDependentsSQL("wisp_dependencies"),
			sqlbuild.DepTargetBranchArgs(issueID)...).Scan(&wisp); err != nil {
			return err
		}
		n = perm + wisp
		return nil
	})
	return n, err
}

// reverseDependentsSQL counts the rows of table whose target is the bound id,
// as three index seeks — one per typed target column — in a single statement.
// The predicate must name the typed columns, never the STORED generated
// `depends_on_id` (defined as their COALESCE): a count(*) projects no real
// columns, so the pure-Go GMS analyzer can prune the base columns that column
// derives from and then fail with "column depends_on_id could not be found in
// any table in scope". The same expression is why the COALESCE form is not used
// as a predicate anywhere here — it cannot be indexed (see
// sqlbuild.DepTargetSelectSQL).
func reverseDependentsSQL(table string) string {
	return sqlbuild.DepTargetCountSQL(table, "%s = ?")
}

// CountDependencies returns the number of issues that issueID depends on.
// Counts both dependency tables so the total matches GetDependenciesWithMetadata:
// a wisp's outgoing edges live in `wisp_dependencies`, a permanent issue's in
// `dependencies`. Counted as two separate queries summed in Go (see
// CountDependents for why a single combined query is avoided).
func (s *DoltStore) CountDependencies(ctx context.Context, issueID string) (int64, error) {
	var n int64
	err := s.withReadTx(ctx, func(tx *sql.Tx) error {
		var perm, wisp int64
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM dependencies WHERE issue_id = ?`, issueID).Scan(&perm); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM wisp_dependencies WHERE issue_id = ?`, issueID).Scan(&wisp); err != nil {
			return err
		}
		n = perm + wisp
		return nil
	})
	return n, err
}

// CountIssueComments returns the number of comments on an issue.
func (s *DoltStore) CountIssueComments(ctx context.Context, issueID string) (int64, error) {
	var n int64
	err := s.withReadTx(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT count(*) FROM comments WHERE issue_id = ?`, issueID).Scan(&n)
	})
	return n, err
}

// CountEvents returns the number of audit events for an issue, capped at limit
// (or unbounded if limit == 0).
func (s *DoltStore) CountEvents(ctx context.Context, issueID string, limit int) (int64, error) {
	var n int64
	err := s.withReadTx(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`SELECT count(*) FROM events WHERE issue_id = ?`, issueID).Scan(&n)
	})
	if err != nil {
		return 0, err
	}
	if limit > 0 && n > int64(limit) {
		n = int64(limit)
	}
	return n, nil
}

// CountDependentsByStatus returns the number of issues that depend on issueID
// and are in the given status. Counts both dependency tables, joining each to
// its home issue table (dependencies→issues, wisp_dependencies→wisps), so wisp
// dependents are included the same way GetDependentsWithMetadata includes them.
// Counted as two separate queries summed in Go (see CountDependents for why a
// single combined query is avoided).
func (s *DoltStore) CountDependentsByStatus(ctx context.Context, issueID string, status types.Status) (int64, error) {
	args := sqlbuild.DepTargetBranchArgs(issueID, string(status))
	var n int64
	err := s.withReadTx(ctx, func(tx *sql.Tx) error {
		var perm, wisp int64
		if err := tx.QueryRowContext(ctx,
			dependentsByStatusSQL("dependencies d JOIN issues i ON i.id = d.issue_id", "i.status = ?"),
			args...).Scan(&perm); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx,
			dependentsByStatusSQL("wisp_dependencies d JOIN wisps w ON w.id = d.issue_id", "w.status = ?"),
			args...).Scan(&wisp); err != nil {
			return err
		}
		n = perm + wisp
		return nil
	})
	return n, err
}

// dependentsByStatusSQL counts the rows of one dependency table whose target is
// the bound id and whose source row (joined in by from) satisfies statusPred.
// Same decomposition as CountDependents: three indexed target equalities per
// table, each joined to its source row's status. Measured on Dolt, the branches
// drive from the target index into the issues/wisps primary key, where the
// COALESCE form scans the whole dependency table.
func dependentsByStatusSQL(from, statusPred string) string {
	return sqlbuild.DepTargetCountSQL(from, "d.%s = ? AND "+statusPred)
}
