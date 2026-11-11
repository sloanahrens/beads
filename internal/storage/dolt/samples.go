package dolt

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/domain/db"
	storeops "github.com/steveyegge/beads/internal/storage/samplesops"
	"github.com/steveyegge/beads/samplesops"
)

// Samples returns the guarded generic-samples surface for this store.
func (s *DoltStore) Samples() (samplesops.Samples, error) {
	if s == nil {
		return nil, &storage.ErrUnsupported{Op: "Samples", Backend: "nil"}
	}
	accessor := &samples{store: s}
	accessor.runWriteTx = func(ctx context.Context, fn func(db.Runner) error) error {
		return s.withRetryTx(ctx, func(tx *sql.Tx) error { return fn(tx) })
	}
	return accessor, nil
}

// samples answers the samples plane from the server-backed store.
//
// There is no shared constructor package for this role: both methods need a
// transaction, which storage.DoltStorage does not publish, so the sharing
// happens one level down in internal/storage/samplesops — this file and the
// unit-of-work body are each a five-line wrapper around the same AddInTx /
// ListInTx, which is ONE body rather than two votes.
type samples struct {
	store *DoltStore
	// runWriteTx is the write-transaction retry boundary. It is a field, not a
	// direct call to store.withRetryTx, so a unit test can drive a retry
	// without a Dolt server and prove the appended row is minted once.
	runWriteTx func(ctx context.Context, fn func(db.Runner) error) error
}

var _ samplesops.Samples = (*samples)(nil)

// Add validates BEFORE opening the write transaction, so a refusal is raised
// outside any write and cannot be mistaken at the call site for a write that
// rolled back.
//
// The row is PREPARED before runWriteTx, so every retry attempt inserts the
// same id. A retried commit whose first result was lost therefore lands no
// second row (see samplesops.AddInTx's idempotent insert).
func (s *samples) Add(ctx context.Context, request samplesops.AddRequest) (samplesops.AddResult, error) {
	if err := samplesops.ValidateAddRequest(request); err != nil {
		return samplesops.AddResult{}, err
	}
	if s.runWriteTx == nil {
		return samplesops.AddResult{}, fmt.Errorf("samples add: write transaction boundary is not wired")
	}
	row := storeops.Prepare(request)
	var result samplesops.AddResult
	if err := s.runWriteTx(ctx, func(runner db.Runner) error {
		var err error
		result, err = storeops.AddInTx(ctx, runner, row)
		return err
	}); err != nil {
		return samplesops.AddResult{}, err
	}
	return result, nil
}

// List validates BEFORE opening the read transaction, for the same reason.
func (s *samples) List(ctx context.Context, request samplesops.ListRequest) (samplesops.ListResult, error) {
	if err := samplesops.ValidateListRequest(request); err != nil {
		return samplesops.ListResult{}, err
	}
	var result samplesops.ListResult
	if err := s.store.withReadTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = storeops.ListInTx(ctx, tx, request)
		return err
	}); err != nil {
		return samplesops.ListResult{}, err
	}
	return result, nil
}
