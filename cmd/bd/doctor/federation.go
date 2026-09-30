package doctor

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/doltserver"
	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/storage/doltutil"
)

// doltDatabaseName returns the configured Dolt database name for the given beads directory.
// Falls back to the default ("beads") if config cannot be read.
func doltDatabaseName(beadsDir string) string {
	dbName := configfile.DefaultDoltDatabase
	if cfg, err := configfile.Load(beadsDir); err == nil && cfg != nil {
		dbName = cfg.GetDoltDatabase()
	}
	return dbName
}

// doltServerConfig returns a read-only dolt.Config populated with server
// connection settings from beads configuration. This ensures federation checks
// use the configured host/port rather than falling back to defaults.
func doltServerConfig(beadsDir, doltPath string) *dolt.Config {
	cfg := &dolt.Config{
		Path:     doltPath,
		ReadOnly: true,
		Database: doltDatabaseName(beadsDir),
	}
	if bcfg, err := configfile.Load(beadsDir); err == nil && bcfg != nil {
		cfg.ServerHost = bcfg.GetDoltServerHost()
		// Carries PortSource with the port: this cfg reaches applyConfigDefaults,
		// which reads a sourceless port as caller-explicit (see
		// dolt.ApplyResolvedServerPort).
		dolt.ApplyResolvedServerPort(beadsDir, cfg)
		cfg.ServerUser = bcfg.GetDoltServerUser()
		cfg.ServerTLS = bcfg.GetDoltServerTLS()
		cfg.ServerPassword = bcfg.GetDoltServerPasswordForPort(cfg.ServerPort)
	}
	dolt.ApplyCLIAutoStart(beadsDir, cfg)
	return cfg
}

// CheckLegacyCLIRemotes warns when legacy filesystem CLI remotes are not
// represented in SQL, because bd now treats SQL remotes as the source of truth.
func CheckLegacyCLIRemotes(path string) DoctorCheck {
	backend, beadsDir := getBackendAndBeadsDir(path)
	if backend != configfile.BackendDolt {
		return DoctorCheck{
			Name:     "Dolt Remote Migration",
			Status:   StatusOK,
			Message:  "N/A (non-Dolt backend)",
			Category: CategoryFederation,
		}
	}

	doltPath := getDatabasePath(beadsDir)
	if _, err := os.Stat(doltPath); os.IsNotExist(err) {
		return DoctorCheck{
			Name:     "Dolt Remote Migration",
			Status:   StatusOK,
			Message:  "N/A (no dolt database)",
			Category: CategoryFederation,
		}
	}

	ctx := context.Background()
	store, err := dolt.New(ctx, doltServerConfig(beadsDir, doltPath))
	if err != nil {
		return DoctorCheck{
			Name:     "Dolt Remote Migration",
			Status:   StatusOK,
			Message:  "Skipped (database unavailable)",
			Detail:   err.Error(),
			Category: CategoryFederation,
		}
	}
	defer func() { _ = store.Close() }()

	sqlRemotes, err := store.ListRemotes(ctx)
	if err != nil {
		return DoctorCheck{
			Name:     "Dolt Remote Migration",
			Status:   StatusOK,
			Message:  "Skipped (SQL remotes unavailable)",
			Detail:   err.Error(),
			Category: CategoryFederation,
		}
	}
	sqlByName := make(map[string]string, len(sqlRemotes))
	for _, remote := range sqlRemotes {
		sqlByName[remote.Name] = remote.URL
	}

	type cliLocation struct {
		label string
		dir   string
	}
	locations := []cliLocation{
		{label: "database CLI directory", dir: store.CLIDir()},
		{label: "Dolt server root", dir: store.Path()},
	}

	var missing []string
	var inspected []string
	var inspectErrors []string
	seenDirs := make(map[string]bool, len(locations))
	for _, loc := range locations {
		if loc.dir == "" {
			continue
		}
		dir := filepath.Clean(loc.dir)
		if seenDirs[dir] {
			continue
		}
		seenDirs[dir] = true

		if _, err := os.Stat(filepath.Join(dir, ".dolt")); err != nil {
			if !os.IsNotExist(err) {
				inspectErrors = append(inspectErrors, fmt.Sprintf("%s (%s): %v", loc.label, dir, err))
			}
			continue
		}

		cliRemotes, err := doltutil.ListCLIRemotes(dir)
		if err != nil {
			inspectErrors = append(inspectErrors, fmt.Sprintf("%s (%s): %v", loc.label, dir, err))
			continue
		}
		inspected = append(inspected, fmt.Sprintf("%s: %s", loc.label, dir))
		for _, remote := range cliRemotes {
			sqlURL := sqlByName[remote.Name]
			if !doltutil.RemoteURLsMatch(sqlURL, remote.URL) {
				missing = append(missing, fmt.Sprintf("%s %s=%s", loc.label, remote.Name, remote.URL))
			}
		}
	}

	if len(inspected) == 0 && len(inspectErrors) > 0 {
		return DoctorCheck{
			Name:     "Dolt Remote Migration",
			Status:   StatusOK,
			Message:  "No legacy CLI remote check available",
			Detail:   strings.Join(inspectErrors, "\n"),
			Category: CategoryFederation,
		}
	}

	if len(missing) == 0 {
		return DoctorCheck{
			Name:     "Dolt Remote Migration",
			Status:   StatusOK,
			Message:  "No legacy CLI-only remotes detected",
			Category: CategoryFederation,
		}
	}

	return DoctorCheck{
		Name:     "Dolt Remote Migration",
		Status:   StatusWarning,
		Message:  fmt.Sprintf("%d legacy CLI remote(s) not visible through SQL", len(missing)),
		Detail:   fmt.Sprintf("Inspected CLI directories:\n%s\nRemotes: %s\nbd dolt remote list, push, and pull use SQL remotes as the source of truth.", strings.Join(inspected, "\n"), strings.Join(missing, ", ")),
		Fix:      "Re-register each remote with 'bd dolt remote add <name> <url>' so it is stored in SQL.",
		Category: CategoryFederation,
	}
}

