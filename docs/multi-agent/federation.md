---
title: Federation Setup Guide
description: Configure peer-to-peer sync of beads databases across workspaces with Dolt remotes, sovereignty tiers, and topologies
---

Federation enables peer-to-peer synchronization of beads databases between
multiple workspaces using Dolt remotes. Each workspace maintains its own database
while sharing work items with configured peers.

## Overview

Federation uses Dolt's distributed version control capabilities to sync issue
data between independent teams or locations. Key benefits:

- **Peer-to-peer**: No central server required; each town is autonomous
- **Database-native versioning**: Built on Dolt's version control, not file exports
- **Flexible infrastructure**: Works with DoltHub, S3, GCS, local paths, or SSH
- **Data sovereignty**: Configurable tiers for compliance (GDPR, regional laws)

<Note>
Just want two machines sharing one database through a bucket you own? The
[Bucket Federation Quickstart](/multi-agent/bucket-federation) walks that path
end to end — bucket, seed push, second replica, sync cadence — with measured
timings and the failure modes.
</Note>

## Prerequisites

1. **Dolt backend**: Federation requires the Dolt storage backend (the only supported backend)

## Configuration

### Enable Federation-Compatible Sync

Edit `.beads/config.yaml` or `~/.config/bd/config.yaml`:

```yaml
federation:
  remote: dolthub://myorg/beads          # Primary remote (optional)
  sovereignty: T2                        # Data sovereignty tier
```

Or via environment variables:

```bash
export BD_FEDERATION_REMOTE="dolthub://myorg/beads"
export BD_FEDERATION_SOVEREIGNTY="T2"
```

### Data Sovereignty Tiers

| Tier | Description | Use Case |
|------|-------------|----------|
| T1 | No restrictions | Public data |
| T2 | Organization-level | Regional/company compliance |
| T3 | Pseudonymous | Identifiers removed |
| T4 | Anonymous | Maximum privacy |

## Adding a Federation Peer

A peer is a Dolt remote: each replica publishes its history to a location the
others can reach, and every workspace registers that location with
`bd dolt remote add`. There is no separate peer registry to keep in step.

```bash
bd dolt remote add <name> <url>
```

### Remote Name Rules

- Must start with a letter
- Alphanumeric, dash, and underscore only
- Maximum 64 characters

### Supported Endpoint Formats

| Format | Example | Description |
|--------|---------|-------------|
| DoltHub | `dolthub://org/repo` | DoltHub hosted repository |
| Google Cloud | `gs://bucket/path` | Google Cloud Storage |
| Amazon S3 | `s3://bucket/path` | Amazon S3 |
| Azure Blob | `az://account.blob.core.windows.net/container/path` | Azure Blob Storage |
| Local | `file:///path/to/backup` | Local filesystem |
| HTTPS | `https://host/path` | HTTPS remote |
| SSH | `ssh://host/path` or `git+ssh://host/path` | SSH remote |
| Git SSH | `git@host:path` | Git SSH shorthand |

### Examples

```bash
# Name the shared DoltHub remote origin, so pull and push use it with no flag
bd dolt remote add origin dolthub://myorg/beads

# A named remote for one peer
bd dolt remote add partner-town dolthub://partner-org/beads

# Other endpoints work the same way
bd dolt remote add backup gs://mybucket/beads-backup
bd dolt remote add backup-s3 s3://mybucket/beads-backup
bd dolt remote add local file:///home/user/beads-backup
```

Adding a remote named `origin` also persists it as `sync.remote` in
`.beads/config.yaml`, which is how every later pull and push finds it without
a `--remote` flag.

### Credentials

Credentials are never stored by `bd`; each pull or push authenticates through
the standard mechanism for the remote's scheme:

| Remote | Credentials |
|--------|-------------|
| DoltHub | `DOLT_REMOTE_USER` and `DOLT_REMOTE_PASSWORD` |
| `gs://` | Application Default Credentials (`gcloud auth application-default login`, or `GOOGLE_APPLICATION_CREDENTIALS`) |
| `s3://` | The standard AWS chain (`AWS_PROFILE`, `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`, or an instance role) |
| `az://` | `AZURE_STORAGE_*` variables |

The [Bucket Federation Quickstart](/multi-agent/bucket-federation) walks the
cloud cases end to end, including the failure modes.

