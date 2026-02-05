# Beads Codebase Tutorial

A guide to understanding the beads codebase for contributors working on the fork.

## Scale

| Component | Size |
|-----------|------|
| `cmd/bd/` | **326 Go files** — the CLI layer |
| `internal/` | **~200 files** across 30+ packages |
| `internal/storage/sqlite/migrations/` | **43 migration files** |
| Total | ~550 Go source files, not counting tests |

This is not a small CLI tool. It's closer to a full database application with a daemon, RPC protocol, federation, and a formula DSL.

## Architecture in Three Layers

```
┌─────────────────────────────────────────────┐
│  CLI Layer (cmd/bd/)                        │
│  326 files: Cobra commands, flags, output   │
│  Entry: main.go (1,075 lines)              │
└──────────────┬──────────────────────────────┘
               │ RPC (JSON over Unix socket)
               │ or direct function calls
┌──────────────▼──────────────────────────────┐
│  Daemon Layer (internal/rpc/, daemon/)      │
│  Background process per .beads/ directory   │
│  40+ RPC operations, auto-sync, file watch  │
└──────────────┬──────────────────────────────┘
               │
┌──────────────▼──────────────────────────────┐
│  Storage Layer (internal/storage/)          │
│  SQLite (primary) | Dolt (versioned) | Mem  │
│  + JSONL export for git portability         │
└─────────────────────────────────────────────┘
```

## How a Command Flows

Taking `bd create "My task"` as the example:

1. **`main.go` PersistentPreRun** — Resolves config, tries to connect to daemon (auto-starts if needed), falls back to direct SQLite
2. **`create.go`** — Parses flags, builds an `Issue` struct
3. **RPC or direct** — Either sends `OpCreate` JSON to daemon socket, or calls `store.CreateIssue()` directly
4. **SQLite** — `INSERT INTO issues` inside a transaction, sets dirty flag
5. **Auto-flush** — Background goroutine exports dirty issues to `.beads/issues.jsonl`
6. **`main.go` PersistentPostRun** — Closes connections, optionally git-commits

## The Daemon

Each `.beads/` directory gets its own daemon process. This is where most of the complexity lives.

- **Auto-starts** on first `bd` command (configurable via `BEADS_AUTO_START_DAEMON`)
- **Unix socket** at `~/.beads-sockets/`
- **Event loop** polls every 5s: check dirty flag → export JSONL → git sync → auto-commit/push
- **Platform-specific** lock files and health checks (Unix/Windows/WASM)

### Socket Protocol

JSON over Unix domain socket. 40+ operations defined in `internal/rpc/protocol.go`:

```go
type Request struct {
    Operation string          // "create", "update", "export", etc.
    Args      json.RawMessage // Operation-specific args
    Actor     string          // Audit trail
    RequestID string          // Correlation ID
    ExpectedDB string         // Database validation
}

type Response struct {
    Success bool
    Data    json.RawMessage
    Error   string
}
```

### Daemon Lifecycle

| File | Purpose |
|------|---------|
| `daemon.go` | Command definition, flag parsing |
| `daemon_start.go` | Initialization and startup |
| `daemon_server.go` | RPC server setup |
| `daemon_event_loop.go` | Main loop (poll, export, sync) |
| `daemon_sync.go` | Git sync coordination |
| `daemon_sync_branch.go` | Sync-branch specific logic |
| `daemon_lock_unix.go` / `_windows.go` | Platform-specific locking |
| `daemon_health_unix.go` / `_windows.go` | Platform-specific health checks |

## Data Persistence (Dual-Write)

The key design insight — **SQLite for speed, JSONL for portability**:

- **SQLite** (`beads.db`) — Fast queries, FTS5 search, dependency graphs. Not committed to git. Gitignored.
- **JSONL** (`issues.jsonl`) — One JSON object per line. Committed to git on a sync branch. This is the source of truth for collaboration.
- **Sync branch** — A dedicated git branch (e.g., `beads-metadata`) managed via a worktree, keeping issue data out of your main branch history.

### JSONL Issue Format

Each line in `issues.jsonl` is a complete issue:

```json
{
  "id": "bd-a3f2",
  "title": "Fix database connection",
  "description": "Connection pool exhausted under load",
  "status": "open",
  "priority": 1,
  "issue_type": "bug",
  "assignee": "alice",
  "created_at": "2025-01-15T10:00:00Z",
  "created_by": "alice",
  "labels": ["backend", "critical"],
  "dependencies": [{"depends_on_id": "bd-b7c1", "type": "blocks"}]
}
```

### Export/Import Cycle

- **Export** happens automatically via the auto-flush pipeline after write operations
- **Import** triggers on startup if JSONL is newer than the SQLite DB, or on `bd sync`
- **Incremental export** kicks in at 1000+ issues (skips if >20% dirty — full export is faster)
- **Content hashing** (SHA256) detects sync conflicts

