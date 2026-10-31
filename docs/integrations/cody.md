---
title: Sourcegraph Cody
description: Add beads workflow guidance to Sourcegraph Cody through a .cody/rules/beads.md project rules file
---

Use Beads with Sourcegraph Cody through a project rules file.

Cody reads `.cody/rules/beads.md` and applies it as project guidance. Create the file with the beads workflow instructions your agents need: run `bd init` to get the canonical section in `AGENTS.md`, then copy that section into `.cody/rules/beads.md`, or use this minimal version:

```markdown
This project uses bd (beads) for issue tracking.

- Run `bd prime` for workflow context and command guidance.
- Use `bd ready`, `bd show <id>`, `bd update <id> --claim`, and `bd close <id>`.
- Use `bd remember "insight"` for persistent project memory.
- Do not use markdown TODO lists for task tracking.
```

Cody has no hook layer here, so the rules file is the whole integration. Ask the agent to run `bd prime` after a context compaction to restore the full workflow and project memories.

## Related

- [IDE Setup](/getting-started/ide-setup) — the three integration layers
