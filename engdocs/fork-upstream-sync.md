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
