---
title: Mux
description: Set up beads for Mux with a managed AGENTS.md section and optional layered instruction files
---

Use Beads with Mux through `AGENTS.md`, with optional layered Mux instruction
files for workspace and global scope.

Run `bd init` to write the managed Beads section to the root `AGENTS.md`:

```bash
bd init                       # Root AGENTS.md
bd init --agents-profile full # Complete command reference instead of a pointer
```

Already initialized? `bd init` refuses to rerun, so use `bd onboard` and paste
the snippet it prints into `AGENTS.md`.

## Workspace and Global Layers

Mux also reads deeper instruction layers. Copy the same Beads section into each
layer where you want it to apply:

| Layer | File |
|-------|------|
| Workspace | `.mux/AGENTS.md` |
| Global | `~/.mux/AGENTS.md` |

The root `AGENTS.md` covers a single project; the workspace and global layers
cover every project Mux opens from that directory or from your home directory.

## Related

- [IDE Setup](/getting-started/ide-setup) — the three integration layers
