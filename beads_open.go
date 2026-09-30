package beads

import (
	"context"
	"fmt"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/dolt"
)

// OpenBestAvailable opens a beads database using the best available backend
// for the given .beads directory. Only Dolt server mode is supported; a
// workspace recorded as embedded returns an error directing the user to
// server mode.
//
// beadsDir is the path to the .beads directory.
func OpenBestAvailable(ctx context.Context, beadsDir string) (Storage, error) {
	cfg, err := configfile.Load(beadsDir)
	if err != nil {
		return nil, fmt.Errorf("loading storage metadata: %w", err)
	}
	if cfg == nil {
		cfg = configfile.DefaultConfig()
	}
	if !configfile.IsSupportedBackend(cfg.Backend) {
		return nil, configuredBackendUnavailable(cfg.Backend)
	}

	if cfg.IsDoltServerMode() {
		store, err := dolt.NewFromConfig(ctx, beadsDir)
		if err != nil {
			return nil, err
		}
		return store, nil
	}
	return nil, fmt.Errorf("embedded Dolt was removed; use server mode (bd init --server)")
}