## Configuration Stack

| File | Scope | Controls |
|------|-------|----------|
| `.beads/config.yaml` | Shared (committed) | Prefix, sync branch, custom types/statuses |
| `.beads/metadata.json` | Local (gitignored) | Backend choice, DB path, retention settings |
| CLI flags | Per-command | Override anything |
| Environment vars | Session | `BD_ACTOR`, `BEADS_AUTO_START_DAEMON`, etc. |

### .beads/config.yaml

```yaml
issue-prefix: bd
sync-branch: beads-metadata
no-db: false          # JSONL-only mode (no SQLite)
types:
  custom: []          # Additional issue types beyond built-ins
status:
  custom: []          # Additional statuses
```

### .beads/metadata.json

```json
{
  "database": "beads.db",
  "jsonl_export": "issues.jsonl",
  "backend": "sqlite",
  "deletions_retention_days": 3,
  "stale_closed_issues_days": 0
}
```

## The Issue Struct

Defined in `internal/types/types.go`. The struct has **134 fields**. The core fields most commands use:

### Core Fields

| Field | Type | Purpose |
|-------|------|---------|
| `ID` | string | Hash-based ID (e.g., `bd-a3f2`) |
| `Title` | string | Issue title |
| `Description` | string | Full description |
| `Status` | Status | open, in_progress, blocked, deferred, closed |
| `Priority` | int | 0-5 (P0=critical) |
| `IssueType` | IssueType | bug, feature, task, epic, chore |
| `Assignee` | string | Who's working on it |
| `CreatedAt` | time.Time | Creation timestamp |
| `UpdatedAt` | time.Time | Last update |
| `Labels` | []string | Tags |

### Advanced Field Groups

| Group | Fields | Purpose |
|-------|--------|---------|
| Dependencies | Blocks, BlockedBy, Related | Dependency graph edges |
| Molecules | MolType, WorkType, BondedTo | Swarm coordination |
| Gates | GateType, GateCondition, GateTimeout | Async coordination (CI, PR, timer) |
| HOP/CV | Creator, Validations, QualityScore | Provenance tracking |
| Compaction | CompactionLevel, CompactedAt | Semantic summarization |
| Wisps | WispType, TTL | Ephemeral issues (heartbeats) |
| Context | Repo, ExternalRef, SourceSystem | Multi-repo and integration linking |

Most advanced features are opt-in and zero-valued by default.

## Storage Layer

### Interface (`internal/storage/storage.go`)

Clean abstraction — all backends implement this:

```go
type Storage interface {
    CreateIssue(ctx, issue, actor) error
    UpdateIssue(ctx, id, updates, actor) error
    DeleteIssue(ctx, id) error
    GetIssue(ctx, id) *Issue
    SearchIssues(ctx, query, filter) []*Issue
    GetReadyWork(ctx, filter) []*Issue
    GetBlockedIssues(ctx, filter) []*BlockedIssue
    AddDependency(ctx, dep, actor) error
    GetDependencyTree(ctx, id, depth, allPaths, reverse) []*TreeNode
    // ... 30+ methods total
}
```

### Backends

| Backend | Location | Use Case |
|---------|----------|----------|
| SQLite | `internal/storage/sqlite/` (41 files) | Primary. Fast queries, FTS5 search. |
| Dolt | `internal/storage/dolt/` (26 files) | Git-native versioning. Embedded or server mode. |
| Memory | `internal/storage/memory/` (8 files) | Testing and `--no-db` mode. |

### SQLite Migrations

43 migration files in `internal/storage/sqlite/migrations/`. Each adds columns, tables, or indices. Any new fields on the Issue struct need a corresponding migration.

## Command Structure (`cmd/bd/`)

Commands are organized by function. All are Cobra commands registered in `main.go`.

### Core Issue Commands

| File | Command | Purpose |
|------|---------|---------|
| `create.go` | `bd create` | Create issues |
| `update.go` | `bd update` | Modify fields |
| `close.go` | `bd close` | Close issues |
| `reopen.go` | `bd reopen` | Reopen closed issues |
| `show.go` | `bd show` | View issue details |
| `list.go` | `bd list` | List/filter issues |
| `ready.go` | `bd ready` | Show unblocked work |
| `delete.go` | `bd delete` | Delete issues |
| `restore.go` | `bd restore` | Recover from git history |

### Dependency Commands

| File | Command | Purpose |
|------|---------|---------|
| `dep.go` | `bd dep add/remove` | Manage dependencies |
| `dependencies.go` | `bd depends-on` | Query dependency tree |

### Sync & Data

