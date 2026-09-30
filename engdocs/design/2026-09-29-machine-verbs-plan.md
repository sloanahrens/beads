# Machine verbs for the gastown refactor (be-qr3, be-cgr, be-pgd, be-u20)

Four small verbs gastown needs so it can reach beads only through the bd CLI
(D1, `d1-machine-surface.md`). Adding verbs and flags does not change an
existing shape, so `contract_version` stays 1. Each verb is one commit, test
first, with a protocol test that drives the built binary under `--machine`.

## be-qr3: `bd rename-prefix <p> --config-only`

Sets `issue_prefix` without touching any id.

- Allowed when no issue row carries an id outside `<p>-`: the stored prefix is
  unset, equal, or stale while the rows already match. Writes the config cell.
  An equal prefix is a no-op that still succeeds (`changed: false`).
- Refused (`refused`, exit 21) when any row would need rewriting. Nothing is
  written. `error.detail` =
  `{current_prefix, new_prefix, mismatched_count, mismatched}` (first five ids).
- `--dry-run` reports the same answer without writing.
- Data: `{old_prefix, new_prefix, changed, issues_count, dry_run}`.

Why rename-prefix rather than `bd config set`: `config set` deliberately
refuses the key and points at the lifecycle commands; the prefix check needs
the row scan rename-prefix already does.

## be-cgr: `bd dep prune-orphans [--dry-run]`

Deletes dependency rows in `dependencies` and `wisp_dependencies` whose source
row or typed target row (`depends_on_issue_id` / `depends_on_wisp_id`) exists
in neither plane. External targets are never orphans. Source and target are
each checked against both planes, so a row is kept whenever either plane still
has the id.

- Storage: `issueops.PruneOrphanDependenciesInTx` (shared SQL), called by
  `dolt.DoltStore.PruneOrphanDependencies` in one write transaction with one
  Dolt commit. Exposed through an optional `storage.OrphanDependencyPruner`
  interface; a backend without it is `refused`.
- Data: `{dry_run, dependencies, wisp_dependencies, total}` (row counts).

## be-pgd: `bd close|delete --if-status=<s> --if-assignee=<a>`

Write-time guards with `bd update`'s semantics: presence by `Changed()`,
`--if-assignee ''` means unassigned, `--if-status` validated against the status
set, mismatch kind `guard_not_held` (exit 13, the code `bd update` guards
already use; the contract lists guard refusals under that kind, not
`refused`).

- close: `BatchCloseItem` gains `ExpectedStatus`/`ExpectedAssignee`, checked in
  the batch transaction before the item closes (`CheckExpectedFieldsInTx`, and
  the loaded row on the unit-of-work leg). A mismatched id is skipped, the rest
  close. A guard that already fails on the resolved snapshot outranks close
  policy. Batch rule: only guard failures gives `guard_not_held` (13) even
  beside successes; mixed failure kinds beside a success give `partial` (22).
  Guards do not waive close policy, so closing another actor's claim still
  needs `--force`.
- delete: `DeleteRequest` gains the same two guards, checked for every named
  id inside the delete transaction. Delete is all-or-nothing by its role
  contract, so any mismatch refuses the whole request, lists every mismatched
  id in `error.ids`, and deletes nothing. Guards with `--cascade` are
  `invalid_args`.
- Proxied-server route: guards refused as `invalid_args` (that route is being
  removed by another branch).

## be-u20: `bd land-record <id>`

The gastown landing worker's one write to a work bead.

```
bd land-record <id> --patch-id P --landed-commit C --gate-result R \
    --om-verdict V --om-score S --route X
bd land-record <id> --reject --kind K [--gate-tail T | --gate-tail-file F] \
    [--om-findings JSON] [--conflicting-file F ...]
```

- Landing writes metadata key `landing` =
  `{patch_id, landed_commit, gate_result, om_verdict, om_score, route,
  recorded_at, recorded_by}` and removes the `rework` label.
- Rejection writes metadata key `landing_rejection` =
  `{kind, gate_tail, om_findings, conflicting_files, recorded_at,
  recorded_by}` and adds
  the `rework` label.
- One `Update` request: metadata and label edits land in one transaction.
- `bd show --json` returns them as `metadata.landing` and
  `metadata.landing_rejection`. No schema migration.
- Data: the updated issue.
- Charter note: the orchestration boundary prefers metadata over commands.
  The record lives in metadata, and the same write is expressible without
  this verb as `bd update <id> --metadata '{"landing":{...}}'
  --remove-label rework` (pinned by `TestProtocol_LandingRecordViaUpdate`).
  The verb is its own commit so it can be dropped if the named contract is
  not wanted in core.

## Gates

`go build ./...`, `go vet ./...`, `make ci-pr-lint`, touched packages' tests
(Docker packages only through `gt slot run`), `./cmd/bd/protocol`, and
`om review -base origin/main` at the midpoint and the end.
