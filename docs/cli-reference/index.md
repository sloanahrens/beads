---
title: CLI Reference
description: Generated reference for every bd command
---

{/* AUTO-GENERATED: do not edit manually */}

Generated from `bd help --docs-root`.

This reference covers all 81 live top-level `bd` commands. Regenerate it with:

```bash
./scripts/generate-cli-docs.sh
```

## Global Flags

These flags apply to all commands:

```
      --actor string              Actor name for audit trail (default: $BEADS_ACTOR, git user.name, $USER)
      --cpu-profile               Generate CPU profile for performance analysis
      --database string           Server database name (bd init only)
      --db string                 Database path (default: auto-discover .beads/*.db)
  -C, --directory string          Change to this directory before running the command (like git -C)
      --dolt-auto-commit string   Dolt auto-commit policy (off|on|batch). 'on': commit after each write. 'batch': defer commits to bd dolt commit; uncommitted changes persist in the working set until then (a live batch-mode bd process also flushes on SIGTERM/SIGHUP). Applies to embedded and direct SQL-server modes. Default: on. Override via config key dolt.auto-commit
      --global                    Use the global shared-server database (beads_global)
      --ignore-schema-skew        Proceed despite forward schema drift (some queries may fail)
      --json                      Output in JSON format
      --machine                   Machine mode for programs (also: BD_MACHINE=1): implies --json, never prompts, never reads stdin unless an argument is '-', no colors or tips
      --mem-profile string        Write heap profile to FILE on exit (also respects BEADS_MEM_PROFILE)
      --no-color                  Disable color output (also: NO_COLOR=1 or CLICOLOR=0)
  -q, --quiet                     Suppress non-essential output (errors only)
      --readonly                  Read-only mode: block write operations (for worker sandboxes)
      --sandbox                   Sandbox mode: disables Dolt auto-push
  -v, --verbose                   Enable verbose/debug output
```

## Commands

- [`bd backup`](/cli-reference/backup)
- [`bd blocked`](/cli-reference/blocked)
- [`bd bootstrap`](/cli-reference/bootstrap)
- [`bd branch`](/cli-reference/branch)
- [`bd capabilities`](/cli-reference/capabilities)
- [`bd children`](/cli-reference/children)
- [`bd close`](/cli-reference/close)
- [`bd comments`](/cli-reference/comments)
- [`bd completion`](/cli-reference/completion)
- [`bd config`](/cli-reference/config)
- [`bd context`](/cli-reference/context)
- [`bd cook`](/cli-reference/cook)
- [`bd count`](/cli-reference/count)
- [`bd create`](/cli-reference/create)
- [`bd create-form`](/cli-reference/create-form)
- [`bd defer`](/cli-reference/defer)
- [`bd delete`](/cli-reference/delete)
- [`bd dep`](/cli-reference/dep)
- [`bd doctor`](/cli-reference/doctor)
- [`bd dolt`](/cli-reference/dolt)
- [`bd epic`](/cli-reference/epic)
- [`bd events`](/cli-reference/events)
- [`bd export`](/cli-reference/export)
- [`bd flatten`](/cli-reference/flatten)
- [`bd forget`](/cli-reference/forget)
- [`bd formula`](/cli-reference/formula)
- [`bd gate`](/cli-reference/gate)
- [`bd gc`](/cli-reference/gc)
- [`bd heartbeat`](/cli-reference/heartbeat)
- [`bd history`](/cli-reference/history)
- [`bd hooks`](/cli-reference/hooks)
- [`bd human`](/cli-reference/human)
- [`bd import`](/cli-reference/import)
- [`bd info`](/cli-reference/info)
- [`bd init`](/cli-reference/init)
- [`bd init-safety`](/cli-reference/init-safety)
- [`bd kv`](/cli-reference/kv)
- [`bd label`](/cli-reference/label)
- [`bd land-record`](/cli-reference/land-record)
- [`bd list`](/cli-reference/list)
- [`bd mail`](/cli-reference/mail)
- [`bd memories`](/cli-reference/memories)
- [`bd merge-slot`](/cli-reference/merge-slot)
- [`bd migrate`](/cli-reference/migrate)
- [`bd migrate-personal`](/cli-reference/migrate-personal)
- [`bd mol`](/cli-reference/mol)
- [`bd note`](/cli-reference/note)
- [`bd onboard`](/cli-reference/onboard)
- [`bd prime`](/cli-reference/prime)
- [`bd priority`](/cli-reference/priority)
- [`bd purge`](/cli-reference/purge)
- [`bd q`](/cli-reference/q)
- [`bd query`](/cli-reference/query)
- [`bd quickstart`](/cli-reference/quickstart)
- [`bd ready`](/cli-reference/ready)
- [`bd recall`](/cli-reference/recall)
- [`bd reclaim`](/cli-reference/reclaim)
- [`bd remember`](/cli-reference/remember)
- [`bd rename`](/cli-reference/rename)
- [`bd rename-prefix`](/cli-reference/rename-prefix)
- [`bd reopen`](/cli-reference/reopen)
- [`bd repo`](/cli-reference/repo)
- [`bd sample`](/cli-reference/sample)
- [`bd schema`](/cli-reference/schema)
- [`bd search`](/cli-reference/search)
- [`bd set-state`](/cli-reference/set-state)
- [`bd show`](/cli-reference/show)
- [`bd sql`](/cli-reference/sql)
- [`bd state`](/cli-reference/state)
- [`bd status`](/cli-reference/status)
- [`bd statuses`](/cli-reference/statuses)
- [`bd supersede`](/cli-reference/supersede)
- [`bd todo`](/cli-reference/todo)
- [`bd unclaim`](/cli-reference/unclaim)
- [`bd undefer`](/cli-reference/undefer)
- [`bd update`](/cli-reference/update)
- [`bd upgrade`](/cli-reference/upgrade)
- [`bd vc`](/cli-reference/vc)
- [`bd version`](/cli-reference/version)
- [`bd where`](/cli-reference/where)
- [`bd worktree`](/cli-reference/worktree)