| File | Command | Purpose |
|------|---------|---------|
| `sync.go` | `bd sync` | Pull/export/push |
| `sync_export.go` | `bd export` | Export DB to JSONL |
| `sync_import.go` | `bd import` | Import JSONL to DB |
| `compact.go` | `bd compact` | Summarize old issues |

### Setup & Diagnostics

| File | Command | Purpose |
|------|---------|---------|
| `init.go` | `bd init` | Initialize .beads/ |
| `doctor/` | `bd doctor` | 40+ diagnostic checks |
| `setup/` | `bd setup` | Agent integration configs |

## Git Sync Strategies

Three modes for how `.beads/issues.jsonl` relates to git:

| Strategy | How It Works | When to Use |
|----------|--------------|-------------|
| **direct** | Issues committed on main branch | Simple projects |
| **sync-branch** | Dedicated branch via worktree | Most projects (default) |
| **protected-branch** | Merge driver for protected branches | CI-protected repos |

The sync-branch strategy creates a git worktree at `.git/beads-worktrees/beads-metadata/` pointing to a `beads-metadata` branch. This keeps issue data completely out of your main branch history.

## Major Subsystems

### Doctor (40+ checks)

Comprehensive diagnostics in `cmd/bd/doctor/`. Checks schema integrity, git state, daemon health, data consistency, and integration status. New features should add corresponding doctor checks.

### Formulas (`internal/formula/`)

A DSL for workflow automation — templated multi-step issue creation with conditions and advice.

### Molecules (`cmd/bd/mol*.go`)

Swarm coordination for multi-agent workflows. Issues can be "bonded" into compounds with patrol, work, and coordination types.

### Compaction (`internal/compact/`)

Semantic summarization of old closed issues to reduce database size and context window usage. Configurable retention policies.

### Federation (`internal/routing/`, `internal/storage/sqlite/federation.go`)

Multi-repo issue routing between Dolt peers. Different from our planned centralized multi-repo model, but the routing and repo-detection plumbing may be reusable.

### Integrations

| Package | Syncs With |
|---------|------------|
| `internal/linear/` | Linear.app |
| `internal/gitlab/` | GitLab Issues |
| `cmd/bd/jira.go` | Jira |
| External refs | GitHub (via linking) |

## Build & Test

### Building

```bash
make build      # → ./bd binary, codesigned on macOS
make install    # → ~/.local/bin/bd
make test       # → scripts/test.sh with coverage
make bench      # → SQLite performance benchmarks (10K/20K issues)
```

Build requires `icu4c` on macOS (`brew install icu4c`) for the Dolt backend's regex support.

### Test Infrastructure

- **Runner**: `scripts/test.sh` wraps `go test` with coverage and skip-list support
- **Skip list**: `.test-skip` — one regex pattern per line to skip flaky tests
- **Coverage**: Atomic coverage profile written to `/tmp/beads.coverage.out`
- **Benchmarks**: CPU profiling with pprof, viewable as flamegraphs
- **Tests per package**: Each package has `*_test.go` files alongside source

### Goreleaser

`.goreleaser.yml` builds for 11 platforms (Linux/macOS/Windows/FreeBSD, amd64/arm64). Binaries are CGO_ENABLED=0 (self-contained). Windows builds get Authenticode signing.

## Adding a New Feature — Checklist

When extending beads, these are the layers you'll typically touch:

1. **`internal/types/types.go`** — Add fields to Issue struct
2. **`internal/storage/sqlite/migrations/`** — New migration file for schema change
3. **`internal/storage/sqlite/issues.go`** — Update SQL queries
4. **`internal/storage/storage.go`** — Extend interface if new operations needed
5. **`internal/rpc/protocol.go`** — Add RPC operation if daemon needs it
6. **`internal/rpc/server_*.go`** — Handle new operation server-side
7. **`cmd/bd/<command>.go`** — New or modified CLI command
8. **`cmd/bd/doctor/`** — Add diagnostic check for new feature
9. **`internal/export/`** — Ensure new fields serialize to JSONL
10. **Tests** — Unit tests in each modified package

## Existing Multi-Repo Plumbing

Before building our multi-repo extension from scratch, these existing pieces are worth studying:

| File | What's There |
|------|-------------|
| `internal/routing/` | Repo detection and routing logic |
| `internal/storage/sqlite/multirepo.go` | Multi-repo storage queries |
| `internal/storage/sqlite/federation.go` | Federation support |
| `internal/config/repos.go` | Multi-repo config parsing |
| `types.Issue.Repo` | Field already exists on the Issue struct |
| `types.Issue.ExternalRef` | External system linking |
| `types.Issue.SourceSystem` | Source system tracking |

The `Repo` field already exists on the Issue type. The federation system routes issues between Dolt peers — different from our centralized model, but the plumbing may overlap significantly.
