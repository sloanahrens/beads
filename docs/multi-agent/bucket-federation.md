---
title: Bucket Federation Quickstart
description: Federate a beads database across machines through a GCS or S3 bucket — remote add, seed push, birth the second replica, and pick a sync cadence
---

Point two machines at one object-storage bucket and you have a beads
federation: no server to run, no hosted account, no ports to open. Dolt speaks
GCS and S3 natively, so the bucket *is* the remote — the same role a git
remote plays for a repo.

This is the BYO-cloud path. [DoltHub](/multi-agent/federation) remains the
zero-config default; reach for a bucket when the data must stay in your own
cloud account, or when you already have one.

## What you end up with

```
   machine one                  gs://my-bucket/beads/myproject                machine two
  ┌─────────────┐                    ┌──────────────┐                      ┌─────────────┐
  │ .beads/dolt │ ◄── push / pull ──►│    bucket    │◄── push / pull ─────►│ .beads/dolt │
  │  (replica)  │                    │  (no server) │                      │  (replica)  │
  └─────────────┘                    └──────────────┘                      └─────────────┘
```

Each machine keeps a full local replica and works offline against it; a timer
runs `bd dolt pull` and `bd dolt push` on both ends. Nothing arbitrates between
them but the bucket.

## Prerequisites

1. **The Dolt backend** (the only supported backend).
2. **A bucket you can write to**, and credentials on both machines:
   - GCS: Application Default Credentials —
     `gcloud auth application-default login` (or a service account via
     `GOOGLE_APPLICATION_CREDENTIALS`).
   - S3: the standard AWS chain — `AWS_PROFILE`, `AWS_ACCESS_KEY_ID`/
     `AWS_SECRET_ACCESS_KEY`, or an instance role.