### JSON Output

For scripting, use the `--json` flag:

```bash
bd --json dolt remote add staging gs://mybucket/staging-beads
# {"name":"staging","url":"gs://mybucket/staging-beads"}
```

### Verify Configuration

List configured remotes:

```bash
bd dolt remote list
```

## Syncing with Peers

Each replica syncs with two commands, run in this order: pull the remote's
history, then push your own commits.

```bash
bd dolt pull                 # fetch and merge the remote's commits
bd dolt push                 # publish your local commits

# A specific named remote
bd dolt pull --remote town-beta
bd dolt push --remote town-beta
```

Pull first. A push from a replica whose history has diverged from the remote's
is rejected, and `bd dolt push --force` overwrites the remote's history — keep
`--force` out of timers and scripts.

A pull that hits a merge conflict the auto-resolver declines stops and reports
the conflicting tables. On embedded storage, re-run it with
`--strategy theirs` or `--strategy ours` to settle the conflict; on server
storage, run `bd conflicts resolve` after the pull reports it.

### Topologies

| Pattern | How | Use Case |
|---------|-----|----------|
| Hub-spoke | One shared remote; every replica names it `origin` | Team with central coordination |
| Mesh | Each replica owns a remote and names the others as extra remotes | Decentralized collaboration |
| Hierarchical | One remote per team, plus a remote above them | Multi-team organizations |

## Architecture Notes

### How It Works

1. Each workspace keeps its own Dolt database — a full replica
2. `bd dolt remote add` registers the shared location (similar to `git remote add`)
3. `bd dolt pull` merges the remote's commits locally; `bd dolt push` publishes local commits
4. Dolt merges cell by cell; a conflict the auto-resolver declines halts the pull for manual resolution

When run against a Dolt SQL server, federation uses two ports: MySQL (3306)
for multi-writer SQL access, and remotesapi (8080) for peer-to-peer
push/pull:

```
┌─────────────────┐         ┌─────────────────┐
│  Workspace A    │◄───────►│  Workspace B    │
│  dolt sql-server│  sync   │  dolt sql-server│
│  :3306 (sql)    │         │  :3306 (sql)    │
│  :8080 (remote) │         │  :8080 (remote) │
└─────────────────┘         └─────────────────┘
```

### Connectivity

Remote connectivity is validated on the first pull or push, not when the
remote is added. This lets you configure remotes before the infrastructure
exists.

### Leases are per-replica

A claim lease (`bd ready --claim` + `bd heartbeat`, reaped by `bd reclaim`) is
only meaningful on the replica that granted it. The `leases` table is
clone-local and never replicates; what crosses the bridge is the claim's
*visibility* — `status`/`assignee` on the issue row — and that is stale on
every other replica by up to one sync interval.

Two rules follow, and a federated deployment owes both:

1. **Grace window > sync interval, and lease TTL > sync interval.** A TTL or
   `bd reclaim --older-than` grace shorter than the cadence at which replicas
   exchange state is meaningless across the bridge: the remote view is a full
   interval old by construction, so a reaper over there would be judging
   liveness from data older than the lease itself. `bd reclaim` defaults its
   grace to 2× the lease TTL; raise the TTL (or the grace) above your sync
   interval, never shrink the interval to fit them.
2. **Reclaim belongs to the granting replica.** Each lease records the replica
   that granted it, and `bd reclaim` skips a lease granted elsewhere, naming
   it on stderr. Reap dead workers on the machine that hired them.

The guard is **opt-in**: it arms only where you name this replica.

```bash
export BEADS_NODE_ID=mini          # per-machine; or `bd config set node_id mini`
```

Two rules about what to name, both load-bearing:

- **`node_id` names the STORE, not the host.** One value per beads *database*.
  Hosts that are clients of the *same* dolt sql-server (`BEADS_DOLT_SERVER_HOST`,
  a systemd/Docker server, Hosted Dolt, a VPS) are **one replica** no matter how
  many machines they are — give them all the same value, or leave it unset. Give
  them distinct ids and you rebuild the very fail-closed regression described
  below: a supervisor would match no worker's lease and reclaim 0 forever. Name a
  replica only where there is a real sync interval between it and the others.
