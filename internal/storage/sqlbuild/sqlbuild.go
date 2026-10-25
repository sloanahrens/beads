// Package sqlbuild holds the pure SQL-text builders shared by the classic
// issueops stack (production, *sql.Tx) and the domain/db repository stack
// (proxied-server, Runner). Functions here map (filter, tables) -> (sql, args)
// and are execution-context-free: no database handles, no transactions.
//
// Both stacks must produce identical row sets for identical filters (pinned
// by the Seam A parity suite in internal/storage/domain/db). Keeping the
// query text in one place is what prevents the two implementations from
// drifting; see bd-6dnrw.46 for the unify-vs-duplicate decision. Execution
// orchestration (issues+wisps merge strategy, hydration, probes) is
// intentionally NOT unified and stays in each stack.
package sqlbuild

import (
	"fmt"
	"strings"
)

// FilterTables configures table names for the filter builders, allowing the
// same filter logic to target both the issues and wisps table families.
type FilterTables struct {
	Main         string // "issues" or "wisps"
	Labels       string // "labels" or "wisp_labels"
	Dependencies string // "dependencies" or "wisp_dependencies"
	Comments     string // "comments" or "wisp_comments"
}

var (
	IssuesFilterTables = FilterTables{Main: "issues", Labels: "labels", Dependencies: "dependencies", Comments: "comments"}
	WispsFilterTables  = FilterTables{Main: "wisps", Labels: "wisp_labels", Dependencies: "wisp_dependencies", Comments: "wisp_comments"}
)

// OptionalWispTable reports whether name is a wisp-plane table a database may
// legitimately not have, which is the set a wisp query may treat as "no wisps"
// rather than a failure. Both stacks read it so the two cannot disagree about
// what a missing table means.
func OptionalWispTable(name string) bool {
	return strings.EqualFold(name, "wisps") || strings.EqualFold(name, "wisp_dependencies")
}

// DepTargetExpr resolves a dependency row's target across the three
// mutually-exclusive target columns. Use it in SELECT projections (aliased as
// depends_on_id); for a WHERE clause prefer DepTargetSelectSQL /
// DepTargetCountSQL, which the index on the target column can serve.
const DepTargetExpr = "COALESCE(depends_on_issue_id, depends_on_wisp_id, depends_on_external)"

// DepTargetColumns are the three mutually-exclusive dependency target columns,
// in COALESCE precedence order: issue, then wisp, then external.
var DepTargetColumns = []string{
	"depends_on_issue_id",
	"depends_on_wisp_id",
	"depends_on_external",
}

// DepTargetSelectSQL renders one SELECT per typed target column, UNION ALL'd in
// DepTargetColumns order: `SELECT <project> FROM <from> WHERE <pred>`, with
// every %s in project and pred replaced by that branch's column name.
//
// This is the indexable form of DepTargetExpr for WHERE clauses. COALESCE(...)
// wraps all three columns in an expression no index can match, so Dolt plans
// every predicate over it as a full Table scan plus a Filter. One bare equality
// per column lets each branch seek that column's own index instead. Measured
// on Dolt 2.3.2 (`bd sql 'EXPLAIN FORMAT=TREE ...'` against a live rig
// database):
//
//	SELECT count(*) FROM wisp_dependencies WHERE COALESCE(...) = 'X'
//	  -> Filter -> Table(name: wisp_dependencies)          (full scan)
//
//	SELECT count(*) FROM (SELECT 1 FROM wisp_dependencies
//	  WHERE depends_on_issue_id = 'X' UNION ALL ... )
//	  -> three IndexedTableAccess, one per target column   (index seeks)
//
// The obvious alternative — `(a = ? OR b = ? OR c = ?)` — is NOT usable: Dolt's
// optimizer does not index-merge an OR of distinct columns and plans it as the
// same full Table scan (measured; see issueops.depTargetEqualsOr, which keeps
// the OR only for the type-filtered paths where the (type, target) composite
// gives it a bounded range anyway). The decomposition therefore has to be a
// UNION ALL, not a single disjunction.
//
// The result set is exactly the one the COALESCE predicate selects: both
// dependency tables carry a ck_*_one_target CHECK requiring exactly one typed
// target column to be non-NULL, so a row matches at most one branch (no
// duplicates) and COALESCE never has a second non-NULL column to prefer over
// the first. Callers bind the same id(s) once per branch, in DepTargetColumns
// order — DepTargetBranchArgs builds that argument list.
func DepTargetSelectSQL(from, project, pred string) string {
	selects := make([]string, 0, len(DepTargetColumns))
	for _, col := range DepTargetColumns {
		selects = append(selects, fmt.Sprintf("SELECT %s FROM %s WHERE %s",
			strings.ReplaceAll(project, "%s", col),
			from,
			strings.ReplaceAll(pred, "%s", col)))
	}
	return strings.Join(selects, "\n\t\t\t\tUNION ALL\n\t\t\t\t")
}

