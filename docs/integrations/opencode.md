---
title: OpenCode
description: Give OpenCode beads workflow context via a managed Beads section in AGENTS.md
---

Use Beads with OpenCode through managed `AGENTS.md` guidance.

OpenCode reads `AGENTS.md` at session start. Run `bd init` to write the managed
Beads section into it:

```bash
bd init                      # Writes the managed section to AGENTS.md
bd init --agents-profile full # Complete command reference instead of a pointer
```

Already initialized? `bd init` refuses to rerun, so use `bd onboard` and paste the snippet it prints into `AGENTS.md`.

OpenCode gets the instruction layer with no tool-specific files. For the full
workflow at session start — commands, the session-close protocol, memories —
also point a `SessionStart` hook at `bd prime --hook-json` where the tool
allows one.
Restart OpenCode after the file changes.

## Verify

Open `AGENTS.md` and confirm the section between the `BEGIN BEADS INTEGRATION`
and `END BEADS INTEGRATION` markers is present, then start a session and ask
what work is ready.

## Related

- [IDE Setup](/getting-started/ide-setup) — the three integration layers