- **`node_id` is per-machine, so it must never be committed.** The project
  `.beads/config.yaml` is a git-**tracked** file. A `node_id` committed there
  propagates one machine's identity to every clone that pulls it, and then every
  comparison matches: the guard is fully *armed* and fully *inert*, and `laptop`
  reaps `mini`'s leases exactly as if they were local — the precise hazard this
  feature exists to close, now happening while you believe you are protected.
  That is worse than not setting it at all. `bd config set node_id` therefore
  writes the **user-global** `~/.config/bd/config.yaml`, alongside the other
  per-machine state (`sync-state.json`, `push-state.json`, `redirect`). Use the
  env var or that command; never hand-add `node_id` to `.beads/config.yaml`.

There is deliberately no hostname fallback. The hostname answers the wrong
question — it names the client *process's* machine, not the store — and
guessing gets it wrong in the topologies that most need automated reclaim:
with a shared or remote dolt sql-server (`BEADS_DOLT_SERVER_HOST`, Hosted
Dolt, a VPS) many hosts are clients of ONE store with no sync interval
between them, so a per-hostname identity would stop a supervisor reaping any
worker's lease at all; in a container the hostname is a per-run container ID;
on macOS the transient hostname follows the network. Each of those would
strand work on a deployment with no federation at all — a worse failure than
the one this guard prevents.

So an unset identity degrades to the old behavior (every lease treated as
local) rather than failing closed: an upgrade, and any single-store
deployment, can never strand a lease the reaper could previously recover.
Leases granted before this feature landed likewise carry no replica and stay
reclaimable until a heartbeat re-stamps them with a configured node.

`bd reclaim --any-replica` disarms the guard. It is for a replica that is
permanently gone (or a node that was renamed and now sees its own old leases
as foreign) — not a normal setting, since only the granting machine has a
first-hand view of whether the holder is alive.

**A heartbeat proves the holder is alive; it does not move the lease.** An
ordinary heartbeat only *backfills* the granting replica when it is still empty
— it does not overwrite a row that positively names one. So a lease normally
keeps its granting replica for life, and states like these strand one where a
local heartbeat is keeping it alive:

- a replica that was **renamed** (`mini` → `mini2`) heartbeats its own leases,
  which now read foreign forever;
- a foreign lease that arrived through the JSONL interchange, whose holder name
  also exists locally, gets heartbeated here but stays labelled with the remote
  node;
- an import that lands on an already-**expired** local lease row takes the whole
  row from the snapshot, granting replica included — so this node's own stale
  lease can come back labelled with a remote one.

(The one path that moves a lease the other way is a heartbeat whose holder is a
different *spelling* of the lease row's holder: it re-arms through the upsert,
which stamps this node.)

Recover a stranded lease with `bd reclaim --any-replica`, once you have
confirmed the granting replica is not still reaping — that confirmation is the
whole point of the guard, so prefer the narrow forms:
`bd reclaim --any-replica --id <id>` for one issue, or `bd unclaim --force
<id>`; the bare global form reverts *every* foreign stale lease, live peers
included. `bd reclaim` names what it declined on stderr — one summary line per
run, with `bd -v` expanding it to the first 20 leases individually.

## Troubleshooting

### The remote URL matches the git origin

`bd dolt remote add` refuses a URL that matches the repository's git origin,
since a Dolt remote and the git remote are different stores. Add
`--allow-git-origin` when the layout is intentional (a monorepo that shares
one URL), and the add proceeds with a warning.

### A pull stops on a merge conflict

Dolt halts rather than auto-resolving a conflict it cannot settle safely, and
re-running the same pull halts the same way until someone resolves the
divergence. See [A pull stops on a merge
conflict](/multi-agent/bucket-federation#a-pull-stops-on-a-merge-conflict).

### Invalid endpoint format

Ensure your endpoint matches one of the supported formats above. The scheme
must be one of: `dolthub://`, `gs://`, `s3://`, `az://`, `file://`,
`https://`, `ssh://`, or git SSH format (`git@host:path`).

### General health check

```bash
bd doctor --deep
```

## Reference

- [Configuration](/reference/configuration) — every `federation.*` setting
- [`bd dolt`](/cli-reference/dolt) — remote, pull, push, and status commands
- [Bucket Federation Quickstart](/multi-agent/bucket-federation) — two machines,
  one bucket, end to end
