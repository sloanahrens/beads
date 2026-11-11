package dolt

import (
	"context"
	"database/sql"

	"github.com/steveyegge/beads/internal/storage"
	storeops "github.com/steveyegge/beads/internal/storage/samplesops"
	"github.com/steveyegge/beads/samplesops"
)

// Samples returns the guarded generic-samples surface for this store.
func (s *DoltStore) Samples() (samplesops.Samples, error) {
	if s == nil {
		return nil, &storage.ErrUnsupported{Op: "Samples", Backend: "nil"}
	}
	return &samples{store: s}, nil
}

// samples answers the samples plane from the server-backed store.
//
// There is no shared constructor package for this role: both methods need a
// transaction, which storage.DoltStorage does not publish, so the sharing
// happens one level down in internal/storage/samplesops — this file and the
// unit-of-work body are each a five-line wrapper around the same AddInTx /
// ListInTx, which is ONE body rather than two votes.
type samples struct{ store *DoltStore }

var _ samplesops.Samples = (*samples)(nil)

// Add validates BEFORE opening the write transaction, so a refusal is raised
// outside any write and cannot be mistaken at the call site for a write that
// rolled back.
func (s *samples) Add(ctx context.Context, request samplesops.AddRequest) (samplesops.AddResult, error) {
	if err := samplesops.ValidateAddRequest(request); err != nil {
		return samplesops.AddResult{}, err
	}
	var result samplesops.AddResult
	if err := s.store.withRetryTx(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = storeops.AddInTx(ctx, tx, request)
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
