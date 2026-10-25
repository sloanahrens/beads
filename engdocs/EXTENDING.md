# Extending bd

This file documents contracts that callers of the storage API must honor.
It is not user-facing; it is for code that embeds bd or talks to the
storage layer directly.

## Lite SELECT shape — `IssueFilter.Lite`

`store.SearchIssues(ctx, query, filter)` accepts an `IssueFilter` value.
When `filter.Lite == true`, the storage layer issues a narrower SELECT
that omits these heavy TEXT columns:

- `description`
- `design`
- `acceptance_criteria`
- `notes`
- `waiters`
- `payload`

### Contract for callers

Code that calls `store.SearchIssues` with `IssueFilter.Lite == true`:

- **MUST NOT** read `Description`, `Design`, `AcceptanceCriteria`,
  `Notes`, `Payload`, or `Waiters` from any returned `*types.Issue`.
  These fields are zero-valued after a lite scan; they did not come from
  the row. Reading them yields no signal.
- **MAY** read every other field — identity, status, priority,
  timestamps, labels, dependencies, metadata, etc. Lite preserves them.
- **MUST** detect lite-fetched records via `issue.IsLitePartial` if
  branching behavior on hydration depth is required. The field is
  internal-only (`json:"-"`) — it never crosses the wire.

To recover the full body for a specific issue after a lite listing,
call `store.GetIssue(ctx, id)` — `GetIssue` always returns the full row.

### Default behavior

`IssueFilter.Lite` defaults to `false`. Every existing call site that
does not opt in retains today's behavior: heavy columns are fully
hydrated, and `Issue.IsLitePartial` is `false`.

### Where the contract is enforced

- Column lists: `internal/storage/issueops/scan.go`
  (`IssueSelectColumns`, `IssueSelectColumnsLite`, `HeavyDropList`).
- Scan helpers: `ScanIssueFrom` (full) and `ScanIssueLiteFrom` (lite,
  sets `IsLitePartial`).
- SELECT dispatch: `internal/storage/issueops/search.go` — `SearchIssuesInTx`
  selects `issueProjection` or `issueLiteProjection` on `filter.Lite`; both
  are `searchProjection[*types.Issue]` literals sharing the wisp-merge and
  hydration machinery in `searchTableInTxT`.
- Schema-parity guard:
  `internal/storage/issueops/scan_test.go::TestIssueSelectColumns_LitePlusHeavyEqualsFull`
  fails CI if a future column is added to `IssueSelectColumns` without
  being classified into `IssueSelectColumnsLite` or `HeavyDropList`.

### Backend coverage

`filter.Lite` is honored by BOTH stacks, on both the counted and the
uncounted read paths:

- **Counted** (`SearchIssuesWithCounts` / `GetReadyWorkWithCounts`, i.e.
  everything that returns `IssueWithCounts`): the flag rides the counts
  mega-query as `sqlbuild.CountsHydration.Lite`, and `ScanReadyWorkRowWithCounts`
  scans with `ScanIssueLiteFrom`. Both seams derive the hydration from the
  same field through their `hydrationFor` helper.
- **Uncounted** (`SearchIssues`, the bare `GetReadyWork`): the issueops
  stack selects `issueLiteProjection` (or `ScanIssueLiteFrom` in
  `GetIssuesByIDsInTx`), and `internal/storage/domain/db` makes the same
  choice in `searchTable`, `searchUnion`'s hydration and `fetchIssuesByIDs`
  (`getReadyWorkUnion` passes the flag through).

The uncounted paths serve the text renderings (`bd list --watch`, the
`--parent` tree walk, `bd ready` without `--json`), which print none of the
six dropped fields — which is why those renderings default to the
projection while `--json` keeps whole rows unless `--brief` is passed.
