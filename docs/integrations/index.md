---
title: Integrations
description: Browse every beads editor and agent integration, from AGENTS.md-aware tools to hook-based agents and MCP clients
---

Beads reaches an agent through three independent layers — an instruction file, a session-start hook that runs `bd prime`, and git hooks. Most pages here describe which layers a given tool uses; [IDE Setup](/getting-started/ide-setup) explains the model once.

| Layer | Command | What it gives the agent |
|-------|---------|-------------------------|
| Instructions | `bd init` | A managed beads section in `AGENTS.md`, or `--agents-file` for a tool that reads a different file |
| Session context | `bd prime --hook-json` in a `SessionStart` hook | The full workflow, ready work, and memories at session start |
| Git hooks | `bd hooks install --beads` | Export refresh on commit, merge, checkout, and push |

## Editor and Agent Guides

| Tool | Instructions | Session context |
|------|--------------|-----------------|
| [Claude Code](/integrations/claude-code) | `AGENTS.md` or the plugin | Plugin, or a `SessionStart` hook |
| [Codex](/integrations/codex) | `AGENTS.md` + the beads skill | The beads plugin |
| [Gemini CLI](/integrations/gemini) | `GEMINI.md` | A `SessionStart` hook |
| [Cursor](/integrations/cursor) | `.cursor/rules/beads.mdc` | Always-applied rule |
| [Aider](/integrations/aider) | `.aider/BEADS.md` | `bd prime` piped into the session |
| [Copilot CLI](/integrations/copilot-cli) | `.github/copilot-instructions.md` | Plugin manifest hooks |
| [Junie](/integrations/junie) | `.junie/guidelines.md` | MCP server |
| [Factory.ai Droid](/integrations/factory) | `AGENTS.md` | — |
| [Mux](/integrations/mux) | `AGENTS.md` | — |
| [OpenCode](/integrations/opencode) | `AGENTS.md` | — |
| [Windsurf](/integrations/windsurf) | `.windsurf/rules/beads.md` | — |
| [Sourcegraph Cody](/integrations/cody) | `.cody/rules/beads.md` | — |
| [Kilo Code](/integrations/kilocode) | `.kilocode/rules/beads.md` | — |
| [Kiro CLI](/integrations/kiro) | `.kiro/steering/beads.md` | — |

## MCP-Based Integrations

These avoid the CLI entirely and talk to the beads MCP server:

- [MCP Server](/integrations/mcp-server) — the beads MCP server for any
  MCP-capable client.
- [GitHub Copilot](/integrations/github-copilot) — Copilot in VS Code via
  MCP.

## Other Integration Surfaces

- [Claude Code Plugin](/integrations/claude-code-plugin) — the packaged
  plugin with slash commands, hooks, and MCP tools (`/plugin install beads`).
