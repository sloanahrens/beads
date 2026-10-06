//go:build cgo

package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func withGraphApplyTestStore(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	testStore := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), ".beads", "beads.db"), "ga")

	oldStore, oldCtx, oldActor := store, rootCtx, actor
	store, rootCtx, actor = testStore, ctx, "graph-apply-test"
	t.Cleanup(func() {
		store, rootCtx, actor = oldStore, oldCtx, oldActor
	})

	return ctx, testStore.DB()
}
