---
title: JSON Output Schema Contract
description: The stable JSON output contract for bd --json commands, covering the schema_version envelope, per-command fields, and consumer guidelines.
---

Last reviewed: 2026-10-30

Freshness source: `cmd/bd/output.go`, `cmd/bd/errors.go`, and
`cmd/bd/protocol/json_contract_test.go`.

Every `bd` command that supports `--json` answers with the same shape: a
`schema_version` envelope with the command's payload under `data`. Object,
array, scalar, and error results all nest the same way, so a consumer needs
one decoder and one version check.

## The envelope

```json
{"schema_version": 1, "data": <payload>}
```

The command's payload sits untouched under `.data` — no type corruption, no
field injection. Object commands put an object there; list commands put an
array.

```json
{
  "schema_version": 1,
  "data": {
    "id": "beads-abc",
    "title": "Example issue",
    "status": "open"
  }
}
```

```json
{
  "schema_version": 1,
  "data": [
    {"id": "beads-abc", "title": "First"},
    {"id": "beads-def", "title": "Second"}
  ]
}
```

A `--limit` that truncates a listing (`bd ready`) adds a `pagination` key at
the envelope level:

```json
{"schema_version": 1, "data": [...], "pagination": {"returned": 10, "total": 42, "truncated": true}}
```

`total` is omitted when unknown, and the whole `pagination` key is absent
when the result was not truncated.

## Schema Version

Current version: **1**

The `schema_version` field is an integer that increments when:
- Fields are added, renamed, or removed
- Output structure changes (e.g., nesting depth)
- Field types change (e.g., string to integer)

Additive changes (new optional fields) do NOT bump the version.

## Errors

Errors with `--json` active use the same envelope, with the error object
under `data`:

```json
{
  "schema_version": 1,
  "data": {
    "error": "issue not found: beads-xyz",
    "code": "not_found"
  }
}
```

`code` and `hint` (a remediation suggestion) are both optional — only
`error` and `schema_version` are always present. Most error paths write the
envelope to stderr, though some command-result paths write it to stdout.
JSON-mode errors exit with code 1.

## Field Contracts by Command

### bd list --json

Required fields per item:
- `id` (string): Issue ID (e.g., "beads-abc")
- `title` (string): Issue title
- `status` (string): open, in_progress, closed, deferred
- `priority` (number): 0-4
- `issue_type` (string): bug, feature, task, epic, chore
- `created_at` (string): RFC3339 timestamp

Optional fields:
- `description`, `owner`, `updated_at`, `closed_at`
- `labels` (string[]): Attached labels
- `dependencies` (object[]): Dependency records
- `dependency_count`, `dependent_count`, `comment_count` (number)
- `parent` (string|null): Parent issue ID

### bd ready --json

Same schema as `bd list --json`. Items are filtered to unblocked issues only.
Each item includes `dependency_count`, `dependent_count`, `comment_count`,
and optional `parent` fields. A `--limit`-truncated result adds the
envelope-level `pagination` key (see [The envelope](#the-envelope)).

### bd blocked --json

Returns issues that are blocked by unresolved dependencies.
Each item includes all standard issue fields plus:
- `blocked_by_count` (number): Number of blocking dependencies
- `blocked_by` (string[]): IDs of blocking issues

### bd backup status --json

`data` holds the `backup`, `dolt` and (when the size is available)
`database_size` objects, so a consumer reads `.data.backup` or
`.data.database_size`. In machine mode (`--machine`, `BD_MACHINE=1`) the bare
object is unchanged.

### bd show --json

Returns an array in `.data` with one element per requested ID. Array elements
are plain issue objects; the envelope carries the `schema_version`. Same
required fields as list items, plus:
- `description` (string)
- `acceptance_criteria` (string)
- `revision` (number): guarded-write optimistic-concurrency token; always
  present, including a legacy `0`
- `dependencies` (object[]): Full dependency records
- `comments` (object[]): Comment thread — present only with `--include-comments`;
  the default response returns `comment_count` only (count-only, be-ijck6q)
- `comments_omitted` (boolean, optional): `true` only when `comment_count` is
  nonzero and `comments` was left out of the response (no `--include-comments`).
  Absent when comments were included or when there are none to omit (ga-clgh)

### `import --json`

Returns a summary object in `.data` when `--json` is active:
- `source` (string): File path or "stdin"
- `created` (number): Issues created
- `updated` (number): Existing issues updated
- `unchanged` (number, optional): Rows identical to local state, untouched
- `skipped` (number): Issues skipped (stale rows + dedup)
- `dedup_skipped` (number): Issues skipped by `--dedup` title match
- `memories` (number): Memory records imported
- `ids` (string[]): IDs of created issues
- `updated_issues` (object[]): Per-issue summary of what an update changed
- `tie_kept_local_ids` (string[]): Equal-`updated_at` rows where local state won
- `stale_skipped_ids` (string[]): Rows older than the local issue, skipped
- `skipped_dependencies` (string[]): Dependency edges whose target id was absent
- `dry_run` (boolean): Whether `--dry-run` was active

### bd export --json

Outputs JSONL (one JSON object per line), not wrapped in an envelope.
Each line is a self-contained issue or memory record, discriminated by
`_type` (`"issue"` / `"memory"`). Export lines do **not** carry
`schema_version` — that field belongs to the `--json` command envelope,
not to the interchange stream. The interchange's own version marker is the
optional `_schema` header record (`{"_schema":"beads-jsonl/1"}`), which
readers skip.

Issue records carry an optional `wisp_plane` boolean: the explicit
wisps-plane marker. Export stamps it on rows that live in the wisps table
when the row flags alone cannot prove the plane (a `no_history: true` record
is otherwise ambiguous — an unpromoted no-history wisp and a promoted one
look identical). Import routes the storage plane by this marker, never by
`no_history`: marker absent means the durable issues table. The marker is a
fresh key rather than a reuse of the legacy `wisp` boolean so that older
readers, which do not know it, degrade to flag routing instead of importing
marked rows as ephemeral (purge-eligible and export-excluded). The v0.35–
v0.37 `wisp` key — those streams' spelling of `ephemeral` — is still honored
as a read-side legacy alias.

## Consumer Guidelines

1. **Check `schema_version`** on the envelope. If the version is
   higher than expected, log a warning but attempt to parse anyway
   (additive changes are backward-compatible).

2. **For list commands**, read the array from `.data`.

3. **Ignore unknown fields**. New fields may be added without bumping
   the schema version.

4. **Use `--json` flag**, not `--format json`. The `--json` flag is
   the stable contract; `--format` is for human-readable variants.
