package uow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/steveyegge/beads/internal/workapi"
	publicops "github.com/steveyegge/beads/issueops"
)

// TestReconcileVersionPersistsAcrossUOW is the one version assertion that
// stays out of the conformance contract, because it is about this backend's
// TRANSACTION rather than about the role: a marker written inside a unit of
// work that is closed without a commit must not be there afterwards.
//
// The role cannot express that — every write through it commits — so the
// rolled-back leg drives the metadata seam directly, the same seam the role's
// body writes through. Everything else about version reconciliation is
// TestVersionReconcilerContract, which runs here and on the two store backends.
func TestReconcileVersionPersistsAcrossUOW(t *testing.T) {
	provider := newTestUOWProvider(t)
	ctx := context.Background()

	reconciler, err := NewVersionReconciler(provider)
	require.NoError(t, err)

	res, err := reconciler.ReconcileVersion(ctx, publicops.VersionReconcileRequest{CLIVersion: "0.5.0"})
	require.NoError(t, err)
	require.Equal(t, "", res.Previous)
	require.Equal(t, "0.5.0", res.Current)
	require.True(t, res.Migrated)

	res, err = reconciler.ReconcileVersion(ctx, publicops.VersionReconcileRequest{CLIVersion: "0.6.0"})
	require.NoError(t, err)
	require.Equal(t, "0.5.0", res.Previous, "a committed marker must persist into a new unit of work")
	require.True(t, res.Migrated)

	// Write the marker forward and abandon the unit of work.
	uw, err := provider.NewUOW(ctx)
	require.NoError(t, err)
	require.NoError(t, uw.ConfigUseCase().SetLocalMetadata(ctx, workapi.MetadataKeyVersion, "0.7.0"))
	uw.Close(ctx)

	recorded, err := reconciler.RecordedVersion(ctx, publicops.RecordedVersionRequest{})
	require.NoError(t, err)
	require.Equal(t, "0.6.0", recorded.Recorded, "a rolled-back marker write must not persist")
}
