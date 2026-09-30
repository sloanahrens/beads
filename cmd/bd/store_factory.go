package main

import (
	"context"
	"fmt"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/backends"
	"github.com/steveyegge/beads/internal/storage/dolt"
)

// newRegisteredBackendStore opens a store from the pluggable backend registry,
// so the registry arm of the root pre-run's open goes through events-journal
// activation like every other construction path.
func newRegisteredBackendStore(ctx context.Context, name, beadsDir string, readOnly bool) (s storage.DoltStorage, err error) {
	defer func() { s, err = activateEventsJournalStore(beadsDir, s, err) }()
	backend, ok := backends.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("storage backend %q is not registered", name)
	}
	if readOnly {
		return backend.OpenReadOnly(ctx, beadsDir)
	}
	return backend.Open(ctx, beadsDir)
}

// newDoltStore creates a storage backend from an explicit config. It applies
// events-journal activation here rather than in the caller — see the note at
// the top of events_journal.go.
func newDoltStore(ctx context.Context, cfg *dolt.Config) (s storage.DoltStorage, err error) {
	defer func() { s, err = activateEventsJournalStore(cfg.BeadsDir, s, err) }()
	if !cfg.ServerMode {
		return nil, fmt.Errorf("%s", embeddedRemovedErrMsg)
	}
	return dolt.New(ctx, cfg)
}

// newDoltStoreFromConfig creates a SQL-server-backed storage backend from config.
func newDoltStoreFromConfig(ctx context.Context, beadsDir string) (s storage.DoltStorage, err error) {
	defer func() { s, err = activateEventsJournalStore(beadsDir, s, err) }()
	cfg, err := configfile.Load(beadsDir)
	if err != nil {
		// Name the real cause: without this, a present-but-unloadable
		// metadata.json surfaces as the misleading "embedded was removed"
		// message below.
		return nil, fmt.Errorf("load %s: %w", configfile.ConfigPath(beadsDir), err)
	}
	if err := validateConfiguredBackend(cfg); err != nil {
		return nil, err
	}
	cfg = normalizeLoadedConfig(cfg)
	if backend, ok := backends.Lookup(cfg.GetBackend()); ok {
		return backend.Open(ctx, beadsDir)
	}
	if cfg != nil && cfg.IsDoltServerMode() {
		return dolt.NewFromConfig(ctx, beadsDir)
	}
	return nil, fmt.Errorf("%s", embeddedRemovedErrMsg)
}

// newReadOnlyStoreFromConfig creates a read-only SQL-server-backed storage
// backend. It does not activate the events journal: the store refuses writes,
// so there is no mutation for a journal row to accompany (exemption is recorded
// in the construction guard).
func newReadOnlyStoreFromConfig(ctx context.Context, beadsDir string) (storage.DoltStorage, error) {
	cfg, err := configfile.Load(beadsDir)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", configfile.ConfigPath(beadsDir), err)
	}
	if err := validateConfiguredBackend(cfg); err != nil {
		return nil, err
	}
	cfg = normalizeLoadedConfig(cfg)
	if backend, ok := backends.Lookup(cfg.GetBackend()); ok {
		return backend.OpenReadOnly(ctx, beadsDir)
	}
	if cfg != nil && cfg.IsDoltServerMode() {
		return dolt.NewFromConfigWithOptions(ctx, beadsDir, &dolt.Config{ReadOnly: true})
	}
	return nil, fmt.Errorf("%s", embeddedRemovedErrMsg)
}

// newPreviewStoreFromConfig is the store open for a preview command
// (--dry-run, --inspect). With no embedded store, preview and read-only opens
// coincide.
func newPreviewStoreFromConfig(ctx context.Context, beadsDir string) (storage.DoltStorage, error) {
	return newReadOnlyStoreFromConfig(ctx, beadsDir)
}

const embeddedRemovedErrMsg = `embedded Dolt was removed; bd runs only against a dolt sql-server.

Initialize this workspace in server mode:
  bd init --server
Requires a running 'dolt sql-server'. See docs/architecture/dolt.md.`
