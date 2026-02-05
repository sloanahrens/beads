# Beads + Linear Sync Setup Guide

How to set up bidirectional issue sync between Beads (`bd`) and Linear for any repository.

## Prerequisites

- `bd` CLI installed (`brew install beads` or built from fork)
- Beads initialized in the repo (`bd init`)
- Linear API key (Settings > API > Personal API Keys)

## One-Time Setup

### 1. Get Your Linear API Key

Go to Linear: **Settings > Account > API > Personal API Keys**

Create a key with read/write access. Save it securely.

### 2. Set the API Key

Choose one approach (environment variable recommended for multi-repo):

**Option A: Environment variable (applies to all repos)**
```bash
# Add to ~/.zshrc or ~/.bashrc
export LINEAR_API_KEY="lin_api_XXXXX"
```

**Option B: Per-repo config (stored in beads DB)**
```bash
cd ~/code/my-repo
bd config set linear.api_key "lin_api_XXXXX"
```

### 3. Find Your Team ID

```bash
bd linear teams
```

This lists all accessible teams with their UUIDs. Note the team ID(s) you need.

## Per-Repo Setup

Run these commands for each repo you want to sync.

### Step 1: Ensure Beads Is Initialized

```bash
cd ~/code/my-repo
bd doctor  # Should show beads is initialized
```

If not initialized:
```bash
bd init
```

### Step 2: Configure Linear Team

```bash
bd config set linear.team_id "YOUR-TEAM-UUID"
```

### Step 3: Scope to a Linear Project (Recommended)

Without this, beads syncs ALL issues from the team. Scope to the relevant project:

```bash
# Find project IDs (look at Linear URLs or use the API)
bd config set linear.project_id "PROJECT-UUID"
```

### Step 4: Initial Import

Preview first:
```bash
bd linear sync --pull --dry-run
```

Then import:
```bash
bd linear sync --pull
```

### Step 5: Verify

```bash
bd list           # Should show imported issues
bd linear status  # Should show last sync time
```

### Step 6: Test Push (Optional)

If you want bidirectional sync, test pushing a local issue:
```bash
bd create "Test sync to Linear" --type task -p 3
bd linear sync --push --dry-run   # Preview
bd linear sync --push             # Actually push
```

Check Linear to confirm the issue appeared.

## Daily Workflow

### Session Start
```bash
bd linear sync --pull     # Get latest from Linear
bd ready                  # See what's unblocked
```

### Session End
```bash
bd linear sync            # Bidirectional sync (newer wins)
bd sync                   # Sync beads to git
```

### Or Integrate with Existing Commands

Add to `/prime-context` flow:
```bash
devbot exec <repo> bd linear sync --pull
```

Add to `/capture-session` flow:
```bash
devbot exec <repo> bd linear sync --push
```

## Configuration Reference

### Required

| Setting | Source | Example |
|---------|--------|---------|
| `linear.api_key` | Config or `LINEAR_API_KEY` env | `lin_api_XXXXX` |
| `linear.team_id` | Config or `LINEAR_TEAM_ID` env | `UUID` |

### Recommended

| Setting | Purpose | Example |
|---------|---------|---------|
| `linear.project_id` | Scope to one project | `UUID` |

### Optional Mappings

These have sensible defaults. Only override if your Linear workflow differs.

**Priority mapping** (Linear 0-4 → Beads 0-4):
```bash
bd config set linear.priority_map.0 4    # No priority → P4
bd config set linear.priority_map.1 0    # Urgent → P0
bd config set linear.priority_map.2 1    # High → P1
bd config set linear.priority_map.3 2    # Medium → P2
bd config set linear.priority_map.4 3    # Low → P3
```

**State mapping** (Linear state → Beads status):
```bash
bd config set linear.state_map.backlog open
bd config set linear.state_map.unstarted open
bd config set linear.state_map.started in_progress
bd config set linear.state_map.completed closed
bd config set linear.state_map.canceled closed
```

**Label → issue type**:
```bash
bd config set linear.label_type_map.bug bug
bd config set linear.label_type_map.feature feature
```

## Sync Behavior

### Conflict Resolution

| Flag | Who Wins |
|------|----------|
| *(default)* | Newer timestamp |
| `--prefer-local` | Beads always |
| `--prefer-linear` | Linear always |

### What Gets Synced

| Direction | Creates | Updates | Deletes |
|-----------|---------|---------|---------|
| Pull | Yes | Yes | No (manual) |
| Push | Yes (no `external_ref`) | Yes (has `external_ref`) | No |

### Incremental Sync

After the first pull, `linear.last_sync` is saved. Subsequent pulls only fetch issues modified since then.

### External References

Synced issues get an `external_ref` field:
```
https://linear.app/team-key/issue/TEAM-123
```

This links beads issues to their Linear counterparts and is used for conflict detection.

## Repos to Set Up

Current repos with `linear_projects` in `~/.claude/config.yaml`:

| Repo | Linear Project(s) | Status |
|------|-------------------|--------|
| hfcu-poc | Hanscom FCU - Plaid Token Manager API | Pending |
| hfcu-platform-infrastructure | Hanscom FCU - Plaid Token Manager API | Pending |
| cloud-services-platform | Service CU - ScribeUp Plaid, SavvyMoney Plaid | Pending |
| mesh-platform | Mesh Platform Stack Infra Layers, Auth Layer V2 | Pending |

**Note:** Repos with multiple Linear projects may need separate beads instances or the multi-repo setup from the beads fork.

## Troubleshooting

### "No API key configured"
```bash
bd config set linear.api_key "lin_api_XXXXX"
# or
export LINEAR_API_KEY="lin_api_XXXXX"
```

### "No team ID configured"
```bash
bd linear teams   # Find your team UUID
bd config set linear.team_id "UUID"
```

### Too many issues imported
Scope to a project:
```bash
bd config set linear.project_id "PROJECT-UUID"
```

### Duplicate issues after sync
```bash
bd duplicates --auto-merge
```

### Conflicts on every sync
Check timestamps — if Linear and beads both modify the same issue, use a resolution strategy:
```bash
bd linear sync --prefer-local    # If beads is source of truth
bd linear sync --prefer-linear   # If Linear is source of truth
```

## Comparison: MCP Plugin vs Native Beads Sync

| Feature | MCP Plugin (current) | Beads Native Sync |
|---------|---------------------|-------------------|
| Read issues | Yes | Yes |
| Create issues | Yes | Yes (push) |
| Update issues | Yes | Yes (bidirectional) |
| Dependencies | No | Yes (mapped from relations) |
| Conflict resolution | No | Yes (3 strategies) |
| Incremental sync | No | Yes (timestamp-based) |
| Offline support | No | Yes (sync when ready) |
| Comment posting | Yes | No (issues only) |
| Custom field mapping | No | Yes (priority, state, type) |

**Recommendation:** Use beads native sync for issue tracking, keep MCP plugin for comment posting on Linear issues (beads sync doesn't handle comments).
