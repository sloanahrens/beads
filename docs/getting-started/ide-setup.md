---
title: IDE Setup
description: Connect a coding agent to beads with an AGENTS.md section, bd prime context injection, the Claude Code plugin, git hooks, and the MCP server
---

Configure your IDE or coding agent for beads integration.

Last reviewed: 2026-10-31

Freshness source: `cmd/bd/init.go`, `cmd/bd/init_agent.go`, `cmd/bd/prime.go`, `internal/templates/agents/`, and `plugins/beads/`.

Three layers connect an agent to beads. The instructions layer tells the agent the workflow; the context layer gives it the full picture at session start; the plumbing keeps issue data current between the agent and the repository.

| Layer | Written by | What the agent gets |
|---|---|---|
| Instructions | `bd init` | A managed beads section in `AGENTS.md`: the workflow commands and when to use them |
| Context | `bd prime` | The command reference, project status, and persistent memories, injected at session start |
| Plumbing | `bd init`, `bd hooks install`, the Claude Code plugin | Git hooks, a plugin with `/beads` commands and a skill, and an optional MCP server |

## Instructions: `bd init` writes `AGENTS.md`

`bd init` creates or updates a beads section in `AGENTS.md`, wrapped in `BEGIN`/`END BEADS INTEGRATION` markers that carry version, profile, and hash metadata. Re-running it updates the section in place; the rest of the file is untouched.

```bash
bd init                            # managed section, minimal profile
bd init --agents-profile full      # full command reference instead of a pointer to bd prime
bd init --agents-file CLAUDE.md    # write the section to a different instructions file
bd init --skip-agents              # leave instruction files alone
```

The `minimal` profile (the default) is a pointer to `bd prime` plus a quick reference, for agents whose session-start hook supplies the full context. The `full` profile carries the command reference in the file itself, for agents with no hook.

<Note>
Commit the instructions file so every team member and every agent session starts from the same workflow.
</Note>

Every AGENTS.md-aware tool — Cursor, Factory Droid, Mux, OpenCode, Zed, Jules — reads this section without further setup. Tool-specific guides live in [Integrations](/integrations/index).

## Context: `bd prime`

`bd prime` prints the workflow context for the current project: available commands, status, workflow patterns, and persistent memories from `bd remember`.

```bash
bd prime                  # full context, for a manual run
bd prime --hook-json      # SessionStart hook envelope (Claude Code, Gemini CLI, Codex)
bd prime --memories-only  # memories only, for compact hook contexts
```

`--hook-json` wraps the markdown in the SessionStart JSON envelope those tools require, so hook stdout stays valid JSON. Cap the injected memories with the `prime.max-memories` and `prime.max-memory-chars` config keys when they crowd the context budget.

### Claude Code

Install the beads plugin:

```bash
# In Claude Code
/plugin marketplace add gastownhall/beads
/plugin install beads
```

The plugin registers `SessionStart` and `PreCompact` hooks that run `bd prime`, and ships the `/beads` commands and skill. Restart Claude Code after installation so the plugin loads. See [Beads Claude Code Plugin](/integrations/claude-code-plugin) for the command list.

### Any tool with SessionStart-style hooks

Add the hook to the tool's settings yourself. For Claude Code that is `~/.claude/settings.json` (all projects) or `.claude/settings.json` (one project):

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

Restart the tool after installation.

## Policy Profiles

Template profiles control how much text goes into the instructions file. Policy profiles control what an agent is authorized to do at handoff:

| Policy | Default scope | Commit/push guidance |
|--------|---------------|----------------------|
| `conservative` | Standalone projects, unknown projects, and one-off assistance | Use `bd` for task tracking, then report changed files, validation, and proposed commands. Do not commit, push, or run Dolt remote sync without explicit user or orchestrator approval. |
| `minimal` | Hook-first integrations where `bd prime` carries the detailed workflow | Same git authority as conservative; the installed file stays short and points to `bd prime`. |
| `team-maintainer` | Repositories that explicitly delegate session close to agents | Agents may close beads, run quality gates, commit, run `bd dolt push`, and `git push` as part of routine work. Current "do not commit" or "do not push" instructions still override the profile. |

The generated beads section and `bd prime` default to conservative git authority. Set the profile explicitly with the `agent.profile` config key or the `BD_AGENT_PROFILE` environment variable (values: `conservative`, `minimal`, `team-maintainer`; the env var takes precedence; an unrecognized value falls back to `conservative`):

```bash
bd config set agent.profile team-maintainer
# or, for a single session/process:
BD_AGENT_PROFILE=team-maintainer bd prime
```

`bd prime` layers this explicit knob on top of its per-branch git-authority checks (stealth mode, no git remote, ephemeral branch, `no-push`); those hard constraints still take precedence, and `team-maintainer` remains subordinate to any explicit "do not commit"/"do not push" instruction. Beads never infers team-maintainer authority merely because a remote exists — it must be set via this knob (or, for tools without config access, via top-level project instructions).

## Git Hooks

`bd init` installs beads-managed git hooks to `.beads/hooks/` and points `core.hooksPath` there. Reinstall or repair them at any time:

```bash
bd hooks install --beads   # .beads/hooks/, the layout bd init uses
bd hooks install           # .git/hooks/ instead
```

The managed hooks are **pre-commit**, **post-merge**, **pre-push**, **post-checkout**, and **prepare-commit-msg**. Each one runs the hook it replaced before its own work, and `prepare-commit-msg` adds agent identity trailers for forensics.

```bash
bd hooks list   # installed, outdated, or missing
bd info         # warns when hooks are outdated
```

## MCP Server (Alternative)

For MCP-only environments (Claude Desktop, no shell access), install the MCP server:

```bash
pip install beads-mcp
```

Add it to Claude Desktop's config:

```json
{
  "mcpServers": {
    "beads": {
      "command": "beads-mcp"
    }
  }
}
```

MCP costs more context than the CLI: the tool schemas alone run 10-50k tokens. Prefer the instructions-plus-`bd prime` path wherever the agent can run shell commands. See [MCP Server](/integrations/mcp-server).

## Verifying Your Setup

```bash
bd version
bd doctor            # project health, including integration status
bd hooks list
bd prime             # the exact context an agent receives
```

**Troubleshooting:**

- *Context not appearing?* Run `bd prime` standalone first; if it fails, fix the underlying beads issue rather than the hook.
- *Hooks not working?* Restart the AI tool after installation, then re-run `bd hooks list` and check `bd doctor` output.
