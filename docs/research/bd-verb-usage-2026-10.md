# bd verb usage, 30 days to 2026-11-05

Bead be-xu2.22 (read-only research; nothing was deleted or changed). Author: dipper. Feeds the deletion beads under be-xu2.6.

## What this answers

The be-xu2.6 hold list is the 36 commands the deep review's B1 inventory marked class D ("no evidence of use"), plus `internal/audit` (the writer of `interactions.jsonl`). The operator held them until a usage count existed. This file is that count: for each verb, how many times the town's agents ran it in the last 30 days, with the command that produced the number and a verdict (delete, keep, ask).

## Sources and what each can and cannot see

| Id | Source | What it sees |
|---|---|---|
| T | Agent transcripts under `~/gt/.claude-town/projects` (polecats and town seats), 2,939 files modified in the 31 days before the run | Bash tool calls an agent made |
| O | Agent transcripts under `~/.claude/projects` (overseer, birds, crew sessions), files modified in the same window | Bash tool calls those sessions made |
| G | `git grep` on gastown `origin/main` (82f547bcd) for `bd <verb>` in code, formulas, templates, plugins and scripts, tests and markdown excluded | Calls a program makes on its own |
| D | `~/gt/daemon/daemon.log` | Calls the daemon logs |
| A | `interactions.jsonl` in the town and in the beads, gastown and om rig directories | What the audit writer recorded |

Limits, stated once:

- Transcript files are selected by modification time, so a file touched in the window may hold older lines. Counts are an upper bound on the window.
- Each tool call is written about twice in the transcript files. The table gives distinct commands (deduplicated on the verb plus the next 45 characters), which undercounts repeated identical calls and is the number to read. A raw line count is in the last column of the table.
- Only Bash tool calls are counted. A verb run inside a script the agent did not type, or by a Go `exec.Command` that builds its argv from variables, is not seen. Source G found none for these verbs.
- Calls by absolute path (`.../bd defer`) were checked separately: 3 hits, all prose.
- Classification of each match is by the token after the verb: an id, `$var`, `2>&1` or a flag counts as an execution; `--help` as a help probe; a plain word as prose (the verb was mentioned in a note, a grep pattern or an echo label). This is a heuristic. I read the 18 town matches and corrected the heuristic by hand in two places: `bd audit --actor, ...` is prose (so `audit` has no executions), and `bd unclaim --force before redispatch` and `bd defer --until now wakes` are prose (one each off the `unclaim` and `defer` counts). The overseer and bird matches (250 lines) were classified by the heuristic and spot-read, not read line by line.

## Commands that produced the counts

