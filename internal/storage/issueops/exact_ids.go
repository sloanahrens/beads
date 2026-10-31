package issueops

import (
	"context"
	"fmt"
	"strings"
)

// ExactIDSetInTx returns the subset of ids that name a row EXACTLY in either
// plane (issues or wisps), reading the whole list as one query set.
//
// It exists for `bd show`'s id resolution (be-mfu5). Before it, every id in a
// list went through its own store lookup — a partial-id search plus a row read
// — before the detail read even started, which is about 15-20 ms an id on the
// gt database. This read answers the same question for the whole list: an id
// in the returned set names a row, so the caller's own resolution (fuzzy,
// prefix-route, auto-route) would have answered with that id and nothing else,
// because it consults the local store first and every routing path only runs
// after the local store says not-found.
//
// The exact-check is deliberately NOT a resolution: it never normalizes,
// retries a case, or matches a prefix. An id it leaves out must still go
// through the caller's resolver, so a miss here can only cost a slower lookup,
// never a different answer.
//
// COST. The wisp half is WispIDSetInTx (one statement per queryBatchSize
// chunk of ids, after its own emptiness probe). The issues half builds one
// branch per queryBatchSize chunk and UNION ALLs the branches into a SINGLE
// statement: the cost this read removes is the round trip per id, so the
// statement count must not grow with the id count either. A database with no
// wisps (the ordinary rig) therefore answers this read in three statements —
// the leases probe below, the wisp probe, and the issues statement — for any
// list length, which is what the statement-count test pins.
func ExactIDSetInTx(ctx context.Context, tx DBTX, ids []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(ids))
	if len(ids) == 0 {
		return set, nil
	}

	// The single-id read this stands in for (getIssueFromTableInTx) refuses a
	// database whose leases table is missing, and so must the existence check:
	// an id it accepted there would be handed to the batch detail read, whose
	// answer on such a database is a whole-call error rather than the per-id
	// failure the resolver would have reported. Refusing here sends the id
	// down the resolver, which keeps that failure outcome exactly as it was
	// (be-2wkz's requireLeasesTableInTx is the same rule on the detail side).
	if err := requireLeasesTableInTx(ctx, tx); err != nil {
		return nil, fmt.Errorf("exact id set: %w", err)
	}

	// Repeats add nothing to an IN list and would make one duplicate argument
	// per repeat, so the lookups see the distinct ids.
	ids = compactIDs(ids)

	wispSet, err := WispIDSetInTx(ctx, tx, ids)
	if err != nil {
		return nil, fmt.Errorf("exact id set: wisps: %w", err)
	}
	for id := range wispSet {
		set[id] = struct{}{}
	}

	branches := make([]string, 0, (len(ids)+queryBatchSize-1)/queryBatchSize)
	args := make([]any, 0, len(ids))
	for start := 0; start < len(ids); start += queryBatchSize {
		end := min(start+queryBatchSize, len(ids))
		placeholders, chunkArgs := buildSQLInClause(ids[start:end])
		branches = append(branches, "SELECT id FROM issues WHERE id IN ("+placeholders+")")
		args = append(args, chunkArgs...)
	}
	//nolint:gosec // G201: the table is a hardcoded constant; placeholders are ? only.
	querySQL := strings.Join(branches, " UNION ALL ")
	rows, err := tx.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("exact id set: issues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if scanErr := rows.Scan(&id); scanErr != nil {
			return nil, fmt.Errorf("exact id set: scan: %w", scanErr)
		}
		set[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("exact id set: rows: %w", err)
	}
	return set, nil
}
