# Fork and Upstream Sync

This repository is a fork of upstream beads, and the town runs it pinned to its
own commit while upstream keeps moving. The two have drifted far enough that
merging upstream main is no longer a routine operation. This page records the
measured divergence, states the stance on taking upstream changes, and gives
the read-only commands that reproduce the numbers.

## The two projects

Upstream beads is [github.com/gastownhall/beads](https://github.com/gastownhall/beads),
formerly `steveyegge/beads`. This repository is the fork the town runs: that
history with a large body of changes on top.

## A snapshot, not a live figure

The numbers here were measured on 2026-11-18 at two commits:

| Side | Commit |
|---|---|
| This fork, main | `0f9f640fe0f1` |
| Upstream, main | `2549ece6e05e` |

Both sides move, so treat these as a snapshot: the shape of the divergence is
stable, the exact counts are not.

The common ancestor of the two mains is merge base `c0d8da42de5f`, dated
2026-09-02. Since then the two sides have diverged in both directions:

| Measure | This fork | Upstream |
|---|---|---|
| Commits the other side lacks | 576 | 879 |
| Files changed since the merge base | 1,897 | 3,900 |

The fork's changes are heavily subtractive: 921 files deleted, 265 added and
692 modified. The deletions concentrate in the CLI and storage layers: 443
under `cmd/bd`, 211 under `internal/storage` and 81 under `internal/httpapi`.
1,029 files were changed on both sides.

A trial merge of upstream main into fork main leaves 790 conflicted files. Of
those, 495 are files this fork deleted and upstream modified, and 317 are
non-test Go files. `go.mod` conflicts as well: this fork builds on Go 1.26.5
with Dolt `v0.40.5-0.20260806213044-796d07497741`, while upstream builds on
Go 1.27.3 with Dolt `v0.40.5-0.20261023035229-05e6bceb1a21`.

## The stance

The town stays pinned to its fork commit. An upstream change arrives one at a
time, as a cherry-pick of a named fix onto a branch, with a test that shows the
fix. Upstream main is never merged in. This is the stance as of 2026-11-18, and
only the operator may change it.

## The schema collision

The file merge is not the only collision. Each side also numbers its database
migrations, and the number is what a database records: a migration's version is
parsed from its filename, the runner reads the cursor's `MAX(version)` and
applies the files numbered above it, and it does not match a pending file by
name. Two sides that choose the same number for different SQL therefore collide
at the database, and whichever binary applies its version first decides what
that number means there.

Each series has its own cursor:

| Series | Cursor table | Migration files |
|---|---|---|
| main | `schema_migrations` | `internal/storage/schema/migrations/` |
| ignored | `ignored_schema_migrations` | `internal/storage/schema/migrations/ignored/` |

The ignored series carries the clone-local schema repairs, and its cursor is
Dolt-ignored like the work it records.

### The numbers the two sides share

| Number | This fork | Upstream |
|---|---|---|
| main 0067 | `0067_create_samples` | `0067_add_versioned_beads_schema` |
| ignored 0026 | `0026_add_wisp_dependency_target_indexes` | `0026_dep_rekey_dedup_marker` |

Upstream's main series runs to `0074` and its ignored series to `0031`, so these
are the first numbers where the two sides disagree, not the last.

The fork's main 0067 file is
`internal/storage/schema/migrations/0067_create_samples.up.sql` in this tree.
The fork's ignored 0026 is
`internal/storage/schema/migrations/ignored/0026_add_wisp_dependency_target_indexes.up.sql`
here and in the pin `b7e5e9ce`; the numbered directory beside it holds the
fork's `0026_update_blocked_issues_view`, a migration for the other series.

### The live database levels

The overseer's tracker bird read the Dolt server on 2026-11-19 at about 00:30
UTC, taking `max(version)` from `schema_migrations` and
`ignored_schema_migrations` on each database. All 12 databases — gt, be, claude,
hq, devops, beaver, om, mango, fractals, agentlab, hm and toolbox — are at
main 66 and ignored 26.

That fixes the collision's position: `0067_create_samples` is applied on no live
database yet, while the fork's ignored 0026 is applied on every one.

### What the collision would do

This is the expected result of the cursor model, not a behaviour observed in a
migration runner:

- A binary carrying upstream's ignored 0026 meets a live database whose claimed
  ignored version is already 26. Matching is by number, so it does not see the
  upstream data fix as pending: it applies the versions above 26 and leaves
  `0026_dep_rekey_dedup_marker` expected never to run, with no error.
- In the other direction, once any database has a version 67, a binary from the
  other side treats its own 0067 as done. A fork binary then never creates the
  samples table on a database that took upstream's
  `0067_add_versioned_beads_schema`, and an upstream binary never applies the
  fork's `0067_create_samples`.

Neither is a malformed migration. Each is a correct migration under a number
the other side had already used.

### The rule

- A fork binary is never swapped for an upstream build; the town stays on the
  pinned commit.
- No upstream migration file is taken under a number the fork already used.
- A merge that takes upstream migrations renumbers or re-bases the fork's own
  migrations first — out of the numbers upstream uses — on a restored backup,
  in the attended window: the gt-v5bo2v rehearsal, run in the gt-jgsmoj window.
  It never runs against a live database.

## Reproducing the numbers

The upstream remote is a setting local to your clone, not something this
repository commits. From the fork worktree:

```bash
git remote add upstream https://github.com/gastownhall/beads.git
git fetch upstream --no-tags
git merge-tree --write-tree --name-only --no-messages origin/main upstream/main
```

None of these changes the working tree. The first reports that the remote
already exists if you have run it before. The third prints the paths a merge
would conflict on and exits non-zero when there are any; it reads
`upstream/main` from your local refs, so fetch first.
