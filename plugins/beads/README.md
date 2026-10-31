# Beads Plugin

This is the shared Claude/Codex plugin package for Beads. Claude and Codex use separate manifest files, but they share the same skill tree.

## Layout

- `.codex-plugin/plugin.json` describes the Codex plugin.
- `.claude-plugin/plugin.json` describes the Claude plugin.
- `skills/beads/` contains the plugin-owned Beads skill.
- `.codex-plugin/hooks/hooks.json` contains Codex-only lifecycle hooks for startup and compaction-aware context refresh.
- The Claude marketplace entry lives at `.claude-plugin/marketplace.json`.

## Codex Hooks

The Codex plugin exposes native hooks through `.codex-plugin/plugin.json`, which points at `.codex-plugin/hooks/hooks.json`.
With Codex 0.129.0+, `/hooks` shows these lifecycle handlers:

- `SessionStart` runs `bd prime` for `startup|resume|clear` and injects full workflow context.
- `PreCompact` runs `bd prime --memories-only` for `manual|auto` and warns if the memories cannot be read.

The plugin is the only source of these hooks; there is no fallback installer.

## Local Development

Claude Code uses `.claude-plugin/marketplace.json`, which points at this shared package root.
