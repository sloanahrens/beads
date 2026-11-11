package uow

import (
	"context"
	"fmt"

	storesamples "github.com/steveyegge/beads/internal/storage/samplesops"
	"github.com/steveyegge/beads/samplesops"
)

// SamplesSource is the capability accessor a unit-of-work provider offers for
// the generic-samples role, the sibling of MemoriesSource and CounterSource.
type SamplesSource interface {
	Samples() (samplesops.Samples, error)
}

// samples answers sample reads and writes through a unit of work.
type samples struct {
	provider UnitOfWorkProvider
}

// Samples returns the guarded generic-samples surface for this provider.
func (p *doltSQLProvider) Samples() (samplesops.Samples, error) {
	return NewSamples(p)
}

// NewSamples constructs a public samples surface backed by provider.
func NewSamples(provider UnitOfWorkProvider) (samplesops.Samples, error) {
	if isNilUnitOfWorkProvider(provider) {
		return nil, fmt.Errorf("new samples: unit-of-work provider must not be nil")
	}
	return &samples{provider: provider}, nil
}

var _ samplesops.Samples = (*samples)(nil)

// Add validates BEFORE the unit of work is opened, so a refusal cannot be
// mistaken at the call site for a write that rolled back, then runs the shared
// AddInTx on this unit of work's own statement runner.
//
// The row is PREPARED before RunTxResult so every retry attempt inserts the
// same id: a retried commit whose first result was lost lands no second row.
func (s *samples) Add(ctx context.Context, request samplesops.AddRequest) (samplesops.AddResult, error) {
	if err := samplesops.ValidateAddRequest(request); err != nil {
		return samplesops.AddResult{}, err
	}
	row := storesamples.Prepare(request)
	return RunTxResult(ctx, s.provider, func(ctx context.Context, uw UnitOfWork) (samplesops.AddResult, string, error) {
		runner, err := statementRunner(uw)
		if err != nil {
			return samplesops.AddResult{}, "", err
		}
		result, err := storesamples.AddInTx(ctx, runner, row)
		if err != nil {
			return samplesops.AddResult{}, "", err
		}
		return result, "bd: sample add " + request.Kind, nil
	})
}

// List reads the rows inside ONE read-only unit of work, through the same
// shared body the store-backed accessor uses.
func (s *samples) List(ctx context.Context, request samplesops.ListRequest) (samplesops.ListResult, error) {
	if err := samplesops.ValidateListRequest(request); err != nil {
		return samplesops.ListResult{}, err
	}
	return RunTxRead(ctx, s.provider, func(ctx context.Context, uw UnitOfWork) (samplesops.ListResult, error) {
		runner, err := statementRunner(uw)
		if err != nil {
			return samplesops.ListResult{}, err
		}
		return storesamples.ListInTx(ctx, runner, request)
	})
}
