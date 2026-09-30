package main

import (
	"fmt"

	"github.com/steveyegge/beads/internal/configfile"
)

func validateConfiguredBackend(cfg *configfile.Config) error {
	if cfg == nil {
		return nil
	}
	if cfg.IsDoltProxiedServerMode() {
		return errProxiedServerModeRemoved()
	}
	switch cfg.Backend {
	case configfile.BackendPostgres, configfile.BackendMySQL, configfile.BackendSQLite:
		return configfile.RemovedBackendError(cfg.Backend)
	case "", configfile.BackendDolt:
		return nil
	default:
		return configfile.UnknownBackendError(cfg.Backend)
	}
}

func requireDoltBackend(cfg *configfile.Config) error {
	if err := validateConfiguredBackend(cfg); err != nil {
		return err
	}
	if cfg != nil && cfg.GetBackend() != configfile.BackendDolt {
		return fmt.Errorf("not using Dolt backend (configured backend %q)", cfg.GetBackend())
	}
	return nil
}

// normalizeLoadedConfig substitutes the default config for an absent
// metadata.json (cfg == nil) so mode inference still runs: a remote host
// supplied via BEADS_DOLT_SERVER_HOST or config.yaml dolt.host (GH#3545)
// must select server mode even when no metadata.json exists — otherwise
// the CLI silently opens the embedded store against a remote-host
// configuration.
func normalizeLoadedConfig(cfg *configfile.Config) *configfile.Config {
	if cfg == nil {
		return configfile.DefaultConfig()
	}
	return cfg
}

func loadDoltBackendConfig(beadsDir string) (*configfile.Config, error) {
	cfg, err := configfile.Load(beadsDir)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if cfg == nil {
		cfg = configfile.DefaultConfig()
	}
	if err := requireDoltBackend(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// errProxiedServerModeRemoved is the fail-closed refusal for a workspace whose
// metadata.json still selects dolt_mode "proxied-server". That mode was removed;
// bd opens nothing rather than guessing at a server-mode equivalent.
func errProxiedServerModeRemoved() error {
	return fmt.Errorf("dolt_mode %q in metadata.json is no longer supported: proxied-server mode was removed; %s; "+
		"convert the workspace to server mode with an older bd (bd migrate from-proxied-server-to-server) and retry",
		configfile.DoltModeProxiedServer, configfile.BackendNotOpenedGuarantee)
}
