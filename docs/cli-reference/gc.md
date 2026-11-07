---
title: "bd gc"
description: "Garbage collect: run Dolt GC to reclaim disk space"
---

{/* AUTO-GENERATED: do not edit manually */}

Generated from `bd help --doc gc`.

Run Dolt garbage collection to reclaim disk space.

GC reclaims unreferenced storage only: it deletes no issues and rewrites no
Dolt history. Deleting closed issues is a separate decision with its own
commands (bd prune, bd purge); rewriting history is an offline procedure, not
part of this command.

Examples:
  bd gc              # Run Dolt GC
  bd gc --dry-run    # Preview what would happen
  bd gc --skip-dolt  # Report what would run, reclaim nothing

```
bd gc [flags]
```

**Flags:**

```
      --dry-run     Preview without making changes
  -f, --force       Accepted for compatibility; gc never prompts
      --skip-dolt   Skip Dolt garbage collection phase
```
