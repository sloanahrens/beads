# bd — Complete Command Reference

Reference for bd Latest. Generated from `bd help --all`.

## Table of Contents

### Working With Issues:

- [bd children](#bd-children) — List child beads of a parent
- [bd close](#bd-close) — Close one or more issues
- [bd comments](#bd-comments) — View or manage comments on an issue
  - [bd comments add](#bd-comments-add) — Add a comment to an issue
  - [bd comments list](#bd-comments-list) — Invalid — use bd comments &lt;issue-id&gt; to list comments
- [bd create](#bd-create) — Create a new issue (or batch from markdown/graph JSON)
- [bd create-form](#bd-create-form) — Create a new issue using an interactive form
- [bd delete](#bd-delete) — Delete one or more issues and clean up references
- [bd gate](#bd-gate) — Manage async coordination gates
  - [bd gate add-waiter](#bd-gate-add-waiter) — Add a waiter to a gate
  - [bd gate check](#bd-gate-check) — Evaluate gates and close resolved ones
  - [bd gate create](#bd-gate-create) — Create a gate that blocks an issue
  - [bd gate discover](#bd-gate-discover) — Discover await_id for gh:run gates
  - [bd gate list](#bd-gate-list) — List gate issues
  - [bd gate resolve](#bd-gate-resolve) — Manually resolve (close) a gate
  - [bd gate show](#bd-gate-show) — Show a gate issue
- [bd heartbeat](#bd-heartbeat) — Refresh the lease on an issue you hold in_progress
- [bd label](#bd-label) — Manage issue labels
  - [bd label add](#bd-label-add) — Add one or more labels to one or more issues
  - [bd label list](#bd-label-list) — List labels for an issue
  - [bd label list-all](#bd-label-list-all) — List all unique labels in the database
  - [bd label propagate](#bd-label-propagate) — Propagate a label from a parent issue to all its children
  - [bd label remove](#bd-label-remove) — Remove one or more labels from one or more issues
- [bd land-record](#bd-land-record) — Record a landing or a landing rejection on a work bead
- [bd list](#bd-list) — List issues
- [bd merge-slot](#bd-merge-slot) — Manage merge-slot gates for serialized conflict resolution
  - [bd merge-slot acquire](#bd-merge-slot-acquire) — Acquire the merge slot
  - [bd merge-slot check](#bd-merge-slot-check) — Check merge slot availability
  - [bd merge-slot create](#bd-merge-slot-create) — Create a merge slot bead for the current rig
  - [bd merge-slot release](#bd-merge-slot-release) — Release the merge slot
- [bd note](#bd-note) — Append a note to an issue
- [bd priority](#bd-priority) — Set the priority of an issue
- [bd q](#bd-q) — Quick capture: create issue and output only ID
- [bd query](#bd-query) — Query issues using a simple query language
- [bd reclaim](#bd-reclaim) — Revert stale-lease in_progress issues back to ready (dead-worker recovery)
- [bd reopen](#bd-reopen) — Reopen one or more closed issues
- [bd search](#bd-search) — Search issues by text query
- [bd set-state](#bd-set-state) — Set operational state (creates event + updates label)
- [bd show](#bd-show) — Show issue details
- [bd state](#bd-state) — Query the current value of a state dimension
  - [bd state list](#bd-state-list) — List all state dimensions on an issue
- [bd todo](#bd-todo) — Manage TODO items (convenience wrapper for task issues)
  - [bd todo add](#bd-todo-add) — Add a new TODO item
  - [bd todo done](#bd-todo-done) — Mark TODO(s) as done
  - [bd todo list](#bd-todo-list) — List TODO items
- [bd unclaim](#bd-unclaim) — Release a claimed issue
- [bd update](#bd-update) — Update one or more issues

### Views & Reports:

- [bd count](#bd-count) — Count issues matching filters
- [bd history](#bd-history) — Show version history for an issue
- [bd status](#bd-status) — Show issue database overview and statistics
- [bd statuses](#bd-statuses) — List valid issue statuses

### Dependencies & Structure:

- [bd dep](#bd-dep) — Manage dependencies
  - [bd dep add](#bd-dep-add) — Add a dependency
  - [bd dep cycles](#bd-dep-cycles) — Detect dependency cycles
  - [bd dep list](#bd-dep-list) — List dependencies or dependents of one or more issues
  - [bd dep prune-orphans](#bd-dep-prune-orphans) — Remove dependency rows whose issue or target no longer exists
  - [bd dep relate](#bd-dep-relate) — Create a bidirectional relates_to link between issues
  - [bd dep remove](#bd-dep-remove) — Remove a dependency
  - [bd dep tree](#bd-dep-tree) — Show dependency tree
  - [bd dep unrelate](#bd-dep-unrelate) — Remove a relates_to link between issues
- [bd epic](#bd-epic) — Epic management commands
  - [bd epic close-eligible](#bd-epic-close-eligible) — Close epics where all children are complete
  - [bd epic status](#bd-epic-status) — Show epic completion status
- [bd supersede](#bd-supersede) — Mark an issue as superseded by a newer one

### Sync & Data:

- [bd backup](#bd-backup) — Back up your beads database
  - [bd backup init](#bd-backup-init) — Set up a Dolt backup destination
  - [bd backup remove](#bd-backup-remove) — Remove the configured backup destination
  - [bd backup restore](#bd-backup-restore) — Restore database from a Dolt backup
  - [bd backup status](#bd-backup-status) — Show last backup status
  - [bd backup sync](#bd-backup-sync) — Push database to configured Dolt backup
- [bd branch](#bd-branch) — List or create branches
- [bd export](#bd-export) — Export issues to JSONL format
- [bd import](#bd-import) — Import issues from a JSONL file or stdin into the database
- [bd vc](#bd-vc) — Version control operations
  - [bd vc commit](#bd-vc-commit) — Create a commit with all staged changes
  - [bd vc merge](#bd-vc-merge) — Merge a branch into the current branch
  - [bd vc status](#bd-vc-status) — Show current branch and uncommitted changes

### Setup & Configuration:

- [bd bootstrap](#bd-bootstrap) — Non-destructive database setup for fresh clones and recovery
- [bd config](#bd-config) — Manage configuration settings
  - [bd config apply](#bd-config-apply) — Reconcile system state to match configuration
  - [bd config drift](#bd-config-drift) — Detect config-vs-reality inconsistencies
  - [bd config get](#bd-config-get) — Get a configuration value
  - [bd config list](#bd-config-list) — List all configuration
  - [bd config set](#bd-config-set) — Set a configuration value
  - [bd config set-many](#bd-config-set-many) — Set multiple configuration values in one operation
  - [bd config show](#bd-config-show) — Show all effective configuration with provenance
  - [bd config unset](#bd-config-unset) — Delete a configuration value
  - [bd config validate](#bd-config-validate) — Validate sync-related configuration
- [bd context](#bd-context) — Show effective backend identity and repository context
- [bd dolt](#bd-dolt) — Configure Dolt database settings
  - [bd dolt clean-databases](#bd-dolt-clean-databases) — Drop stale test databases from the Dolt server
  - [bd dolt commit](#bd-dolt-commit) — Create a Dolt commit from pending changes
  - [bd dolt killall](#bd-dolt-killall) — Kill all orphan Dolt server processes
  - [bd dolt pull](#bd-dolt-pull) — Pull commits from Dolt remote
  - [bd dolt push](#bd-dolt-push) — Push commits to Dolt remote
  - [bd dolt remote](#bd-dolt-remote) — Manage Dolt remotes
  - [bd dolt set](#bd-dolt-set) — Set a Dolt configuration value
  - [bd dolt show](#bd-dolt-show) — Show current Dolt configuration with connection status
  - [bd dolt start](#bd-dolt-start) — Start the Dolt SQL server for this project
  - [bd dolt status](#bd-dolt-status) — Show Dolt engine status
  - [bd dolt stop](#bd-dolt-stop) — Stop the Dolt SQL server for this project
  - [bd dolt test](#bd-dolt-test) — Test connection to Dolt server
- [bd forget](#bd-forget) — Remove a persistent memory
- [bd hooks](#bd-hooks) — Manage git hooks for beads integration
  - [bd hooks install](#bd-hooks-install) — Install bd git hooks
  - [bd hooks list](#bd-hooks-list) — List installed git hooks status
  - [bd hooks run](#bd-hooks-run) — Execute a git hook (called by thin shims)
  - [bd hooks uninstall](#bd-hooks-uninstall) — Uninstall bd git hooks
- [bd human](#bd-human) — Show essential commands for human users
  - [bd human dismiss](#bd-human-dismiss) — Dismiss a human-needed bead
  - [bd human list](#bd-human-list) — List human-needed beads
  - [bd human respond](#bd-human-respond) — Respond to a human-needed bead
  - [bd human stats](#bd-human-stats) — Show summary statistics for human-needed beads
- [bd info](#bd-info) — Show database information
- [bd init](#bd-init) — Initialize bd in the current directory
- [bd kv](#bd-kv) — Key-value store commands
  - [bd kv clear](#bd-kv-clear) — Delete a key-value pair
  - [bd kv get](#bd-kv-get) — Get a value by key
  - [bd kv list](#bd-kv-list) — List all key-value pairs
  - [bd kv set](#bd-kv-set) — Set a key-value pair
- [bd memories](#bd-memories) — List or search persistent memories
- [bd migrate-personal](#bd-migrate-personal) — Move personal planning issues from the project database to your planning repo
- [bd onboard](#bd-onboard) — Display minimal snippet for agent instructions file
- [bd prime](#bd-prime) — Output AI-optimized workflow context
- [bd quickstart](#bd-quickstart) — Quick start guide for bd
- [bd recall](#bd-recall) — Retrieve a specific memory
- [bd remember](#bd-remember) — Store a persistent memory
- [bd where](#bd-where) — Show active beads location

### Maintenance:

- [bd doctor](#bd-doctor) — Check and fix beads installation health (start here)
- [bd events](#bd-events) — Read and manage the durable events journal
  - [bd events export](#bd-events-export) — Print the entire journal from the beginning (JSON lines)
  - [bd events prune](#bd-events-prune) — Delete journal records below a sequence number (retention)
  - [bd events tail](#bd-events-tail) — Print journal records after a sequence number (JSON lines)
- [bd flatten](#bd-flatten) — Squash all Dolt history into a single commit
- [bd gc](#bd-gc) — Garbage collect: run Dolt GC to reclaim disk space
- [bd migrate](#bd-migrate) — Database migration commands
  - [bd migrate hooks](#bd-migrate-hooks) — Plan or apply git hook migration to marker-managed format
  - [bd migrate issues](#bd-migrate-issues) — Move issues between repositories
  - [bd migrate legacy-sqlite](#bd-migrate-legacy-sqlite) — Read an authenticated legacy SQLite database as JSONL
  - [bd migrate schema](#bd-migrate-schema) — Apply pending schema migrations (idempotent)
  - [bd migrate sync](#bd-migrate-sync) — Set up sync.branch workflow for multi-clone setups
- [bd purge](#bd-purge) — Delete closed ephemeral beads to reclaim space
- [bd rename-prefix](#bd-rename-prefix) — Rename the issue prefix for all issues in the database
- [bd sql](#bd-sql) — Execute raw SQL against the beads database
- [bd upgrade](#bd-upgrade) — Check and manage bd version upgrades
  - [bd upgrade ack](#bd-upgrade-ack) — Acknowledge the current bd version
  - [bd upgrade review](#bd-upgrade-review) — Review changes since last bd version
  - [bd upgrade status](#bd-upgrade-status) — Check if bd version has changed
- [bd worktree](#bd-worktree) — Manage git worktrees for parallel development
  - [bd worktree create](#bd-worktree-create) — Create a worktree
  - [bd worktree info](#bd-worktree-info) — Show worktree info for current directory
  - [bd worktree list](#bd-worktree-list) — List all git worktrees
  - [bd worktree remove](#bd-worktree-remove) — Remove a worktree with safety checks

### Integrations & Advanced:

- [bd capabilities](#bd-capabilities) — Describe the commands, flags and error kinds this bd supports (JSON)
- [bd repo](#bd-repo) — Manage multiple repository configuration
  - [bd repo add](#bd-repo-add) — Add an additional repository to sync
  - [bd repo list](#bd-repo-list) — List all configured repositories
  - [bd repo remove](#bd-repo-remove) — Remove a repository from sync configuration
  - [bd repo sync](#bd-repo-sync) — Manually trigger multi-repo sync

### Other Commands:

- [bd blocked](#bd-blocked) — Show blocked issues
- [bd completion](#bd-completion) — Generate the autocompletion script for the specified shell
  - [bd completion bash](#bd-completion-bash) — Generate the autocompletion script for bash
  - [bd completion fish](#bd-completion-fish) — Generate the autocompletion script for fish
  - [bd completion powershell](#bd-completion-powershell) — Generate the autocompletion script for powershell
  - [bd completion zsh](#bd-completion-zsh) — Generate the autocompletion script for zsh
- [bd cook](#bd-cook) — Compile a formula into a proto (ephemeral by default)
- [bd defer](#bd-defer) — Defer one or more issues for later
- [bd formula](#bd-formula) — Manage workflow formulas
  - [bd formula convert](#bd-formula-convert) — Convert formula from JSON to TOML
  - [bd formula lint](#bd-formula-lint) — Report every key, gate type or var a formula would have silently dropped
  - [bd formula list](#bd-formula-list) — List available formulas
  - [bd formula schema](#bd-formula-schema) — Show the formula schema index (every exported struct in types.go)
  - [bd formula show](#bd-formula-show) — Show formula details
- [bd help](#bd-help) — Help about any command
- [bd init-safety](#bd-init-safety) — Explain bd init flag semantics and the destroy-token format
- [bd mail](#bd-mail) — Delegate to mail provider (e.g., gt mail)
- [bd mol](#bd-mol) — Molecule commands (work templates)
  - [bd mol bond](#bd-mol-bond) — Bond two protos or molecules together
  - [bd mol burn](#bd-mol-burn) — Delete a molecule without creating a digest
  - [bd mol current](#bd-mol-current) — Show current position in molecule workflow
  - [bd mol distill](#bd-mol-distill) — Extract a formula from an existing epic
  - [bd mol last-activity](#bd-mol-last-activity) — Show last activity timestamp for a molecule
  - [bd mol pour](#bd-mol-pour) — Instantiate a proto as a persistent mol (solid -&gt; liquid)
  - [bd mol progress](#bd-mol-progress) — Show molecule progress summary
  - [bd mol ready](#bd-mol-ready) — Find molecules ready for gate-resume dispatch
  - [bd mol seed](#bd-mol-seed) — Verify formula accessibility
  - [bd mol show](#bd-mol-show) — Show molecule details
  - [bd mol squash](#bd-mol-squash) — Compress molecule execution into a digest
  - [bd mol stale](#bd-mol-stale) — Detect complete-but-unclosed molecules
  - [bd mol wisp](#bd-mol-wisp) — Create or manage wisps (ephemeral molecules)
- [bd ready](#bd-ready) — Show ready work (open, no active blockers)
- [bd rename](#bd-rename) — Rename an issue ID
- [bd schema](#bd-schema) — Print the JSON Schema for bd's --json / export output
- [bd undefer](#bd-undefer) — Undefer one or more issues (restore to open)
- [bd version](#bd-version) — Print version information

---

## Global Flags

These flags apply to all commands:

```
      --actor string              Actor name for audit trail (default: $BEADS_ACTOR, git user.name, $USER)
      --cpu-profile               Generate CPU profile for performance analysis
      --database string           Server database name (bd init only)
      --db string                 Database path (default: auto-discover .beads/*.db)
  -C, --directory string          Change to this directory before running the command (like git -C)
      --dolt-auto-commit string   Dolt auto-commit policy (off|on|batch). 'on': commit after each write. 'batch': defer commits to bd dolt commit; uncommitted changes persist in the working set until then (a live batch-mode bd process also flushes on SIGTERM/SIGHUP). Applies to embedded and direct SQL-server modes. Default: on. Override via config key dolt.auto-commit
      --global                    Use the global shared-server database (beads_global)
      --ignore-schema-skew        Proceed despite forward schema drift (some queries may fail)
      --json                      Output in JSON format
      --machine                   Machine mode for programs (also: BD_MACHINE=1): implies --json, never prompts, never reads stdin unless an argument is '-', no colors or tips
      --mem-profile string        Write heap profile to FILE on exit (also respects BEADS_MEM_PROFILE)
      --no-color                  Disable color output (also: NO_COLOR=1 or CLICOLOR=0)
  -q, --quiet                     Suppress non-essential output (errors only)
      --readonly                  Read-only mode: block write operations (for worker sandboxes)
      --sandbox                   Sandbox mode: disables Dolt auto-push
  -v, --verbose                   Enable verbose/debug output
```

---

## Working With Issues:

### bd children

List all beads that are children of the specified parent bead.

This is a convenience alias for 'bd list --parent &lt;id&gt; --status all'.
Unlike plain 'bd list', children includes closed issues by default,
since the primary use case is inspecting all work under a parent.

Examples:
  bd children hq-abc123        # List all children of hq-abc123
  bd children hq-abc123 --json # List children in JSON format
  bd children hq-abc123 --pretty # Show children in tree format

```
bd children <parent-id> [flags]
```

**Flags:**

```
      --pretty   Show children in tree format
```

### bd close

Close one or more issues.

If no issue ID is provided, closes the last touched issue (from most recent
create, update, show, or close operation). This fallback only applies in
interactive sessions (stdin is a terminal); in scripts and agent sessions a
missing ID is an error, so a command built from an empty variable cannot
silently close an unrelated issue. Set BD_LAST_TOUCHED_FALLBACK=1 to allow
the fallback anywhere, or =0 to disable it entirely.

When closing multiple issues, provide one --reason for all IDs or repeat
--reason once per ID. Reasons map positionally: the first --reason applies
to the first ID, the second --reason to the second ID, regardless of where
the flags appear in the command line.

```
bd close [id...] [flags]
```

**Aliases:** done

**Flags:**

```
      --claim-next           Automatically claim the next highest priority available issue
      --continue             Auto-advance to next step in molecule
  -f, --force                Force close pinned issues or unsatisfied gates
      --if-assignee string   Only close while the current assignee equals this value (--if-assignee '' requires unassigned); a mismatch writes nothing for that id and exits 13 (guard_not_held)
      --if-status string     Only close while the current status equals this value; a mismatch writes nothing for that id and exits 13 (guard_not_held)
      --no-auto              With --continue, show next step but don't claim it
  -r, --reason string        Reason for closing
      --reason-file string   Read close reason from file (use - for stdin)
      --session string       Claude Code session ID (or set CLAUDE_SESSION_ID env var)
      --suggest-next         Show newly unblocked issues after closing
```

### bd comments

View or manage comments on an issue.

Examples:
  # List all comments on an issue (issue id is required — there is no "comments list")
  bd comments bd-123

  # List comments in JSON format
  bd comments bd-123 --json

  # Add a comment
  bd comments add bd-123 "This is a comment"

  # Add a comment from a file
  bd comments add bd-123 -f notes.txt

```
bd comments [issue-id] [flags]
bd comments [command]
```

**Flags:**

```
      --local-time   Show timestamps in local time instead of UTC
```

#### bd comments add

Add a comment to an issue.

Examples:
  # Add a comment
  bd comments add bd-123 "Working on this now"

  # Add a comment from a file
  bd comments add bd-123 -f notes.txt

```
bd comments add [issue-id] [text...] [flags]
```

**Flags:**

```
  -a, --author string   Add author to comment
  -f, --file string     Read comment text from file
```

#### bd comments list

Invalid — use bd comments &lt;issue-id&gt; to list comments

```
bd comments list
```

### bd create

Create a new issue (or batch from markdown/graph JSON)

```
bd create [title] [flags]
```

**Aliases:** new

**Flags:**

```
      --acceptance string         Acceptance criteria
      --allow-empty-description   Allow empty description input from stdin or file
      --append-notes string       Append to existing notes (with newline separator)
  -a, --assignee string           Assignee
      --body-file string          Read description from file (use - for stdin)
      --context string            Additional context for the issue
      --defer string              Defer until date (issue hidden from bd ready until then). Same formats as --due
      --deps strings              Dependencies as 'type:id' or bare 'id'. Bare 'id', 'depends-on:id', and 'blocked-by:id' all make THIS issue depend on id; 'blocks:id' reverses direction (id depends on this issue). E.g. 'blocked-by:bd-20,discovered-from:bd-15'
  -d, --description string        Issue description
      --design string             Design notes
      --design-file string        Read design from file (use - for stdin)
      --dry-run                   Preview what would be created without actually creating
      --due string                Due date/time. Formats: +6h, +1d, +2w, tomorrow, next monday, 2025-01-15
      --ephemeral                 Create as ephemeral (short-lived, subject to TTL compaction)
  -e, --estimate int              Time estimate in minutes (e.g., 60 for 1 hour)
      --event-actor string        Entity URI who caused this event (requires --type=event)
      --event-category string     Event category (e.g., patrol.muted, agent.started) (requires --type=event)
      --event-payload string      Event-specific JSON data (requires --type=event)
      --event-target string       Entity URI or bead ID affected (requires --type=event)
      --external-ref string       External reference (e.g., 'gh-9', 'jira-ABC', Linear URL)
  -f, --file string               Create one issue per ## heading (### fields attach to that issue); for title plus description-from-file, use --body-file
      --force                     Force creation even if prefix doesn't match database prefix
      --graph string              Create a graph of issues with dependencies from JSON plan file
      --id string                 Explicit issue ID (e.g., 'bd-42' for partitioning)
  -l, --labels strings            Labels (comma-separated)
      --metadata string           Set custom metadata (JSON string or @file.json to read from file)
      --mol-type string           Molecule type: swarm (multi-agent), patrol (recurring ops), work (default)
      --no-history                Skip Dolt commit history without making GC-eligible (for permanent agent beads)
      --no-inherit-labels         Don't inherit labels from parent issue
      --notes string              Additional notes
      --parent string             Parent issue ID for hierarchical child (e.g., 'bd-a3f8e9')
  -p, --priority string           Priority (0-4 or P0-P4, 0=highest) (default "2")
      --repo string               Filesystem path of the target workspace (a directory containing .beads/), not a rig or repository name; overrides auto-routing
      --silent                    Output only the issue ID (for scripting)
      --skills string             Required skills for this issue
      --spec-id string            Link to specification document
  -s, --status string             Initial status
      --stdin                     Read description from stdin (alias for --body-file -)
      --storage-class string      Storage class: versioned, unversioned, or ephemeral (default: storage-class.<type> config, else versioned)
      --title string              Issue title (alternative to positional argument)
  -t, --type string               Issue type (bug|feature|task|epic|chore|decision|spike|story|milestone); custom types require types.custom config; aliases: enhancement/feat→feature, dec/adr→decision (default "task")
      --validate                  Validate description contains required sections for issue type
      --waits-for string          Spawner issue ID to wait for (creates waits-for dependency for fanout gate)
      --waits-for-gate string     Gate type: all-children (wait for all) or any-children (wait for first) (default "all-children")
      --wisp-type string          Wisp type for TTL-based compaction: heartbeat, ping, patrol, gc_report, recovery, error, escalation
```

**Graph plan schema (`--graph`):**

`bd create --graph plan.json` creates a whole issue graph atomically from a
JSON plan. Node field names follow the issue model's JSON tags (the same
names `bd show --json` emits), and every field single-issue `bd create` can
set is addressable from a plan — plus an initial `status` and `pinned`,
which `bd create` has no flags for:

```json
{
  "commit_message": "optional Dolt commit message",
  "nodes": [
    {
      "key": "api",
      "title": "Design the API",
      "type": "task",
      "description": "…", "design": "…", "acceptance_criteria": "…", "notes": "…",
      "status": "in_progress",
      "priority": 1,
      "assignee": "alice", "assign_after_create": false,
      "owner": "alice@example.com",
      "labels": ["backend"],
      "estimated_minutes": 45,
      "due_at": "2030-01-02T15:04:05Z",
      "defer_until": "2030-01-01T00:00:00Z",
      "spec_id": "bd-spec1", "external_ref": "gh-42",
      "metadata": {"any": "json", "count": 3},
      "metadata_refs": {"tracker": "impl"},
      "id": "bd-a1b2c3",
      "ephemeral": false, "no_history": false,
      "wisp_type": "heartbeat", "mol_type": "swarm",
      "pinned": false
    },
    { "key": "impl", "title": "Implement it", "parent_key": "api" },
    { "key": "gate", "title": "Fanout gate" },
    { "key": "launch", "title": "Launch announced", "type": "event",
      "event_kind": "agent.started", "actor": "agent://a", "target": "bead://b", "payload": "{}" }
  ],
  "edges": [
    { "from_key": "gate", "to_key": "api", "type": "blocks" },
    { "from_key": "gate", "to_key": "impl", "type": "waits-for",
      "gate": "any-children", "spawner_key": "impl" }
  ]
}
```

Node notes:

- `key` is a plan-local symbolic name; the result maps each key to the minted
  issue ID. `id` optionally pins an explicit issue ID instead (must match the
  database prefix, like `bd create --id`, and must not already exist — a plan
  never overwrites an existing issue).
- `parent_key` (a plan node key) or `parent_id` (an existing issue ID) adds a
  `parent-child` dependency on the created node.
- `parent` and `estimate` are accepted aliases for `parent_key` and
  `estimated_minutes`; the canonical field wins when both spellings are set.
- `status` sets the initial status (default `open`, or `deferred` when
  `defer_until` is in the future). Nodes created `closed` get `closed_at`
  auto-filled. Timestamps are RFC3339; relative forms like `+6h` are not
  accepted in plans.
- `ephemeral` / `no_history` override the plan-wide `--ephemeral` /
  `--no-history` flags per node (mutually exclusive per node; in
  proxied-server mode the effective storage class must be uniform across the
  plan).
- `event_kind`, `actor`, `target`, and `payload` require `"type": "event"`.
- `metadata` accepts arbitrary JSON values; `metadata_refs` values name other
  node keys and are replaced with their minted IDs after creation.
- Unlike `bd create --parent`, graph children do not inherit parent labels
  and receive flat (non-hierarchical) IDs; plans are explicit artifacts.

Edge notes:

- `type` defaults to `blocks`. `parent_key`/`parent_id` on nodes already
  create `parent-child` edges; don't repeat them in `edges`.
- `gate` (`all-children` | `any-children`) and `spawner_key`/`spawner_id`
  attach fanout-gate metadata to `waits-for` edges, mirroring
  `--waits-for`/`--waits-for-gate`. The `to` endpoint of a `waits-for` edge
  *is* the spawner (gate evaluation watches its children), so `spawner_key`
  must match `to_key` and `spawner_id` must match `to_id`; they exist to make
  plans self-documenting. Since an explicit `to_id` overrides `to_key` as the
  target, `spawner_key` cannot be combined with `to_id` — use `spawner_id`.
- Edges with ready-work-affecting types (`blocks`, `waits-for`, …) may not
  duplicate or reverse a `parent_key`/`parent_id` relationship, and a parent
  may not be connected to its own child through them.
- `thread_id` threads conversation edges (e.g. `replies-to`).

Comments cannot be created from a plan; use `bd comments add` after creation.
Model fields without `bd create` flags (`sender`, `work_type`, `is_template`,
`await_*`, `bonded_from`, `source_*`) are likewise not settable from plans.

### bd create-form

Create a new issue using an interactive terminal form.

This command provides a user-friendly form interface for creating issues,
with fields for title, description, type, priority, labels, and more.

Use --parent to create a sub-issue under an existing parent issue.
The child will get an auto-generated hierarchical ID (e.g., parent-id.1).

The form uses keyboard navigation:
  - Tab/Shift+Tab: Move between fields
  - Enter: Submit the form (on the last field or submit button)
  - Ctrl+C: Cancel and exit
  - Arrow keys: Navigate within select fields

```
bd create-form [flags]
```

**Flags:**

```
      --parent string   Parent issue ID for creating a hierarchical child (e.g., 'bd-a3f8e9')
```

### bd delete

Delete one or more issues and clean up all references to them.
This command will:
1. Remove all dependency links (any type, both directions) involving the issues
2. Update text references to "[deleted:ID]" in directly connected issues
3. Permanently delete the issues from the database

This is a destructive operation that cannot be undone. Use with caution.

BATCH DELETION:
Delete multiple issues at once:
  bd delete bd-1 bd-2 bd-3 --force

Delete from file (one ID per line):
  bd delete --from-file deletions.txt --force

Preview before deleting:
  bd delete --from-file deletions.txt --dry-run

DEPENDENCY HANDLING (the same on a local database and against a team server):
Default: Fails if any issue has dependents not in deletion set
  bd delete bd-1 bd-2

Cascade: Recursively delete all dependents
  bd delete bd-1 --cascade --force

Force: Delete and orphan dependents
  bd delete bd-1 --force

```
bd delete <issue-id> [issue-id...] [flags]
```

**Flags:**

```
      --cascade              Recursively delete all dependent issues
      --dry-run              Preview what would be deleted without making changes
  -f, --force                Actually delete (without this flag, shows preview)
      --from-file string     Read issue IDs from file (one per line)
      --if-assignee string   Only delete while the current assignee equals this value (--if-assignee '' requires unassigned); a mismatch writes nothing for that id and exits 13 (guard_not_held)
      --if-status string     Only delete while the current status equals this value; a mismatch writes nothing for that id and exits 13 (guard_not_held)
```

### bd gate

Gates are async wait conditions that block workflow steps.

Gates are created automatically when a formula step has a gate field.
They must be closed (manually or via watchers) for the blocked step to proceed.

Gate types:
  human   - Requires manual bd close (Phase 1)
  timer   - Expires after timeout (Phase 2)
  gh:run  - Waits for GitHub workflow (Phase 3)
  gh:pr   - Waits for PR merge (Phase 3)
  bead    - Waits for another bead to close (Phase 4)

For bead gates, await_id may be a bead ID in this rig's database (e.g.,
"bd-abc123") or the historical cross-rig form &lt;rig&gt;:&lt;bead-id&gt;. Cross-rig
targets resolve through the bead ID's prefix route in routes.jsonl.

Examples:
  bd gate list           # Show all open gates
  bd gate list --all     # Show all gates including closed
  bd gate check          # Evaluate all open gates
  bd gate check --type=bead  # Evaluate only bead gates
  bd gate resolve &lt;id&gt;   # Close a gate manually

```
bd gate [command]
```

#### bd gate add-waiter

Register an agent as a waiter on a gate bead.

When the gate closes, the waiter will receive a wake notification via 'bd gate wake'.
The waiter is typically the worker's address (e.g., "my-project/workers/agent-1").

This is used by 'bd done --phase-complete' to register for gate wake notifications.

```
bd gate add-waiter <gate-id> <waiter>
```

#### bd gate check

Evaluate gate conditions and automatically close resolved gates.

By default, checks all open gates. Use --type to filter by gate type.

Gate types:
  gh       - Check all GitHub gates (gh:run and gh:pr)
  gh:run   - Check GitHub Actions workflow runs
  gh:pr    - Check pull request merge status
  timer    - Check timer gates (auto-expire based on timeout)
  bead     - Check cross-rig bead gates
  all      - Check all gate types

GitHub gates use the 'gh' CLI to query status:
  - gh:run checks 'gh run view &lt;id&gt; --json status,conclusion'
  - gh:pr checks 'gh pr view &lt;id&gt; --json state,title'

A gate is resolved when:
  - gh:run: status=completed AND conclusion=success
  - gh:pr: state=MERGED
  - timer: current time &gt; created_at + timeout
  - bead: target bead status=closed

A gate is escalated when:
  - gh:run: status=completed AND conclusion in (failure, canceled)
  - gh:pr: state=CLOSED

Examples:
  bd gate check              # Check all gates
  bd gate check --type=gh    # Check only GitHub gates
  bd gate check --type=gh:run # Check only workflow run gates
  bd gate check --type=timer # Check only timer gates
  bd gate check --type=bead  # Check only cross-rig bead gates
  bd gate check --dry-run    # Show what would happen without changes
  bd gate check --escalate   # Escalate expired/failed gates

```
bd gate check [flags]
```

**Flags:**

```
      --dry-run       Show what would happen without making changes
  -e, --escalate      Escalate failed/expired gates
  -l, --limit int     Limit results (default 100) (default 100)
  -t, --type string   Gate type to check (gh, gh:run, gh:pr, timer, bead, all)
```

#### bd gate create

Create an ad-hoc gate issue that blocks another issue until resolved.

The blocked issue will not appear in 'bd ready' until the gate is resolved
via 'bd gate resolve'.

Gate types:
  human   - Requires manual 'bd gate resolve' (default)
  timer   - Auto-resolves after --timeout duration
  gh:run  - Waits for GitHub Actions workflow
  gh:pr   - Waits for PR merge

Examples:
  bd gate create --blocks bd-abc
  bd gate create --type=human --blocks bd-abc --reason="Need design review"
  bd gate create --type=timer --blocks bd-abc --timeout=2h
  bd gate create --type=gh:pr --blocks bd-abc --await-id=42
  bd gate create --blocks bd-abc --title="Gate: awaiting owner sign-off"

```
bd gate create [flags]
```

**Flags:**

```
      --await-id string   Condition identifier (run ID, PR number, etc.)
      --blocks string     Issue ID to block (required)
  -r, --reason string     Reason for the gate
      --timeout string    Timeout duration (e.g., 2h, 30m)
      --title string      Custom gate title (default: "Gate: <type>")
  -t, --type string       Gate type (human, timer, gh:run, gh:pr) (default "human")
```

#### bd gate discover

Discovers GitHub workflow run IDs for gates awaiting CI/CD completion.

This command finds open gates with await_type="gh:run" that don't have an await_id,
queries recent GitHub workflow runs, and matches them using heuristics:
  - Branch name matching
  - Commit SHA matching
  - Time proximity (runs within 5 minutes of gate creation)

Once matched, the gate's await_id is updated with the GitHub run ID, enabling
subsequent polling to check the run's status.

A gate whose metadata.repo targets another repository is only matched
against runs queried from that repository, never against the current
repository's runs of a same-named workflow.

Examples:
  bd gate discover           # Auto-discover run IDs for all matching gates
  bd gate discover --dry-run # Preview what would be matched (no updates)
  bd gate discover --branch main --limit 10  # Only match runs on 'main' branch

```
bd gate discover [flags]
```

**Flags:**

```
  -b, --branch string      Filter runs by branch (default: current branch)
  -n, --dry-run            Preview mode: show matches without updating
  -l, --limit int          Max runs to query from GitHub (default 10)
  -a, --max-age duration   Max age for gate/run matching (default 30m0s)
```

#### bd gate list

List gate issues.

With no argument, lists all gate issues in the current beads database.
With an [issue-id] argument, lists ONLY the gates that block that issue
(its own dependency gates) — not every gate in the database.

By default, shows only open gates. Use --all to include closed gates.

```
bd gate list [issue-id] [flags]
```

**Flags:**

```
  -a, --all         Show all gates including closed
  -n, --limit int   Limit results (default 50) (default 50)
```

#### bd gate resolve

Close a gate issue to unblock the step waiting on it.

This is equivalent to 'bd close &lt;gate-id&gt;' but with a more explicit name.
Use --reason to provide context for why the gate was resolved.

```
bd gate resolve <gate-id> [flags]
```

**Flags:**

```
  -r, --reason string   Reason for resolving the gate
```

#### bd gate show

Display details of a gate issue including its waiters.

This is similar to 'bd show' but validates that the issue is a gate.

```
bd gate show <gate-id>
```

### bd heartbeat

Refresh the lease on an issue you currently hold in_progress.

A claim carries a lease that expires after a TTL. A worker keeps its claim alive
by heartbeating faster than the TTL; once it stops (because it died), the lease
goes stale and 'bd reclaim' reverts the issue to ready so another worker can pick
it up. Heartbeat pushes lease_expires_at forward and stamps heartbeat_at = now.

Only the current owner may heartbeat. If the lease has already been reclaimed or
the issue closed, heartbeat fails so the worker learns to stop.

Leases live in an ephemeral, node-local table: heartbeats write no Dolt commit
and no history, so any cadence comfortably below the TTL is fine. Leases are
only enforceable on the node that granted them; cross-machine claim visibility
rides the issue's status and assignee, which do commit.

Examples:
  bd heartbeat bd-123
  bd hb bd-123

```
bd heartbeat <id>
```

**Aliases:** hb

### bd label

Manage issue labels

```
bd label [command]
```

#### bd label add

Add labels to issues. Issue IDs come first; the final argument is the label. Pass multiple labels comma-separated: bd label add bd-123 label1,label2

```
bd label add [issue-id...] [label[,label...]]
```

#### bd label list

List labels for an issue

```
bd label list [issue-id]
```

#### bd label list-all

List all unique labels in the database

```
bd label list-all
```

#### bd label propagate

Push a label from a parent down to all direct children that don't already have it. Useful for applying branch: labels across an epic's subtasks.

```
bd label propagate [parent-id] [label]
```

#### bd label remove

Remove labels from issues. Issue IDs come first; the final argument is the label. Pass multiple labels comma-separated: bd label remove bd-123 label1,label2

```
bd label remove [issue-id...] [label[,label...]]
```

### bd land-record

Record the outcome of landing a work bead's branch, in one write.

A landing writes metadata.landing = &#123;patch_id, landed_commit, gate_result,
om_verdict, om_score, route, recorded_at, recorded_by&#125; and removes the
"rework" label:

  bd land-record &lt;id&gt; --patch-id P --landed-commit C --gate-result pass \
      --om-verdict approve --om-score 8.5 --route merge-queue

A rejection writes metadata.landing_rejection = &#123;kind, gate_tail,
om_findings, conflicting_files, recorded_at, recorded_by&#125; and adds the
"rework" label:

  bd land-record &lt;id&gt; --reject --kind gate_failed --gate-tail-file tail.txt \
      --om-findings '[...]' --conflicting-file a.go --conflicting-file b.go

The metadata edit and the label edit land in one transaction. Read the record
back with bd show &lt;id&gt; --json (.metadata.landing, .metadata.landing_rejection).
Each call replaces the previous record of the same kind.

```
bd land-record <id> [flags]
```

**Flags:**

```
      --conflicting-file stringArray   Rejection: a file that conflicted (repeatable)
      --gate-result string             Landing: quality-gate result (e.g. pass)
      --gate-tail string               Rejection: tail of the failing gate output
      --gate-tail-file string          Rejection: read the gate tail from a file (- for stdin)
      --kind string                    Rejection: what failed (e.g. gate_failed, om_rejected, conflict)
      --landed-commit string           Landing: commit that landed on the target branch
      --om-findings string             Rejection: om findings as a JSON value
      --om-score float                 Landing: om review score
      --om-verdict string              Landing: om review verdict
      --patch-id string                Landing: git patch-id of the landed change
      --reject                         Record a landing rejection instead of a landing (adds the rework label)
      --route string                   Landing: how the change landed (e.g. merge-queue, direct)
```

### bd list

List issues

```
bd list [flags]
```

**Flags:**

```
      --all                          Show all issues including closed (overrides default filter)
  -a, --assignee string              Filter by assignee
      --brief                        Omit the free-form text (description, design, acceptance criteria, notes, payload, waiters) from each row. Filters that read those fields, such as --desc-contains, still select on them. An omitted field is indistinguishable from an empty one in --json; fetch a whole issue with bd show. Default in text mode except --long; meaningful for --json.
      --closed-after string          Filter issues closed after date (YYYY-MM-DD or RFC3339)
      --closed-before string         Filter issues closed before date (YYYY-MM-DD or RFC3339)
      --created-after string         Filter issues created after date (YYYY-MM-DD or RFC3339)
      --created-before string        Filter issues created before date (YYYY-MM-DD or RFC3339)
      --defer-after string           Filter issues deferred after date (supports relative: +6h, tomorrow)
      --defer-before string          Filter issues deferred before date (supports relative: +6h, tomorrow)
      --deferred                     Show only issues with defer_until set
      --deps string[="scheduling"]   Annotate tree with dependency edges and order siblings by them: 'scheduling' (bare --deps) or 'all'
      --desc-contains string         Filter by description substring (case-insensitive)
      --due-after string             Filter issues due after date (supports relative: +6h, tomorrow)
      --due-before string            Filter issues due before date (supports relative: +6h, tomorrow)
      --empty-description            Filter issues with empty or missing description
      --exclude-label strings        Exclude issues that have ANY of these labels
      --exclude-type strings         Exclude issue types from results (comma-separated or repeatable, e.g., --exclude-type=convoy,epic)
      --external-contains string     Filter by external ref substring (case-insensitive)
      --external-ref string          Filter by exact external_ref value
      --flat                         Disable tree format and use legacy flat list output
      --format string                Output format: 'digraph' (for golang.org/x/tools/cmd/digraph), 'dot' (Graphviz), or Go template
      --has-metadata-key string      Filter issues that have this metadata key set
      --id string                    Filter by specific issue IDs (comma-separated, e.g., bd-1,bd-5,bd-10)
      --include-ephemeral            Include ephemeral wisp-plane rows in output (normally hidden)
      --include-gates                Include gate issues in output (normally hidden)
      --include-infra                Include infrastructure beads (agent/role/message) and ephemeral wisps (wisp-* ids, molecules, mail, cleanup beads), which are hidden by default
      --include-templates            Include template molecules in output
  -l, --label strings                Filter by labels (AND: must have ALL). Can combine with --label-any
      --label-any strings            Filter by labels (OR: must have AT LEAST ONE). Can combine with --label
      --label-pattern string         Filter by label glob pattern (e.g., 'tech-*' matches tech-debt, tech-legacy)
      --label-regex string           Filter by label regex pattern (e.g., 'tech-(debt|legacy)')
  -n, --limit int                    Limit results (default 50, use 0 for unlimited) (default 50)
      --long                         Show detailed multi-line output for each issue
      --max-rows int                 Hard upper bound on rows fetched from storage. Returns a non-zero exit (code 2) and an error to stderr if exceeded. 0 disables (the default). Overrides BEADS_MAX_ROWS for this invocation. Useful in CI/agent rigs that want a circuit breaker against pathological queries.
      --metadata-field stringArray   Filter by metadata field (key=value, repeatable)
      --mol-type string              Filter by molecule type: swarm, patrol, or work
      --no-assignee                  Filter issues with no assignee
      --no-labels                    Filter issues with no labels
      --no-pager                     Disable pager output
      --no-parent                    Exclude child issues (show only top-level issues)
      --no-pinned                    Exclude pinned issues
      --notes-contains string        Filter by notes substring (case-insensitive)
      --offset int                   Not supported: any value above 0 is rejected (page with --limit).
      --overdue                      Show only issues with due_at in the past (not closed)
      --parent string                Filter by parent issue ID (shows children of specified issue)
      --pinned                       Show only pinned issues
      --pretty                       Display issues in a tree format with status/priority symbols
  -p, --priority string              Priority (0-4 or P0-P4, 0=highest)
      --priority-max string          Filter by maximum priority (inclusive, 0-4 or P0-P4)
      --priority-min string          Filter by minimum priority (inclusive, 0-4 or P0-P4)
      --ready                        Show only ready issues (no active blockers, same semantics as bd ready)
  -r, --reverse                      Invert the sort field's default direction (created/updated/closed default to newest-first, so --sort updated --reverse is oldest-first)
      --skip-labels                  Skip label hydration. The labels field in output will be empty regardless of actual labels. Use only when the caller does not depend on label data. Cannot combine with --label, --label-any, --label-pattern, --label-regex, --exclude-label, or --no-labels.
      --sort string                  Sort by field: priority, created, updated, closed, status, id, title, type, assignee
      --spec string                  Filter by spec_id prefix
  -s, --status string                Filter by stored status (open, in_progress, blocked, deferred, closed). Comma-separated for multiple: --status open,in_progress. Note: repeating -s/--status silently overwrites the previous value — always use the comma-separated form for multi-status filters.
      --title string                 Filter by title text (case-insensitive substring match)
      --title-contains string        Filter by title substring (case-insensitive)
      --tree                         Hierarchical tree format (default: true; use --flat to disable) (default true)
  -t, --type string                  Filter by type (bug, feature, task, epic, chore, decision, merge-request, molecule, gate, convoy). Aliases: mr→merge-request, feat→feature, mol→molecule, dec/adr→decision
      --updated-after string         Filter issues updated after date (YYYY-MM-DD or RFC3339)
      --updated-before string        Filter issues updated before date (YYYY-MM-DD or RFC3339)
  -w, --watch                        Watch for changes and auto-update display (implies --pretty)
      --wisp-type string             Filter by wisp type: heartbeat, ping, patrol, gc_report, recovery, error, escalation
```

### bd merge-slot

Merge-slot gates serialize conflict resolution in the merge queue.

A merge slot is an exclusive access primitive: only one agent can hold it at a time.
This prevents "monkey knife fights" where multiple polecats race to resolve conflicts
and create cascading conflicts.

Each rig has one merge slot bead: &lt;prefix&gt;-merge-slot (labeled gt:slot).
The slot uses:
  - status=open: slot is available
  - status=in_progress: slot is held
  - metadata.holder: who currently holds the slot
  - metadata.waiters: priority-ordered queue of waiters

Examples:
  bd merge-slot create              # Create merge slot for current rig
  bd merge-slot check               # Check if slot is available
  bd merge-slot acquire             # Try to acquire the slot
  bd merge-slot release             # Release the slot

```
bd merge-slot [command]
```

#### bd merge-slot acquire

Attempt to acquire the merge slot for exclusive access.

If the slot is available (status=open), it will be acquired:
  - status set to in_progress
  - holder set to the requester

If the slot is held (status=in_progress), the command fails unless
--wait is passed, which adds the requester to the waiters queue.

Use --holder to specify who is acquiring (default: BEADS_ACTOR env var).

```
bd merge-slot acquire [flags]
```

**Flags:**

```
      --holder string   Who is acquiring the slot (default: BEADS_ACTOR)
      --wait            Add to waiters list if slot is held
```

#### bd merge-slot check

Check if the merge slot is available or held.

Returns:
  - available: slot can be acquired
  - held by &lt;holder&gt;: slot is currently held
  - not found: no merge slot exists for this rig

```
bd merge-slot check
```

#### bd merge-slot create

Create a merge slot bead for serialized conflict resolution.

The slot ID is automatically generated based on the beads prefix (e.g., gt-merge-slot).
The slot is created with status=open (available).

```
bd merge-slot create
```

#### bd merge-slot release

Release the merge slot after conflict resolution is complete.

Sets status back to open and clears the holder field.
If there are waiters, the highest-priority waiter should then acquire.

```
bd merge-slot release [flags]
```

**Flags:**

```
      --holder string   Who is releasing the slot (for verification)
```

### bd note

Append a note to an issue's notes field.

Shorthand for 'bd update &lt;id&gt; --append-notes "text"'.

Examples:
  bd note gt-abc "Fixed the flaky test"
  bd note gt-abc Fixed the flaky test
  echo "note from pipe" | bd note gt-abc --stdin
  bd note gt-abc --file notes.txt

Note: "note" has NO subcommands — it only appends.
To read notes on an issue, use: bd show &lt;id&gt;

```
bd note <id> [text...] [flags]
```

**Flags:**

```
      --file string   Read note text from file
      --stdin         Read note text from stdin
```

### bd priority

Set the priority of an issue.

Shorthand for 'bd update &lt;id&gt; --priority &lt;n&gt;'.

Priority levels:
  0 - Critical (security, data loss, broken builds)
  1 - High (major features, important bugs)
  2 - Medium (default)
  3 - Low (polish, optimization)
  4 - Backlog (future ideas)

Examples:
  bd priority bd-123 0    # Critical
  bd priority bd-123 2    # Medium

```
bd priority <id> <n>
```

### bd q

Quick capture creates an issue and outputs only the issue ID.
Designed for scripting and AI agent integration.

Example:
  bd q "Fix login bug"           # Outputs: bd-a1b2
  ISSUE=$(bd q "New feature")    # Capture ID in variable
  bd q "Task" | xargs bd show    # Pipe to other commands
  bd q "Subtask" --parent=bd-a1b2  # Hierarchical child (outputs: bd-a1b2.1)

```
bd q [title] [flags]
```

**Flags:**

```
  -l, --labels strings    Labels
      --parent string     Parent issue ID for hierarchical child (e.g., 'bd-a3f8e9')
  -p, --priority string   Priority (0-4 or P0-P4) (default "2")
  -t, --type string       Issue type (default "task")
```

### bd query

Query issues using a simple query language that supports compound filters,
boolean operators, and date-relative expressions.

The query language enables complex filtering that would otherwise require
multiple flags or piping through jq.

Syntax:
  field=value       Equality comparison
  field!=value      Inequality comparison
  field&gt;value       Greater than
  field&gt;=value      Greater than or equal
  field&lt;value       Less than
  field&lt;=value      Less than or equal

Boolean operators (case-insensitive):
  expr AND expr     Both conditions must match
  expr OR expr      Either condition can match
  NOT expr          Negates the condition
  (expr)            Grouping with parentheses

Supported fields:
  status            Stored status (open, in_progress, blocked, deferred, closed). Note: dependency-blocked issues stay "open"; use 'bd blocked' to find them
  priority          Priority level (0-4)
  type              Issue type (bug, feature, task, epic, chore, decision)
  assignee          Assigned user (use "none" for unassigned)
  owner             Issue owner
  label             Issue label (use "none" for unlabeled)
  title             Search in title (contains)
  description       Search in description (contains, "none" for empty)
  notes             Search in notes (contains)
  created           Creation date/time
  updated           Last update date/time
  started           Date/time issue first transitioned to in_progress
  closed            Close date/time
  id                Issue ID (supports wildcards: bd-*)
  spec              Spec ID (supports wildcards)
  pinned            Boolean (true/false)
  ephemeral         Boolean (true/false)
  template          Boolean (true/false)
  parent            Parent issue ID
  mol_type          Molecule type (swarm, patrol, work)

Date values:
  Relative durations: 7d (7 days ago), 24h (24 hours ago), 2w (2 weeks ago)
  Absolute dates: 2025-01-15, 2025-01-15T10:00:00Z
  Natural language: tomorrow, "next monday", "in 3 days"

Examples:
  bd query "status=open AND priority&gt;1"
  bd query "status=open AND priority&lt;=2 AND updated&gt;7d"
  bd query "(status=open OR status=blocked) AND priority&lt;2"
  bd query "type=bug AND label=urgent"
  bd query "NOT status=closed"
  bd query "assignee=none AND type=task"
  bd query "created&gt;30d AND status!=closed"
  bd query "label=frontend OR label=backend"
  bd query "title=authentication AND priority=0"

```
bd query [expression] [flags]
```

**Flags:**

```
  -a, --all           Include closed issues (default: exclude closed)
  -n, --limit int     Limit results (default: 50, 0 = unlimited) (default 50)
      --long          Show detailed multi-line output for each issue
      --offset int    Not supported: any value above 0 is rejected (page with --limit).
      --parse-only    Only parse the query and show the AST (for debugging)
  -r, --reverse       Invert the sort field's default direction (created/updated/closed default to newest-first, so --sort updated --reverse is oldest-first)
      --sort string   Sort by field: priority, created, updated, closed, status, id, title, type, assignee
```

### bd reclaim

Revert in_progress issues whose lease has gone stale back to ready.

When a worker claims an issue it takes a lease that expires after a TTL, kept
alive by 'bd heartbeat'. A worker that dies stops heartbeating, so its lease
expires and its issue would otherwise stay in_progress forever. reclaim is the
reaper: it finds in_progress issues whose lease expired more than --older-than
ago, clears the assignee, and sets them back to open so another worker can
claim them. The previous owner's stale lease is recorded as a recovery event.

--older-than is a grace window past lease expiry: only leases that expired at
least this long ago are reclaimed, so a worker briefly paused (GC, clock skew)
is not robbed of live work. Run it from a supervisor on a timer with a window
of roughly 2× the claim TTL.

By default reclaim covers every stale lease THIS replica granted. The scope
filters below narrow it further, using the same label surface claiming is
scoped by (--label / --label-any / --exclude-label), plus --assignee and --id.
Filters AND-combine and never widen the set: a reclaimed lease must still be
stale.

Replicas and leases (federated deployments)
-------------------------------------------
A lease is only meaningful on the replica that granted it. Every other
replica's view of the holder's liveness is stale by up to one sync interval,
so a reaper elsewhere can revert a unit that is very much alive over there.
reclaim therefore records the granting replica on each lease and SKIPS a lease
another replica granted, summarizing what it declined on stderr (one line per
run; 'bd -v' expands it to the first 20 leases individually). Reap it where it
was granted; use --any-replica only when that replica is permanently gone (or
when this node was renamed and its own old leases now look foreign — an
ordinary heartbeat keeps a lease alive but does not re-home it to the node
heartbeating it). Prefer the narrow form '--any-replica --id &lt;id&gt;': bare
--any-replica reverts EVERY foreign stale lease, live peers included.

Two invariants the guard cannot enforce for you:

  grace window &gt; sync interval, and lease TTL &gt; sync interval.

A TTL or grace shorter than the cadence at which replicas exchange state is
meaningless across the bridge — the remote view is a full interval old by
construction. Raise the TTL/grace above the sync interval, never the reverse.
The guard is opt-in: set BEADS_NODE_ID, or run 'bd config set node_id &lt;name&gt;'
(which writes the per-machine ~/.config/bd/config.yaml — never commit a node_id
to the git-tracked .beads/config.yaml, or every clone reads the same name and
the guard goes armed-but-inert). One id per STORE, not per host: machines that
are clients of the same dolt sql-server are ONE replica and must share one value
or leave it unset. There is no hostname fallback — the hostname names the client
process's machine, not the store — so an unnamed deployment keeps the old,
unguarded behavior instead of stranding its own work.

Examples:
  bd reclaim                       # default grace window (2× the lease TTL)
  bd reclaim --older-than 10m      # reclaim leases expired &gt;10m ago
  bd reclaim --older-than 0s       # reclaim every currently-expired lease
  bd reclaim --label lane-a        # only this machine's claim partition
  bd reclaim --label-any lane-a,lane-b --exclude-label pinned
  bd reclaim --assignee zelda --assignee epona   # only these workers' leases
  bd reclaim --id wy-abc --id wy-def             # exactly these issues
  bd reclaim --any-replica         # also reap leases granted by a departed replica

```
bd reclaim [flags]
```

**Flags:**

```
      --any-replica             Also reclaim leases granted by ANOTHER replica (unsafe unless that replica is gone; see 'Replicas and leases')
  -a, --assignee strings        Only reclaim leases held by these assignees (repeatable)
      --exclude-label strings   Never reclaim issues carrying ANY of these labels
      --id strings              Only reclaim these issue IDs (repeatable)
  -l, --label strings           Only reclaim issues with ALL these labels (AND). Can combine with --label-any
      --label-any strings       Only reclaim issues with AT LEAST ONE of these labels (OR). Can combine with --label
      --older-than duration     Only reclaim leases that expired at least this long ago (grace window) (default 10m0s)
```

### bd reopen

Reopen closed issues by setting status to 'open' and clearing the closed_at timestamp.
This is more explicit than 'bd update --status open' and emits a Reopened event.

```
bd reopen [id...] [flags]
```

**Flags:**

```
  -r, --reason string   Reason for reopening
```

### bd search

Search issues by words across title, description, notes and ID (all
statuses, including closed).

The query is split on whitespace; every word must appear (case-insensitive
substring) in the title, description or notes, in any order. Quoting a
multi-word argument does not change that. Use --phrase to match the whole
query as one adjacent phrase in the title instead.
ID-like queries (e.g., "bd-123", "hq-319") use fast exact/prefix matching.
Use --desc-contains for a description-only filter.
Use --status open (etc.) to narrow; closed issues are included by default
so "was this already filed/fixed?" cannot silently answer no. Matches
beyond --limit are dropped status-blind, so when hunting live work in a
large DB, narrow with --status open or raise --limit.

Examples:
  bd search "authentication bug"
  bd search "restart nudge"  # both words, any order, title/description/notes
  bd search "overrun guard" --phrase  # adjacent phrase in the title only
  bd search "login" --status open
  bd search "database" --label backend --limit 10
  bd search --query "performance" --assignee alice
  bd search "bd-5q" # Search by partial ID (fast prefix match)
  bd search "security" --priority-min 0 --priority-max 2
  bd search "bug" --created-after 2025-01-01
  bd search "refactor" --status open  # Only open issues
  bd search "bug" --sort priority
  bd search "task" --sort created --reverse
  bd search "api" --desc-contains "endpoint"
  bd search "cleanup" --no-assignee --no-labels

```
bd search [query] [flags]
```

**Flags:**

```
  -a, --assignee string              Filter by assignee
      --closed-after string          Filter issues closed after date (YYYY-MM-DD or RFC3339)
      --closed-before string         Filter issues closed before date (YYYY-MM-DD or RFC3339)
      --created-after string         Filter issues created after date (YYYY-MM-DD or RFC3339)
      --created-before string        Filter issues created before date (YYYY-MM-DD or RFC3339)
      --desc-contains string         Filter by description substring (case-insensitive)
      --empty-description            Filter issues with empty or missing description
      --external-contains string     Filter by external ref substring (case-insensitive)
      --has-metadata-key string      Filter issues that have this metadata key set
  -l, --label strings                Filter by labels (AND: must have ALL)
      --label-any strings            Filter by labels (OR: must have AT LEAST ONE)
  -n, --limit int                    Limit results (default: 50) (default 50)
      --long                         Show detailed multi-line output for each issue
      --metadata-field stringArray   Filter by metadata field (key=value, repeatable)
      --no-assignee                  Filter issues with no assignee
      --no-labels                    Filter issues with no labels
      --notes-contains string        Filter by notes substring (case-insensitive)
      --phrase                       Match the whole query as one adjacent phrase in the title instead of AND-ing its words
      --priority-max string          Filter by maximum priority (inclusive, 0-4 or P0-P4)
      --priority-min string          Filter by minimum priority (inclusive, 0-4 or P0-P4)
      --query string                 Search query (alternative to positional argument)
  -r, --reverse                      Invert the sort field's default direction (created/updated/closed default to newest-first, so --sort updated --reverse is oldest-first)
      --sort string                  Sort by field: priority, created, updated, closed, status, id, title, type, assignee
  -s, --status string                Filter by stored status (comma-separated for OR; open, in_progress, blocked, deferred, closed, all). Default searches all statuses including closed. Note: dependency-blocked issues use 'bd blocked'
  -t, --type string                  Filter by type (bug, feature, task, epic, chore, decision, merge-request, molecule, gate)
      --updated-after string         Filter issues updated after date (YYYY-MM-DD or RFC3339)
      --updated-before string        Filter issues updated before date (YYYY-MM-DD or RFC3339)
```

### bd set-state

Atomically set operational state on an issue.

This command:
1. Creates an event bead recording the state change (source of truth)
2. Removes any existing label for the dimension
3. Adds the new dimension:value label (fast lookup cache)

State labels follow the convention &lt;dimension&gt;:&lt;value&gt;, for example:
  patrol:active, patrol:muted
  mode:normal, mode:degraded
  health:healthy, health:failing

Examples:
  bd set-state agent-abc patrol=muted --reason "Investigating stuck worker"
  bd set-state agent-abc mode=degraded --reason "High error rate detected"
  bd set-state agent-abc health=healthy

The --reason flag provides context for the event bead (recommended).

```
bd set-state <issue-id> <dimension>=<value> [flags]
```

**Flags:**

```
      --reason string   Reason for the state change (recorded in event)
```

### bd show

Show issue details

```
bd show [id...] [--id=<id>...] [--current] [flags]
```

**Aliases:** view

**Flags:**

```
      --as-of string         Show issue as it existed at a specific commit hash or branch (requires Dolt)
      --brief-deps           Reduce each dependency to its identity fields in JSON output (--json only; drops description, design, notes and acceptance criteria)
      --children             Show only the children of this issue
      --current              Show the currently active issue (in-progress, hooked, or last touched)
      --id stringArray       Issue ID (use for IDs that look like flags, e.g., --id=gt--xyz)
      --include-comments     Stream full comment bodies in JSON output (--json only; may be slow on issues with many comments)
      --include-dependents   Stream full dependent issues in JSON output (--json only; may be slow on hub beads)
      --local-time           Show timestamps in local time instead of UTC
      --long                 Show all available fields (extended metadata, agent identity, gate fields, etc.)
      --refs                 Show issues that reference this issue (reverse lookup)
      --short                Show compact one-line output per issue
      --thread               Show full conversation thread (for messages)
  -w, --watch                Watch for changes and auto-refresh display
```

### bd state

Query the current value of a state dimension from an issue's labels.

State labels follow the convention &lt;dimension&gt;:&lt;value&gt;, for example:
  patrol:active
  mode:degraded
  health:healthy

This command extracts the value for a given dimension.

Examples:
  bd state witness-abc patrol     # Output: active
  bd state witness-abc mode       # Output: normal
  bd state witness-abc health     # Output: healthy

```
bd state <issue-id> <dimension>
bd state [command]
```

#### bd state list

List all state labels (dimension:value format) on an issue.

This filters labels to only show those following the state convention.

Example:
  bd state list witness-abc
  # Output:
  #   patrol: active
  #   mode: normal
  #   health: healthy

```
bd state list <issue-id>
```

### bd todo

Manage TODO items as lightweight task issues.

TODOs are regular task-type issues with convenient shortcuts:
  bd todo add "Title"    -&gt; bd create "Title" -t task -p 2
  bd todo                -&gt; bd list --type task --status open
  bd todo done &lt;id&gt;      -&gt; bd close &lt;id&gt;

TODOs can be promoted to full issues by changing type or priority:
  bd update todo-123 --type bug --priority 0

```
bd todo
bd todo [command]
```

#### bd todo add

Add a new TODO item

```
bd todo add <title> [flags]
```

**Flags:**

```
  -d, --description string   Description
  -p, --priority int         Priority (0-4, default 2) (default 2)
```

#### bd todo done

Mark TODO(s) as done

```
bd todo done <id> [<id>...] [flags]
```

**Flags:**

```
      --reason string   Reason for closing (default: Completed)
```

#### bd todo list

List TODO items

```
bd todo list [flags]
```

**Flags:**

```
      --all   Show all TODOs including completed
```

### bd unclaim

Release a claimed issue by clearing the assignee and resetting status to 'open'.

Use this when an agent crashes mid-work or you need to abandon a claimed task.
The issue becomes available for re-claiming by other agents.

Only the current assignee can release its own claim. Releasing another
actor's claim requires --force and should be coordinated with the holder
first — their claim may be live even if the issue looks idle. Prefer
letting lease expiry reclaim genuinely abandoned work.

With --if-assignee, the release is an atomic compare-and-swap (the inverse of
claim): the issue is released only while it is still assigned to the given
assignee. If the holder differs — e.g. the claim was already reclaimed and
re-taken by another worker — nothing is changed and bd exits nonzero with an
error naming the current holder. Use this from supervisors that must return a
specific worker's issue without ever clobbering someone else's live claim.
--if-assignee requires a non-empty assignee and cannot be combined with --force
(they encode contradictory intent).

Exit status: 0 when every issue was released; 1 when any release failed
(including an --if-assignee mismatch).

Examples:
  bd unclaim bd-123
  bd unclaim bd-123 --reason "Agent crashed"
  bd unclaim bd-123 bd-456
  bd unclaim bd-123 --if-assignee worker-7   # only if still held by worker-7

```
bd unclaim [id...] [flags]
```

**Flags:**

```
      --force                Release the claim even if held by a different actor (admin/reaper use)
      --if-assignee string   Only release if still assigned to this assignee (atomic compare-and-swap; exits nonzero without changing the issue when the holder differs)
  -r, --reason string        Reason for unclaiming
```

### bd update

Update one or more issues.

If no issue ID is provided, updates the last touched issue (from most recent
create, update, show, or close operation). This fallback only applies in
interactive sessions (stdin is a terminal); in scripts and agent sessions a
missing ID is an error, so a command built from an empty variable cannot
silently mutate an unrelated issue. Set BD_LAST_TOUCHED_FALLBACK=1 to allow
the fallback anywhere, or =0 to disable it entirely.

Updates are applied per issue ID, not atomically across IDs: when some IDs
fail, the remaining issues are still updated, every failed ID is reported on
stderr, and the command exits nonzero.

Exit codes: 1 for general failures; 13 when every failure is a stale
--if-assignee/--if-status guard (the precondition no longer held, nothing was
written — another actor won the race, so retrying the same guard is
pointless).

```
bd update [id...] [flags]
```

**Flags:**

```
      --acceptance string            Acceptance criteria
      --add-label strings            Add labels (repeatable)
      --allow-empty-description      Allow an empty or whitespace-only description replacement (refused by default)
      --append-notes string          Append to existing notes (with newline separator)
  -a, --assignee string              Assignee
      --await-id string              Set gate await_id (e.g., GitHub run ID for gh:run gates)
      --body-file string             Read description from file (use - for stdin)
      --claim                        Atomically claim the issue (sets assignee to you, status to in_progress; idempotent if already claimed by you; issues assigned to a pool alias listed in the claim.pools config are claimable too)
      --defer string                 Defer until date (empty to clear). Issue hidden from bd ready until then, then auto-wakes to open
  -d, --description string           Issue description
      --design string                Design notes
      --design-file string           Read design from file (use - for stdin)
      --due string                   Due date/time (empty to clear). Formats: +6h, +1d, +2w, tomorrow, next monday, 2025-01-15
      --ephemeral                    Mark issue as ephemeral (wisp) - not exported to JSONL
  -e, --estimate int                 Time estimate in minutes (e.g., 60 for 1 hour)
      --external-ref string          External reference (e.g., 'gh-9', 'jira-ABC', Linear URL)
      --force                        Override two refusals: let -a/--assignee overwrite another actor's live in_progress claim (use only for abandoned claims — crashed agent, expired lease; prefer bd reclaim), and let -s/--status move the issue into closed (or a configured done status) despite open children or a live blocker (same as bd close --force)
      --history                      Clear no-history flag (re-enable Dolt commit history)
      --if-assignee string           Apply the update only if the current assignee equals this value (--if-assignee '' requires unassigned); a mismatch writes nothing and exits 13 (vs 1 for other failures). Requires a field update; cannot combine with --claim
      --if-status string             Apply the update only if the current status equals this value; a mismatch writes nothing and exits 13 (vs 1 for other failures). Requires a field update; cannot combine with --claim
      --metadata string              Set custom metadata (JSON string or @file.json to read from file)
      --no-history                   Mark issue as no-history (skip Dolt commits, not GC-eligible)
      --notes string                 Additional notes (replaces existing notes; use --append-notes to append)
      --parent string                New parent issue ID (reparents the issue, use empty string to remove parent)
      --persistent                   Mark issue as persistent (promote wisp to regular issue)
  -p, --priority string              Priority (0-4 or P0-P4, 0=highest)
      --remove-label strings         Remove labels (repeatable)
      --session string               Claude Code session ID for status=closed (or set CLAUDE_SESSION_ID env var)
      --set-labels strings           Set labels, replacing all existing (repeatable)
      --set-metadata stringArray     Set metadata key=value (repeatable, e.g., --set-metadata team=platform)
      --spec-id string               Link to specification document
  -s, --status string                New status
      --stdin                        Read description from stdin (alias for --body-file -)
      --title string                 New title
  -t, --type string                  New type (bug|feature|task|epic|chore|decision|spike|story|milestone); custom types require types.custom config; aliases: enhancement/feat→feature, dec/adr→decision
      --unset-metadata stringArray   Remove metadata key (repeatable, e.g., --unset-metadata team)
```

## Views & Reports:

### bd count

Count issues matching the specified filters.

By default, returns the total count of issues matching the filters.
Use --by-* flags to group counts by different attributes.

Examples:
  bd count                          # Count all issues
  bd count --status open            # Count open issues
  bd count --by-status              # Group count by status
  bd count --by-priority            # Group count by priority
  bd count --by-type                # Group count by issue type
  bd count --by-assignee            # Group count by assignee
  bd count --by-label               # Group count by label
  bd count --assignee alice --by-status  # Count alice's issues by status
  bd count --include-infra          # Count issues + wisps tier (matches 'bd list --include-infra --all' cardinality)


```
bd count [flags]
```

**Flags:**

```
  -a, --assignee string              Filter by assignee
      --by-assignee                  Group count by assignee
      --by-label                     Group count by label
      --by-priority                  Group count by priority
      --by-status                    Group count by status
      --by-type                      Group count by issue type
      --closed-after string          Filter issues closed after date (YYYY-MM-DD or RFC3339)
      --closed-before string         Filter issues closed before date (YYYY-MM-DD or RFC3339)
      --created-after string         Filter issues created after date (YYYY-MM-DD or RFC3339)
      --created-before string        Filter issues created before date (YYYY-MM-DD or RFC3339)
      --desc-contains string         Filter by description substring
      --empty-description            Filter issues with empty description
      --id string                    Filter by specific issue IDs (comma-separated)
      --include-infra                Include infrastructure beads and the wisps tier (matches 'bd list --include-infra --all' cardinality)
  -l, --label strings                Filter by labels (AND: must have ALL)
      --label-any strings            Filter by labels (OR: must have AT LEAST ONE)
      --metadata-field stringArray   Filter by metadata field (key=value, repeatable)
      --no-assignee                  Filter issues with no assignee
      --no-labels                    Filter issues with no labels
      --notes-contains string        Filter by notes substring
  -p, --priority int                 Filter by priority (0-4: 0=critical, 1=high, 2=medium, 3=low, 4=backlog)
      --priority-max int             Filter by maximum priority (inclusive)
      --priority-min int             Filter by minimum priority (inclusive)
  -s, --status string                Filter by stored status (open, in_progress, blocked, deferred, closed). Note: dependency-blocked issues use 'bd blocked'
      --title string                 Filter by title text (case-insensitive substring match)
      --title-contains string        Filter by title substring
  -t, --type string                  Filter by type (bug, feature, task, epic, chore, decision, merge-request, molecule, gate)
      --updated-after string         Filter issues updated after date (YYYY-MM-DD or RFC3339)
      --updated-before string        Filter issues updated before date (YYYY-MM-DD or RFC3339)
```

### bd history

Show the complete version history of an issue, including all commits
where the issue was modified.

Examples:
  bd history bd-123           # Show all history for issue bd-123
  bd history bd-123 --limit 5 # Show last 5 changes
  bd history bd-123 --events  # Show database audit events

```
bd history <id> [flags]
```

**Flags:**

```
      --events      Show database audit events instead of commit snapshots
      --limit int   Limit number of history entries (0 = all)
```

### bd status

Show a quick snapshot of the issue database state and statistics.

This command provides a summary of issue counts by state (open, in_progress,
blocked, closed), ready work, extended statistics (pinned issues,
average lead time), and recent activity over the last 24 hours from git history.

Similar to how 'git status' shows working tree state, 'bd status' gives you
a quick overview of your issue database without needing multiple queries.

Use cases:
  - Quick project health check
  - Onboarding for new contributors
  - Integration with shell prompts or CI/CD
  - Daily standup reference
  - Fast CI status checks that don't need blocked-count accuracy

Examples:
  bd status                    # Show summary with activity
  bd status --no-activity      # Skip git activity (faster)
  bd status --no-blocked       # Skip slow blocked-count scan (faster)
  bd stats --no-blocked --json # JSON output without blocked count
  bd status --json             # JSON format output
  bd status --assigned         # Show issues assigned to current user
  bd stats                     # Alias for bd status

```
bd status [flags]
```

**Aliases:** stats

**Flags:**

```
      --all           Show all issues (default behavior)
      --assigned      Show issues assigned to current user
      --no-activity   Skip git activity summary (faster)
      --no-blocked    Skip blocked-count computation (faster on large rigs)
```

### bd statuses

List all valid issue statuses and their categories.

Built-in statuses (open, in_progress, blocked, etc.) are always valid.
Additional statuses can be configured via status.custom:

  bd config set status.custom "in_review:active,qa_testing:wip,on_hold:frozen"

Categories control behavior:
  active  — appears in 'bd ready' and default 'bd list'
  wip     — excluded from 'bd ready', visible in default 'bd list'
  done    — excluded from 'bd ready' and default 'bd list'
  frozen  — excluded from 'bd ready' and default 'bd list'

Statuses without a category (legacy format) are valid but excluded from 'bd ready'.

Examples:
  bd statuses            # List all statuses with icons and categories
  bd statuses --json     # Output as JSON


```
bd statuses
```

## Dependencies & Structure:

### bd dep

Manage dependencies between issues.

When called with an issue ID and --blocks flag, creates a blocking dependency:
  bd dep &lt;blocker-id&gt; --blocks &lt;blocked-id&gt;

This is equivalent to:
  bd dep add &lt;blocked-id&gt; &lt;blocker-id&gt;

Examples:
  bd dep bd-xyz --blocks bd-abc    # bd-xyz blocks bd-abc
  bd dep add bd-abc bd-xyz         # Same as above (bd-abc depends on bd-xyz)

```
bd dep [issue-id] [flags]
bd dep [command]
```

**Flags:**

```
  -b, --blocks string    Issue ID that this issue blocks (shorthand for: bd dep add <blocked> <blocker>)
      --no-cycle-check   Skip per-edge cycle checks for speed (bulk wiring); bulk --file adds still run one final whole-graph check before commit
```

#### bd dep add

Add a dependency between two issues.

The depends-on-id can be provided as:
  - A positional argument: bd dep add issue-123 issue-456
  - A flag: bd dep add issue-123 --blocked-by issue-456
  - A flag: bd dep add issue-123 --depends-on issue-456

The --blocked-by and --depends-on flags are aliases and both mean "issue-123
depends on (is blocked by) the specified issue."

The depends-on-id can be:
  - A local issue ID (e.g., bd-xyz)
  - An external reference: external:&lt;project&gt;:&lt;capability&gt;

For bulk wiring, pass newline-delimited JSON with --file. Each line must be an
object with "from" and "to" fields, and may include "type". The aliases
"issue_id" and "depends_on_id" are also accepted. Use --file - to read stdin.

External references are stored as-is and resolved at query time using
the external_projects config. They block the issue until the capability
is "shipped" in the target project.

With no -t/--type the edge is created as type=blocks, which excludes the
dependent from bd ready. When stderr is an interactive terminal, an advisory
note says so once per command; it is silent for scripted and agent callers
(non-TTY stderr) and can be turned off with --quiet or BD_NO_DEP_TYPE_WARNING=1.

Examples:
  bd dep add bd-42 bd-41                              # Positional args
  bd dep add bd-42 --blocked-by bd-41                 # Flag syntax (same effect)
  bd dep add bd-42 --depends-on bd-41                 # Alias (same effect)
  bd dep add gt-xyz external:beads:mol-run-assignee   # Cross-project dependency
  bd dep add bd-42 bd-41 --no-cycle-check             # Skip cycle check (bulk wiring)
  bd dep add --file deps.jsonl                        # Bulk JSONL: &#123;"from":"bd-42","to":"bd-41"&#125;

```
bd dep add [issue-id] [depends-on-id] [flags]
```

**Flags:**

```
      --blocked-by string   Issue ID that blocks the first issue (alternative to positional arg)
      --depends-on string   Issue ID that the first issue depends on (alias for --blocked-by)
      --file string         Read dependency edges from JSONL file, or '-' for stdin
      --no-cycle-check      Skip per-edge cycle checks for speed (bulk wiring); bulk --file adds still run one final whole-graph check before commit
  -t, --type string         Dependency type (blocks|tracks|related|parent-child|discovered-from|until|caused-by|validates|relates-to|supersedes); 'blocked-by' and 'depends-on' are accepted as aliases for 'blocks' (default "blocks")
```

#### bd dep cycles

Detect dependency cycles

```
bd dep cycles
```

#### bd dep list

List dependencies or dependents of one or more issues with optional type filtering.

By default shows dependencies (what issues depend on). Use --direction to control:
  - down: Show dependencies (what this issue depends on) - default
  - up:   Show dependents (what depends on this issue)

Multiple IDs can be provided for batch dep listing. With --json, the output
is a flat array of dependency records across all requested issues.

Use --type to filter by dependency type (e.g., tracks, blocks, parent-child).

Examples:
  bd dep list gt-abc                     # Show what gt-abc depends on
  bd dep list gt-abc gt-def              # Batch: deps for both issues
  bd dep list gt-abc --direction=up      # Show what depends on gt-abc
  bd dep list gt-abc --direction=up -t tracks  # Show what tracks gt-abc (convoy tracking)

```
bd dep list [issue-id...] [flags]
```

**Flags:**

```
      --direction string   Direction: 'down' (dependencies), 'up' (dependents) (default "down")
  -t, --type string        Filter by dependency type (e.g., tracks, blocks, parent-child)
```

#### bd dep prune-orphans

Remove dependency rows whose issue or target no longer exists.

Scans both dependency tables (issues and wisps). A row is an orphan when its
source id, or its issue/wisp target id, names a row in neither plane. External
targets are never orphans. Sources of pruned edges have their blocked state
recomputed in the same transaction.

  bd dep prune-orphans --dry-run   # count only
  bd dep prune-orphans             # delete and report counts

```
bd dep prune-orphans [flags]
```

**Flags:**

```
      --dry-run   Count orphan rows without deleting them
```

#### bd dep relate

Create a loose 'see also' relationship between two issues.

The relates_to link is bidirectional - both issues will reference each other.
This enables knowledge graph connections without blocking or hierarchy.

Examples:
  bd relate bd-abc bd-xyz    # Link two related issues
  bd relate bd-123 bd-456    # Create see-also connection

```
bd dep relate <id1> <id2>
```

#### bd dep remove

Remove a dependency

```
bd dep remove [issue-id] [depends-on-id]
```

**Aliases:** rm

#### bd dep tree

Show dependency tree rooted at the given issue.

By default, shows dependencies (what blocks this issue). Use --direction to control:
  - down: Show dependencies (what blocks this issue) - default
  - up:   Show dependents (what this issue blocks)
  - both: Show full graph in both directions

Examples:
  bd dep tree gt-0iqq                    # Show what blocks gt-0iqq
  bd dep tree gt-0iqq --direction=up     # Show what gt-0iqq blocks
  bd dep tree gt-0iqq --status=open      # Only show open issues
  bd dep tree gt-0iqq --depth=3          # Limit to 3 levels deep

A node reached by two paths is shown ONCE, under the first path that got
there, and a cycle simply ends the descent. --show-all-paths is a deprecated
no-op; use 'bd dep cycles' to find circular dependencies.

--max-rows / BEADS_MAX_ROWS caveat: the tree walk has no query filter to
thread the cap through, so the full tree is always built first and the
node count is checked afterward (post-hoc), not during the walk. The cap is
honored on the --proxied-server route too, which it was not before.

```
bd dep tree [issue-id] [flags]
```

**Flags:**

```
      --direction string   Tree direction: 'down' (dependencies), 'up' (dependents), or 'both'
      --format string      Output format: 'mermaid' for Mermaid.js flowchart
  -d, --max-depth int      Maximum tree depth to display (safety limit) (default 50)
      --max-rows int       Hard upper bound on rows fetched from storage. Returns a non-zero exit (code 2) and an error to stderr if exceeded. 0 disables (the default). Overrides BEADS_MAX_ROWS for this invocation. Useful in CI/agent rigs that want a circuit breaker against pathological queries.
      --reverse            Show dependent tree (deprecated: use --direction=up)
      --show-all-paths     Deprecated no-op: accepted and ignored. A node reached by two paths is shown once, under the first.
      --status string      Filter to only show issues with this status (open, in_progress, blocked, deferred, closed)
```

#### bd dep unrelate

Remove a relates_to relationship between two issues.

Removes the link in both directions.

Example:
  bd unrelate bd-abc bd-xyz

```
bd dep unrelate <id1> <id2>
```

### bd epic

Epic management commands

```
bd epic [command]
```

#### bd epic close-eligible

Close epics where all children are complete

```
bd epic close-eligible [flags]
```

**Flags:**

```
      --dry-run         Preview what would be closed without making changes
  -r, --reason string   Close reason applied to every epic closed (default "All children completed")
```

#### bd epic status

Show epic completion status

```
bd epic status [flags]
```

**Flags:**

```
      --eligible-only   Show only epics eligible for closure
```

### bd supersede

Mark an issue as superseded by a newer version.

The superseded issue is automatically closed with a reference to the replacement.
Useful for design docs, specs, and evolving artifacts.

Examples:
  bd supersede bd-old --with bd-new    # Mark bd-old as superseded by bd-new

```
bd supersede <id> --with <new> [flags]
```

**Flags:**

```
      --with string   Replacement issue ID (required)
```

## Sync & Data:

### bd backup

Back up your beads database for off-machine recovery.

This is a Dolt-native database backup. It preserves the database state,
including tables, branches, commit history, and working-set data. This is
different from 'bd export', which writes issue records to JSONL for migration
and interoperability.

Commands:
  bd backup init &lt;path&gt;    Set up a backup destination (filesystem or DoltHub)
  bd backup sync           Push to configured backup destination
  bd backup restore [path] Restore from a backup directory
  bd backup remove         Remove backup destination
  bd backup status         Show backup status

DoltHub is recommended for cloud backup:
  bd backup init https://doltremoteapi.dolthub.com/&lt;user&gt;/&lt;repo&gt;
  Set DOLT_REMOTE_USER and DOLT_REMOTE_PASSWORD for authentication.

Auto-backup default:
  When backup.enabled is unset, auto-backup turns ON in embedded mode if a
  git remote exists, and stays OFF in sql-server / shared-server mode. In
  server mode many bd clients share one Dolt server, and each would register
  a server-side backup remote under the same name pointing at its own local
  dir and full-sync the whole database — a self-amplifying storm. To back up
  a shared server, run 'bd backup' explicitly (or set backup.enabled=true and
  coordinate destinations). 'bd config get backup.enabled' shows the effective
  value and its source.

```
bd backup [command]
```

#### bd backup init

Configure a filesystem path or URL as a backup destination.

The path can be a local directory (external drive, NAS, Dropbox folder) or a
DoltHub remote URL. If the destination was previously configured, it is
updated to the new path.

Filesystem examples:
  bd backup add /mnt/usb/beads-backup
  bd backup add ~/Dropbox/beads-backup

DoltHub (recommended for cloud backup):
  bd backup add https://doltremoteapi.dolthub.com/myuser/beads-backup

After adding, run 'bd backup sync' to push your data.

```
bd backup init <path>
```

**Aliases:** add

#### bd backup remove

Remove the configured backup destination.

This unregisters the backup remote from Dolt and removes the local
backup configuration. The backup data at the destination is not deleted.

```
bd backup remove
```

**Aliases:** rm

#### bd backup restore

Restore the beads database from a Dolt-native backup.

By default, reads from .beads/backup/ (or the configured backup directory).
Optionally specify a path to a directory containing a Dolt backup.

This restores a full database backup created by 'bd backup sync' or an
equivalent Dolt backup. JSONL files produced by 'bd export' are issue exports,
not restore targets for this command.

Use --force to overwrite an existing database with the backup contents.

The database must already be initialized (run 'bd init' first if needed).
To initialize and restore in one step, use: bd init &amp;&amp; bd backup restore

```
bd backup restore [path] [flags]
```

**Flags:**

```
      --force   Overwrite existing database with backup contents
```

#### bd backup status

Show last backup status

```
bd backup status
```

#### bd backup sync

Sync the current beads database to the configured Dolt backup destination.

This pushes the entire database state (all branches, full history) to the
backup location configured with 'bd backup init'.

The backup is atomic — if the sync fails, the previous backup state is preserved.

Run 'bd backup init &lt;path&gt;' first to configure a destination.

```
bd backup sync
```

### bd branch

List all branches or create a new branch.

This command requires the Dolt storage backend. Without arguments,
it lists all branches. With an argument, it creates a new branch.

Examples:
  bd branch                    # List all branches
  bd branch feature-xyz        # Create a new branch named feature-xyz

```
bd branch [name]
```

### bd export

Export all issues to JSONL (newline-delimited JSON) format.

Each line is a complete JSON object representing one issue, including its
labels, dependencies, and comments.

This command is for issue export, migration, and interoperability. It exports
records from the issues table; it is not a full database backup and does not
capture Dolt branches, commit history, working-set state, or non-issue tables.
For supported full backup/restore flows, use 'bd backup init', 'bd backup sync',
and 'bd backup restore'.

By default, exports only regular issues (excluding infrastructure beads
like agents, roles, and messages). Use --all to include everything.

Memories (from 'bd remember') are excluded by default because they may
contain sensitive agent context. Use --include-memories or --all to
include them.

EXAMPLES:
  bd export                              # Export issues to stdout
  bd export -o issues.jsonl              # Export issues to file
  bd export --include-memories           # Export issues + memories
  bd export --all -o full.jsonl          # Include infra + templates + gates + memories
  bd export --scrub -o clean.jsonl       # Exclude test/pollution records

```
bd export [flags]
```

**Flags:**

```
      --all                         Include all records (infra, templates, gates, memories)
      --exclude-owner stringArray   Exclude issues created by this identity (repeatable; also reads export.exclude_owners config)
      --include-infra               Include infrastructure beads (agents, roles, messages)
      --include-memories            Include persistent memories (from 'bd remember') in the export
  -o, --output string               Output file path (default: stdout)
      --scrub                       Exclude test/pollution records
      --verbose                     Print filtered issue count when owners are excluded
```

### bd import

Import issues from a JSONL file (newline-delimited JSON) into the database.

If no file is specified, imports from the configured import.path under .beads/
(default: issues.jsonl). Use "-" to read from stdin; redirecting stdin without
"-" or a file argument is an error, so a typo'd 'bd import &lt; file' cannot
silently import the default file instead. This is the incremental counterpart to
'bd export': new issues are created and existing issues are updated (upsert
semantics).

Memory records (lines with "_type":"memory") are automatically detected and
imported as persistent memories (equivalent to 'bd remember'). This makes
'bd export | bd import' a full round-trip for both issues and memories.

Each JSONL line should map to an issue. The importer accepts every field
'bd export' emits — see 'bd export' output for the canonical schema. Only
"title" is required; everything else is optional.

Common fields:
  title                  Required. Short summary.
  description            Long-form body.
  design, notes,         Additional content sections.
    acceptance_criteria
  issue_type             bug | feature | task | epic | chore | ...
  priority               0-4 (0 = critical). 0 is preserved (no omitempty).
  status                 open | in_progress | blocked | closed | ...
                         (rows with status "tombstone" are skipped)
  assignee, owner,       Ownership metadata.
    created_by
  labels                 Array of strings.
  dependencies           Array of &#123;issue_id, depends_on_id, type, ...&#125;.
  comments               Array of comment objects.
  external_ref,          Cross-system identifiers (e.g. "gh-9").
    source_system
  due_at, defer_until    RFC3339 timestamps for scheduling.
  metadata               Arbitrary JSON object preserved verbatim.

Timestamps (created_at, updated_at, started_at, closed_at) are preserved
when present in the JSONL and otherwise filled in by the importer. The
legacy "wisp" boolean is accepted as an alias for "ephemeral".

By default a row only rewrites an existing local issue when its
updated_at is strictly newer. Older rows are skipped (reported as
stale_skipped_ids) and rows with the same updated_at keep every local
column — updated_at has second granularity, so a timestamp tie can be
two distinct same-second updates, and the local row wins the tie
(reported as tie_kept_local_ids; the row's labels/comments/dependencies
still merge). The guard is also enforced inside the upsert itself, so a
local update that lands while the import is running is preserved rather
than overwritten. Existing issues that the import did rewrite are listed
with a field-level summary (updated_issues), so local state changed by
an import is visible. To deliberately restore an older snapshot, pass
--allow-stale, which imports every row even when it overwrites newer
local state.

Large imports are written in bounded transactions (a few hundred issues
each, with a short pause between commits) with progress on stderr, so
concurrent bd commands keep working while the import runs instead of
stalling on one batch-wide write lock. Rows land in dependency order
with their blocking edges in the same transaction, so a half-finished
`import never shows a blocked issue as ready. If an import fails partway,`
the already-committed chunks are durable and the command exits nonzero;
re-running the same import is safe and converges (rows upsert,
labels/comments/dependencies deduplicate).

EXAMPLES:
  bd import                        # Import from configured import.path
  bd import backup.jsonl           # Import from a specific file
  bd import -i backup.jsonl        # Legacy alias for a specific file
  bd import -                      # Read JSONL from stdin
  cat issues.jsonl | bd import -   # Pipe JSONL from another tool
  bd import --dry-run              # Show what would be imported
  bd import --dedup                # Skip issues with duplicate titles
  bd import --allow-stale old.jsonl # Restore an older snapshot (overwrites newer local rows)
  bd import --json                 # Structured output with created and skipped IDs

```
bd import [file|-] [flags]
```

**Flags:**

```
      --allow-stale    Import rows even when older than the local issue (required to restore an older snapshot)
      --dedup          Skip lines whose title matches an existing open issue
      --dry-run        Show what would be imported without importing
  -i, --input string   Read JSONL from a specific file
```

### bd vc

Version control operations for the beads database.

These commands provide git-like version control for your issue data, including branching, merging, and
viewing history.

Note: 'bd history', 'bd diff', and 'bd branch' also work for quick access.
This subcommand provides additional operations like merge and commit.

```
bd vc [command]
```

#### bd vc commit

Create a new Dolt commit with all current changes.

Examples:
  bd vc commit -m "Added new feature issues"
  bd vc commit --message "Fixed priority on several issues"
  echo "Multi-line message" | bd vc commit --stdin

```
bd vc commit [flags]
```

**Flags:**

```
  -m, --message string   Commit message
      --stdin            Read commit message from stdin
```

#### bd vc merge

Merge the specified branch into the current branch.

If there are merge conflicts, they will be reported. You can resolve
conflicts with --strategy.

Examples:
  bd vc merge feature-xyz                    # Merge feature-xyz into current branch
  bd vc merge feature-xyz --strategy ours    # Merge, preferring our changes on conflict
  bd vc merge feature-xyz --strategy theirs  # Merge, preferring their changes on conflict

```
bd vc merge <branch> [flags]
```

**Flags:**

```
      --strategy string   Conflict resolution strategy: 'ours' or 'theirs'
```

#### bd vc status

Show the current branch, commit hash, and any uncommitted changes.

Examples:
  bd vc status

```
bd vc status
```

## Setup & Configuration:

### bd bootstrap

Bootstrap sets up the beads database without destroying existing data.
Unlike 'bd init --force', bootstrap will never delete existing issues.

Bootstrap auto-detects the right action:
  • If sync.remote is configured: clones from the remote
  • If git origin has Dolt data (refs/dolt/data): clones from git and wires origin for future push/pull
  • If .beads/backup/*.jsonl exists: restores from backup
  • If .beads/issues.jsonl exists: imports from git-tracked JSONL
  • If no database exists: creates a fresh one
  • If database already exists: validates and reports status

This is the recommended command for:
  • Setting up beads on a fresh clone
  • Recovering after moving to a new machine
  • Repairing a broken database configuration

Non-interactive mode (--non-interactive, --yes/-y, or BD_NON_INTERACTIVE=1):
  Skips the confirmation prompt before executing the bootstrap plan.
  Also auto-detected when stdin is not a terminal or CI=true is set.

Examples:
  bd bootstrap              # Auto-detect and set up
  bd bootstrap --dry-run    # Show what would be done
  bd bootstrap --json       # Output plan as JSON
  bd bootstrap --yes        # Skip confirmation prompt


```
bd bootstrap [flags]
```

**Flags:**

```
      --dry-run           Show what would be done without doing it
      --non-interactive   Alias for --yes
  -y, --yes               Skip confirmation prompts (for CI/automation)
```

### bd config

Manage configuration settings for external integrations and preferences.

Configuration is stored per-project in the beads database and is version-control-friendly.

Common namespaces:
  - export.*          Auto-export settings (stored in config.yaml)
  - import.*          JSONL import settings (stored in config.yaml)
  - jira.*            Jira integration settings
  - linear.*          Linear integration settings
  - github.*          GitHub integration settings
  - gitlab.*          GitLab integration settings
  - ado.*             Azure DevOps integration settings
  - notion.*          Notion integration settings
  - custom.*          Custom integration settings
  - status.*          Issue status configuration
  - claim.*           Claim arbitration settings (pool-aware claiming)
  - doctor.suppress.* Suppress specific bd doctor warnings (GH#1095)
  - lint.*            Additional per-type lint sections (stored in config.yaml)

Auto-Export (config.yaml):
  Optional JSONL export to .beads/issues.jsonl after write commands (throttled).
  Useful for viewers (bv), interchange, and issue-level migration; not a backup.
  It is not cross-machine sync; use bd dolt push/pull with a Dolt remote.
  Disabled by default. Enable only for integrations that need fresh JSONL.
  Auto-staging is separate and disabled by default.

  Keys:
    export.auto       Enable/disable auto-export (default: false)
    export.path       Output filename relative to .beads/ (default: issues.jsonl)
    export.interval   Minimum time between exports (default: 60s)
    export.git-add    Auto-stage the export file (default: false)

Auto-Import (config.yaml):
  Reads .beads/issues.jsonl by default when a JSONL import path is implied.
  Use a relative filename/path so the import stays within the project .beads/
  directory and remains portable across machines.

  Keys:
    import.path       Input filename relative to .beads/ (default: issues.jsonl)

Custom Status States:
  You can define custom status states for multi-step pipelines using the
  status.custom config key. Statuses should be comma-separated.

  Example:
    bd config set status.custom "awaiting_review,awaiting_testing,awaiting_docs"

  This enables issues to use statuses like 'awaiting_review' in addition to
  the built-in statuses (open, in_progress, blocked, deferred, closed).

Claim Pools:
  A dispatcher can pre-assign issues to a pool pseudo-assignee (e.g.
  "fable-crew") and let any actor take them with --claim. List the pool
  aliases in the claim.pools config key, comma-separated:

    bd config set claim.pools "fable-crew,night-crew"

  Issues assigned to a real actor (or to an alias not in the list) keep
  their anti-steal protection. Pool takes carry the normal lease; note
  that if a taker's lease expires, bd reclaim returns the issue to the
  unassigned pool, not to the pool alias it was dispatched to.

Suppressing Doctor Warnings:
  Suppress specific bd doctor warnings by check name slug:
    bd config set doctor.suppress.pending-migrations true
    bd config set doctor.suppress.git-hooks true
  Check names are converted to slugs: "Git Hooks" → "git-hooks".
  Only warnings are suppressed (errors and passing checks always show).
  To unsuppress: bd config unset doctor.suppress.&lt;slug&gt;

Examples:
  bd config set export.auto true                       # Enable auto-export for viewer integrations
  bd config set export.path "beads.jsonl"              # Custom export filename
  bd config set import.path "beads.jsonl"              # Custom import filename
  bd config set export.git-add true                    # Also stage the export file
  bd config set jira.url "https://company.atlassian.net"
  bd config set jira.project "PROJ"
  bd config set status.custom "awaiting_review,awaiting_testing"
  bd config set claim.pools "fable-crew,night-crew"    # Pool aliases claimable by any actor
  bd config set doctor.suppress.pending-migrations true
  bd config set dolt.debug true                        # Enable Dolt sql-server debug mode (loglevel=debug, --prof cpu)
  bd config set dolt.local-only true                   # Skip wiring a Dolt sync remote during bd init
  bd config set lint.sections.epic "Standards scorecard, Cost"   # Extra sections bd lint requires for epics
  bd config get export.auto
  bd config list
  bd config unset jira.url

```
bd config [command]
```

#### bd config apply

Reconcile actual system state to match declared configuration.

Runs drift detection and then fixes any mismatches it finds:

  - hooks     Reinstall git hooks if missing or outdated
  - remote    Add/update Dolt origin remote to match federation.remote
  - server    Start Dolt server if dolt.shared-server is enabled

This command is idempotent — safe to run multiple times. Use --dry-run
to preview what would change without making modifications.

Examples:
  bd config apply
  bd config apply --dry-run
  bd config apply --json

```
bd config apply [flags]
```

**Flags:**

```
      --dry-run   Show what would change without making modifications
```

#### bd config drift

Detect drift between declared configuration and actual system state.

This is a read-only diagnostic that answers "is my environment consistent
with my config?" — no mutations are performed.

Checks:
  - hooks     Git hooks installed and up-to-date
  - remote    Dolt remote matches federation.remote config
  - server    Server state matches dolt.shared-server config

Exit codes:
  0  No drift detected (all checks ok/info/skipped)
  1  Drift detected (at least one check has status "drift")

Examples:
  bd config drift
  bd config drift --json

```
bd config drift
```

#### bd config get

Get a configuration value

```
bd config get <key>
```

#### bd config list

List all configuration

```
bd config list
```

#### bd config set

Set a configuration value

```
bd config set <key> <value> [flags]
```

**Flags:**

```
      --force-git-tracked   Allow writing secret keys to git-tracked config files (use with caution)
```

#### bd config set-many

Set multiple configuration values at once with a single auto-commit and auto-push.

Each argument must be in key=value format. All values are validated before
any writes occur. This is faster and less noisy than separate 'bd config set'
calls, especially in CI.

Examples:
  bd config set-many ado.state_map.open=New ado.state_map.closed=Closed
  bd config set-many jira.url=https://example.atlassian.net jira.project=PROJ

```
bd config set-many <key=value>... [flags]
```

**Flags:**

```
      --force-git-tracked   Allow writing secret keys to git-tracked config files (use with caution)
```

#### bd config show

Display a unified view of all effective configuration across all sources
with annotations showing where each value comes from.

Sources (by precedence for Viper-managed keys):
  - env          Environment variable (BD_* or BEADS_*)
  - config.yaml  Project config file (.beads/config.yaml)
  - default      Built-in default value

Additional sources:
  - metadata     Connection settings from .beads/metadata.json
  - database     Integration config stored in the Dolt database
  - git          Git config (e.g., beads.role)

Examples:
  bd config show
  bd config show --json
  bd config show --source config.yaml

```
bd config show [flags]
```

**Flags:**

```
      --source string   Filter by source (e.g., config.yaml, env, default, metadata, database, git)
```

#### bd config unset

Delete a configuration value

```
bd config unset <key>
```

#### bd config validate

Validate sync-related configuration settings.

Checks:
  - federation.sovereignty is valid (T1, T2, T3, T4, or empty)
  - federation.remote is set for Dolt sync
  - Remote URL format is valid (dolthub://, gs://, s3://, az://, file://)
  - routing.mode is valid (auto, maintainer, contributor, explicit)

	Examples:
	  bd config validate
	  bd config validate --json

```
bd config validate
```

### bd context

Show the effective backend identity information including repository paths,
backend configuration, and sync settings.

This command reads directly from config files and does not require the
database to be open, making it useful for diagnostics in degraded states.

Examples:
  bd context           # Show context information
  bd context --json    # Output in JSON format


```
bd context
```

### bd dolt

Configure and manage Dolt database settings and server lifecycle.

Beads runs Dolt embedded (in-process) by default: there is no sql-server and
nothing is auto-started. A database only uses a dolt sql-server when it is
configured for one: shared-server mode, an explicit server mode, or a
non-localhost dolt_server_host. The server-only commands below fail with
"not supported in embedded mode (no Dolt server)" on an embedded database.

Server lifecycle (server mode only):
  bd dolt start        Start the Dolt server for this project
  bd dolt stop         Stop the Dolt server for this project

Diagnostics (both modes):
  bd dolt status       Show Dolt engine status (embedded: in-process, data dir)
  bd dolt show         Show current Dolt configuration with connection test

Configuration (server mode only):
  bd dolt set &lt;k&gt; &lt;v&gt;  Set a configuration value
  bd dolt test         Test server connection

Version control:
  bd dolt commit       Commit pending changes
  bd dolt push         Push commits to Dolt remote
  bd dolt pull         Pull commits from Dolt remote

Remote management:
  bd dolt remote add &lt;name&gt; &lt;url&gt;   Add a Dolt remote
  bd dolt remote list                List configured remotes
  bd dolt remote remove &lt;name&gt;       Remove a Dolt remote

Configuration keys for 'bd dolt set':
  database  Database name (default: issue prefix or "beads")
  host      Server host (default: 127.0.0.1)
  port      Server port (auto-detected; override with bd dolt set port &lt;N&gt;)
  user      MySQL user (default: root)
  data-dir  Custom dolt data directory (absolute path; default: .beads/dolt)

Remote server authentication (password + TLS) is NOT stored via 'bd dolt set'
(keeps secrets out of metadata.json). Configure them with:

  BEADS_DOLT_PASSWORD       Server password (highest priority)
  BEADS_DOLT_SERVER_TLS     Enable TLS (set to "1" or "true")
  BEADS_DOLT_SERVER_USER    MySQL user override (else use 'bd dolt set user')
  BEADS_CREDENTIALS_FILE    Optional path to credentials file

  Default credentials file: ~/.config/beads/credentials (Linux/macOS)
                            %APPDATA%\beads\credentials (Windows)
  Format (INI, section = host:port of the resolved connection):
    [127.0.0.1:3307]
    password = secret

  Password resolution: BEADS_DOLT_PASSWORD → credentials [host:port] → empty.
  Full reference: docs/architecture/dolt.md (Environment Variables / Credentials).

Flags for 'bd dolt set':
  --update-config  Also write to config.yaml for team-wide defaults

Examples:
  bd dolt set database myproject
  bd dolt set host 192.168.1.100 --update-config
  bd dolt set data-dir /home/user/.beads-dolt/myproject
  `export BEADS_DOLT_PASSWORD=... BEADS_DOLT_SERVER_TLS=1`
  bd dolt test

```
bd dolt [command]
```

#### bd dolt clean-databases

Identify and drop leftover test and agent databases that accumulate
on the shared Dolt server from interrupted test runs and terminated agents.

Stale database prefixes: testdb_*, beads_test*, beads_pt*, beads_vr*, doctest_*, doctortest_*, benchdb_*

These waste server memory and can degrade performance under concurrent load.
Use --dry-run to see what would be dropped without actually dropping.

DROP DATABASE only marks a database as dropped; Dolt keeps its directory
under .dolt_dropped_databases/ so it can be restored with
CALL DOLT_UNDROP(name) until an explicit purge — disk is not reclaimed
until then. Pass --purge-dropped to run CALL DOLT_PURGE_DROPPED_DATABASES()
after cleanup.

--purge-dropped is SERVER-GLOBAL and IRREVERSIBLE. Dolt has no way to scope
the purge to only the databases this run dropped: it permanently deletes
every dropped-but-not-yet-purged database on the server, including ones
dropped by something else entirely (e.g. an operator's accidental
DROP DATABASE on an unrelated database that was still recoverable via
DOLT_UNDROP). It also purges pre-existing residue from earlier
clean-databases runs even if this run finds no stale databases to drop.
Only pass it when nothing else on the server may be relying on DOLT_UNDROP
recovery.

```
bd dolt clean-databases [flags]
```

**Flags:**

```
      --dry-run         Show what would be dropped without dropping
      --purge-dropped   After dropping, also run CALL DOLT_PURGE_DROPPED_DATABASES() — server-global and irreversible, see --help
```

#### bd dolt commit

Create a Dolt commit from any uncommitted changes in the working set.

This is the primary commit point for batch mode. When auto-commit is set to
"batch", changes accumulate in the working set across multiple bd commands and
are committed together here with a descriptive summary message.

Also useful before push operations that require a clean working set, or when
auto-commit was off or changes were made externally.

For more options (--stdin, custom messages), see: bd vc commit

```
bd dolt commit [flags]
```

**Flags:**

```
  -m, --message string   Commit message (default: auto-generated)
```

#### bd dolt killall

Find and kill orphan dolt sql-server processes not tracked by the
canonical PID file for the current repo's Dolt data directory.

Under an orchestrator, the canonical server lives at $GT_TOWN_ROOT/.beads/. Any other
dolt sql-server processes using that shared data directory are considered
orphans and will be killed.

In standalone mode, only dolt sql-server processes using the current
project's Dolt data directory are eligible for cleanup. Other projects'
servers are preserved.

```
bd dolt killall
```

#### bd dolt pull

Pull commits from the configured Dolt remote into the local database.

Requires a Dolt remote to be configured in the database directory.
For Hosted Dolt, set DOLT_REMOTE_USER and DOLT_REMOTE_PASSWORD environment
variables for authentication.

Use --remote to pull from a specific named remote instead of the default.
The remote must already exist (see 'bd dolt remote add').

Use --strategy ours|theirs to resolve conflicts the auto-resolver declines
(e.g. both sides edited the same issue since the last sync) instead of
aborting the pull for manual resolution. Embedded storage only (#4992); on
server-mode/sql-server storage use 'bd conflicts resolve' after a pull that
reports conflicts.

```
bd dolt pull [flags]
```

**Flags:**

```
      --remote string     Pull from a specific named remote instead of the default
      --strategy string   Conflict resolution strategy for conflicts the auto-resolver declines: 'ours' or 'theirs' (embedded storage only, #4992)
```

#### bd dolt push

Push local Dolt commits to the configured remote.

Requires a Dolt remote to be configured in the database directory. With no
remote configured, bd can adopt one derived from git origin — only with
consent: interactively, or via --yes; --no-adopt or BD_NO_REMOTE_ADOPT=1
disables adoption entirely.
For Hosted Dolt, set DOLT_REMOTE_USER and DOLT_REMOTE_PASSWORD environment
variables for authentication.

Use --force to overwrite remote changes (e.g., when the remote has
uncommitted changes in its working set).

Use --remote to push to a specific named remote instead of the default.
The remote must already exist (see 'bd dolt remote add').

```
bd dolt push [flags]
```

**Flags:**

```
      --force           Force push (overwrite remote changes)
      --no-adopt        Never derive a Dolt remote from git origin (also BD_NO_REMOTE_ADOPT=1)
      --remote string   Push to a specific named remote instead of the default
  -y, --yes             Consent to adopting a Dolt remote derived from git origin when none is configured
```

#### bd dolt remote

Manage Dolt remotes for push/pull replication.

Subcommands:
  add &lt;name&gt; &lt;url&gt;     Add a new remote
  list                 List all configured remotes
  remove &lt;name&gt;        Remove a remote
  reset-data &lt;name&gt;    Replace a remote's data plane after a history squash

```
bd dolt remote [command]
```

##### bd dolt remote add

Add a Dolt remote

```
bd dolt remote add <name> <url> [flags]
```

**Flags:**

```
      --allow-git-origin   Allow adding a Dolt remote whose URL matches the git origin (proceed with a warning instead of aborting)
```

##### bd dolt remote list

List configured Dolt remotes

```
bd dolt remote list
```

##### bd dolt remote remove

Remove a Dolt remote

```
bd dolt remote remove <name>
```

##### bd dolt remote reset-data

Replace a Dolt remote's stored data with a fresh copy of local HEAD.

After a history squash (see the History Bloat recovery runbook), a plain
'bd dolt push --force' re-points the remote's refs but deletes nothing:
Dolt remotes accumulate chunks monotonically, so the remote keeps the full
pre-squash store. This command rebuilds the remote's data plane so it holds
only live chunks:

  - Git-backed remotes (issue data riding a git remote under refs/dolt/data):
    deletes the Dolt data refs on the git remote, then force-pushes to
    rebuild a fresh store. Code branches are untouched.
  - Native file remotes (file:// paths): clears the store directory, then
    force-pushes to rebuild it.
  - Cloud/hosted remotes (aws://, gs://, dolthub://, ...): bd cannot clear
    the stored data safely — replace the remote with a fresh URL or prefix:
      bd dolt remote remove &lt;name&gt;
      bd dolt remote add &lt;name&gt; &lt;fresh-url&gt;
      bd dolt push --force

This rewrites the remote's data plane. Every other clone must re-clone from
the reset remote (that is already true after the squash itself). Refuses to
run with uncommitted working-set changes: the rebuilt remote holds exactly
HEAD, and anything uncommitted would not be part of it.

Examples:
  bd dolt remote reset-data origin          # prompts for confirmation
  bd dolt remote reset-data origin --yes    # no prompt (scripts, agents)

```
bd dolt remote reset-data <name> [flags]
```

**Flags:**

```
  -y, --yes   Skip the confirmation prompt (required in non-interactive use)
```

#### bd dolt set

Set a Dolt configuration value in metadata.json.

Keys:
  database  Database name (default: issue prefix or "beads")
  host      Server host (default: 127.0.0.1)
  port      Server port (auto-detected; override with bd dolt set port &lt;N&gt;)
  user      MySQL user (default: root)
  data-dir  Custom dolt data directory (absolute path; default: .beads/dolt)

There is no 'password' or 'tls' key here on purpose — secrets and TLS must
not land in metadata.json. Use environment variables or the credentials file:

  BEADS_DOLT_PASSWORD     Server password (highest priority)
  BEADS_DOLT_SERVER_TLS   Enable TLS ("1" or "true")
  BEADS_CREDENTIALS_FILE  Optional override path for credentials

  Default credentials file: ~/.config/beads/credentials
  Format:
    [host:port]
    password = secret

  See: bd dolt --help and docs/architecture/dolt.md

Use --update-config to also write to config.yaml for team-wide defaults.

Examples:
  bd dolt set database myproject
  bd dolt set host 192.168.1.100
  bd dolt set port 3307 --update-config
  bd dolt set data-dir /home/user/.beads-dolt/myproject
  `export BEADS_DOLT_PASSWORD=... BEADS_DOLT_SERVER_TLS=1`

```
bd dolt set <key> <value> [flags]
```

**Flags:**

```
      --update-config   Also write to config.yaml for team-wide defaults
```

#### bd dolt show

Show current Dolt configuration with connection status

```
bd dolt show
```

#### bd dolt start

Start a dolt sql-server for the current beads project.

The server runs in the background on a per-project port derived from the
project path. PID and logs are stored in .beads/.

The server auto-starts transparently when needed, so manual start is rarely
required. Use this command for explicit control or diagnostics.

```
bd dolt start
```

#### bd dolt status

Show the status of the Dolt engine for the current project.

In embedded mode, reports that the Dolt engine runs in-process and shows
the on-disk data directory. For beads-managed (local) servers, displays
PID, port, and data directory from the local PID file. For externally-
managed servers — a shared server (dolt.shared-server: true), a remote
dolt_server_host, or a local server managed outside bd (dolt.auto-start:
false, e.g. an orchestrator-shared sql-server) — pings the configured
endpoint via SQL and reports reachability, server version, and database.

```
bd dolt status
```

#### bd dolt stop

Stop the dolt sql-server managed by beads for the current project.

This sends a graceful shutdown signal. The server will restart automatically
on the next bd command unless auto-start is disabled.

For a managed proxied server, --force can recover unverifiable or legacy
process records (both the proxy and its backend) only after each live process
executable is matched to bd or dolt and its command line ties it to this
workspace. In that recovery path, force still refuses to signal a process
whose executable identity cannot be matched to bd or dolt, or whose workspace
scope cannot be established.

```
bd dolt stop [flags]
```

**Flags:**

```
      --force   Force stop (proxied recovery still requires a bd/dolt executable match)
```

#### bd dolt test

Test the connection to the configured Dolt server.

This verifies that:
  1. The server is reachable at the configured host:port
  2. The connection can be established

Use this before switching to server mode to ensure the server is running.

```
bd dolt test
```

### bd forget

Remove a memory by its key.

Use 'bd memories' to see available keys.

Examples:
  bd forget dolt-phantoms
  bd forget auth-jwt

```
bd forget <key>
```

### bd hooks

Install, uninstall, or list git hooks for beads integration.

The hooks provide:
- pre-commit: Run chained hooks before commit
- post-merge: Run chained hooks after pull/merge
- pre-push: Run chained hooks before push
- post-checkout: Run chained hooks after branch checkout
- prepare-commit-msg: Add agent identity trailers for forensics

```
bd hooks [command]
```

#### bd hooks install

Install git hooks for beads integration.

By default, hooks are installed to .git/hooks/ in the current repository.
Use --beads to install to .beads/hooks/ (recommended for Dolt backend).
Use --shared to install to a versioned directory (.beads-hooks/) that can be
committed to git and shared with team members.

Hooks use section markers to coexist with existing hooks — any user content
outside the markers is preserved across installs and upgrades.

Installed hooks:
  - pre-commit: Run chained hooks before commit
  - post-merge: Run chained hooks after pull/merge
  - pre-push: Run chained hooks before push
  - post-checkout: Run chained hooks after branch checkout
  - prepare-commit-msg: Add agent identity trailers (for orchestrator agents)

```
bd hooks install [flags]
```

**Flags:**

```
      --beads    Install hooks to .beads/hooks/ (recommended for Dolt backend)
      --chain    No-op, kept for compatibility (existing hook content always runs alongside the bd section)
      --force    No-op, kept for compatibility (section markers always preserve non-bd content)
      --shared   Install hooks to .beads-hooks/ (versioned) instead of .git/hooks/
```

#### bd hooks list

Show the status of bd git hooks (installed, outdated, missing).

```
bd hooks list
```

#### bd hooks run

Execute the logic for a git hook. This command is typically called by
thin shim scripts installed in .git/hooks/.

Supported hooks:
  - pre-commit: Run chained hooks before commit
  - post-merge: Run chained hooks after pull/merge
  - pre-push: Run chained hooks before push
  - post-checkout: Run chained hooks after branch checkout
  - prepare-commit-msg: Add agent identity trailers for forensics

The thin shim keeps delegated hook logic in sync with the installed bd
version. Upgrading bd updates that delegated behavior. To adopt changes to the
shim's generated shell policy, refresh it with 'bd hooks install'.

```
bd hooks run <hook-name> [args...]
```

#### bd hooks uninstall

Remove bd git hooks from .git/hooks/ directory.

```
bd hooks uninstall
```

### bd human

Display a focused help menu showing only the most common commands.

bd has 70+ commands - many for AI agents, integrations, and advanced workflows.
This command shows the ~15 essential commands that human users need most often.

For the full command list, run: bd --help

SUBCOMMANDS:
  human list              List human-needed beads (issues with 'human' label; hides closed by default)
  human respond &lt;id&gt;      Respond to a human-needed bead (adds comment and closes)
  human dismiss &lt;id&gt;      Dismiss a human-needed bead permanently
  human stats             Show summary statistics for human-needed beads

```
bd human
bd human [command]
```

#### bd human dismiss

Dismiss a human-needed bead permanently without responding.

The issue is closed with a "Dismissed" reason and optional note.
The reason can be given as positional arguments or --reason.

Examples:
  bd human dismiss bd-123
  bd human dismiss bd-123 "No longer applicable"
  bd human dismiss bd-123 --reason "No longer applicable"

```
bd human dismiss <issue-id> [reason...] [flags]
```

**Flags:**

```
      --reason string   Reason for dismissal (optional)
```

#### bd human list

List issues labeled with 'human' tag.

These are issues that require human intervention or input. Every
human-labeled bead shows regardless of type (including gates and wisps).
By default closed, pinned, and other done/frozen beads are hidden; use
--status to select specific statuses, or --status=all to include every
status.

Examples:
  bd human list
  bd human list --status=closed
  bd human list --status=all
  bd human list --json

```
bd human list [flags]
```

**Flags:**

```
  -s, --status string   Filter by status (open, closed, etc.; comma-separated for multiple, 'all' for every status)
```

#### bd human respond

Respond to a human-needed bead by adding a comment and closing it.

The response is added as a comment and the issue is closed with reason "Responded".
The response text can be given as positional arguments, --response, --file, or --stdin.

Examples:
  bd human respond bd-123 "Use OAuth2 for authentication"
  bd human respond bd-123 -r "Approved, proceed with implementation"
  bd human respond bd-123 --file response.md
  echo "Approved" | bd human respond bd-123 --stdin

```
bd human respond <issue-id> [response...] [flags]
```

**Flags:**

```
      --file string       Read response text from file
  -r, --response string   Response text
      --stdin             Read response text from stdin
```

#### bd human stats

Display summary statistics for human-needed beads.

Shows counts for total, pending (open), responded (closed without dismiss),
and dismissed beads.

Example:
  bd human stats

```
bd human stats
```

### bd info

Display information about the current database.

This command helps debug issues where bd is using an unexpected database. It shows:
  - The absolute path to the database file
  - Database statistics (issue count)
  - Schema information (with --schema flag)
  - What's new in recent versions (with --whats-new flag)

Examples:
  bd info
  bd info --json
  bd info --schema --json
  bd info --whats-new
  bd info --whats-new --json
  bd info --thanks

```
bd info [flags]
```

**Flags:**

```
      --schema      Include schema information in output
      --thanks      Show thank you page for contributors
      --whats-new   Show agent-relevant changes from recent versions
```

### bd init

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

### bd kv

Commands for working with the beads key-value store.

The key-value store is useful for storing flags, environment variables,
or other user-defined data that persists across sessions.

Examples:
  bd kv set mykey myvalue    # Set a value
  bd kv get mykey            # Get a value
  bd kv clear mykey          # Delete a key
  bd kv list                 # List all key-value pairs

```
bd kv [command]
```

#### bd kv clear

Delete a key from the beads key-value store.

Examples:
  bd kv clear feature_flag
  bd kv clear api_endpoint

```
bd kv clear <key>
```

#### bd kv get

Get a value from the beads key-value store.

Examples:
  bd kv get feature_flag
  bd kv get api_endpoint

```
bd kv get <key>
```

#### bd kv list

List all key-value pairs in the beads key-value store.

Examples:
  bd kv list
  bd kv list --json

```
bd kv list
```

#### bd kv set

Set a key-value pair in the beads key-value store.

This is useful for storing flags, environment variables, or other
user-defined data that persists across sessions.

Examples:
  bd kv set feature_flag true
  bd kv set api_endpoint https://api.example.com
  bd kv set max_retries 3

```
bd kv set <key> <value>
```

### bd memories

List all memories, or search by keyword.

Examples:
  bd memories              # list all memories
  bd memories dolt         # search for memories about dolt
  bd memories "race flag"  # search for a phrase

```
bd memories [search]
```

### bd migrate-personal

Identify issues you created in the project database and move them to your
personal planning repository (~/.beads-planning by default).

This is a one-time migration for contributors who created personal planning
issues before contributor routing was configured.

The command:
  1. Finds all issues in the project database created by your git identity
  2. Shows you the list and asks for confirmation
  3. Moves them to the planning repo configured in routing.contributor

EXAMPLES:
  bd migrate-personal        # Interactive: show list and prompt
  bd migrate-personal -y     # Non-interactive: skip confirmation

```
bd migrate-personal [flags]
```

**Flags:**

```
  -y, --yes   Skip confirmation prompt
```

### bd onboard

Display a minimal snippet to add to your agent instructions file for bd integration.

By default, the agent instructions file is AGENTS.md. Use 'bd init --agents-file'
to configure a different filename (e.g. BEADS.md).

This outputs a small (~10 line) snippet that points to 'bd prime' for full
workflow context. This is the same minimal profile that 'bd init' generates
by default. This approach:

  • Keeps your agent file lean (doesn't bloat with instructions)
  • bd prime provides dynamic, always-current workflow details
  • Hooks auto-inject bd prime at session start

For agents or environments that do not auto-inject hook output, use
'bd init --agents-profile=full' to embed the complete command reference.

```
bd onboard
```

### bd prime

Output essential Beads workflow context in AI-optimized markdown format.

Automatically detects if MCP server is active and adapts output:
- MCP mode: Brief workflow reminders (~50 tokens)
- CLI mode: Full command reference (~1-2k tokens)

Designed for Claude Code, Gemini CLI, and Codex SessionStart hooks to prevent
agents from forgetting bd workflow after context compaction.

Config options:
- no-git-ops: When true, outputs stealth mode (no git commands in session close protocol).
  Set via: bd config set no-git-ops true
  Useful when you want to control when commits happen manually.
- agent.profile: Explicit policy profile for git/commit authority wording
  (conservative | minimal | team-maintainer; default conservative).
  Set via: bd config set agent.profile team-maintainer
  Or per-session: BD_AGENT_PROFILE=team-maintainer (env var takes precedence).
  See docs/getting-started/ide-setup.md#policy-profiles for what each profile means.

	Workflow customization:
	- Place a .beads/PRIME.md file in the local clone or resolved workspace to override the default workflow text. Persistent memories (from bd remember) are still appended so memory injection keeps working under a custom template.
	- Use --export to dump the default content for customization.
	- Use --memories-only for hook contexts that should inject only persistent memories; this returns only the memories section even when a custom PRIME.md is present.
	- Use --no-memories to omit the persistent memories section (useful when the memories section is large and would dominate a context budget). --memories-only takes precedence if both are set.

Memory injection caps:
	Large memory sets can exceed what a session-start hook host will ingest,
	and hosts truncate silently. Cap what prime injects with --max-memories N
	and/or --max-memory-chars N (or the prime.max-memories /
	prime.max-memory-chars config keys; an explicit flag wins, and an explicit
	0 forces unlimited). Caps apply at whole-memory boundaries, at least one
	memory is always emitted, and a banner ahead of the entries reports how
	many were elided and how to browse the rest with bd memories.
	--max-memory-chars caps the total bytes of the injected memory entries;
	the section header and elision banner are excluded from the budget.

```
bd prime [flags]
```

**Flags:**

```
      --export                 Output default content (ignores PRIME.md override)
      --full                   Force full CLI output (ignore MCP detection)
      --hook-json              Wrap output in the SessionStart hook JSON envelope (Claude Code, Gemini CLI, Codex)
      --max-memories int       Cap injected persistent memories to N entries (0 = unlimited; falls back to the prime.max-memories config key)
      --max-memory-chars int   Cap the total bytes of injected memory entries, at whole-memory boundaries; section header and banner are not counted (0 = unlimited; falls back to the prime.max-memory-chars config key)
      --mcp                    Force MCP mode (minimal output)
      --memories-only          Output only persistent memories for compact hook contexts
      --no-memories            Omit the persistent memories section (ignored when --memories-only is set, which wins)
      --stealth                Stealth mode (no git operations, flush only)
```

### bd quickstart

Display a quick start guide showing common bd workflows and patterns.

```
bd quickstart
```

### bd recall

Retrieve the full content of a memory by its key.

Examples:
  bd recall dolt-phantoms
  bd recall auth-jwt

```
bd recall <key>
```

### bd remember

Store a memory that persists across sessions and account rotations.

Memories are injected at prime time (bd prime) so you have them
in every session without manual loading.

The positional arg is the memory CONTENT (the key is auto-generated from it
unless --key is given). As a convenience, if the arg is a bare key naming an
existing memory, it is RECALLED instead of stored (same as 'bd recall');
a bare key naming nothing is refused. Use --key to store slug-like content.

Examples:
  bd remember "always run tests with -race flag"
  bd remember "Dolt phantom DBs hide in three places" --key dolt-phantoms
  bd remember "auth module uses JWT not sessions" --key auth-jwt
  bd remember dolt-phantoms        # bare existing key: reads it (= bd recall)

```
bd remember "<insight>" [flags]
```

**Flags:**

```
      --key string   Explicit key for the memory (auto-generated from content if not set). If a memory with this key already exists, it will be updated in place
```

### bd where

Show the active beads database location, including redirect information.

	This command is useful for debugging when using redirects, to understand
	which beads workspace is actually being used.

Examples:
  bd where           # Show active beads location
  bd where --json    # Output in JSON format


```
bd where
```

## Maintenance:

### bd doctor

Sanity check the beads installation for the current directory or specified path.

This command checks:
  - If .beads/ directory exists
  - Database version and migration status
  - Schema compatibility (all required tables and columns present)
  - Whether using hash-based vs sequential IDs
  - If CLI version is current (checks GitHub releases)
  - If Claude plugin is current (when running in Claude Code)
  - File permissions
  - Circular dependencies
  - Git hooks (pre-commit, post-merge, pre-push)
  - .beads/.gitignore up to date
  - Metadata.json version tracking (LastBdVersion field)

Storage Availability:
  Full diagnostics, --perf, --deep, --server, --migration, and
  --check=validate currently require Dolt server mode. Embedded Dolt
  supports --check=artifacts, --check=conventions, and
  --check=pollution. --check-health has a limited hook-health fallback.
  Unsupported combinations return a notice without changing storage.

Performance Mode (--perf):
  Run performance diagnostics on your database:
  - Times key operations (bd ready, bd list, bd show, etc.)
  - Collects system info (OS, arch, database stats)
  - Generates CPU profile for analysis
  - Outputs shareable report for bug reports

Export Mode (--output):
  Save diagnostics to a JSON file for historical analysis and bug reporting.
  Includes timestamp and platform info for tracking intermittent issues.

Specific Check Mode (--check):
  Run a specific check in detail. Available checks:
  - artifacts: Detect and optionally clean beads classic artifacts
    (stale JSONL, SQLite files, cruft .beads dirs). Use with --clean.
  - conventions: Check for convention drift (lint warnings, stale
    issues, orphaned issues). Advisory only - warns, never blocks.
  - pollution: Detect and optionally clean test issues from database
  - validate: Run focused data-integrity checks (duplicates, orphaned
    deps, test pollution, git conflicts). Use with --fix to auto-repair.

Deep Validation Mode (--deep):
  Validate full graph integrity. May be slow on large databases.
  Additional checks:
  - Parent consistency: All parent-child deps point to existing issues
  - Dependency integrity: All deps reference valid issues
  - Epic completeness: Find epics ready to close (all children closed)
  - Agent bead integrity: Agent beads have valid state values
  - Mail thread integrity: Thread IDs reference existing issues
  - Molecule integrity: Molecules have valid parent-child structures

Server Mode (--server):
  Run health checks for Dolt server mode connections (bd-dolt.2.3):
  - Server reachable: Can connect to configured host:port?
  - Dolt version: Is it a Dolt server (not vanilla MySQL)?
  - Database exists: Does the 'beads' database exist?
  - Schema compatible: Can query beads tables?
  - Connection pool: Pool health metrics

Legacy Dolt Migration Validation Mode (--migration):
  Retained for older SQLite-to-Dolt migration workflows and available only in
  Dolt server mode. It is not the migration path for removed backends
  (PostgreSQL, MySQL, SQLite); those fail closed with export/import guidance.
  Combine
  with --json for machine-parseable diagnostic output.

Agent Mode (--agent):
  Output diagnostics designed for AI agent consumption. Instead of terse
  pass/fail messages, each issue includes:
  - Observed state: what the system actually looks like
  - Expected state: what it should look like
  - Explanation: full prose context about the issue and why it matters
  - Commands: exact remediation commands to run
  - Source files: where in the codebase to investigate further
  - Severity: blocking (prevents operation), degraded (partial function),
    or advisory (informational only)
  ZFC-compliant: Go observes and reports, the agent decides and acts.
  Combine with --json for structured agent-facing output.

Suppressing Warnings:
  Suppress specific warnings by setting doctor.suppress.&lt;check-slug&gt; config:
    bd config set doctor.suppress.pending-migrations true
    bd config set doctor.suppress.git-hooks true
  Check names are converted to slugs: "Git Hooks" → "git-hooks".
  Only warnings are suppressed; errors and passing checks always show.
  To unsuppress: bd config unset doctor.suppress.&lt;slug&gt;

Examples:
  bd doctor              # Check current directory
  bd doctor /path/to/repo # Check specific repository
  bd doctor --json       # Machine-readable output
  bd doctor --agent      # Agent-facing diagnostic output
  bd doctor --agent --json  # Structured agent diagnostics (JSON)
  bd doctor --fix        # Automatically fix issues (with confirmation)
  bd doctor --fix --yes  # Automatically fix issues (no confirmation)
  bd doctor --fix -i     # Confirm each fix individually
  bd doctor --fix --fix-child-parent  # Also fix child→parent deps (opt-in)
  bd doctor --fix --force # Force repair even when database can't be opened
  bd doctor --fix --source=jsonl # Rebuild database from a JSONL export
  bd doctor --dry-run    # Preview what --fix would do without making changes
  bd doctor --perf       # Performance diagnostics
  bd doctor --output diagnostics.json  # Export diagnostics to file
  bd doctor --check=artifacts           # Show classic artifacts (JSONL, SQLite, cruft dirs)
  bd doctor --check=artifacts --clean  # Delete safe-to-delete artifacts (with confirmation)
  bd doctor --check=conventions        # Convention drift check (lint, stale, orphans)
  bd doctor --check=pollution          # Show potential test issues
  bd doctor --check=pollution --clean  # Delete test issues (with confirmation)
  bd doctor --check=validate         # Data-integrity checks only
  bd doctor --check=validate --fix   # Auto-fix data-integrity issues
  bd doctor --deep             # Full graph integrity validation
  bd doctor --server           # Dolt server mode health checks
  bd doctor --migration=pre    # Legacy Dolt-server migration diagnostic
  bd doctor --migration=post   # Legacy Dolt-server completion diagnostic
  bd doctor --migration=pre --json  # Machine-parseable legacy diagnostic

```
bd doctor [path] [flags]
```

**Flags:**

```
      --agent                                   Agent-facing diagnostic mode: rich context for AI agents (ZFC-compliant)
      --check string                            Run specific check in detail (e.g., 'pollution')
      --check-health                            Quick health check for git hooks (silent on success)
      --clean                                   For pollution check: delete detected test issues
      --deep                                    Validate full graph integrity
      --dry-run                                 Preview fixes without making changes
      --fix                                     Automatically fix issues where possible
      --fix-child-parent                        Remove child→parent dependencies (opt-in)
  -i, --interactive                             Confirm each fix individually
      --migration string                        Run legacy Dolt-server migration diagnostics: 'pre' or 'post'
      --orchestrator                            Running in orchestrator multi-workspace mode (routes.jsonl is expected, higher duplicate tolerance)
      --orchestrator-duplicates-threshold int   Duplicate tolerance threshold for orchestrator mode (wisps are ephemeral) (default 1000)
  -o, --output string                           Export diagnostics to JSON file
      --perf                                    Run performance diagnostics and generate CPU profile
      --server                                  Run Dolt server mode health checks (connectivity, version, schema)
  -v, --verbose                                 Show all checks (default shows only warnings/errors)
  -y, --yes                                     Skip confirmation prompt (for non-interactive use)
```

### bd events

Read and manage the durable events journal (bd_events_journal).

The journal records every committed issue mutation as an ordered, replayable
row. Enable it with 'bd config set events-journal true' (or
BD_EVENTS_JOURNAL=1). Records are emitted only while it is enabled.

Retention is automatic: an enabled journal is bounded to the retention floors
(events-journal-retain-days / -rows, 7 days / 100k rows by default) without
anyone running a command. Disable both floors for an unbounded ledger, or
events-journal-auto-prune for manual control; 'bd events prune' remains for an
earlier, on-demand cut below the floors.

Coverage and scope:
  - Every mutation through bd's normal write paths (create, update, close,
    reopen, delete, claim, dependency add/remove, label add/remove, comment) is
    journaled in the same transaction as the change. Raw DML run through
    'bd sql' bypasses those paths and is NOT journaled — a known non-coverage.
  - The journal is per-branch working-set state (dolt_ignored): it records the
    mutations committed on the writer's active branch. Rows arrive by direct
    write, not by merge, so a consumer must read the journal on the same branch
    the writer commits to; a branch checkout or merge does not carry journal
    rows across branches.
  - For the same reason the journal is per REPLICA. 'bd dolt pull' and the
    changes a merge settles into this clone are not journaled: those rows
    arrived as data, not as local mutations, and nothing on this clone wrote
    them through the mutation seam. A consumer that mirrors a synced workspace
    must re-baseline (a fresh export or a full re-read) after a sync, because
    the journal describes only what THIS clone mutated.
    Each replica also has its OWN seq space, counted from its own first
    mutation. A checkpoint taken against one replica is meaningless against
    another — the same seq names a different record, and a seq above the other
    replica's head reads as "caught up" and stalls forever. Track a checkpoint
    per replica, and re-baseline rather than carry one across.
  - A few writes that happen while a store is being OPENED are unjournaled by
    design: schema migrations and the version reconciliation that runs before
    the workspace's configuration has been applied to the store. They touch
    schema and clone-local metadata, never a bead, so a replaying consumer has
    nothing to apply them to.
  - Dependency records are not symmetric, in two ways.
    Count: a dep_add is emitted for every accepted add, INCLUDING an idempotent
    same-type re-add that only refreshes the edge's metadata. The audit 'events'
    table deduplicates that case and writes nothing; the journal does not. Treat
    dep_add as an upsert of the edge, not as proof the edge is new. A dep_remove
    naming an edge that is already gone emits nothing at all.
    Payload: dep.metadata differs in provenance between the two ops. On dep_add
    it is the value being written, as the caller supplied it; on dep_remove it is
    the raw stored column read back just before the delete. The two can differ
    byte for byte while meaning the same thing, so compare parsed values.

Structural dependency edits — the ones bd wires up itself rather than a 'bd dep'
verb — write no audit event but DO journal, by design: a replaying consumer
needs the edge either way.

```
bd events [command]
```

#### bd events export

Print every events journal record from seq 1, in order, as JSON lines.

Equivalent to 'bd events tail --since 0'. Like tail, it FAILS rather than
present a pruned journal's surviving suffix as a complete history.

```
bd events export [flags]
```

**Flags:**

```
      --limit int   maximum number of records to return (0 = no limit)
```

#### bd events prune

Delete events journal records with seq less than --before.

Retention is already enforced automatically: after a mutating command commits,
and on a timer in 'bd serve', bd deletes everything the floors below do not
protect. This command is for an EARLIER, on-demand cut BELOW the floors — after
a consumer has durably processed a span you do not want to wait out. It cannot
cut deeper than the floors: shrinking the retained window itself means lowering
them. The journal is clone-local operational state, so pruning never affects
issue data.

Two retention floors compose onto --before and can only reduce what a prune
removes. They bound the automatic prune and this one identically:
  events-journal-retain-days   keep every row younger than N days (default 7)
  events-journal-retain-rows   always keep the newest N rows (default 100000)

Set BOTH floors to 0 for an unbounded ledger: automatic pruning then does
nothing, and this command becomes the only thing that deletes a record. To keep
the floors but own deletion yourself, set events-journal-auto-prune false.

Note the floors are time-based and count-based — they are NOT a consumer
watermark. They protect only the recent window; a consumer that has fallen
further behind than both floors allow will be pruned past and lose records.
Consumers are responsible for tracking their own watermark (the highest seq they
have durably processed) and for sizing the floors to the longest outage they
intend to survive. Pruned history cannot be recovered from the workspace — the
journal is the only local copy. Pruning frees rows, not disk: pair it with
'dolt gc' to reclaim the space, since the table is working-set (dolt_ignored)
state that ordinary Dolt commits never garbage-collect.

```
bd events prune [flags]
```

**Flags:**

```
      --before int   delete records with seq less than this value
```

#### bd events tail

Print events journal records with seq greater than --since, in order.

Each line is a JSON record:
  &#123;"seq":N,"ts":"...","op":"create|update|close|delete|dep_add|dep_remove|comment",
   "issue_id":"...","actor":"...","issue":&#123;...|null&#125;,"dep":&#123;"kind":..,"target":..,"metadata":..&#125;,"comment":&#123;...&#125;&#125;

Record contract (stable for external consumers):
  seq       int64   counter-assigned inside the mutation's transaction; gapless,
                    strictly increasing in commit order, never reused or reset
  ts        string  UTC insert time, stamped inside the committing transaction
  op        string  one of the seven ops above
  issue_id  string  the mutated issue's id
  actor     string  the acting identity that performed the mutation, as resolved
                    for the audit-events table (on a comment row: the comment's
                    author); empty (omitted) when the mutation path has no
                    actor — derived maintenance, deletes (other than a rename's
                    synthetic delete), and rows older than the column. Never
                    user attribution when empty.
  issue     object  full issue state AFTER the mutation; null on delete
  dep       object  &#123;"kind","target","metadata"&#125; for dep_add / dep_remove; omitted otherwise
  comment   object  &#123;"id","author","text","created_at","source"&#125; for comment; omitted otherwise

Poll with the highest seq seen to consume new mutations incrementally, or pass
--follow to keep printing new records as they are committed (Ctrl-C to stop).

Retention boundary: if --since is below the oldest retained record — the prefix
you asked for was pruned — the read FAILS instead of silently skipping ahead or
returning an empty success. With --json the failure carries
&#123;"code":"events_journal_truncated","since":N,"floor":F,"head":H&#125;: floor is the
oldest seq still retained, head the highest ever assigned. Resume from floor-1
to continue with a known gap, or rebuild from a full export.

```
bd events tail [flags]
```

**Flags:**

```
      --follow      keep printing new records as they are committed (Ctrl-C to stop)
      --limit int   maximum number of records to return (0 = no limit)
      --since int   return records with seq greater than this value
```

### bd flatten

Nuclear option: squash ALL Dolt commit history into a single commit.

This uses the Tim Sehn recipe:
  1. Create a new branch from the current state
  2. Soft-reset to the initial commit (preserving all data)
  3. Commit everything as a single snapshot
  4. Swap main branch to the new flattened branch
  5. Prune remote-tracking refs (they would keep the old history alive;
     the next push or fetch re-creates them at the new tip)
  6. Run Dolt GC to reclaim space from old history

This is irreversible — all commit history is lost. The resulting database
has exactly one commit containing all current data.

Use this when:
  - Your .beads/dolt directory has grown very large
  - You don't need commit-level history (time travel)
  - You want to start fresh with minimal storage

Examples:
  bd flatten --dry-run               # Preview: show commit count and disk usage
  bd flatten --force                 # Actually squash all history
  bd flatten --force --json          # JSON output

```
bd flatten [flags]
```

**Flags:**

```
      --dry-run   Preview without making changes
  -f, --force     Confirm irreversible history squash
```

### bd gc

Run Dolt garbage collection to reclaim disk space.

GC reclaims unreferenced storage only: it deletes no issues and rewrites no
Dolt history. Deleting closed issues is a separate decision with its own
commands (bd prune, bd purge); rewriting history is an offline procedure, not
part of this command.

Examples:
  bd gc              # Run Dolt GC
  bd gc --dry-run    # Preview what would happen
  bd gc --skip-dolt  # Report what would run, reclaim nothing

```
bd gc [flags]
```

**Flags:**

```
      --dry-run     Preview without making changes
  -f, --force       Accepted for compatibility; gc never prompts
      --skip-dolt   Skip Dolt garbage collection phase
```

### bd migrate

Database migration and data transformation commands.

Without subcommand, checks and updates database metadata to current version.

Subcommands:
  hooks                            Plan git hook migration to marker-managed format
  issues                           Move issues between repositories
  schema                           Apply pending schema migrations (idempotent)
  sync                             Set up sync.branch workflow for multi-clone setups

On a remote-backed database with pending schema migrations bd refuses to
migrate in place (#4259): migrating two clones independently forks the schema
so bd dolt pull can no longer merge — the break is silent and unrecoverable.
Use --force to confirm you are the single designated migrator, after which you
should publish the migrated schema with 'bd dolt push'. The env-var equivalent
BD_ALLOW_REMOTE_MIGRATE=1 remains supported for scripted/CI use.


```
bd migrate [flags]
bd migrate [command]
```

**Flags:**

```
      --dry-run          Show what would be done without making changes
      --force            Bypass the remote-migrate gate as the single designated migrator (equivalent to BD_ALLOW_REMOTE_MIGRATE=1)
      --inspect          Show migration plan and database state for AI agent analysis
      --json             Output migration statistics in JSON format
      --update-repo-id   Update repository ID (use after changing git remote)
      --yes              Auto-confirm prompts
```

#### bd migrate hooks

Analyze git hook files and sidecar artifacts for migration to marker-managed format.

Modes:
  --dry-run  Preview migration operations without changing files
  --apply    Apply migration operations

Examples:
  bd migrate hooks --dry-run
  bd migrate hooks --apply
  bd migrate hooks --apply --yes
  bd migrate hooks --dry-run --json

```
bd migrate hooks [path] [flags]
```

**Flags:**

```
      --apply     Apply planned hook migration changes
      --dry-run   Show what would be done without making changes
      --json      Output in JSON format
      --yes       Skip confirmation prompt for --apply
```

#### bd migrate issues

Move issues from one source repository to another with filtering and dependency preservation.

This command updates the source_repo field for selected issues, allowing you to:
- Move contributor planning issues to upstream repository
- Reorganize issues across multi-phase repositories
- Consolidate issues from multiple repos

Examples:
  # Preview migration from planning repo to current repo
  bd migrate-issues --from ~/.beads-planning --to . --dry-run

  # Move all open P1 bugs
  bd migrate-issues --from ~/repo1 --to ~/repo2 --priority 1 --type bug --status open

  # Move specific issues with their dependencies
  bd migrate-issues --from . --to ~/archive --id bd-abc --id bd-xyz --include closure

  # Move issues with label filter
  bd migrate-issues --from . --to ~/feature-work --label frontend --label urgent

```
bd migrate issues [flags]
```

**Flags:**

```
      --dry-run            Show plan without making changes
      --from string        Source repository (required)
      --id strings         Specific issue IDs to migrate (can specify multiple)
      --ids-file string    File containing issue IDs (one per line)
      --include string     Include dependencies: none/upstream/downstream/closure (default "none")
      --label strings      Filter by labels (can specify multiple)
      --priority int       Filter by priority (0-4) (default -1)
      --status string      Filter by status (open/closed/all)
      --strict             Fail on orphaned dependencies or missing repos
      --to string          Destination repository (required)
      --type string        Filter by issue type (bug/feature/task/epic/chore/decision)
      --within-from-only   Only include dependencies from source repo (default true)
      --yes                Skip confirmation prompt
```

#### bd migrate legacy-sqlite

Read an authenticated legacy SQLite database as JSONL

```
bd migrate legacy-sqlite --source-db PATH --output PATH|- [flags]
```

**Flags:**

```
      --output string      JSONL output path, or - for stdout
      --source-db string   Legacy SQLite database (read-only)
```

#### bd migrate schema

Apply pending schema migrations idempotently.

Schema migrations also run automatically on store open, so this subcommand
is typically a no-op. It exists to make migration explicit and observable
in CI, release gates, and recovery scenarios.

Example:
  bd migrate schema
  bd migrate schema --json

```
bd migrate schema [flags]
```

**Flags:**

```
      --force   Bypass the remote-migrate gate as the single designated migrator (equivalent to BD_ALLOW_REMOTE_MIGRATE=1)
      --json    Output in JSON format
```

#### bd migrate sync

Configure separate branch workflow for multi-clone setups.

This sets the sync.branch config value so that issue data is committed
to a dedicated branch, keeping your main branch clean.

Example:
  bd migrate sync beads-sync

```
bd migrate sync <branch> [flags]
```

**Flags:**

```
      --dry-run   Show what would be done without making changes
      --json      Output in JSON format
```

### bd purge

Permanently delete closed ephemeral beads and their associated data.

Closed ephemeral beads (wisps, transient molecules) accumulate rapidly and
have no value once closed. This command removes them to reclaim storage.

Deletes: issues, dependencies, labels, events, and comments for matching beads.
Skips: pinned beads (protected).

To delete closed non-ephemeral beads (regular tasks, features, bugs, etc.)
use `bd prune` instead.

For full Dolt storage reclaim after deleting many rows, follow with `bd flatten`
so history can be collapsed and old chunks can be garbage-collected.

EXAMPLES:
  bd purge                           # Preview what would be purged
  bd purge --force                   # Delete all closed ephemeral beads
  bd purge --older-than 7d --force   # Only purge items closed 7+ days ago
  bd purge --pattern "*-wisp-*"      # Only purge matching ID pattern
  bd purge --dry-run                 # Detailed preview with stats

```
bd purge [flags]
```

**Flags:**

```
      --dry-run             Preview what would be purged with stats
  -f, --force               Actually purge (without this, shows preview)
      --older-than string   Only purge beads closed more than N ago (e.g., 7d, 2w, 30)
      --pattern string      Only purge beads matching ID glob pattern (e.g., *-wisp-*)
```

### bd rename-prefix

Rename the issue prefix for all issues in the database.
This will update all issue IDs and all text references across all fields.

USE CASES:
- Shortening long prefixes (e.g., 'knowledge-work-' → 'kw-')
- Rebranding project naming conventions
- Consolidating multiple prefixes after database corruption
- Migrating to team naming standards

Prefix validation rules:
- Allowed characters: lowercase letters, numbers, hyphens
- Must start with a letter
- Must end with a hyphen (e.g., 'kw-', 'work-')
- Cannot be empty or just a hyphen

Multiple prefix detection and repair:
If issues have multiple prefixes (corrupted database), use --repair to consolidate them.
The --repair flag will rename all issues with incorrect prefixes to the new prefix,
preserving issues that already have the correct prefix.

EXAMPLES:
  bd rename-prefix kw-                # Rename from 'knowledge-work-' to 'kw-'
  bd rename-prefix mtg- --repair      # Consolidate multiple prefixes into 'mtg-'
  bd rename-prefix team- --dry-run    # Preview changes without applying

NOTE: This is a rare operation. Most users never need this command.

```
bd rename-prefix <new-prefix> [flags]
```

**Flags:**

```
      --config-only   Only set the issue_prefix config cell; refused (exit 21 under --machine) when any issue id would need rewriting
      --dry-run       Preview changes without applying them
      --repair        Repair database with multiple prefixes by consolidating them
```

### bd sql

Execute a raw SQL query against the underlying database (Dolt).

Useful for debugging, maintenance, and working around bugs in higher-level commands.

Examples:
  bd sql 'SELECT COUNT(*) FROM issues'
  bd sql 'SELECT id, title FROM issues WHERE status = "open" LIMIT 5'
  bd sql 'DELETE FROM dirty_issues WHERE issue_id = "bd-abc123"'
  bd sql --csv 'SELECT id, title, status FROM issues'

The query is passed directly to the database. SELECT queries return results as a
table (or JSON/CSV with --json/--csv). Non-SELECT queries (INSERT, UPDATE, DELETE)
report the number of rows affected.

In proxied-server mode, multiple statements separated by ';' run as a single
committed batch and report "OK", and --database runs the query against a
different server database (equivalent to a session USE) without changing the
project's configured database.

WARNING: Direct database access bypasses the storage layer. Use with caution.

```
bd sql <query> [flags]
```

**Flags:**

```
      --csv   Output results in CSV format
```

### bd upgrade

Commands for checking bd version upgrades and reviewing changes.

The upgrade command helps you stay aware of bd version changes:
  - bd upgrade status: Check if bd version changed since last use
  - bd upgrade review: Show what's new since your last version
  - bd upgrade ack: Acknowledge the current version

Version tracking is automatic - bd updates metadata.json on every run.

```
bd upgrade [command]
```

#### bd upgrade ack

Mark the current bd version as acknowledged.

This updates metadata.json to record that you've seen the current
version. Mainly useful after reviewing upgrade changes to suppress
future upgrade notifications.

Note: Version tracking happens automatically, so you don't need to
run this command unless you want to explicitly mark acknowledgement.

Examples:
  bd upgrade ack
  bd upgrade ack --json

```
bd upgrade ack
```

#### bd upgrade review

Show what's new in bd since the last version you used.

Unlike 'bd info --whats-new' which shows the last 3 versions,
this command shows ALL changes since your specific last version.

If you're upgrading from an old version, you'll see the complete
changelog of everything that changed since then.

Examples:
  bd upgrade review
  bd upgrade review --json

```
bd upgrade review
```

#### bd upgrade status

Check if bd has been upgraded since you last used it.

This command uses the version tracking that happens automatically
at startup to detect if bd was upgraded.

Examples:
  bd upgrade status
  bd upgrade status --json

```
bd upgrade status
```

### bd worktree

Manage git worktrees with proper beads configuration.

Worktrees allow multiple working directories sharing the same git repository,
enabling parallel development (e.g., multiple agents or features).

Worktrees automatically share the same beads database as the main repository
via git common directory discovery — no manual redirect configuration needed.

Examples:
  bd worktree create feature-auth           # Create worktree
  bd worktree create bugfix --branch fix-1  # Create with specific branch name
  bd worktree list                          # List all worktrees
  bd worktree remove feature-auth           # Remove worktree (with safety checks)
  bd worktree info                          # Show info about current worktree

```
bd worktree [command]
```

#### bd worktree create

Create a git worktree for parallel development.

This command:
1. Creates a git worktree at ./&lt;name&gt; (or specified path)
2. Adds the worktree path to .gitignore (if inside repo root)

The worktree automatically shares the same beads database as the main
repository via git common directory discovery — no redirect file needed.

Examples:
  bd worktree create feature-auth           # Create at ./feature-auth
  bd worktree create bugfix --branch fix-1  # Create with branch name
  bd worktree create ../agents/worker-1     # Create at relative path

```
bd worktree create <name> [--branch=<branch>] [flags]
```

**Flags:**

```
      --branch string   Branch name for the worktree (default: same as name)
```

#### bd worktree info

Show information about the current worktree.

If the current directory is in a git worktree, shows:
- Worktree path and name
- Branch
- Beads configuration (redirect or main)
- Main repository location

Examples:
  bd worktree info          # Show current worktree info
  bd worktree info --json   # JSON output

```
bd worktree info
```

#### bd worktree list

List all git worktrees and their beads configuration state.

Shows each worktree with:
- Name (directory name)
- Path (full path)
- Branch
- Beads state: "redirect" (uses shared db), "shared" (is main), "none" (no beads)

Examples:
  bd worktree list          # List all worktrees
  bd worktree list --json   # JSON output

```
bd worktree list
```

#### bd worktree remove

Remove a registered git worktree with fail-closed safety checks.

Without --force, the target must be clean and its pinned HEAD must be contained
in either the configured upstream or the single comparator selected by
--merged-into. Comparators may be full refs, unambiguous short ref names, or
full commit object IDs. Revision expressions and worktree-local pseudorefs such
as HEAD and ORIG_HEAD are rejected.

--force skips cleanliness and containment requirements, but it does not skip
registered-identity and concurrent-change checks. --force and --merged-into
are mutually exclusive, and each flag may be specified at most once.

Worktree removal and .gitignore cleanup are not atomic. If removal succeeds but
cleanup fails, this command returns an error that explicitly reports the
worktree as removed; it does not claim or attempt a rollback.

Examples:
  bd worktree remove feature-auth                    # Check the configured upstream
  bd worktree remove feature-auth --merged-into main # Check containment in main
  bd worktree remove feature-auth --force            # Skip clean/containment checks

```
bd worktree remove <name> [flags]
```

**Flags:**

```
      --force                Skip cleanliness and containment checks
      --merged-into string   Require worktree HEAD to be contained in this ref
```

## Integrations & Advanced:

### bd capabilities

Print this binary's command tree, every command's flags (its own and the
persistent ones it inherits), the JSON contract version, and the machine-mode
error kinds with their exit codes. Programs check this instead of probing for
flags with trial invocations. Output is always JSON.

```
bd capabilities
```

### bd repo

Configure and manage multiple repository support for multi-repo hydration.

Multi-repo support allows hydrating issues from multiple beads repositories
into a single database for unified cross-repo issue tracking.

Configuration is stored in .beads/config.yaml under the 'repos' section:

  repos:
    primary: "."
    additional:
      - ~/beads-planning
      - ~/work-repo

Examples:
  bd repo add ~/beads-planning       # Add planning repo
  bd repo add ../other-repo          # Add relative path repo
  bd repo list                       # Show all configured repos
  bd repo remove ~/beads-planning    # Remove by path
  bd repo sync                       # Sync from all configured repos

```
bd repo [command]
```

#### bd repo add

Add a repository path to the repos.additional list in config.yaml.

The path should point to a directory containing a .beads folder.
Paths can be absolute or relative (they are stored as-is).

This modifies .beads/config.yaml, which is version-controlled and
shared across all clones of this repository.

```
bd repo add <path> [flags]
```

**Flags:**

```
      --json   Output JSON
```

#### bd repo list

List all repositories configured in .beads/config.yaml.

Shows the primary repository (always ".") and any additional
repositories configured for hydration.

```
bd repo list [flags]
```

**Flags:**

```
      --json   Output JSON
```

#### bd repo remove

Remove a repository path from the repos.additional list in config.yaml.

The path must exactly match what was added (e.g., if you added "~/foo",
you must remove "~/foo", not "/home/user/foo").

This command also removes any previously-hydrated issues from the database
that came from the removed repository.

```
bd repo remove <path> [flags]
```

**Flags:**

```
      --json   Output JSON
```

#### bd repo sync

Synchronize issues from all configured additional repositories.

Reads issues.jsonl from each additional repository and imports them into
the primary database with their original prefixes and source_repo set.
Uses mtime caching to skip repos whose JSONL hasn't changed.

Also triggers Dolt push/pull if a remote is configured.

```
bd repo sync [flags]
```

**Flags:**

```
      --json      Output JSON
      --verbose   Show detailed sync progress
```

## Other Commands:

### bd blocked

Show blocked issues

```
bd blocked [flags]
```

**Flags:**

```
      --exclude-label strings   Exclude issues that have ANY of these labels
  -l, --label strings           Filter by labels (AND: must have ALL). Can combine with --label-any
      --label-any strings       Filter by labels (OR: must have AT LEAST ONE). Can combine with --label
      --parent string           Filter to descendants of this bead/epic
```

### bd completion

Generate the autocompletion script for bd for the specified shell.
See each sub-command's help for details on how to use the generated script.


```
bd completion [command]
```

#### bd completion bash

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source &lt;(bd completion bash)

To load completions for every new session, execute once:

#### Linux:

	bd completion bash &gt; /etc/bash_completion.d/bd

#### macOS:

	bd completion bash &gt; $(brew --prefix)/etc/bash_completion.d/bd

You will need to start a new shell for this setup to take effect.


```
bd completion bash
```

**Flags:**

```
      --no-descriptions   disable completion descriptions
```

#### bd completion fish

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	bd completion fish | source

To load completions for every new session, execute once:

	bd completion fish &gt; ~/.config/fish/completions/bd.fish

You will need to start a new shell for this setup to take effect.


```
bd completion fish [flags]
```

**Flags:**

```
      --no-descriptions   disable completion descriptions
```

#### bd completion powershell

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	bd completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.


```
bd completion powershell [flags]
```

**Flags:**

```
      --no-descriptions   disable completion descriptions
```

#### bd completion zsh

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" &gt;&gt; ~/.zshrc

To load completions in your current shell session:

	source &lt;(bd completion zsh)

To load completions for every new session, execute once:

#### Linux:

	bd completion zsh &gt; "$&#123;fpath[1]&#125;/_bd"

#### macOS:

	bd completion zsh &gt; $(brew --prefix)/share/zsh/site-functions/_bd

You will need to start a new shell for this setup to take effect.


```
bd completion zsh [flags]
```

**Flags:**

```
      --no-descriptions   disable completion descriptions
```

### bd cook

Cook transforms a .formula.json file into a proto.

By default, cook outputs the resolved formula as JSON to stdout for
ephemeral use. The output can be inspected, piped, or saved to a file.

Two cooking modes are available:

  COMPILE-TIME (default, --mode=compile):
    Produces a proto with &#123;&#123;variable&#125;&#125; placeholders intact.
    Use for: modeling, estimation, contractor handoff, planning.
    Variables are NOT substituted - the output shows the template structure.

  RUNTIME (--mode=runtime or when --var flags provided):
    Produces a fully-resolved proto with variables substituted.
    Use for: final validation before pour, seeing exact output.
    Requires all variables to have values (via --var or defaults).

Formulas are high-level workflow templates that support:
  - Variable definitions with defaults and validation
  - Step definitions that become issue hierarchies
  - Composition rules for bonding formulas together
  - Inheritance via extends

The --persist flag enables the legacy behavior of writing the proto
to the database. This is useful when you want to reuse the same
proto multiple times without re-cooking.

For most workflows, prefer ephemeral protos: pour and wisp commands
accept formula names directly and cook inline.

Examples:
  bd cook mol-feature.formula.json                    # Compile-time: keep &#123;&#123;vars&#125;&#125;
  bd cook mol-feature --var name=auth                 # Runtime: substitute vars
  bd cook mol-feature --mode=runtime --var name=auth  # Explicit runtime mode
  bd cook mol-feature --dry-run                       # Preview steps
  bd cook mol-release.formula.json --persist          # Write to database
  bd cook mol-release.formula.json --persist --force  # Replace existing

Output (default):
  JSON representation of the resolved formula with all steps.

Output (machine mode, BD_MACHINE=1 or --machine, without --persist/--dry-run):
  The cooked step tree bd would pour: extends, expansions, aspects, the
  overlay from formula.overlay-dir and step conditions applied, vars
  substituted (defaults plus --var; --mode=compile keeps placeholders).
  Invalid formulas are error.kind invalid_args (exit 27) with file, key and
  line; an unknown formula is not_found (exit 20). No database is opened.

Output (--persist):
  Creates a proto bead in the database with:
  - ID matching the formula name (e.g., mol-feature)
  - The "template" label for proto identification
  - Child issues for each step
  - Dependencies matching depends_on relationships

```
bd cook <formula-file> [flags]
```

**Flags:**

```
      --dry-run               Preview what would be created
      --force                 Replace existing proto if it exists (requires --persist)
      --mode string           Cooking mode: compile (keep placeholders) or runtime (substitute vars)
      --persist               Persist proto to database (legacy behavior)
      --prefix string         Prefix to prepend to proto ID (e.g., 'gt-' creates 'gt-mol-feature')
      --search-path strings   Additional paths to search for formula inheritance
      --strict                Reject formula keys bd would drop (always on under machine mode)
      --var stringArray       Variable substitution (key=value), enables runtime mode
```

### bd defer

Defer issues to put them on ice for later.

Deferred issues are deliberately set aside - not blocked by anything specific,
just postponed for future consideration. Unlike blocked issues, there's no
dependency keeping them from being worked. Unlike closed issues, they will
be revisited.

Deferred issues don't show in 'bd ready' but remain visible in 'bd list'.

A defer WITH a date is a snooze: once --until passes, the next ready-front
read returns the issue to open automatically (same shape as 'bd undefer').
A defer WITHOUT a date is the indefinite icebox: it stays deferred until
someone runs 'bd undefer'.

Examples:
  bd defer bd-abc                  # Icebox indefinitely (until bd undefer)
  bd defer bd-abc --until=tomorrow # Snooze: auto-wakes once the date passes
  bd defer bd-abc --reason="waiting on API access"
  bd defer bd-abc bd-def           # Defer multiple issues

```
bd defer [id...] [flags]
```

**Flags:**

```
      --reason string   Record why this issue is being deferred (appended to notes)
      --until string    Defer until specific time (e.g., +1h, tomorrow, next monday)
```

### bd formula

Manage workflow formulas - the source layer for molecule templates.

Formulas are TOML/JSON files that define workflows with composition rules.
Define formulas, cook them into protos, then pour or wisp them into work.

Search paths (in order):
  1. &lt;resolved-beads-dir&gt;/formulas/ (active project)
  2. &lt;checkout-root&gt;/.beads/formulas/ (repo-local formulas)
  3. ~/.beads/formulas/ (user)
  4. $GT_TOWN_ROOT/.beads/formulas/ (shared workspace root, if GT_TOWN_ROOT set)

Commands:
  list    List available formulas from all search paths
  show    Show formula details, steps, and composition rules
  lint    Report keys, gate types and vars a formula would silently drop
  schema  Show the formula schema index (alias: primitives)

Discovering primitives:
  bd formula schema                 # list every declared formula struct
  bd formula schema gate            # show Gate fields, types, and tags
  bd formula primitives gate        # alias; same handler as 'schema'
  examples/formulas/primitives/     # curated, smoke-tested wired fixtures
  docs/workflows/formulas.md          # narrative reference

```
bd formula [command]
```

#### bd formula convert

Convert formula files from JSON to TOML format.

TOML format provides better ergonomics:
  - Multi-line strings without \n escaping
  - Human-readable diffs
  - Comments allowed

The convert command reads a .formula.json file and outputs .formula.toml.
The original JSON file is preserved (use --delete to remove it).

Examples:
  bd formula convert shiny              # Convert shiny.formula.json to .toml
  bd formula convert ./my.formula.json  # Convert specific file
  bd formula convert --all              # Convert all JSON formulas
  bd formula convert shiny --delete     # Convert and remove JSON file
  bd formula convert shiny --stdout     # Print TOML to stdout

```
bd formula convert <formula-name|path> [--all] [flags]
```

**Flags:**

```
      --all      Convert all JSON formulas
      --delete   Delete JSON file after conversion
      --stdout   Print TOML to stdout instead of file
```

#### bd formula lint

Lint a .formula.toml file, or every .formula.toml directly in a directory,
against the strict decoder bd cooks with. It reports, per file:

  unknown_key        a key bd does not know (it used to be dropped silently)
  invalid_value      a var field of the wrong type (dropped the same way)
  invalid_gate_type  a step gate type no watcher or command resolves
  validation         structural errors, e.g. a var both required and defaulted
  syntax             the file is not valid TOML

A formula that extends another is resolved against its own directory, then
--search-path. Exits 1 when any file has a problem (machine mode: the
report is data and error.kind is invalid_args, exit 27).

Examples:
  bd formula lint ~/gt/.beads/formulas
  bd formula lint mol-shutdown-dance.formula.toml --json

```
bd formula lint <path|dir> [flags]
```

**Flags:**

```
      --search-path strings   Additional paths to search when resolving extends
```

#### bd formula list

List all formulas from search paths.

Search paths (in order of priority):
  1. &lt;resolved-beads-dir&gt;/formulas/ (active project - highest priority)
  2. &lt;checkout-root&gt;/.beads/formulas/ (repo-local formulas)
  3. ~/.beads/formulas/ (user)
  4. $GT_TOWN_ROOT/.beads/formulas/ (shared workspace root, if GT_TOWN_ROOT set)

Formulas in earlier paths shadow those with the same name in later paths.

To list the declared formula schema structs an agent can write inside a .formula.toml,
use 'bd formula schema' (alias: 'bd formula primitives').

Examples:
  bd formula list
  bd formula list --json
  bd formula list --type workflow
  bd formula list --type convoy

```
bd formula list [flags]
```

**Flags:**

```
      --type string   Filter by type (workflow, expansion, aspect, convoy)
```

#### bd formula schema

Show the formula schema index: every exported struct declared
in a .formula.toml/.formula.json, with field names, types, and tags.

The index is generated from internal/formula/types.go via go:generate; the
struct definitions are the source of truth, so this list cannot drift. It is
structural reference, not proof that every declared runtime behavior is wired.

Examples:
  bd formula schema                 # list every declared schema struct
  bd formula schema gate            # show Gate fields
  bd formula primitives step        # alias; shows Step fields
  bd formula schema --json          # machine-readable index

Curated smoke-tested fixtures for wired primitives live in
examples/formulas/primitives/ (with a smoke harness that proves they work).

```
bd formula schema [primitive]
```

**Aliases:** primitives

#### bd formula show

Show detailed information about a formula.

Displays:
  - Formula metadata (name, type, description)
  - Variables with defaults and constraints
  - Steps with dependencies
  - Composition rules (extends, aspects, expansions)
  - Bond points for external composition

To inspect the structure of an individual primitive (e.g. Gate, ExpandRule)
rather than a user-authored formula, use 'bd formula schema &lt;primitive&gt;'.

Examples:
  bd formula show shiny
  bd formula show rule-of-five
  bd formula show security-audit --json

```
bd formula show <formula-name>
```

### bd help

Help provides help for any command in the application.
Simply type bd help [path to command] for full details.

```
bd help [command] [flags]
```

**Flags:**

```
      --all                Show help for all commands in a single document
      --doc string         Generate markdown docs for a single command
      --docs-root string   Generate repository CLI docs under this root
  -h, --help               help for help
      --list               List all available commands
```

### bd init-safety

bd init flag safety contract.

Every bd init invocation resolves project_id from exactly one explicitly
named source (local reinit, remote adoption, or a fresh mint). When the
source is ambiguous, bd init refuses.

FLAG SURFACE

  bd init                       Mint a new identity. Bootstraps from
                                origin if it has refs/dolt/data.

  bd init --reinit-local        Re-initialize local .beads/ over existing
                                local data. Does NOT authorize discarding
                                remote history. If origin has Dolt data
                                this will refuse — pair with
                                --discard-remote to override.

  bd init --reinit-local \      Discard the remote's Dolt history and
      --discard-remote          replace it with the local reinit. First
                                bd dolt push after this will be a
                                history-replacing force-push.

  bd init --force               Deprecated alias for --reinit-local.
                                Kept working for ≥2 releases.

  bd init --from-jsonl          Import from configured import.path. If
                                origin has Dolt data, this refuses unless
                                --discard-remote authorizes replacing that
                                remote history.

ADOPTING A REMOTE

  If you want to use the remote's existing history, use:

      bd bootstrap

  bd init will automatically suggest this when a remote is detected.

DESTROY-TOKEN (non-interactive only)

  When running with no TTY (CI, agents, piped input), destroying local
  data requires an explicit --destroy-token value: --reinit-local over an
  existing local database, or --discard-remote authorizing a
  remote-history overwrite. The token format is:

      DESTROY-&lt;issue-prefix&gt;

  For example, if your issue prefix is "bd", the token is "DESTROY-bd":

      bd init --reinit-local --discard-remote --destroy-token=DESTROY-bd

  In interactive (TTY) mode you confirm via a typed prompt instead. The
  token is not echoed by bd's runtime error messages — this is a
  deliberate guard against pattern-matched one-liners (see
  engdocs/adr/0002-init-safety-invariants.md).

EXIT CODES

  10    refused: remote has Dolt history and you selected local history
        without --discard-remote
  11    refused: existing local data and you declined the destroy confirm
        (interactive), or re-ran with no valid --destroy-token
        (non-interactive)
  12    refused: --discard-remote passed without a valid --destroy-token
        (non-interactive mode)

RECOVERY

  If you hit a refusal, see docs/recovery/init-safety.md for step-by-step recovery
  playbooks for each exit code.


```
bd init-safety
```

### bd mail

Delegates mail operations to an external mail provider.

Agents often type 'bd mail' when working with beads, but mail functionality
is typically provided by the orchestrator. This command bridges that gap
by delegating to the configured mail provider.

Configuration (checked in order):
  1. BEADS_MAIL_DELEGATE or BD_MAIL_DELEGATE environment variable
  2. 'mail.delegate' config setting (bd config set mail.delegate "gt mail")

Examples:
  # Configure delegation (one-time setup)
  `export BEADS_MAIL_DELEGATE="gt mail"`
  # or
  bd config set mail.delegate "gt mail"

  # Then use bd mail as if it were gt mail
  bd mail inbox                    # Lists inbox
  bd mail send mayor/ -s "Hi"      # Sends mail
  bd mail read msg-123             # Reads a message

```
bd mail [subcommand] [args...]
```

### bd mol

Manage molecules - work templates for agent workflows.

Protos are template epics with the "template" label. They define a DAG of work
that can be spawned to create real issues (molecules).

The molecule metaphor:
  - A proto is an uninstantiated template (reusable work pattern)
  - Spawning creates a molecule (real issues) from the proto
  - Variables (&#123;&#123;key&#125;&#125;) are substituted during spawning
  - Bonding combines protos or molecules into compounds
  - Distilling extracts a proto from an ad-hoc epic

Commands:
  show       Show proto/molecule structure and variables
  pour       Instantiate proto as persistent mol (liquid phase)
  wisp       Instantiate proto as ephemeral wisp (vapor phase)
  bond       Polymorphic combine: proto+proto, proto+mol, mol+mol
  squash     Condense molecule to digest
  burn       Discard wisp
  distill    Extract proto from ad-hoc epic

Use "bd formula list" to list available formulas.

```
bd mol [command]
```

**Aliases:** protomolecule

#### bd mol bond

Bond two protos or molecules to create a compound.

The bond command is polymorphic - it handles different operand types:

  formula + formula → cook both, compound proto
  formula + proto   → cook formula, compound proto
  formula + mol     → cook formula, spawn and attach
  proto + proto     → compound proto (reusable template)
  proto + mol       → spawn proto, attach to molecule
  mol + proto       → spawn proto, attach to molecule
  mol + mol         → join into compound molecule

Formula names (e.g., mol-polecat-arm) are cooked inline as ephemeral protos.
This avoids needing pre-cooked proto beads in the database.

Bond types:
  sequential (default) - B runs after A completes
  parallel            - B runs alongside A
  conditional         - B runs only if A fails

Phase control:
  By default, spawned protos follow the target's phase:
  - Attaching to mol (Ephemeral=false) → spawns as persistent (Ephemeral=false)
  - Attaching to ephemeral issue (Ephemeral=true) → spawns as ephemeral (Ephemeral=true)

  Override with:
  --pour  Force spawn as liquid (persistent, Ephemeral=false)
  --ephemeral  Force spawn as vapor (ephemeral, Ephemeral=true, excluded from Dolt sync via dolt_ignore)

Dynamic bonding (Christmas Ornament pattern):
  Use --ref to specify a custom child reference with variable substitution.
  This creates IDs like "parent.child-ref" instead of random hashes.

  Example:
    bd mol bond mol-worker-arm bd-patrol --ref arm-&#123;&#123;worker_name&#125;&#125; --var worker_name=ace
    # Creates: bd-patrol.arm-ace (and children like bd-patrol.arm-ace.capture)

Use cases:
  - Found important bug during patrol? Use --pour to persist it
  - Need ephemeral diagnostic on persistent feature? Use --ephemeral
  - Spawning per-worker arms on a patrol? Use --ref for readable IDs

Examples:
  bd mol bond mol-feature mol-deploy                    # Compound proto
  bd mol bond mol-feature mol-deploy --type parallel    # Run in parallel
  bd mol bond mol-feature bd-abc123                     # Attach proto to molecule
  bd mol bond bd-abc123 bd-def456                       # Join two molecules
  bd mol bond mol-critical-bug wisp-patrol --pour       # Persist found bug
  bd mol bond mol-temp-check bd-feature --ephemeral          # Ephemeral diagnostic
  bd mol bond mol-arm bd-patrol --ref arm-&#123;&#123;name&#125;&#125; --var name=ace  # Dynamic child ID

```
bd mol bond <A> <B> [flags]
```

**Aliases:** fart

**Flags:**

```
      --as string         Custom title for compound proto (proto+proto only)
      --dry-run           Preview what would be created
      --ephemeral         Force spawn as vapor (ephemeral, Ephemeral=true)
      --pour              Force spawn as liquid (persistent, Ephemeral=false)
      --ref string        Custom child reference with {{var}} substitution (e.g., arm-{{polecat_name}})
      --type string       Bond type: sequential, parallel, or conditional (default "sequential")
      --var stringArray   Variable substitution for spawned protos (key=value)
```

#### bd mol burn

Burn a molecule, deleting it without creating a digest.

Unlike squash (which creates a permanent digest before deletion), burn
completely removes the molecule with no trace. Use this for:
  - Abandoned patrol cycles
  - Crashed or failed workflows
  - Test/debug molecules you don't want to preserve

The burn operation differs based on molecule phase:
  - Wisp (ephemeral): Direct delete
  - Mol (persistent): Cascade delete (syncs to remotes)

CAUTION: This is a destructive operation. The molecule's data will be
permanently lost. If you want to preserve a summary, use 'bd mol squash'.

Example:
  bd mol burn bd-abc123              # Delete molecule with no trace
  bd mol burn bd-abc123 --dry-run    # Preview what would be deleted
  bd mol burn bd-abc123 --force      # Skip confirmation
  bd mol burn bd-a1 bd-b2 bd-c3      # Batch delete multiple wisps

```
bd mol burn <molecule-id> [molecule-id...] [flags]
```

**Flags:**

```
      --dry-run   Preview what would be deleted
      --force     Skip confirmation prompt
```

#### bd mol current

Show where you are in a molecule workflow.

If molecule-id is given, show status for that molecule.
If not given, infer from in_progress issues assigned to current agent.

The output shows all steps with status indicators:
  [done]     - Step is complete (closed)
  [current]  - Step is in_progress (you are here)
  [ready]    - Step is ready to start (unblocked)
  [blocked]  - Step is blocked by dependencies
  [pending]  - Step is waiting

For large molecules (&gt;100 steps), a summary is shown instead.
Use --limit or --range to view specific steps:
  bd mol current &lt;id&gt; --limit 50       # Show first 50 steps
  bd mol current &lt;id&gt; --range 100-150  # Show steps 100-150

```
bd mol current [molecule-id] [flags]
```

**Flags:**

```
      --for string     Show molecules for a specific agent/assignee
      --limit int      Maximum number of steps to display (0 = auto, use 'all' threshold)
      --range string   Display specific step range (e.g., '1-50', '100-150')
```

#### bd mol distill

Distill a molecule by extracting a reusable formula from an existing epic.

This is the reverse of pour: instead of formula → molecule, it's molecule → formula.

The distill command:
  1. Loads the existing epic and all its children
  2. Converts the structure to a .formula.json file
  3. Replaces concrete values with &#123;&#123;variable&#125;&#125; placeholders (via --var flags)

Use cases:
  - Team develops good workflow organically, wants to reuse it
  - Capture tribal knowledge as executable templates
  - Create starting point for similar future work

Variable syntax (both work - we detect which side is the concrete value):
  --var branch=feature-auth    Spawn-style: variable=value (recommended)
  --var feature-auth=branch    Substitution-style: value=variable

Output locations (first writable wins):
  1. &lt;resolved-beads-dir&gt;/formulas/ (project-level, default)
  2. &lt;checkout-root&gt;/.beads/formulas/ (repo-local formulas)
  3. ~/.beads/formulas/     (user-level, if project not writable)

Examples:
  bd mol distill bd-o5xe my-workflow
  bd mol distill bd-abc release-workflow --var feature_name=auth-refactor

```
bd mol distill <epic-id> [formula-name] [flags]
```

**Flags:**

```
      --dry-run           Preview what would be created
      --output string     Output directory for formula file
      --var stringArray   Replace value with {{variable}} placeholder (variable=value)
```

#### bd mol last-activity

Show the most recent activity timestamp for a molecule.

Returns the timestamp of the most recent change to any step in the molecule,
making it easy to detect stale or stuck molecules.

Activity sources:
  step_closed      - A step was closed
  step_updated     - A step was updated (claimed, edited, etc.)
  molecule_updated - The molecule root itself was updated

Examples:
  bd mol last-activity hq-wisp-0laki
  bd mol last-activity hq-wisp-0laki --json

```
bd mol last-activity <molecule-id>
```

#### bd mol pour

Pour a proto into a persistent mol - like pouring molten metal into a mold.

This is the chemistry-inspired command for creating PERSISTENT work from templates.
The resulting mol is stored as persistent beads in the issue database and
syncs like any other bead (bd dolt push / pull).

Phase transition: Proto (solid) -&gt; pour -&gt; Mol (liquid)

WHEN TO USE POUR vs WISP:
  pour (liquid): Persistent work that needs audit trail
    - Feature implementations spanning multiple sessions
    - Work you may need to reference later
    - Anything worth preserving in git history

  wisp (vapor): Ephemeral work that auto-cleans up
    - Release workflows (one-time execution)
    - Operational loops and recurring cycles
    - Health checks and diagnostics
    - Any operational workflow without audit value

TIP: Formulas can specify phase:"vapor" to recommend wisp usage.
     If you pour a vapor-phase formula, you'll get a warning.

Examples:
  bd mol pour mol-feature --var name=auth    # Persistent feature work
  bd mol pour mol-review --var pr=123        # Persistent code review

```
bd mol pour <proto-id> [flags]
```

**Flags:**

```
      --assignee string      Assign the root issue to this agent/user
      --attach strings       Proto to attach after spawning (repeatable)
      --attach-type string   Bond type for attachments: sequential, parallel, or conditional (default "sequential")
      --dry-run              Preview what would be created
      --strict               Reject formula keys bd would drop instead of warning
      --var stringArray      Variable substitution (key=value)
```

#### bd mol progress

Show efficient progress summary for a molecule.

This command uses indexed queries to count progress without loading all steps,
making it suitable for very large molecules (millions of steps).

If no molecule-id is given, shows progress for any molecule you're working on.

Output includes:
  - Progress: completed / total (percentage)
  - Current step: the in-progress step (if any)
  - Rate: steps/hour based on closure times
  - ETA: estimated time to completion

Example:
  bd mol progress bd-hanoi-xyz

```
bd mol progress [molecule-id]
```

#### bd mol ready

Find molecules where a gate has closed and the workflow is ready to resume.

This command discovers molecules waiting at a gate step where:
1. The molecule has a gate bead that blocks a step
2. The gate bead is now closed (condition satisfied)
3. The blocked step is now ready to proceed
4. No agent currently has this molecule hooked

This enables discovery-based resume without explicit waiter tracking.
The patrol system uses this to find and dispatch gate-ready molecules.

Examples:
  bd mol ready --gated           # Find all gate-ready molecules
  bd mol ready --gated --json    # JSON output for automation

```
bd mol ready --gated [flags]
```

**Flags:**

```
      --gated   Find molecules ready for gate-resume dispatch (always on for this subcommand)
```

#### bd mol seed

Verify that a formula is accessible and can be cooked.

The seed command checks formula search paths to ensure a formula exists
and can be loaded. This is useful for verifying system health before
attempting to spawn work from a formula.

Formula search paths (checked in order):
  1. &lt;resolved-beads-dir&gt;/formulas/ (active project)
  2. &lt;checkout-root&gt;/.beads/formulas/ (repo-local formulas)
  3. ~/.beads/formulas/ (user level)
  4. $GT_TOWN_ROOT/.beads/formulas/ (shared workspace root, if GT_TOWN_ROOT set)

Examples:
  bd mol seed mol-feature                 # Verify specific formula
  bd mol seed mol-review --var name=test  # Verify with variable substitution

```
bd mol seed <formula-name> [flags]
```

**Flags:**

```
      --var stringArray   Variable substitution for condition filtering (key=value)
```

#### bd mol show

Show molecule structure and details.

The --parallel flag highlights parallelizable steps:
  - Steps with no blocking dependencies can run in parallel
  - Shows which steps are ready to start now
  - Identifies parallel groups (steps that can run concurrently)

Example:
  bd mol show bd-patrol --parallel

```
bd mol show <molecule-id> [flags]
```

**Flags:**

```
  -p, --parallel   Show parallel step analysis
```

#### bd mol squash

Squash a molecule's ephemeral children into a single digest issue.

This command collects all ephemeral child issues of a molecule (Ephemeral=true),
generates a summary digest, and promotes the wisps to persistent by
clearing their Wisp flag (or optionally deletes them).

The squash operation:
  1. Loads the molecule and all its children
  2. Filters to only wisps (ephemeral issues with Ephemeral=true)
  3. Generates a digest (summary of work done)
  4. Creates a permanent digest issue (Ephemeral=false)
  5. Clears Wisp flag on children (promotes to persistent)
     OR keeps them with --keep-children (default: delete)

AGENT INTEGRATION:
Use --summary to provide an AI-generated summary. This keeps bd as a pure
tool - the calling agent (orchestrator worker, Claude Code, etc.) is responsible
for generating intelligent summaries. Without --summary, a basic concatenation
of child issue content is used.

This is part of the wisp workflow: spawn creates wisps,
execution happens, squash compresses the trace into an outcome (digest).

Example:
  bd mol squash bd-abc123                    # Squash and promote children
  bd mol squash bd-abc123 --dry-run          # Preview what would be squashed
  bd mol squash bd-abc123 --keep-children    # Keep wisps after digest
  bd mol squash bd-abc123 --summary "Agent-generated summary of work done"

```
bd mol squash <molecule-id> [flags]
```

**Flags:**

```
      --dry-run          Preview what would be squashed
      --keep-children    Don't delete ephemeral children after squash
      --summary string   Agent-provided summary (bypasses auto-generation)
```

#### bd mol stale

Detect molecules (epics with children) that are complete but still open.

A molecule is considered stale if:
  1. All children are closed (Completed == Total)
  2. Root issue is still open
  3. Not assigned to anyone (optional, use --unassigned)
  4. Is blocking other work (optional, use --blocking)

By default, shows all complete-but-unclosed molecules.

Examples:
  bd mol stale              # List all stale molecules
  bd mol stale --json       # Machine-readable output
  bd mol stale --blocking   # Only show those blocking other work
  bd mol stale --unassigned # Only show unassigned molecules
  bd mol stale --all        # Include molecules with 0 children

```
bd mol stale [flags]
```

**Flags:**

```
      --all          Include molecules with 0 children
      --blocking     Only show molecules blocking other work
      --unassigned   Only show unassigned molecules
```

#### bd mol wisp

Create or manage wisps - EPHEMERAL molecules for operational workflows.

When called with a proto-id argument, creates a wisp from that proto.
When called with a subcommand (list, gc), manages existing wisps.

Wisps are issues with Ephemeral=true in the main database. They're stored
locally but NOT synced via git.

WHEN TO USE WISP vs POUR:
  wisp (vapor): Ephemeral work that auto-cleans up
    - Release workflows (one-time execution)
    - Operational loops and recurring cycles
    - Health checks and diagnostics
    - Any operational workflow without audit value

  pour (liquid): Persistent work that needs audit trail
    - Feature implementations spanning multiple sessions
    - Work you may need to reference later
    - Anything worth preserving in git history

TIP: Formulas can specify phase:"vapor" to recommend wisp usage.
     If you use pour on a vapor-phase formula, you'll get a warning.

The wisp lifecycle:
  1. Create: bd mol wisp &lt;proto&gt; or bd create --ephemeral
  2. Execute: Normal bd operations work on wisp issues
  3. Squash: bd mol squash &lt;id&gt; (clears Ephemeral flag, promotes to persistent)
  4. Or burn: bd mol burn &lt;id&gt; (deletes without creating digest)

Examples:
  bd mol wisp beads-release --var version=1.0  # Release workflow
  bd mol wisp mol-my-workflow                  # Ephemeral operational cycle
  bd mol wisp list                             # List all wisps
  bd mol wisp gc                               # Garbage collect old wisps

Subcommands:
  list  List all wisps in current context
  gc    Garbage collect orphaned wisps

```
bd mol wisp [proto-id] [flags]
bd mol wisp [command]
```

**Flags:**

```
      --dry-run           Preview what would be created
      --strict            Reject formula keys bd would drop instead of warning
      --var stringArray   Variable substitution (key=value)
```

##### bd mol wisp create

Create a wisp from a proto - sublimation from solid to vapor.

This is the chemistry-inspired command for creating ephemeral work from templates.
The resulting wisp is stored in the main database with Ephemeral=true and NOT synced via git.

Phase transition: Proto (solid) -&gt; Wisp (vapor)

Use wisp for:
  - Operational loops and recurring cycles
  - Health checks and monitoring
  - One-shot orchestration runs
  - Routine operations with no audit value

The wisp will:
  - Be stored in main database with Ephemeral=true flag
  - NOT be synced via git
  - Either evaporate (burn) or condense to digest (squash)

Examples:
  bd mol wisp create mol-patrol                    # Ephemeral patrol cycle
  bd mol wisp create mol-health-check              # One-time health check
  bd mol wisp create mol-diagnostics --var target=db  # Diagnostic run

```
bd mol wisp create <proto-id> [flags]
```

**Flags:**

```
      --dry-run           Preview what would be created
      --var stringArray   Variable substitution (key=value)
```

##### bd mol wisp gc

Garbage collect old or abandoned wisps from the database.

A wisp is considered abandoned if:
  - It hasn't been updated in --age duration and is not closed
  - AND it is not live work: blocked steps (waiting on a dependency), pinned
    beads, wisps currently hooked by a live agent (referenced as hook_bead on
    a non-closed issue, regardless of the wisp's own status), and any step
    whose status category is wip (in_progress, blocked, hooked) or frozen
    (deferred, pinned) are never reclaimed by age, no matter how long they
    have been waiting (GH#4394, be-yqp). Custom statuses count by their
    configured category, so only plain open (active) and closed (done) steps
    are age-reclaimable. If the blocked set, hooked set, or the custom-status
    list cannot be read, the GC aborts rather than risk reclaiming live steps.

Abandoned wisps are deleted without creating a digest. Use 'bd mol squash'
if you want to preserve a summary before garbage collection.

Use --closed to purge closed wisps (regardless of age). This is the fastest
way to reclaim space from accumulated wisp bloat. Safe by default: requires
--force to actually delete. Pinned and configured infra wisps are never
purged, and neither is a closed wisp that is a step of a molecule which is
not itself closed — deleting those would erase completed work from a molecule
that is still running (be-96h).

Note: This uses time-based cleanup, appropriate for ephemeral wisps.
For graph-pressure staleness detection (blocking other work), see 'bd mol stale'.

Examples:
  bd mol wisp gc                                    # Clean abandoned wisps (default: 1h threshold)
  bd mol wisp gc --dry-run                          # Preview what would be cleaned
  bd mol wisp gc --age 24h                          # Custom age threshold
  bd mol wisp gc --all                              # Also clean closed wisps older than threshold
  bd mol wisp gc --closed                           # Preview closed wisp deletion
  bd mol wisp gc --closed --force                   # Delete all closed wisps
  bd mol wisp gc --closed --dry-run                 # Explicit dry-run (same as no --force)
  bd mol wisp gc --exclude-type agent,rig           # Protect agent and rig wisps from GC
  bd mol wisp gc --closed --force --exclude-type mol # Delete closed wisps except mol type

```
bd mol wisp gc [flags]
```

**Flags:**

```
      --age string             Age threshold for abandoned wisp detection (default "1h")
      --all                    Also clean closed wisps older than threshold
      --closed                 Delete all closed wisps (ignores --age threshold)
      --dry-run                Preview what would be cleaned
      --exclude-type strings   Exclude wisps of these types from GC (comma-separated, e.g., agent,rig)
  -f, --force                  Actually delete (default: preview only)
```

##### bd mol wisp list

List all wisps (ephemeral molecules) in the current context.

Wisps are issues with Ephemeral=true in the main database. They are stored
locally but not synced via git.

The list shows:
  - ID: Issue ID of the wisp
  - Title: Wisp title
  - Status: Current status (open, in_progress, closed)
  - Started: When the wisp was created
  - Updated: Last modification time

Old wisp detection:
  - Old wisps haven't been updated in 24+ hours
  - Use 'bd mol wisp gc' to clean up old/abandoned wisps

Examples:
  bd mol wisp list              # List all wisps
  bd mol wisp list --json       # JSON output for programmatic use
  bd mol wisp list --all        # Include closed wisps

```
bd mol wisp list [flags]
```

**Flags:**

```
      --all           Include closed wisps
      --type string   Filter by issue type (e.g., agent, task, patrol)
```

### bd ready

Show ready work (open issues with no active blockers).

Excludes in_progress, blocked, deferred, and hooked issues. This uses the
GetReadyWork API which applies blocker-aware semantics to find truly claimable work.

Note: 'bd list --ready' uses the same blocker-aware ready-work semantics.

Use --mol to filter to a specific molecule's steps:
  bd ready --mol bd-patrol   # Show ready steps within molecule

Use --gated to find molecules ready for gate-resume dispatch:
  bd ready --gated           # Find molecules where a gate closed

Use --claim to atomically claim the first ready issue matching the filters:
  bd ready --claim --json

This is useful for agents executing molecules to see which steps can run next.

```
bd ready [flags]
```

**Flags:**

```
      --after string                 Keyset cursor: return the ready issues after this one (pass back pagination.next_cursor). Requires --json and --sort priority or oldest
  -a, --assignee string              Filter by assignee
      --brief                        Omit the free-form text (description, design, acceptance criteria, notes, payload, waiters) from each row. Filters that read those fields still select on them. An omitted field is indistinguishable from an empty one; fetch a whole issue with bd show. Default in text mode, whose renderings print none of those fields; meaningful for --json. Cannot be combined with --claim, --gated, --mol or --explain.
      --claim                        Atomically claim the first ready issue matching the filters
      --exclude-label strings        Exclude issues that have ANY of these labels
      --exclude-type strings         Exclude issue types from results (comma-separated or repeatable, e.g., --exclude-type=convoy,epic)
      --explain                      Show dependency-aware reasoning for why issues are ready or blocked
      --flat                         Alias for --plain, spelled the way bd list spells it
      --gated                        Find molecules ready for gate-resume dispatch
      --has-metadata-key string      Filter issues that have this metadata key set
      --include-deferred             Include issues with future defer_until timestamps
      --include-ephemeral            Include ephemeral issues (wisps) in results
  -l, --label strings                Filter by labels (AND: must have ALL). Can combine with --label-any
      --label-any strings            Filter by labels (OR: must have AT LEAST ONE). Can combine with --label
      --label-pattern string         Filter by label glob pattern (e.g., 'tech-*' matches tech-debt, tech-legacy)
      --label-regex string           Filter by label regex pattern (e.g., 'tech-(debt|legacy)')
  -n, --limit int                    Maximum issues to show (use 0 for unlimited) (default 100)
      --max-rows int                 Hard upper bound on rows fetched from storage. Returns a non-zero exit (code 2) and an error to stderr if exceeded. 0 disables (the default). Overrides BEADS_MAX_ROWS for this invocation. Useful in CI/agent rigs that want a circuit breaker against pathological queries.
      --metadata-field stringArray   Filter by metadata field (key=value, repeatable)
      --mol string                   Filter to steps within a specific molecule
      --mol-type string              Filter by molecule type: swarm, patrol, or work
      --offset int                   Not supported: any value above 0 is rejected (page with --limit).
      --parent string                Filter to descendants of this bead/epic
      --plain                        Display issues as a plain numbered list
      --pretty                       Display issues in a tree format with status/priority symbols (default true)
  -p, --priority int                 Filter by priority
  -s, --sort string                  Sort policy: priority (default), hybrid, oldest (default "priority")
  -t, --type string                  Filter by issue type (task, bug, feature, epic, decision, merge-request). Aliases: mr→merge-request, feat→feature, mol→molecule, dec/adr→decision
  -u, --unassigned                   Show only unassigned issues
```

### bd rename

Rename an issue from one ID to another.

This updates:
- The issue's primary ID
- All references in other issues (descriptions, titles, notes, etc.)
- Dependencies pointing to/from this issue
- Labels, comments, and events

Examples:
  bd rename bd-w382l bd-dolt     # Rename to memorable ID
  bd rename gt-abc123 gt-auth    # Use descriptive ID

Note: The new ID must use a valid prefix for this database.

```
bd rename <old-id> <new-id>
```

### bd schema

Print the JSON Schema for bd's canonical output record types.

The schema is reflected from the same Go structs bd serializes, so it stays in
lockstep with the actual --json / export output. Use it to generate typed
consumer models (datamodel-code-generator, quicktype, ...) rather than
hand-maintaining them.

  bd schema | jq '.types.issue'        # the issue record schema
  bd schema | jq '.types.dependency'   # the dependency record schema

```
bd schema
```

### bd undefer

Undefer issues to restore them to open status.

This brings issues back from the icebox so they can be worked on again.
Issues will appear in 'bd ready' if they have no blockers.

Examples:
  bd undefer bd-abc        # Undefer a single issue
  bd undefer bd-abc bd-def # Undefer multiple issues

```
bd undefer [id...]
```

### bd version

Print version information

```
bd version
```
