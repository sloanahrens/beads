---
title: "bd init"
description: "Initialize bd in the current directory"
---

{/* AUTO-GENERATED: do not edit manually */}

Generated from `bd help --doc init`.

Initialize bd in the current directory by creating a .beads/ directory
and its storage (a Dolt database by default). Optionally specify a custom issue prefix.

Dolt is the default and only supported storage backend, with full version
control (history, branching, sync).

Use --database to specify an existing server database name, overriding the
default prefix-based naming. This is useful when an external tool (e.g. an orchestrator)
has already created the database.

With --stealth: configures per-repository git settings for invisible beads usage:
  • .git/info/exclude to prevent beads files from being committed
  Perfect for personal use without affecting repo collaborators.

beads runs against a dolt sql-server (server mode is the default; --server
is accepted for compatibility). Set connection details with --server-host,
--server-port, and --server-user.
Password should be set via BEADS_DOLT_PASSWORD environment variable.

Creating a new database on a Dolt server that already serves other databases is
refused unless you pass --allow-new-database or set BD_ALLOW_NEW_DATABASE=1, so
a scratch "bd init" cannot leave a database nobody owns. The first init on a
server with no other databases, --shared-server mode, gateway servers and test-
prefixed database names are not affected. Scripts and orchestrators that run
bd init against a shared server must set the opt-in.

Auto-export is optional. When enabled, bd exports issues to
.beads/issues.jsonl after write commands (throttled to once per 60s). This is
for viewers (bv), interchange, and issue-level migration; not backup.
Cross-machine sync and backups use Dolt remotes/backups, not JSONL import/export.
To enable: bd config set export.auto true

Non-interactive mode (--non-interactive or BD_NON_INTERACTIVE=1):
  Skips all interactive prompts, using sensible defaults:
  • Role defaults to "maintainer" (override with --role)
  • Fork exclude auto-configured when fork detected
  • Auto-export left at default (disabled)
  • --contributor and --team flags are rejected (wizards require interaction)
  Also auto-detected when stdin is not a terminal or CI=true is set.

```
bd init [flags]
```

**Flags:**

```
      --agents-file string       Custom filename for agent instructions (default: AGENTS.md)
      --agents-profile string    AGENTS.md profile: 'minimal' (default, pointer to bd prime) or 'full' (complete command reference)
      --agents-template string   Path to custom AGENTS.md template (overrides embedded default)
      --allow-new-database       Allow creating a new database on a Dolt server that already serves other databases (or set BD_ALLOW_NEW_DATABASE=1)
      --backend string           Storage backend: dolt (default). Removed backends (postgres, mysql, sqlite) print migration guidance.
      --contributor              Run OSS contributor setup wizard
      --debug                    Run the managed Dolt sql-server with --loglevel=debug and CPU profiling (--prof cpu). Persisted to config.yaml as dolt.debug. No effect on externally-managed servers.
      --destroy-token string     Explicit confirmation token for destructive re-init in non-interactive mode (format: 'DESTROY-<prefix>')
      --discard-remote           Authorize discarding the configured remote's Dolt history when re-initializing. Requires --destroy-token in non-interactive mode; see 'bd help init-safety'.
      --external                 Server is externally managed (skip server startup); use with --shared-server or --server
      --force                    Deprecated alias for --reinit-local. Bypasses only the LOCAL data-safety guard; does NOT authorize remote divergence (see 'bd help init-safety').
      --from-jsonl               Import issues from configured import.path; refuses remote history unless --discard-remote authorizes replacement
      --init-if-missing          If the workspace is already initialized, skip init and exit 0 instead of failing (idempotent init for scaffolds)
      --non-interactive          Skip all interactive prompts (auto-detected in CI or non-TTY environments)
  -p, --prefix string            Issue prefix (default: current directory name)
  -q, --quiet                    Suppress output (quiet mode)
      --reinit-local             Re-initialize local .beads/ over existing local data. Does NOT authorize remote divergence; see --discard-remote.
      --remote string            Dolt remote URL to clone from and persist as sync.remote
      --role string              Set beads role without prompting: "maintainer" or "contributor"
      --server                   Use a dolt sql-server (the default; kept for compatibility)
      --server-host string       Dolt server host (default: 127.0.0.1)
      --server-port int          Dolt server port (default: 3307)
      --server-socket string     Unix domain socket path (overrides host/port; pass '' to ignore an ambient BEADS_DOLT_SERVER_SOCKET and use TCP)
      --server-tls               Require TLS for the init-time Dolt server connection (overrides BEADS_DOLT_SERVER_TLS for this run; not persisted - set the env var or credentials file for later commands)
      --server-user string       Dolt server MySQL user (default: root)
      --setup-exclude            Configure .git/info/exclude to keep beads files local (for forks)
      --shared-server            Enable shared Dolt server mode (all projects share one server at ~/.beads/shared-server/)
      --skip-agents              Skip AGENTS.md and Claude/Codex/Cursor setup generation
      --skip-hooks               Skip git hooks installation
      --stealth                  Enable stealth mode: global gitattributes and gitignore, no local repo tracking
      --team                     Run team workflow setup wizard
```
