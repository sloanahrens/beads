---
title: GitHub Copilot CLI Integration Design
description: Design rationale and setup for the Copilot CLI integration, which uses a plugin manifest plus repository instructions
---

This document explains design decisions for GitHub Copilot CLI integration in beads.

For **VS Code + MCP**, see [GitHub Copilot](/integrations/github-copilot).

## Integration Approach

**Recommended: Copilot CLI plugin + repository instructions** - Beads uses Copilot CLI's native plugin manifest plus repository instructions:
- `.copilot-plugin/plugin.json` registers `bd prime` hooks natively
- `.github/copilot-instructions.md` provides repository-specific workflow guidance
- Direct CLI commands with `--json` flags remain the primary operational interface

**Alternative: VS Code MCP** - For Copilot Chat in the editor:
- Native tool calling through MCP
- Higher context overhead from tool schemas
- Use when you want editor-native tool access instead of terminal-first workflow

## Why Plugin + Instructions?

**The plugin manifest already models the behavior we want:**

1. **Hooks belong in the tool's native format**
   - Copilot CLI understands plugin manifests directly
   - `SessionStart` and `PreCompact` are declared as data rather than custom logic
   - This keeps beads core smaller and easier to maintain

2. **Instructions stay explicit and reviewable**
   - Repository guidance lives in `.github/copilot-instructions.md`
   - Teams can review the instructions like any other project documentation
   - The hook behavior and the human-readable guidance stay separate

3. **Nothing tool-specific lives in core**
   - Both files are plain data you copy into your repository
   - Beads ships no Copilot-specific installer or doctor check

## Why Copilot CLI Over MCP for Terminal Work?

**Context efficiency still matters**, even with large context windows:

1. **Compute cost scales with tokens** - Every token in context is processed on every inference
2. **Latency increases with context** - Smaller prompts keep the CLI more responsive
3. **Energy consumption** - Lean prompts are more sustainable over long sessions
4. **Attention quality** - Models generally perform better with tighter, more relevant context

**The math:**
- MCP tool schemas can add 10-50k tokens to context
- `bd prime` adds ~1-2k tokens of workflow context
- That is an order-of-magnitude reduction in overhead

## Installation

Copy two files into your repository:

- `.copilot-plugin/plugin.json` — copy from `plugins/beads/.copilot-plugin/plugin.json`. It declares the `SessionStart` hook (runs `bd prime` when Copilot CLI starts a session) and the `PreCompact` hook (runs `bd prime` before context compaction).
- `.github/copilot-instructions.md` — the repository workflow guidance. Run `bd init` to get the canonical beads section in `AGENTS.md`, then copy that section into this file.

```bash
mkdir -p .copilot-plugin .github
# copy the manifest from the beads repository into .copilot-plugin/plugin.json
# then add the beads section of AGENTS.md to .github/copilot-instructions.md
```

To remove the integration, delete both files.

## Related Files

- `plugins/beads/.copilot-plugin/plugin.json` - Source plugin manifest for the shared plugin package
- [GitHub Copilot integration](/integrations/github-copilot) - VS Code MCP integration

## References

- [GitHub Copilot CLI docs](https://docs.github.com/en/copilot/how-tos/use-copilot-agents/use-copilot-cli)
- [Adding repository custom instructions for GitHub Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli/add-custom-instructions)
