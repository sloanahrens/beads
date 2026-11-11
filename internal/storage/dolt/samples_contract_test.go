package dolt

import (
	"context"
	"testing"

	"github.com/steveyegge/beads/backend/conformance"
)

// TestSamplesContract runs the Samples contract against the server-backed
// store, which reaches AddInTx/ListInTx through withRetryTx/withReadTx. The
// unit-of-work leg reaches the same body through its statement runner, so the
// two wirings are ONE reading plus an engine check.
//
// The cases are subtests of one parent so the whole role suite shares one store
// and one copy-on-write branch, and no subtest calls t.Parallel: each case
// namespaces its kind with the fixture prefix, but the ordering cases contrast
// rows they seeded against the same kind. setupTestStore already marks the
// PARENT parallel.
func TestSamplesContract(t *testing.T) {
	fixture, ctx, cleanup := newDoltSamplesFixture(t, "smp")
	defer cleanup()

	t.Run("AddReturnsTheStoredRow", func(t *testing.T) {
		conformance.RunSamplesAddReturnsTheStoredRow(t, ctx, fixture)
	})
	t.Run("ListByKindIsNewestFirstAndScoped", func(t *testing.T) {
		conformance.RunSamplesListByKindIsNewestFirstAndScoped(t, ctx, fixture)
	})
	t.Run("ListSinceIsInclusive", func(t *testing.T) {
		conformance.RunSamplesListSinceIsInclusive(t, ctx, fixture)
	})
	t.Run("ListLimitKeepsTheNewestRows", func(t *testing.T) {
		conformance.RunSamplesListLimitKeepsTheNewestRows(t, ctx, fixture)
	})
	t.Run("ListOfAKindWithNoRowsIsEmpty", func(t *testing.T) {
		conformance.RunSamplesListOfAKindWithNoRowsIsEmpty(t, ctx, fixture)
	})
	t.Run("AddRefusesAnInvalidRequest", func(t *testing.T) {
		conformance.RunSamplesAddRefusesAnInvalidRequest(t, ctx, fixture)
	})
	t.Run("ListRefusesANegativeLimit", func(t *testing.T) {
		conformance.RunSamplesListRefusesANegativeLimit(t, ctx, fixture)
	})
}

func newDoltSamplesFixture(t *testing.T, prefix string) (conformance.SamplesFixture, context.Context, func()) {
	t.Helper()
	store, storeCleanup := setupTestStore(t)
	ctx, cancel := testContext(t)
	// Through the accessor, never the concrete body: the accessor is where each
	// storage decorator adds its layer.
	samples, err := store.Samples()
	if err != nil {
		cancel()
		storeCleanup()
		t.Fatalf("Samples(): %v", err)
	}
	return conformance.SamplesFixture{KindPrefix: prefix, Samples: samples}, ctx, func() {
		cancel()
		storeCleanup()
	}
}
