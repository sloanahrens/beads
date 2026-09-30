package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/doltserver"
	"github.com/steveyegge/beads/internal/metrics"
	"github.com/steveyegge/beads/internal/storage/domain"
)

// ContextInfo contains the effective backend identity and repository context.
type ContextInfo struct {
	BeadsDir      string `json:"beads_dir"`
	RepoRoot      string `json:"repo_root"`
	CWDRepoRoot   string `json:"cwd_repo_root,omitempty"`
	IsRedirected  bool   `json:"is_redirected"`
	IsWorktree    bool   `json:"is_worktree"`
	Backend       string `json:"backend"`
	DoltMode      string `json:"dolt_mode"`
	ServerHost    string `json:"server_host,omitempty"`
	ServerPort    int    `json:"server_port,omitempty"`
	Database      string `json:"database"`
	DataDir       string `json:"data_dir,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
	SyncRemote    string `json:"sync_remote,omitempty"`
	SyncGitRemote string `json:"sync_git_remote,omitempty"` // Deprecated: use sync_remote
	Role          string `json:"role,omitempty"`
	BdVersion     string `json:"bd_version"`
}

var contextCmd = &cobra.Command{
	Use:     "context",
	GroupID: "setup",
	Short:   "Show effective backend identity and repository context",
	Long: `Show the effective backend identity information including repository paths,
backend configuration, and sync settings.

This command reads directly from config files and does not require the
database to be open, making it useful for diagnostics in degraded states.

Examples:
  bd context           # Show context information
  bd context --json    # Output in JSON format
`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		evt := metrics.NewCommandEvent("context")
		defer func() {
			if c := metrics.Global(); c != nil {
				c.CloseEventAndAdd(evt)
			}
		}()

		// The direct route reads config files itself rather than through the
		// contextinfo provider — it must answer in degraded states where no
		// database can be opened — but it assembles the SAME snapshot the
		// proxied route gets from the provider, and hands it to the same
		// view. That is what keeps `bd context` one answer across two routes,
		// and what puts both of them on the projection GET /v0/beads/context
		// serves. TestContextRoutesNameOneWorkspaceTheSameWay holds them to it;
		// until it existed both routes carried their own `Backend: dolt` and
		// agreed by telling the same lie.
		snapshot := domain.ContextInfo{BdVersion: Version}

		if selected := selectedNoDBBeadsDir(cmd); selected != "" {
			prepareSelectedNoDBContext(selected)
		}

		rc, err := beads.GetRepoContext()
		if err != nil {
			if jsonOutput {
				if jerr := outputJSON(map[string]string{"error": fmt.Sprintf("cannot resolve repo context: %v", err)}); jerr != nil {
					return jerr
				}
				return SilentExit()
			}
			return HandleError("cannot resolve repo context: %v", err)
		}

		snapshot.BeadsDir = rc.BeadsDir
		snapshot.RepoRoot = rc.RepoRoot
		snapshot.CWDRepoRoot = rc.CWDRepoRoot
		snapshot.IsRedirected = rc.IsRedirected
		snapshot.IsWorktree = rc.IsWorktree

		if role, ok := rc.Role(); ok {
			snapshot.Role = string(role)
		}

		cfg, err := configfile.LoadForDiscovery(rc.BeadsDir)
		if err != nil {
			cfg = configfile.DefaultConfig()
		}
		if cfg == nil {
			cfg = configfile.DefaultConfig()
		}

		applyContextBackend(&snapshot, rc.BeadsDir, cfg)

		snapshot.SyncRemote = resolveSyncRemoteFromDir(rc.BeadsDir)

		info := contextInfoView(snapshot)
		if jsonOutput {
			return outputJSON(info)
		}
		printContextText(info)
		return nil
	},
}

// applyContextBackend records the backend half of the direct route's snapshot,
// off the config files it just read.
//
// It is a named function rather than a run of assignments inside the command so
// that the claim above it — that both `bd context` routes assemble the SAME
// snapshot — is something a test can drive rather than something a reader has
// to check by eye. TestContextRoutesNameOneWorkspaceTheSameWay does exactly
// that, comparing this against the contextinfo provider the proxied route and
// GET /v0/beads/context both go through.
//
// The identity itself goes through domain.SetBackendIdentity, which is the one
// policy both routes share: it is what stops a non-Dolt workspace from being
// described as embedded Dolt on database "beads", which is what both routes did
// while each held its own copy of `Backend: configfile.BackendDolt`.
func applyContextBackend(snapshot *domain.ContextInfo, beadsDir string, cfg *configfile.Config) {
	snapshot.SetBackendIdentity(cfg.GetBackend(), cfg.GetDoltMode(), cfg.GetDoltDatabase())
	snapshot.ProjectID = cfg.ProjectID

	if cfg.IsDoltServerMode() {
		snapshot.ServerHost = cfg.GetDoltServerHost()
		snapshot.ServerPort = doltserver.DefaultConfig(beadsDir).Port
	}
	if dataDir := cfg.GetDoltDataDir(); dataDir != "" {
		snapshot.DataDir = dataDir
	}
}

func printContextText(info ContextInfo) {
	fmt.Printf("bd version:     %s\n", info.BdVersion)
	fmt.Println()

	// Repository
	fmt.Println("Repository:")
	fmt.Printf("  beads dir:    %s\n", info.BeadsDir)
	fmt.Printf("  repo root:    %s\n", info.RepoRoot)
	if info.CWDRepoRoot != "" && info.CWDRepoRoot != info.RepoRoot {
		fmt.Printf("  cwd repo:     %s\n", info.CWDRepoRoot)
	}
	if info.IsRedirected {
		fmt.Printf("  redirected:   yes\n")
	}
	if info.IsWorktree {
		fmt.Printf("  worktree:     yes\n")
	}
	if info.Role != "" {
		fmt.Printf("  role:         %s\n", info.Role)
	}
	fmt.Println()

	// Backend
	fmt.Println("Backend:")
	fmt.Printf("  type:         %s\n", info.Backend)
	// Dolt-only identity. A registered backend reports neither, and a bare
	// "mode:" with nothing after it reads as a failure to determine rather than
	// as not-applicable — so omit them, like every other optional field here.
	if info.DoltMode != "" {
		fmt.Printf("  mode:         %s\n", info.DoltMode)
	}
	if info.Database != "" {
		fmt.Printf("  database:     %s\n", info.Database)
	}
	if info.ServerHost != "" {
		fmt.Printf("  server:       %s:%d\n", info.ServerHost, info.ServerPort)
	}
	if info.DataDir != "" {
		fmt.Printf("  data dir:     %s\n", info.DataDir)
	}
	if info.ProjectID != "" {
		fmt.Printf("  project id:   %s\n", info.ProjectID)
	}

	// Sync
	if info.SyncRemote != "" {
		fmt.Println()
		fmt.Println("Sync:")
		fmt.Printf("  remote:       %s\n", info.SyncRemote)
	}
}

func init() {
	rootCmd.AddCommand(contextCmd)
	readOnlyCommands["context"] = true
}

// contextInfoView projects a workspace snapshot onto what `bd context` prints
// and marshals.
//
// The workspace identity half comes from domain.PublishedContext, the same
// projection GET /v0/beads/context serves, so the two surfaces cannot name the
// same workspace differently.
//
// Everything below that call is LOCAL-ONLY and deliberately so, which is why
// it is written out here rather than added to the shared projection. These are
// operator diagnostics printed to the terminal of whoever owns the workspace:
// the redirect and worktree flags, the absolute host paths, the database bind
// endpoint, the role, and the sync remote — which is exactly the member the
// shared projection has no room for, because a remote URL routinely embeds a
// credential and an HTTP client is not the person who configured it.
func contextInfoView(d domain.ContextInfo) ContextInfo {
	published := domain.PublishedContext(d)
	return ContextInfo{
		BdVersion: published.BdVersion,
		Backend:   published.Backend,
		DoltMode:  published.DoltMode,
		Database:  published.Database,
		BeadsDir:  published.BeadsDir,
		RepoRoot:  published.RepoRoot,
		ProjectID: published.ProjectID,

		CWDRepoRoot:   d.CWDRepoRoot,
		IsRedirected:  d.IsRedirected,
		IsWorktree:    d.IsWorktree,
		ServerHost:    d.ServerHost,
		ServerPort:    d.ServerPort,
		DataDir:       d.DataDir,
		Role:          d.Role,
		SyncRemote:    d.SyncRemote,
		SyncGitRemote: d.SyncRemote,
	}
}
