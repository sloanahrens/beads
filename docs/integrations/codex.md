---
title: Codex
description: Set up beads for Codex with the beads skill and a managed AGENTS.md section
---

Use Beads with Codex through the `beads` skill and managed `AGENTS.md`
guidance.

## Instructions

Codex reads `AGENTS.md` when it starts a session. Run `bd init` to write the
managed Beads section:

```bash
bd init                       # Writes the managed section to AGENTS.md
bd init --agents-profile full # Complete command reference instead of a pointer
```

The default `minimal` profile gives Codex the quick reference and tells it to
run `bd prime` for the full workflow. Use `full` if you want the complete
command reference in the file itself.

Already initialized? `bd init` refuses to rerun, so use `bd onboard` and paste
the snippet it prints into `AGENTS.md`.

`bd init` also installs the git hooks unless you pass `--skip-hooks`. To set up
a single project without touching the instruction file, pass `--skip-agents`.

## The Beads Skill

The shared plugin package declares the beads skill for Codex in
`plugins/beads/.codex-plugin/plugin.json` (`"skills": "./skills/"`). The skill
tree is the same one Claude Code uses — `SKILL.md` for the workflow,
`commands/` for the per-command pages, and `resources/` for deeper material.
With the plugin installed, `$beads` pulls beads workflow detail on demand
instead of carrying it in every prompt. The plugin also registers native
`SessionStart` and `PreCompact` hooks that run `bd prime`, so the workflow
context loads at session start and again before compaction.

## Verify

Start a Codex session and ask what work is ready. It should run `bd ready`.
If it doesn't, confirm `AGENTS.md` holds the section between the
`BEGIN BEADS INTEGRATION` and `END BEADS INTEGRATION` markers.

## Related

- [IDE Setup](/getting-started/ide-setup) — the three integration layers