3. **A Dolt commit identity on every machine** — see
   [Failure modes](#failure-modes); a fresh machine usually has none, and the
   first merge is what discovers it.

## 1. Create the bucket path

One bucket path per beads database. Two databases sharing a path will fight
over the same Dolt history.

```bash
# GCS
gcloud storage buckets create gs://my-bucket --location=us-central1

# S3
aws s3 mb s3://my-bucket
```

No layout to prepare inside it — the first push creates everything.

## 2. Machine one: register the remote and seed it

Run this in the workspace that already holds the database you want to share:

```bash
bd dolt remote add origin gs://my-bucket/beads/myproject
bd dolt push
```

Supported schemes are `gs://`, `s3://` (or Dolt's `aws://`), `az://`,
`dolthub://`, `https://`, `file://`, and git SSH.

Two details worth knowing:

- **Use `bd dolt remote add`, not raw `dolt remote add`.** bd registers the
  remote through the store API so a running Dolt SQL server sees it
  immediately. A remote added with the `dolt` CLI lands in filesystem config
  only, and push/pull then fail with *remote not found* until the server
  restarts.
- **Naming it `origin` also persists `sync.remote`** into `.beads/config.yaml`.
  That is the remote `bd dolt pull` and `bd dolt push` use when you pass no
  `--remote` flag. Any other name works, but every pull and push then needs
  `--remote <name>`.

Verify:

```bash
bd dolt remote list
```

## 3. Machine two: birth the replica from the bucket

```bash
bd init --remote gs://my-bucket/beads/myproject
```

`bd init --remote` clones the Dolt database from the bucket and persists
`sync.remote`, so machine two is ready to pull and push immediately. This is a
*clone*, not an import: no JSONL round-trip, no re-keying, and the full commit
history comes with it.

Verify the birth before you trust it:

```bash
bd dolt remote list          # points at the bucket
bd list --status all --json | jq length   # issue-count parity with machine one
```

An issue that machine one closed *after* the last push will still look open
here. That is federation lag, not a bad clone — it arrives on the next pull.

<Note>
Already have a `.beads/` directory on machine two that you want to replace with
the bucket's copy? Don't clone over it. Move it aside first, then run
`bd init --remote`; see [Init Safety](/recovery/init-safety) for the guard
rails.
</Note>

## 4. Pick a sync cadence

Each replica syncs with two commands, run in this order: pull the bucket's
history, then push your own commits.

```bash
bd dolt pull                 # default remote: fetch and merge the bucket's history
bd dolt push                 # default remote: publish your local commits
bd dolt pull --remote mini   # a specific named remote
bd dolt push --remote mini
```

Pull first. A push from a replica whose history has diverged from the bucket's
is rejected, and `bd dolt push --force` overwrites the bucket's history, so keep
`--force` out of timers.

Run the pair from a timer on both machines. A 60-second cadence is comfortable
at the scale measured below; the pair is a no-op when nothing changed.

```bash
# cron, every minute: push runs only if pull succeeded
* * * * * cd /path/to/workspace && { /usr/local/bin/bd dolt pull && /usr/local/bin/bd dolt push; } >> /tmp/bd-sync.log 2>&1
```

Both commands exit 1 on failure, so a timer can branch on the exit status
without parsing output. A failed pull is not retried by the next tick: a merge
conflict repeats the same failure until someone resolves it (see
[A pull stops on a merge conflict](#a-pull-stops-on-a-merge-conflict)). Alert a
human on a failure that repeats; a transport or auth error usually clears by
itself, a conflict does not.

Three rules for choosing the interval:

- **Staleness is the interval.** Every replica's view of the others is up to
  one full interval old, by construction.
- **Lease TTL and reclaim grace must both exceed the interval.** A lease is
  only meaningful on the replica that granted it, and a reaper on the other
  machine would be judging liveness from data older than the lease. See
  [Leases are per-replica](/multi-agent/federation#leases-are-per-replica),
  and name each replica with `bd config set node_id <name>` so the
  cross-replica reclaim guard arms.
- **A longer interval means more conflicts, not just staler data.**
  `updated_at` is touched by *every* bd mutation, so two replicas editing the
  same issue between pulls conflict even when the fields they changed are
  disjoint. Disjoint edits to *different* issues merge cleanly at any cadence.

## Measured cost

From a production two-machine deployment (laptop + Mac Mini, ~1.3k issues,
~115k chunks, Dolt 2.1.10, GCS remote):

| Operation | Time |
|-----------|------|
| Full cold push (seeding an empty bucket) | 28s |
| Full clone (birthing a replica) | 5s |
| Incremental push | ~4s |
| Pull + merge | ~1s |

A 60-second cadence therefore spends a few seconds per tick, and the steady
state is dominated by no-ops.

## Failure modes

### `bd dolt push` says the remote does not exist

The remote was added with raw `dolt remote add`, so it exists in filesystem
config but not in the SQL server's `dolt_remotes` table. Re-register it:

```bash
bd dolt remote add origin gs://my-bucket/beads/myproject
```

### Pull fails on a fresh machine with no conflicts reported

A machine with no Dolt commit identity cannot author the merge commit a pull
creates, so even a no-op pull fails. It is the most common first-sync failure
on a newly provisioned box:

```bash
dolt config --global --add user.name  "Your Name"
dolt config --global --add user.email "you@example.com"
```

### Auth works in the shell but pull or push fails in server mode

With an external Dolt SQL server, `CALL DOLT_PUSH/PULL` runs *inside the
server process*, which only has the environment it inherited at startup.
Credentials exported afterwards never reach it. bd detects cloud credentials
matching the remote's scheme (`GOOGLE_*`/`GCS_*` for `gs://`, `AWS_*` for
`s3://`/`aws://`, `AZURE_STORAGE_*` for `az://`) and routes push/pull through
a `dolt` CLI subprocess, which inherits the current environment. If the pull
or push still cannot authenticate, restart the server with the credentials in
its environment.

### The remote is named something other than `origin`

A replica born with `bd init --remote` (or `dolt clone`) names its remote
`origin`; a machine where someone added the remote by hand may have named it
anything. Timers must pass the right `--remote` on each machine — or rename
the remote so both ends match.

### A pull stops on a merge conflict

`bd dolt pull` fails with `merge conflicts in <tables> require operator
resolution` when a merge brings in changes it cannot settle safely. It never
auto-resolves what it cannot settle, and it does not push. Repeated pulls keep
failing the same way until an operator resolves the divergence. Where the
conflicts land depends on how the remote is reached:

- **Remotes reached through the `dolt` CLI** (git-protocol and cloud-credential
  remotes) leave the conflicts in the working set. Look at them with
  `bd conflicts list` (which tables and issues) and `bd conflicts show` (field
  by field). Resolve each one with `bd conflicts resolve <issue-id> --ours` or
  `--theirs`, or resolve a whole table with `bd conflicts resolve --all --theirs`.
  The merge is committed once no conflicts remain. If you resolved with
  `--no-commit`, finish it with `bd conflicts resolve --conclude`.
- **Remotes reached through SQL** roll the merge back, so the working set is
  restored and `bd conflicts list` shows nothing. Re-run the pull once the other
  side is fixed, or settle it on the pull itself with
  `bd dolt pull --strategy ours` or `--strategy theirs` (embedded storage only).

After the merge is settled, push it with `bd dolt push`, then let the timer
resume.

### One bucket path, two databases

A bucket path is a single Dolt history. Pointing a second, unrelated beads
database at the same path produces a divergence no merge can reconcile. Give
each database its own path.

## Reference

- [Federation Setup](/multi-agent/federation) — peers, sovereignty, topologies
- [Dolt Architecture](/architecture/dolt) — remotes, push/pull, storage layout
- [`bd dolt`](/cli-reference/dolt) · [`bd init`](/cli-reference/init)
- [Sync Failures](/recovery/sync-failures) — recovering a wedged sync
