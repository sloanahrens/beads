# bd verb usage, 31 days to 2026-11-05

Bead be-xu2.22 (read-only research; nothing was deleted or changed). Author: dipper. Feeds the deletion beads under be-xu2.6.

**The window is 31 days, used everywhere below:** transcript files with a modification time inside the 31 days before the run on 2026-11-05 (`find -mtime -31`), and audit lines with `created_at` on or after 2026-10-05. The first draft said 30 days in some places; every figure here was recounted against the 31-day window.

## What this answers

The be-xu2.6 hold list is the 36 commands the deep review's B1 inventory marked class D ("no evidence of use"), plus `internal/audit` (the writer of `interactions.jsonl`). The operator held them until a usage count existed. This file is that count: for each verb, how many times the town's agents ran it in the last 31 days, with the command that produced the number and a verdict (delete, keep, ask).

## Sources and what each can and cannot see

| Id | Source | What it sees |
|---|---|---|---|
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
C3  sort -u | awk -f classify.awk | sort | uniq -c      (classify.awk is printed below)
C4  git -C <gastown clone> grep -n -a -E '(^|[^a-z])bd (VERBS)([ `"'\''\\]|$)' origin/main -- . ':!*.md' ':!docs' ':!*_test.go'
C5  grep -a -c -E "(^|[ ;\"'])bd (VERBS)( |$)" ~/gt/daemon/daemon.log
C6  read each interactions.jsonl; count lines whose created_at is within 31 days, by kind and actor
```

The C3 classifier, in full (its input is the lines `bd <verb> <rest>` that C1 and C2 print; it reads the first word after the verb):

```
{ verb=$2; rest=$0; sub(/^bd [^ ]+ ?/,"",rest); split(rest,t," "); n=t[1]
  c="prose"
  if (n ~ /^--help/ || n ~ /^-h$/) c="help"
  else if (n ~ /^[a-z]{1,8}-[a-z0-9.]+/ || n ~ /^\$/ || n ~ /^\\"?\$/ || n ~ /^2>/ || n ~ /^--[a-z-]+/ || n ~ /^(list|--json)$/ || rest ~ /^$/ ) c="exec"
  print verb "\t" c }
```

The Command column therefore reproduces a count up to the by-hand corrections listed under "Limits" (3 matches reclassified as prose). Results are exact only for the files present on 2026-11-05; transcript files change.

The regex was proved on a multi-line command (`cd x\nbd defer abc`), which it matches, and on `bd show` and `bd comments` (147 and 87 matches in a 40-file sample). The working files are in `~/gt/.runtime/dipper/bd-verb/`.

## Results by source

- **T (polecats and town seats):** 18 matching lines in 31 days, which is 14 distinct commands after deduplication: 3 executions (`bd comment list <id>` once, written on 3 lines; `bd provenance <id>` once; `bd statuses` once), 4 `--help` probes, 7 prose mentions. The table counts distinct commands; the 18 is the raw line count (5 execution lines, 6 help lines, 7 prose lines). Almost none of the 36 verbs is run by a polecat.
- **O (overseer, birds, crew):** 250 matching lines, 119 distinct commands before the hand corrections. Real use is concentrated in a few verbs: `unclaim`, `defer`, `undefer`, `supersede`, `heartbeat`, `statuses`.
- **G (programs):** a text search finds no call to any of the 36 verbs in code, formulas, templates, plugins or scripts; the hits are comments, a profile comment and recorded event data under `.beads/backup`. A second search for the verb as a quoted Go string found one program that runs verbs through an argv slice: the golden recorder in `cmd/bdshim-record`. `exec.go` runs `bd import -i <fixture>` to seed its scratch database, and `table.go` lists `defer`, `undefer`, `unclaim` and `comment` as rows it records against a fixture. These are test-tooling calls on a scratch copy, so the table counts them under G and not as agent use. `internal/bdshim` has handlers for the same verbs (`flags.go`, `handlers_*.go`); they answer calls agents make and are not independent evidence of use.
- **D (daemon log):** 0 hits for all 36 verbs.
- **A (audit writer):** four interactions files exist (town, beads, gastown and om rigs; polecat worktrees were not read). In the 31 days only the beads rig file has lines: 34 entries, all `llm_call` with the actor `test-actor`. I treat `test-actor` as test output, not as real use, because that actor string is what the beads integration tests pass to the store (`cmd/bd/*_integration_test.go`); no other actor appears in the window. Excluding it leaves 0 entries in the window, and the last remaining entry in every file is dated 2026-09-10 (`field_change`, actors `daemon`, `mayor` and the witness and refinery seats). `bd audit` was never executed (one `--help` probe). The writer is still called from `close.go` (`audit.LogFieldChange`).

## Table: the 36 verbs

`exec` = distinct executed commands over T and O. `help` = `--help` probes. `prose` = mentions. `raw` = executed lines before deduplication. All rows come from C1 to C3 filtered on `bd <verb>`; the last column gives the G (program call sites), D (daemon log) and A (audit) result for that verb; the sources are described above.

| Verb | exec | help | prose | raw | Verdict | Note | Command | G / D / A |
|---|---|---|---|---|---|---|---|---|
| admin | 0 | 1 | 1 | 0 | delete | one `--help` probe | C1+C2 piped to C3, filtered on `bd admin` | G 0, D 0, A 0 |
| assign | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd assign` | G 0, D 0, A 0 |
| audit | 0 | 1 | 3 | 0 | ask | the command is unused; the writer is covered below | C1+C2 piped to C3, filtered on `bd audit` | G 0, D 0, A: see writer row |
| batch | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd batch` | G 0, D 0, A 0 |
| comment | 1 | 3 | 11 | 3 | ask | usage alone says delete: the one execution in T was `bd comment list <id>`, a misuse of the singular verb (agents mean `comments`), and 3 of 5 help lines are the same confusion. It is not a delete yet only because the shim and the golden recorder still carry it, which is a dependency and not use; the operator decides | C1+C2 piped to C3, filtered on `bd comment` | G 1 (recorder `table.go`), D 0 |
| conflicts | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd conflicts` | G 0, D 0, A 0 |
| defer | 24 | 4 | 4 | 56 | keep | used daily by the overseer and birds; the shim has a handler | C1+C2 piped to C3, filtered on `bd defer` | G 1 (recorder `table.go`), D 0 |
| diff | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd diff` | G 0, D 0, A 0 |
| duplicate | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd duplicate` | G 0, D 0, A 0 |
| duplicates | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd duplicates` | G 0, D 0, A 0 |
| find-duplicates | 0 | 1 | 0 | 0 | delete | one `--help` probe | C1+C2 piped to C3, filtered on `bd find-duplicates` | G 0, D 0, A 0 |
| flatten | 0 | 0 | 2 | 0 | ask | not run as `bd flatten` in the window; the town flattened Dolt history on 2026-10-25 by a route I did not read | C1+C2 piped to C3, filtered on `bd flatten` | G 0, D 0, A 0 |
| graph | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd graph` | G 0, D 0, A 0 |
| heartbeat | 4 | 1 | 0 | 9 | keep | run by seats and the overseer; the shim lists the alias `hb` | C1+C2 piped to C3, filtered on `bd heartbeat` | G 0, D 0, A 0 |
| import | 0 | 0 | 2 | 0 | keep | no agent ran it in the window (2 prose mentions); the golden recorder runs `bd import -i` to seed its scratch database, a test-tooling call counted in G. Keep while the recorder exists | C1+C2 piped to C3, filtered on `bd import` | G 1 (recorder, `exec.go`), D 0 |
| link | 0 | 0 | 1 | 0 | delete | | C1+C2 piped to C3, filtered on `bd link` | G 0, D 0, A 0 |
| lint | 0 | 0 | 0 | 0 | delete | `gt spec lint` is a different command and is not counted | C1+C2 piped to C3, filtered on `bd lint` | G 0, D 0, A 0 |
| orphans | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd orphans` | G 0, D 0, A 0 |
| ping | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd ping` | G 0, D 0, A 0 |
| preflight | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd preflight` | G 0, D 0, A 0 |
| promote | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd promote` | G 0, D 0, A 0 |
| provenance | 1 | 0 | 0 | 1 | ask | one execution by one agent | C1+C2 piped to C3, filtered on `bd provenance` | G 0, D 0, A 0 |
| prune | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd prune` | G 0, D 0, A 0 |
| recompute-blocked | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd recompute-blocked` | G 0, D 0, A 0 |
| restore | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd restore` | G 0, D 0, A 0 |
| rules | 0 | 0 | 1 | 0 | delete | | C1+C2 piped to C3, filtered on `bd rules` | G 0, D 0, A 0 |
| setup | 0 | 0 | 5 | 0 | delete | already scheduled (be-xu2.6 slice 3); prose only | C1+C2 piped to C3, filtered on `bd setup` | G 0, D 0, A 0 |
| ship | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd ship` | G 0, D 0, A 0 |
| stale | 0 | 1 | 0 | 0 | delete | one `--help` probe | C1+C2 piped to C3, filtered on `bd stale` | G 0, D 0, A 0 |
| statuses | 4 | 0 | 0 | 7 | keep | agents run it to read the status set | C1+C2 piped to C3, filtered on `bd statuses` | G 0, D 0, A 0 |
| supersede | 4 | 1 | 0 | 8 | keep | used by the overseer and birds | C1+C2 piped to C3, filtered on `bd supersede` | G 0, D 0, A 0 |
| swarm | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd swarm` | G 0, D 0, A 0 |
| tag | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd tag` | G 0, D 0, A 0 |
| types | 0 | 0 | 0 | 0 | delete | | C1+C2 piped to C3, filtered on `bd types` | G 0, D 0, A 0 |
| unclaim | 29 | 3 | 3 | 63 | keep | most used of the group; the shim has a handler | C1+C2 piped to C3, filtered on `bd unclaim` | G 1 (recorder `table.go`), D 0 |
| undefer | 8 | 4 | 5 | 16 | keep | the shim has a handler | C1+C2 piped to C3, filtered on `bd undefer` | G 1 (recorder `table.go`), D 0 |
| `internal/audit` (writer) | | | | | ask | 34 lines in 31 days, all actor `test-actor` (excluded as test output; see A above), so 0 real entries; last real entry 2026-09-10; `close.go` still calls `LogFieldChange` | C6 (interactions.jsonl, last 31 days) | A: 34 test-actor, 0 real |

Totals over the 36 verbs: 25 delete, 7 keep (defer, heartbeat, import, statuses, supersede, unclaim, undefer), 4 ask (audit, comment, flatten, provenance). The `internal/audit` writer is a 37th row and is also ask.

## What the verdicts mean for the deletion beads

- Delete candidates have zero executions by agents, no program call site and no daemon log hit in 31 days. A deletion bead should still run its own importer grep and, for any verb that has a shim handler or a row in the differential test, remove that too.
- Keep means an agent ran it in the window, except `import`, which is kept for the golden recorder. `defer`, `undefer`, `unclaim`, `supersede`, `heartbeat` and `statuses` should not be on any hold list for deletion; the shim and the tracker must keep answering them.
- Ask needs the operator: `audit` and the writer (keep or delete the whole `internal/audit` package given no real entries since 2026-09-10), `comment` (usage says delete; the shim and recorder still carry it), `flatten` (find out how the 2026-10-25 flatten was run before deleting), `provenance` (one use).

## Not verified

- Evidence for the delete verdicts is thin by design: it is agent Bash calls (T and O), a program-call search (G), the daemon log (D) and the audit files (A). G, D and A found nothing for any delete verb (last column of the table), and T and O are the only source that can show use. A verb run by a human, by a script an agent did not type, or in a session whose transcript was pruned is invisible, so a delete verdict means no evidence of use, not proof of none.
- Usage by a human typing in a terminal outside an agent session is invisible here.
- Transcripts older than the mtime window, and transcripts of sessions whose files were pruned, are not counted.
- Audit files inside polecat worktrees and sandboxes were not read; only the town and three rig directories were.
