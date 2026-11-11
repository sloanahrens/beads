// Package samplesops describes the workspace's GENERIC SAMPLES PLANE: the
// append-only rows behind `bd sample add` and `bd sample list`, which an
// external orchestrator writes through the bd CLI instead of owning a table in
// the beads schema.
//
// It is a sibling of issueops, journalops and memoryops rather than more
// methods on any of them, and the reason is not that the role rule forbids
// appending (it does): a sample is not a bead, a journal record, or a setting.
// It is an OBSERVATION stamped at a moment — a provider balance, a plan
// reading, a usage record — whose payload this package never interprets. The
// CLI exists so a caller that must not touch the beads schema (gastown ADR
// 0001: bd is the single owner of schema and invariants) can still keep those
// rows in Dolt, next to the issues they describe.
//
// THE PAYLOAD IS BYTES, NOT A DECODED VALUE, and that is deliberate. The
// contract is that a row lists back exactly what was written, so the plane
// stores the caller's JSON text verbatim and hands it back verbatim. The role
// validates the SHAPE once, at write time — a JSON object, size-capped — and
// never re-serializes it: re-encoding would reorder keys and drop whitespace,
// which are exactly the differences a consumer comparing two samples would
// notice. The size cap is on the bytes given, not on any normalized form.
//
// KINDS ARE OPAQUE STRINGS. The first ones gastown uses are deepseek_balance,
// claude_plan and usage_record (operator ruling 2026-11-11), but the plane
// deliberately does not enumerate them: bd validates the shape of a kind and
// leaves its vocabulary to the producer. A registry here would make every new
// producer a bd release.
//
// THERE IS NO DELETE OR RETENTION VERB. The plane is append-only; retention is
// a decision for whoever produces the rows, and the first kind that needs one
// gets its own role conversation rather than a delete on this one.
package samplesops
