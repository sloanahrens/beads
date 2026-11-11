package uow

import (
	"context"
	"testing"

	"github.com/steveyegge/beads/backend/conformance"
)

// TestSamplesContract runs the Samples contract against the unit-of-work
// provider, which reaches the same AddInTx/ListInTx body as the store through
// its statement runner. One body across two wrappers is ONE reading plus an
// engine check, and the part this leg genuinely adds is the composition: a
// RunTxRead where RunTx belongs (or the reverse) is visible here and nowhere
// else.
//
// One provider for the whole suite (each newUOWRoleFixtureProvider boots a real
// Dolt sql-server) and NO t.Parallel: this backend has no per-test
// copy-on-write branch, and the cases run in declaration order.
func TestSamplesContract(t *testing.T) {
	ctx := context.Background()
	fixture := newUOWSamplesFixture(t, ctx, "smp")

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

func newUOWSamplesFixture(t *testing.T, ctx context.Context, prefix string) conformance.SamplesFixture {
	t.Helper()
	provider := newUOWRoleFixtureProvider(t, ctx, prefix)
	// Through the capability accessor, not NewSamples: a provider that stopped
	// offering the role is the regression a constructor call would hide.
	source, ok := provider.(SamplesSource)
	if !ok {
		t.Fatalf("provider %T does not offer the Samples accessor", provider)
	}
	samples, err := source.Samples()
	if err != nil {
		t.Fatalf("Samples(): %v", err)
	}
	return conformance.SamplesFixture{KindPrefix: prefix, Samples: samples}
}
