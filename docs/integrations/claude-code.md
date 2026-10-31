---
title: Claude Code
description: Wire beads into Claude Code with a SessionStart hook that primes context, using the CLI instead of MCP
---

How to use beads with Claude Code.

## Setup

Two layers cover Claude Code: the [beads plugin](/integrations/claude-code-plugin), which ships the session hook and the `/beads` slash commands, and the `AGENTS.md` instructions `bd init` writes.

### Plugin Setup (Recommended)

```bash
# In Claude Code
/plugin marketplace add gastownhall/beads
/plugin install beads
# Restart Claude Code
```

The plugin's `SessionStart` hook runs `bd prime --hook-json`, so the workflow context loads when a session starts, resumes, or clears — SessionStart fires again after context compaction, which means the same hook refreshes context automatically.

### Hook Setup Without the Plugin

Add to `.claude/settings.json` (project) or `~/.claude/settings.json` (global):

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "bd prime --hook-json" }
        ]
      }
    ]
  }
}
```

The `--hook-json` flag wraps the output in the hook JSON envelope Claude Code expects. No `PreCompact` hook is needed - SessionStart fires again after compaction.

### Instructions

Run `bd init` to write the managed beads section into `AGENTS.md`, which Claude Code reads at session start. Pass `--agents-file CLAUDE.md` to write it there instead:

```bash
bd init
bd init --agents-file CLAUDE.md
```

Already initialized? `bd init` refuses to rerun, so use `bd onboard` and paste the snippet it prints into `AGENTS.md` (or `CLAUDE.md`).

### Verify Setup

```bash
bd prime --hook-json   # Prints the envelope the hook returns
```

## How It Works

1. **Session starts** → `bd prime` injects ~1-2k tokens of context
2. **You work** → Use `bd` CLI commands directly
3. **Session compacts** → SessionStart fires again and `bd prime` refreshes workflow context
4. **Session ends** → `bd dolt push` syncs changes

### Why CLI + hooks instead of MCP?

Context efficiency. MCP tool schemas can add 10-50k tokens to every request; `bd prime` adds ~1-2k tokens of workflow context - 10-50x less overhead, which means lower cost, lower latency, and better model attention. Prefer CLI + hooks in any environment with shell access; use the [MCP server](/integrations/mcp-server) only where the CLI is unavailable, such as Claude Desktop.

### Why not Claude Skills?

Beads doesn't ship or require Claude Skills (`.claude/skills/`). `bd prime` already delivers the workflow context, and the workflow fits a simple command set (ready → create → update → close → sync). Skills are also Claude-specific, which would break beads' editor-agnostic approach - the same CLI works in Cursor, Windsurf, and every other shell-capable editor. You can create your own Skills on top of beads, but none are needed.

## Essential Commands for Agents

### Creating Issues

```bash
# Always include description for context
bd create "Fix authentication bug" \
  --description="Login fails with special characters in password" \
  -t bug -p 1 --json

# Link discovered issues
bd create "Found SQL injection" \
  --description="User input not sanitized in query builder" \
  --deps discovered-from:bd-42 --json
```

### Working on Issues

```bash
# Find ready work
bd ready --json

# Start work
bd update bd-42 --claim --json

# Complete work
bd close bd-42 --reason "Fixed in commit abc123" --json
```

### Querying

```bash
# List open issues
bd list --status open --json

# Show issue details
bd show bd-42 --json

# Check blocked issues
bd blocked --json
```

### Syncing

```bash
# ALWAYS run at session end
bd dolt push
```

## Best Practices

### Always Use `--json`

```bash
bd list --json          # Parse programmatically
bd create "Task" --json # Get issue ID from output
bd show bd-42 --json    # Structured data
```

### Always Include Descriptions

```bash
# Good
bd create "Fix auth bug" \
  --description="Login fails when password contains quotes" \
  -t bug -p 1 --json

# Bad - no context for future work
bd create "Fix auth bug" -t bug -p 1 --json
```

### Link Related Work

```bash
# When you discover issues during work
bd create "Found related bug" \
  --deps discovered-from:bd-current --json
```

### Push Before Session End

```bash
# ALWAYS run before ending
bd dolt push
```

## Plugin (Optional)

For slash commands and enhanced UX, install the [beads plugin](/integrations/claude-code-plugin):

```bash
# In Claude Code
/plugin marketplace add gastownhall/beads
/plugin install beads
# Restart Claude Code
```

Adds slash commands:
- `/beads:ready` - Show ready work
- `/beads:create` - Create issue
- `/beads:show` - Show issue
- `/beads:update` - Update issue
- `/beads:close` - Close issue

## Troubleshooting

### Context not injected

```bash
# Check the hook returns JSON
bd prime --hook-json

# Manually prime
bd prime
```

### Changes not syncing

```bash
# Force push
bd dolt push

# Check system health
bd doctor
```

### Database not found

```bash
# Initialize beads
bd init --quiet
```

## See Also

- [Beads Claude Code Plugin](/integrations/claude-code-plugin) - Packaged plugin with slash commands
- [MCP Server](/integrations/mcp-server) - For MCP-only environments
- [IDE Setup](/getting-started/ide-setup) - Other editors