// CheckDoltServerModeMismatch checks for mismatch between Dolt init and server mode.
// This detects cases where:
// - Server mode is expected but no server is running
// - Embedded mode is being used when server mode should be used (federation with peers)
func CheckDoltServerModeMismatch(path string) DoctorCheck {
	backend, beadsDir := getBackendAndBeadsDir(path)

	// Only relevant for Dolt backend
	if backend != configfile.BackendDolt {
		return DoctorCheck{
			Name:     "Dolt Mode",
			Status:   StatusOK,
			Message:  "N/A (non-Dolt backend)",
			Category: CategoryFederation,
		}
	}

	// Check if dolt directory exists
	doltPath := getDatabasePath(beadsDir)
	if _, err := os.Stat(doltPath); os.IsNotExist(err) {
		return DoctorCheck{
			Name:     "Dolt Mode",
			Status:   StatusOK,
			Message:  "N/A (no dolt database)",
			Category: CategoryFederation,
		}
	}

	// Check if server is reachable by trying to connect
	cfg, _ := configfile.Load(beadsDir)
	serverReachable := false
	if cfg != nil {
		host := cfg.GetDoltServerHost()
		port := doltserver.DefaultConfig(beadsDir).Port
		addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
		if _, err := doltserver.ProbeSQLServer("tcp", addr, 2*time.Second); err == nil {
			serverReachable = true
		}
	}

	// Open storage to check for remotes
	ctx := context.Background()
	store, err := dolt.New(ctx, doltServerConfig(beadsDir, doltPath))
	if err != nil {
		return DoctorCheck{
			Name:     "Dolt Mode",
			Status:   StatusWarning,
			Message:  "Unable to open database",
			Detail:   err.Error(),
			Category: CategoryFederation,
		}
	}
	defer func() { _ = store.Close() }()

	// Check for configured remotes
	remotes, err := store.ListRemotes(ctx)
	if err != nil {
		return DoctorCheck{
			Name:     "Dolt Mode",
			Status:   StatusWarning,
			Message:  "Unable to list remotes",
			Detail:   err.Error(),
			Category: CategoryFederation,
		}
	}

	// Count federation peers (exclude origin)
	peerCount := 0
	for _, r := range remotes {
		if r.Name != "origin" {
			peerCount++
		}
	}

	// Determine expected vs actual mode
	if peerCount > 0 && !serverReachable {
		return DoctorCheck{
			Name:     "Dolt Mode",
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Server not reachable with %d peers configured", peerCount),
			Detail:   "Federation with peers requires a running dolt sql-server",
			Fix:      "Start dolt sql-server manually",
			Category: CategoryFederation,
		}
	}

	if serverReachable {
		return DoctorCheck{
			Name:     "Dolt Mode",
			Status:   StatusOK,
			Message:  "Server mode (connected)",
			Detail:   fmt.Sprintf("%d peers configured", peerCount),
			Category: CategoryFederation,
		}
	}

	return DoctorCheck{
		Name:     "Dolt Mode",
		Status:   StatusOK,
		Message:  "Embedded mode",
		Detail:   "No federation peers configured",
		Category: CategoryFederation,
	}
}
