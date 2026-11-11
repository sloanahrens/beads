package telemetry

import (
	"context"

	"github.com/steveyegge/beads/samplesops"
)

// Samples returns the inner store's generic-samples surface wrapped in this
// layer's instrumentation. It recurses instead of delegating: a blind
// delegation would return the inner surface unspanned and untimed.
func (s *InstrumentedStorage) Samples() (samplesops.Samples, error) {
	inner, err := s.Unwrap().Samples()
	if err != nil {
		return nil, err
	}
	return s.WrapSamples(inner), nil
}

// WrapSamples instruments generic-sample writes and reads with this storage
// layer's existing telemetry meter and tracer.
func (s *InstrumentedStorage) WrapSamples(inner samplesops.Samples) samplesops.Samples {
	return &instrumentedSamples{storage: s, inner: inner}
}

type instrumentedSamples struct {
	storage *InstrumentedStorage
	inner   samplesops.Samples
}

func (s *instrumentedSamples) Add(ctx context.Context, request samplesops.AddRequest) (result samplesops.AddResult, err error) {
	ctx, span, started := s.storage.op(ctx, "Samples.Add")
	result, err = s.inner.Add(ctx, request)
	s.storage.done(ctx, span, started, err)
	return result, err
}

func (s *instrumentedSamples) List(ctx context.Context, request samplesops.ListRequest) (result samplesops.ListResult, err error) {
	ctx, span, started := s.storage.op(ctx, "Samples.List")
	result, err = s.inner.List(ctx, request)
	s.storage.done(ctx, span, started, err)
	return result, err
}
