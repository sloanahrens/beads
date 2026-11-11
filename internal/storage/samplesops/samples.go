// Package samplesops holds the transaction-level body of samplesops.Samples:
// the two SQL statements behind `bd sample add` and `bd sample list`.
//
// It lives below the store interface for the reason internal/storage/memoryops
// does: both legs need a transaction, and storage.DoltStorage publishes methods
// rather than transactions. The body takes domain/db.Runner, which *sql.Tx and
// the unit of work's statementRunner both satisfy, so the two legs run ONE body
// — the dolt accessor through withRetryTx/withReadTx and the unit of work
// through statementRunner — rather than two copies of the SQL that a contract
// would then have to prove agree.
//
// VALIDATION IS NOT HERE. Both accessors call samplesops.ValidateAddRequest /
// ValidateListRequest before opening a transaction, so a refusal is raised
// outside any write and cannot be mistaken at the call site for one that rolled
// back. This body therefore trusts the request it is handed; the functions are
// unexported to the module and only the two accessors call them.
package samplesops

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/steveyegge/beads/internal/storage/domain/db"
	"github.com/steveyegge/beads/samplesops"
)

// AddInTx appends one validated sample inside the caller's transaction and
// returns the row as it was stored.
//
// The id and created_at are minted here rather than left to a column DEFAULT so
// the value Add returns is the value that went into the row: a DEFAULT would
// make the returned row a guess at what the engine chose. id is a random UUID
// because the plane has no natural key — two samples may share a kind, an
// instant and a payload, and both are legitimate observations.
func AddInTx(ctx context.Context, runner db.Runner, request samplesops.AddRequest) (samplesops.AddResult, error) {
	// Both timestamps are truncated to the DATETIME(6) precision the columns
	// store BEFORE the insert, so the row Add returns is the row that was
	// written rather than a nanosecond-precision value the engine then rounded.
	row := samplesops.Sample{
		ID:        uuid.NewString(),
		Kind:      request.Kind,
		At:        request.At.UTC().Truncate(time.Microsecond),
		Payload:   string(request.Payload),
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	const insert = `INSERT INTO samples (id, kind, at, payload, created_at) VALUES (?, ?, ?, ?, ?)`
	if _, err := runner.ExecContext(ctx, insert, row.ID, row.Kind, row.At, row.Payload, row.CreatedAt); err != nil {
		return samplesops.AddResult{}, fmt.Errorf("storing sample %s: %w", row.ID, err)
	}
	return samplesops.AddResult{Sample: row}, nil
}

// ListInTx reads one kind's samples inside the caller's transaction, newest
// first.
//
// The order is total — at, then created_at, then id — so two calls over an
// unchanged table answer the same sequence. Ordering by at alone would leave
// rows that share an instant in whatever order the engine happened to return,
// which reads as a feed that reshuffles itself between polls.
func ListInTx(ctx context.Context, runner db.Runner, request samplesops.ListRequest) (samplesops.ListResult, error) {
	query := `SELECT id, kind, at, payload, created_at FROM samples WHERE kind = ?`
	args := []any{request.Kind}
	if request.Since != nil {
		// INCLUSIVE, as samplesops.ListRequest.Since promises: a consumer that
		// re-reads from the last At it saw must not miss a row stamped at it.
		query += ` AND at >= ?`
		args = append(args, request.Since.UTC().Truncate(time.Microsecond))
	}
	query += ` ORDER BY at DESC, created_at DESC, id DESC`
	if request.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, request.Limit)
	}

	rows, err := runner.QueryContext(ctx, query, args...)
	if err != nil {
		return samplesops.ListResult{}, fmt.Errorf("listing samples of kind %q: %w", request.Kind, err)
	}
	defer rows.Close()

	samples := make([]samplesops.Sample, 0)
	for rows.Next() {
		var sample samplesops.Sample
		if err := rows.Scan(&sample.ID, &sample.Kind, &sample.At, &sample.Payload, &sample.CreatedAt); err != nil {
			return samplesops.ListResult{}, fmt.Errorf("reading samples of kind %q: %w", request.Kind, err)
		}
		sample.At = sample.At.UTC()
		sample.CreatedAt = sample.CreatedAt.UTC()
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return samplesops.ListResult{}, fmt.Errorf("reading samples of kind %q: %w", request.Kind, err)
	}
	return samplesops.ListResult{Samples: samples}, nil
}
