---
title: "bd land-record"
description: "Record a landing or a landing rejection on a work bead"
---

{/* AUTO-GENERATED: do not edit manually */}

Generated from `bd help --doc land-record`.

Record the outcome of landing a work bead's branch, in one write.

A landing writes metadata.landing = &#123;patch_id, landed_commit, gate_result,
om_verdict, om_score, route, recorded_at, recorded_by&#125; and removes the
"rework" label:

  bd land-record &lt;id&gt; --patch-id P --landed-commit C --gate-result pass \
      --om-verdict approve --om-score 8.5 --route merge-queue

A rejection writes metadata.landing_rejection = &#123;kind, gate_tail,
om_findings, conflicting_files, recorded_at, recorded_by&#125; and adds the
"rework" label:

  bd land-record &lt;id&gt; --reject --kind gate_failed --gate-tail-file tail.txt \
      --om-findings '[...]' --conflicting-file a.go --conflicting-file b.go

The metadata edit and the label edit land in one transaction. Read the record
back with bd show &lt;id&gt; --json (.metadata.landing, .metadata.landing_rejection).
Each call replaces the previous record of the same kind.

```
bd land-record <id> [flags]
```

**Flags:**

```
      --conflicting-file stringArray   Rejection: a file that conflicted (repeatable)
      --gate-result string             Landing: quality-gate result (e.g. pass)
      --gate-tail string               Rejection: tail of the failing gate output
      --gate-tail-file string          Rejection: read the gate tail from a file (- for stdin)
      --kind string                    Rejection: what failed (e.g. gate_failed, om_rejected, conflict)
      --landed-commit string           Landing: commit that landed on the target branch
      --om-findings string             Rejection: om findings as a JSON value
      --om-score float                 Landing: om review score
      --om-verdict string              Landing: om review verdict
      --patch-id string                Landing: git patch-id of the landed change
      --reject                         Record a landing rejection instead of a landing (adds the rework label)
      --route string                   Landing: how the change landed (e.g. merge-queue, direct)
```
