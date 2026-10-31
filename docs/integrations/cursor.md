---
title: Cursor
description: Set up beads for Cursor with an always-applied project rules file
---

Use Beads with Cursor through a project rules file that keeps the workflow
guidance in front of the agent every turn.

Create **`.cursor/rules/beads.mdc`** with the beads workflow instructions: run
`bd init` to get the canonical section in `AGENTS.md`, then copy that section
into the rule file, or use this minimal version:

```markdown
---
alwaysApply: true
---

This project uses bd (beads) for issue tracking.

- Run `bd prime` for workflow context and command guidance.
- Use `bd ready`, `bd show <id>`, `bd update <id> --claim`, and `bd close <id>`.
- Use `bd remember "insight"` for persistent project memory.
- Do not use markdown TODO lists for task tracking.
```

Because the rule is always applied, Cursor re-includes it every turn, including
after a context compaction. Restart Cursor after adding the file so the rule
loads.

## Verifying it works

In a session, the agent should know your bd workflow without being told — ask
it what work is ready and it should run `bd ready`. If it doesn't, confirm
`.cursor/rules/beads.mdc` exists and restart Cursor.

After a context compaction, have the agent run `bd prime` to restore the
workflow context and project memories.

## Related

- [IDE Setup](/getting-started/ide-setup) — all editor integrations
- [Claude Code](/integrations/claude-code)
- [Codex](/integrations/codex)
