---
id: kiro
title: Kiro CLI
---

## Kiro CLI Integration

Use Beads with Kiro CLI through workspace steering guidance.

Kiro reads workspace steering files when it starts a session. Write the beads workflow instructions to `.kiro/steering/beads.md`: run `bd init` to get the canonical section in `AGENTS.md`, then copy that section into the steering file, or use this minimal version:

```markdown
This project uses bd (beads) for issue tracking.

- Run `bd prime` for workflow context and command guidance.
- Use `bd ready`, `bd show <id>`, `bd update <id> --claim`, and `bd close <id>`.
- Use `bd remember "insight"` for persistent project memory.
- Do not use markdown TODO lists for task tracking.
```

Kiro has no hook layer here, so the steering file is the whole integration. Ask the agent to run `bd prime` after a context compaction to restore the full workflow and project memories.

## Related

- [IDE Setup](/getting-started/ide-setup) — the three integration layers