// DepTargetCountSQL is the counting form of DepTargetSelectSQL: a reverse-
// dependency count that is ONE statement of three index seeks, rather than
// `count(*) WHERE DepTargetExpr = ?`, which no plan can seek. from may carry
// its own joins and aliases; pred is a single-branch predicate with one %s for
// the target column (prefix aliases, e.g. `d.%s = ?`, when from is aliased).
func DepTargetCountSQL(from, pred string) string {
	return fmt.Sprintf("SELECT count(*) FROM (%s) dep_targets",
		DepTargetSelectSQL(from, "1", pred))
}

// DepTargetBranchArgs repeats perBranch once per entry in DepTargetColumns:
// the argument list, in bind order, for a DepTargetSelectSQL predicate whose
// per-branch parameters are perBranch.
func DepTargetBranchArgs(perBranch ...any) []any {
	out := make([]any, 0, len(DepTargetColumns)*len(perBranch))
	for range DepTargetColumns {
		out = append(out, perBranch...)
	}
	return out
}

// IssueBaseColumns is the column list for the issues/wisps row itself,
// without the lease overlay. Use IssueSelectColumns for full hydration;
// this split exists so callers that alias the main table (QualifyColumns)
// can qualify the row columns without mangling the leases.* references.
const IssueBaseColumns = `id, content_hash, title, description, design, acceptance_criteria, notes,
	       status, priority, issue_type, assignee, estimated_minutes,
	       created_at, created_by, owner, updated_at, started_at, closed_at, external_ref, spec_id,
	       compaction_level, compacted_at, compacted_at_commit, original_size, source_repo, close_reason, closed_by_session,
	       sender, ephemeral, no_history, wisp_type, pinned, is_template,
	       await_type, await_id, timeout_ns, waiters,
	       mol_type,
	       event_kind, actor, target, payload,
	       due_at, defer_until,
	       work_type, source_system, metadata, row_lock, storage_class`

// IssueBaseColumnsLite is IssueBaseColumns minus the heavy TEXT columns
// (issueops.HeavyDropList). It lives here rather than beside its scan function
// for the same reason IssueBaseColumns does: the counts mega-query aliases the
// main table and has to qualify the row columns without mangling the leases.*
// references, and that query is rendered in this package. issueops re-exports
// the full list as IssueSelectColumnsLite and pins it against IssueBaseColumns
// with the schema-parity test.
const IssueBaseColumnsLite = `id, content_hash, title,
	       status, priority, issue_type, assignee, estimated_minutes,
	       created_at, created_by, owner, updated_at, started_at, closed_at, external_ref, spec_id,
	       compaction_level, compacted_at, compacted_at_commit, original_size, source_repo, close_reason, closed_by_session,
	       sender, ephemeral, no_history, wisp_type, pinned, is_template,
	       await_type, await_id, timeout_ns,
	       mol_type,
	       event_kind, actor, target,
	       due_at, defer_until,
	       work_type, source_system, metadata, row_lock, storage_class`

// LeaseSelectColumns is the lease overlay for full issue hydration. Leases
// live in the ephemeral leases table (bd-lrgn1), not on the issues row, so
// every query selecting these must also add LeaseJoin to its FROM clause —
// a query that forgets the join fails loudly on the leases.* reference.
const LeaseSelectColumns = `leases.lease_expires_at, leases.heartbeat_at, leases.granted_node`

// IssueSelectColumns is the canonical column list for full issue hydration.
// Every query that reads a complete types.Issue should use this constant
// and include LeaseJoin(table) in its FROM clause; the scan side is
// issueops.ScanIssueFrom, which scans positionally and must stay in
// column-for-column agreement with this list.
const IssueSelectColumns = IssueBaseColumns + `,
	       ` + LeaseSelectColumns

// IssueSelectColumnsLite is the lite counterpart of IssueSelectColumns. The
// scan side is issueops.ScanIssueLiteFrom and the same LeaseJoin requirement
// applies.
const IssueSelectColumnsLite = IssueBaseColumnsLite + `,
	       ` + LeaseSelectColumns

// LeaseJoin returns the FROM-clause fragment that overlays the ephemeral
// leases table onto the given issues/wisps table reference (a table name or
// alias). LEFT JOIN: rows without a live claim have no lease row and hydrate
// nil lease fields.
func LeaseJoin(tableRef string) string {
	return "LEFT JOIN leases ON leases.issue_id = " + tableRef + ".id"
}

// QueryBatchSize bounds IN-clause sizes when long ID lists are folded into
// WHERE fragments.
const QueryBatchSize = 200

// QualifyColumns prefixes every column in a comma-separated list (e.g.
// IssueSelectColumns) with the given table qualifier such as "i.".
func QualifyColumns(columns, prefix string) string {
	raw := strings.ReplaceAll(columns, "\n", " ")
	raw = strings.ReplaceAll(raw, "\t", " ")
	parts := strings.Split(raw, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

// InPlaceholders renders "?,?,..." and the matching arg slice for an IN clause.
func InPlaceholders[T ~string](values []T) (string, []any) {
	ph := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		ph[i] = "?"
		args[i] = string(v)
	}
	return strings.Join(ph, ","), args
}

// CompactNonEmptyStrings drops empty entries, returning nil when none remain.
func CompactNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
