---
title: "bd sample"
description: "Store and read generic timestamped samples"
---

{/* AUTO-GENERATED: do not edit manually */}

Generated from `bd help --doc sample`.

Store and read generic timestamped samples — JSON observations such as a
provider balance or a usage record — in the beads database.

A sample has a kind, an event time and a JSON-object payload. Kinds are opaque
strings chosen by the producer; bd validates their shape, never their
vocabulary. The database stores the payload bytes as given; bd sample list
--json embeds the stored value as compact JSON, so whitespace and key order are
normalized on output.

The plane is append-only: there is no delete or retention verb.

```
bd sample [command]
```

## bd sample add

Append one sample row.

  bd sample add --kind &lt;kind&gt; --at &lt;rfc3339&gt; --json &lt;file|-&gt;

--at is the event time as an RFC3339 timestamp and is stored in UTC. --json is
the payload: a path to a file holding a JSON object, or "-" to read standard
input. The payload is size-capped at 65536 bytes, validated as a JSON object,
and stored as the exact bytes given.

Examples:
  bd sample add --kind deepseek_balance --at 2026-11-11T09:30:00Z --json balance.json
  printf '&#123;"used":1200&#125;' | bd sample add --kind usage_record --at 2026-11-11T09:31:00Z --json -

```
bd sample add [flags]
```

**Flags:**

```
      --at string     Event time as an RFC3339 timestamp (required; stored in UTC)
      --json string   Payload source: a file holding a JSON object, or - for standard input (required)
      --kind string   Sample kind (required)
```

## bd sample list

List one kind's samples, newest first by event time.

  bd sample list --kind &lt;kind&gt; [--since &lt;rfc3339&gt;] [--limit N]

--since keeps rows whose event time is at or after it (inclusive); --limit 0,
the default, is uncapped. With --json the output is the usual envelope:

  &#123;"schema_version": 1,
   "data": [&#123;"id": "...", "kind": "...", "at": "2026-11-11T09:30:00Z",
             "payload": &#123;"used": 1200&#125;, "created_at": "..."&#125;]&#125;

at and created_at are RFC3339 timestamps in UTC; payload is the stored JSON
object embedded as compact JSON — the value is unchanged, but the encoder
normalizes whitespace and key order, so the bytes may differ from those added.

Examples:
  bd sample list --kind deepseek_balance --limit 10 --json
  bd sample list --kind usage_record --since 2026-11-11T00:00:00Z

```
bd sample list [flags]
```

**Flags:**

```
      --kind string    Sample kind to list (required)
      --limit int      Maximum rows to return (0 = uncapped)
      --since string   Keep rows whose event time is at or after this RFC3339 timestamp
```