```
C1  find ~/gt/.claude-town/projects -name '*.jsonl' -mtime -31 -print0 | xargs -0 grep -a -h -o -E "$RE"
C2  find ~/.claude/projects        -name '*.jsonl' -mtime -31 -print0 | xargs -0 grep -a -h -o -E "$RE"
    where RE='"command":"([^"\\]|\\.)*((^|[ ;&|("])|\\[nt])bd (VERBS)[ \\"].{0,45}'
    and VERBS is the 36 names joined with |
C3  sort -u | awk (classify the token after the verb: exec, help, prose) | sort | uniq -c
C4  git -C <gastown clone> grep -n -a -E '(^|[^a-z])bd (VERBS)([ `"'\''\\]|$)' origin/main -- . ':!*.md' ':!docs' ':!*_test.go'
C5  grep -a -c -E "(^|[ ;\"'])bd (VERBS)( |$)" ~/gt/daemon/daemon.log
C6  read each interactions.jsonl; count lines whose created_at is within 30 days, by kind and actor
```

The regex was proved on a multi-line command (`cd x\nbd defer abc`), which it matches, and on `bd show` and `bd comments` (147 and 87 matches in a 40-file sample). The working files are in `~/gt/.runtime/dipper/bd-verb/`.

## Results by source

- **T (polecats and town seats):** 18 matching calls in 30 days: 5 executions (`bd comment list <id>` three times, `bd provenance <id>` once, `bd statuses` once), 6 `--help` probes, 7 prose mentions. Almost none of the 36 verbs is run by a polecat.
- **O (overseer, birds, crew):** 250 matching lines. Real use is concentrated in a few verbs: `unclaim`, `defer`, `undefer`, `supersede`, `heartbeat`, `statuses`.
- **G (programs):** no `exec` site runs any of the 36 verbs. The only programmatic caller is the recorder `cmd/bdshim-record/exec.go`, which runs `bd import -i <fixture>` to seed the golden fixture. `internal/bdshim` carries handlers for `comment`, `defer`, `undefer`, `unclaim` and other verbs for tracker rigs; they exist because agents type those verbs, so they are not independent evidence.
- **D (daemon log):** 0 hits.
- **A (audit writer):** in the 30 days, 15 entries in the beads rig file, all `llm_call` with the actor `test-actor` (test output). The town, gastown and om files have 0 entries in the window; their last entries are dated 2026-09-10 (`field_change`, actor `daemon`). `bd audit` was never executed (one `--help` probe). The writer is still called from `close.go` (`audit.LogFieldChange`).

## Table: the 36 verbs

`exec` = distinct executed commands over T and O. `help` = `--help` probes. `prose` = mentions. `raw` = executed lines before deduplication. All rows come from C1 to C3 filtered on `bd <verb>`; the G, D and A columns are in the section above.

| Verb | exec | help | prose | raw | Verdict | Note |
|---|---|---|---|---|---|---|
| admin | 0 | 1 | 1 | 0 | delete | one `--help` probe |
| assign | 0 | 0 | 0 | 0 | delete | |
| audit | 0 | 1 | 3 | 0 | ask | the command is unused; the writer is covered below |
| batch | 0 | 0 | 0 | 0 | delete | |
| comment | 1 | 3 | 11 | 3 | keep | agents type the singular form (executed once in T as `bd comment list`, probably a misuse); the shim has a handler and a golden row |
| conflicts | 0 | 0 | 0 | 0 | delete | |
| defer | 24 | 4 | 4 | 56 | keep | used daily by the overseer and birds; the shim has a handler |
| diff | 0 | 0 | 0 | 0 | delete | |
| duplicate | 0 | 0 | 0 | 0 | delete | |
| duplicates | 0 | 0 | 0 | 0 | delete | |
| find-duplicates | 0 | 1 | 0 | 0 | delete | one `--help` probe |
| flatten | 0 | 0 | 2 | 0 | ask | not run as `bd flatten` in the window; the town flattened Dolt history on 2026-10-25 by a route I did not read |
| graph | 0 | 0 | 0 | 0 | delete | |
| heartbeat | 4 | 1 | 0 | 9 | keep | run by seats and the overseer; the shim lists the alias `hb` |
| import | 0 | 0 | 2 | 0 | keep | the golden recorder runs `bd import -i`; also used to seed rehearsals |
| link | 0 | 0 | 1 | 0 | delete | |
| lint | 0 | 0 | 0 | 0 | delete | `gt spec lint` is a different command and is not counted |
| orphans | 0 | 0 | 0 | 0 | delete | |
| ping | 0 | 0 | 0 | 0 | delete | |
| preflight | 0 | 0 | 0 | 0 | delete | |
| promote | 0 | 0 | 0 | 0 | delete | |
| provenance | 1 | 0 | 0 | 1 | ask | one execution by one agent |
| prune | 0 | 0 | 0 | 0 | delete | |
| recompute-blocked | 0 | 0 | 0 | 0 | delete | |
| restore | 0 | 0 | 0 | 0 | delete | |
| rules | 0 | 0 | 1 | 0 | delete | |
| setup | 0 | 0 | 5 | 0 | delete | already scheduled (be-xu2.6 slice 3); prose only |
| ship | 0 | 0 | 0 | 0 | delete | |
| stale | 0 | 1 | 0 | 0 | delete | one `--help` probe |
| statuses | 4 | 0 | 0 | 7 | keep | agents run it to read the status set |
| supersede | 4 | 1 | 0 | 8 | keep | used by the overseer and birds |
| swarm | 0 | 0 | 0 | 0 | delete | |
| tag | 0 | 0 | 0 | 0 | delete | |
| types | 0 | 0 | 0 | 0 | delete | |
| unclaim | 29 | 3 | 3 | 63 | keep | most used of the group; the shim has a handler |
| undefer | 8 | 4 | 5 | 16 | keep | the shim has a handler |
| `internal/audit` (writer) | | | | | ask | 0 real entries in 30 days; last real entry 2026-09-10; `close.go` still calls `LogFieldChange` |

Totals over the 36 verbs: 25 delete, 8 keep (comment, defer, heartbeat, import, statuses, supersede, unclaim, undefer), 3 ask (audit, flatten, provenance). The `internal/audit` writer is a 37th row and is also ask.

## What the verdicts mean for the deletion beads

- Delete candidates have zero executions by agents, no program call site and no daemon log hit in 30 days. A deletion bead should still run its own importer grep and, for any verb that has a shim handler or a row in the differential test, remove that too.
- Keep means an agent ran it in the window. `defer`, `undefer`, `unclaim`, `supersede`, `heartbeat` and `statuses` should not be on any hold list for deletion; the shim and the tracker must keep answering them.
- Ask needs the operator: `audit` and the writer (keep or delete the whole `internal/audit` package given no real entries since 2026-09-10), `flatten` (find out how the 2026-10-25 flatten was run before deleting), `provenance` (one use).

## Not verified

- Usage by a human typing in a terminal outside an agent session is invisible here.
- Transcripts older than the mtime window, and transcripts of sessions whose files were pruned, are not counted.
- Audit files inside polecat worktrees and sandboxes were not read; only the town and three rig directories were.
