# D1: bd machine surface (epic be-3xa)

Decision: gastown reaches beads only through the bd CLI. bd therefore needs a
machine surface: a mode that never blocks, never prints prose on stdout, and
answers every invocation with one typed JSON envelope.

## 1. Machine mode (be-3xa.1)

Turned on by `BD_MACHINE=1` (also `true`/`yes`) or `--machine`. `--machine=false`
turns it off even when the env var is set. Detection happens in `main()` from
the environment and `os.Args`, before cobra runs, so the process is set up
before any command code executes. A persistent `--machine` flag is registered
so cobra accepts it.

In machine mode bd:

- implies `--json` (every rebind of `jsonOutput` from config ORs machine mode in);
- disables colors, emoji and hyperlinks (`internal/ui` checks `BD_MACHINE` at
  package init, so the terminal background probe never runs);
- resolves metrics as disabled: no user-config bootstrap, no queue, no detached
  upload child, no first-run notice;
- skips the molecules loader (`molecules.NewLoader(...).LoadAll`) in the root
  pre-run, skips Dolt auto-push, skips tips;
- replaces `os.Stdin` with `/dev/null` unless an argument asks for stdin (`-`,
  `--flag=-`, `--stdin`), so no prompt or implicit piped read can block;
- refuses `--watch`/`--follow` style streaming modes with
  `invalid_args` (machine mode never blocks);
- captures stdout: `os.Stdout` becomes a pipe for the life of the command, and the
  one envelope is written to the real stdout at exit (section 2).

Files: `cmd/bd/machine.go` (new), `cmd/bd/main.go` (pre-run guards, `main()`),
`internal/ui/terminal.go`, `cmd/bd/dolt_autopush.go`, `cmd/bd/edit.go`.

## 2. Envelope and typed errors (be-3xa.2)

Under machine mode every invocation writes exactly one JSON document to stdout:

```json
{
  "schema_version": 1,
  "contract_version": 1,
  "data": <command payload or null>,
  "pagination": null | {"returned": N, "total": N?, "truncated": bool,
                        "limit": N?, "next_cursor": "..."?},
  "error": null | {"kind": "...", "message": "...",
                   "ids": [{"id": "...", "kind": "...", "message": "..."}]?,
                   "detail": {...}?}
}
```

All five keys are always present. `data` is the command's legacy `--json`
payload, unchanged, so a consumer migrates by unwrapping one level.
`contract_version` is `JSONContractVersion` in `cmd/bd/envelope.go`; bump it on
any breaking change to the envelope or to a `data` shape.

One writer: `emitEnvelope` in `cmd/bd/envelope.go`. `outputJSON`,
`outputJSONWithPagination`, `outputJSONRaw` and the JSON error helpers
(`HandleError*`, `outputJSONError`, `handleSchemaSkewJSON`) stage their value or
error instead of writing when machine mode is on. `main()` calls
`finishMachineMode(err)` once, which builds the envelope and sets the exit code.
Anything else a command writes to stdout is captured: if nothing was staged and
the capture parses as JSON (or JSON lines), it becomes `data`; otherwise it is
forwarded to stderr, never to stdout.

Error kinds and exit codes (machine mode):

| kind | exit | raised by |
|---|---|---|
| (none) | 0 | success |
| internal | 1, or the command's own legacy code | anything not classified below |
| guard_not_held | 13 | `update --if-assignee/--if-status` refusal (unchanged code) |
| not_found | 20 | show/close/update/defer/undefer of an unknown id |
| refused | 21 | policy refusal (close blocker/gate, defer/undefer precondition, read-only mode, migration freeze) with nothing done |
| partial | 22 | batch where at least one id succeeded and at least one failed; `error.ids` lists the failures |
| truncated | 23 | list/query/ready cut by the DEFAULT limit (caller passed no `--limit`); events journal pruned past `--since` |
| route_unreachable | 24 | a prefix route matched but its database has no `dolt_database` or failed to open |
| store_unavailable | 25 | no workspace, or the store failed to open |
| schema_skew | 26 | database schema ahead of the binary |
| invalid_args | 27 | cobra arg/flag errors, unknown command, flags machine mode refuses |

A batch whose failures are all guard refusals keeps exit 13 (legacy rule). A
batch with no successes and mixed failure kinds reports the first failure's
kind. A page cut by an explicit `--limit` is not an error:
`pagination.truncated` is true and the exit is 0.

What changes outside machine mode (legacy `--json` shapes are untouched):

- `bd close A B` exits 1 when any id is refused (was 0 on partial success);
  `bd defer`/`bd undefer` exit 1 when any id fails (were always 0). `bd undefer`
  of an id whose lookup returns no row now reports it instead of panicking.
  `TestProtocol_ClosePartialFailureExitsZero` is inverted to pin this.
  `bd show` of several ids keeps exiting 0 when some are missing: it is a
  read, and only machine mode reports that as `partial`.
- `routed.go` returns a typed `routeUnreachableError` instead of dropping the
  prefix-route failure, so `bd show hq-x` says "could not reach ..." instead
  of "not found". Exit stays 1.

Commands verified to emit the envelope: show, list, ready, query, create,
update, close, comments, dep, mol wisp list, config get, stats, version,
events tail, capabilities. Every other command gets the envelope through the
staging and capture path; its `data` is whatever it printed as JSON.

## 3. version and capabilities (be-3xa.3)

`bd version --json` adds `commit` (always present; ldflags `main.Commit`, else
VCS build info, else `""`), `build_id` (same value; the handshake key),
`schema_ceiling` (`{"main": N, "ignored": M}`), and `contract_version`.
`db_schema_version` keeps its meaning: the schema level this binary migrates a
database to, which the gastown handshake compares with `schema_migrations`.

`bd capabilities --json` (new, no store) walks the cobra tree and returns
`{contract_version, commands: [{path, aliases, hidden, flags: [{name,
shorthand, type, default, persistent}]}], error_kinds: {kind: exit}}`.

## 4. Hot reads (be-3xa.4)

- `bd events tail --since S [--limit N]` under machine mode: `data` =
  `{"records": [...]}`; reads N+1 rows to detect more; `pagination.next_cursor`
  is the last seq returned. A pruned-past checkpoint is `error.kind=truncated`
  with `detail {code, since, floor, head}`.
- `bd ready --json` under machine mode: always carries pagination. New
  `--after <cursor>` pages by keyset over `(priority, created_at, id)` (sort
  `priority`) or `(created_at, id)` (sort `oldest`); hybrid is refused because its
  order moves with the clock. The keyset is applied in bd over the ready set, not
  pushed into SQL; pushing it down is a follow-up.
- `--after` is refused with `--claim`, `--gated`, `--mol`, `--explain` and under
  `--proxied-server`, where it would be ignored and hand a paging caller page
  one forever.
- Contract test `runHotReadContract` (`cmd/bd/machine_contract_test.go`): one
  assertion set run against an in-memory source that calls the same page
  functions, and (`machine_contract_embedded_test.go`) against the built binary
  on a throwaway embedded-Dolt workspace when `BEADS_TEST_EMBEDDED_DOLT=1`.
  Embedded Dolt needs no server and no Docker.

## Not in this change

- Legacy `--json` shapes and `BD_JSON_ENVELOPE=1` behavior are unchanged.
- The proxied-server twins of close/defer/undefer follow the same batch rule
  and carry typed per-id kinds.
- `os.Exit` calls inside commands other than CheckReadonly and
  CheckMigrationFreeze bypass the envelope.
