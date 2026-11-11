-- samples: the generic observation plane behind `bd sample add` and
-- `bd sample list` (be-1qqu).
--
-- An external orchestrator that must not own a table in the beads schema
-- (gastown ADR 0001: bd is the single owner of schema and invariants) keeps
-- cost and usage rows here through the CLI. This is a VERSIONED table on the
-- ordinary main-plane path, not a clone-local dolt_ignored one: the rows are
-- project data a sibling process and a teammate's clone must read, unlike the
-- per-clone journal (0064) and audit events (0062).
--
-- payload is LONGTEXT, NOT the JSON column type, and the difference is the
-- contract rather than the size. The role promises that a row lists back the
-- exact bytes it was given, so a consumer can compare samples without a
-- normalization step; MySQL's JSON type re-encodes its input, which reorders
-- keys and drops whitespace. LONGTEXT also clears the 64 KiB cap: TEXT stops
-- at 65535 and the cap is 65536 (samplesops.MaxPayloadBytes).
--
-- at is event time in UTC with microsecond precision (DATETIME(6)); created_at
-- is ingest time, written explicitly by the role so the value Add returns is
-- the value stored. Neither column carries a DEFAULT or any other time
-- function, so this file has nothing for check B of
-- scripts/check-migration-hygiene.sh to justify and the Dolt CLI bundle needs
-- no override in cli_migrations.go.
--
-- idx_samples_kind_at serves the only read the plane has: one kind, newest
-- first.
CREATE TABLE IF NOT EXISTS samples (
    id CHAR(36) NOT NULL,
    kind VARCHAR(64) NOT NULL,
    at DATETIME(6) NOT NULL,
    payload LONGTEXT NOT NULL,
    created_at DATETIME(6) NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_samples_kind_at (kind, at)
);
