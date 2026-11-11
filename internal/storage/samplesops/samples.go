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

// Prepared is one validated add with the fields the plane mints already fixed.
//
// It exists so the id and created_at survive a retried transaction: a caller
// prepares the row ONCE, before it opens (or reopens) its write transaction,
// and every attempt inserts the same row instead of minting a fresh id.
type Prepared struct {
	ID        string
	Kind      string
	At        time.Time
	Payload   string
	CreatedAt time.Time
}

// Prepare fixes the row's server-minted fields for one add.
//
// The id and created_at are minted here rather than left to a column DEFAULT so
// the value Add returns is the value that went into the row: a DEFAULT would
// make the returned row a guess at what the engine chose. id is a random UUID
// because the plane has no natural key — two samples may share a kind, an
// instant and a payload, and both are legitimate observations.
//
// Callers must call this ONCE per add and pass the result to every attempt of
// the transaction. AddInTx's insert is idempotent on ID, so a retry after an
// ambiguous commit lands no second row.
func Prepare(request samplesops.AddRequest) Prepared {
	// Both timestamps are truncated to the DATETIME(6) precision the columns
	// store BEFORE the insert, so the row Add returns is the row that was
	// written rather than a nanosecond-precision value the engine then rounded.
	return Prepared{
		ID:        uuid.NewString(),
		Kind:      request.Kind,
		At:        request.At.UTC().Truncate(time.Microsecond),
		Payload:   string(request.Payload),
		CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

// AddInTx appends one prepared sample inside the caller's transaction and
// returns the row as it was stored.
//
// The insert is IDEMPOTENT on the primary key: if a previous attempt committed
// but its result was lost, the retry carrying the same Prepared row inserts
// nothing and cannot double-count an observation. ON DUPLICATE KEY UPDATE is
// the MySQL/Dolt spelling of that no-op.
func AddInTx(ctx context.Context, runner db.Runner, row Prepared) (samplesops.AddResult, error) {
	const insert = `INSERT INTO samples (id, kind, at, payload, created_at) VALUES (?, ?, ?, ?, ?) ON DUPLICATE KEY UPDATE id = id`
	if _, err := runner.ExecContext(ctx, insert, row.ID, row.Kind, row.At, row.Payload, row.CreatedAt); err != nil {
		return samplesops.AddResult{}, fmt.Errorf("storing sample %s: %w", row.ID, err)
	}
	return samplesops.AddResult{Sample: samplesops.Sample{
		ID:        row.ID,
		Kind:      row.Kind,
		At:        row.At,
		Payload:   row.Payload,
		CreatedAt: row.CreatedAt,
	}}, nil
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
