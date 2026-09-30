# Fast Unit Tier Implementation Plan (be-b23, wayfinder D9)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `make test` runs a unit tier that never migrates a Dolt store, never starts a Dolt server and never needs Docker, and `make test-integration` runs every real-store test in require-mode.

**Architecture:** A runtime tripwire, not a convention. `BD_TEST_TIER=unit` (exported by the hermetic test env) makes `schema.MigrateUp` refuse pending migration work and `doltserver.Start` refuse to start a server, in the test process and in every spawned `bd`. A unit-tier test that opens a fresh store therefore fails with a message naming the tier. Tests that need a real store move behind the existing `integration` build tag and run in `make test-integration`, where infra-missing skips become failures.

**Tech Stack:** Go test, build tags, bash test runner (`scripts/test.sh`, `scripts/ci/lib/test-env.sh`), Make.

**Spec:** epic be-b23 (children be-b23.1 to be-b23.4) and the operator brief for this branch; evidence in deep review B2-08, B2-13, B5-17.

## Global Constraints

- Do not weaken any assertion. Do not delete tests except named duplicates.
- Never touch the live Dolt server on :3307; stop any Dolt server a test run starts.
- The embedded storage stack and the proxied stack are being deleted on another branch (be-c94.1, be-xu2.2). Change their tests only as test wiring.
- `BD_TEST_TIER` is `BD_`-prefixed because the cmd/bd subprocess helpers strip `BEADS_*` but keep `BD_*`. Production never sets it.
- Gates by exit code: `go build ./...`, `go vet ./...`, `make ci-pr-lint`, `make test`, `make test-integration`.

## Measurements (2026-09-29, shared host, load 16 to 99)

All runs use `./scripts/test.sh` (the `make test` runner, without coverage) and are judged by exit code.

| Run | Wall | cmd/bd | Exit |
|---|---|---|---|
| Before: origin/main, full suite | 945 s | 793 s | 1 (internal/utils import cycle, pre-existing) |
| After: unit tier, full suite | 222 s | 168 s | 0 |

Before the change the unit env still ran every cmd/bd test that spawns `bd init`: each creates an embedded store and runs the full migration chain (8.4 s for one init on this host under load). The tripwire named 37 such tests in cmd/bd and 50 more in internal/storage/embeddeddolt, internal/tracker and scripts/repro-dolt-prod-timeouts.

---

### Task 1: The tripwire

**Files:**
- Create: `internal/testtier/testtier.go`, `internal/testtier/testtier_test.go`
- Modify: `internal/storage/schema/schema.go` (MigrateUp, after the no-work short-circuit)
- Modify: `internal/doltserver/doltserver.go` (top of `Start`)
- Test: `internal/storage/schema/testtier_guard_test.go`, `internal/doltserver/testtier_guard_test.go`

**Interfaces:**
- Produces: `testtier.EnvVar = "BD_TEST_TIER"`, `testtier.Unit() bool`, `testtier.Refuse(op string) error` wrapping `testtier.ErrUnitTier`.

- [x] Failing tests: `TestRefuseInUnitTier`, `TestMigrateUpRefusesPendingMigrationsInUnitTier` (sqlmock: seed no-op, cursor at v42, expect ErrUnitTier before any dolt_status query), `TestStartRefusedInUnitTier`.
- [x] Implement; run the three packages green.

### Task 2: The hermetic env exports the tier

**Files:**
- Modify: `scripts/ci/lib/test-env.sh` (after the BD_ sweep: `unit` unless `BEADS_TEST_ENV_RUN_DOLT=1`, then `integration`)
- Test: `scripts/test_env_hermetic_test.go` `TestHermeticEnvExportsTestTier` (a caller-exported tier is swept and replaced)

- [x] Failing test, implement, green.

### Task 3: Move the 33 offenders to the integration tier

Whole-file moves add `integration` to the build constraint (`//go:build cgo && integration`). Mixed files move only the offending tests, verbatim, into a sibling `<stem>_integration_test.go`; helpers used only by moved tests move with them.

| File | Offenders / tests | Move |
|---|---|---|
| close_last_touched_test.go | 1/1 | tag file |
| cook_qnt_test.go | 2/2 | tag file |
| create_embedded_test.go | 4/16 | split |
| export_auto_test.go | 3/42 | split |
| init_metrics_test.go | 4/10 | split |
| init_noninteractive_test.go | 1/3 | split |
| last_touched_guard_test.go | 1/1 | tag file |
| migration_freeze_gate_test.go | 7/7 | tag file |
| mol_bond_gwn_test.go | 1/1 | tag file |
| prime_gemini_hook_test.go | 1/1 | tag file |
| serve_registered_backend_test.go | 1/3 | split |
| update_multi_id_exit_test.go | 4/4 | tag file |
| update_stray_positional_test.go | 2/3 | split |
| where_cgo_test.go | 1/1 | tag file |

- [x] Move; `go vet -tags gms_pure_go ./cmd/bd` and `go vet -tags gms_pure_go,integration ./cmd/bd` both clean. The integration build of cmd/bd did not compile on main (two `runBD` helpers); reparent_test.go's is renamed.
- [x] `./scripts/test.sh ./cmd/bd` exits 0 with the tripwire armed.
- [x] Run the whole unit tier; moved the further offenders it named: embeddeddolt (16 files tagged, 3 split), tracker (1 file), repro-dolt-prod-timeouts (1 test). The sqlmock `MigrateUp` tests in internal/storage/schema clear the tier per test.
- [x] Several cmd/bd helpers strip every `BEADS_*` and `BD_*` variable, so the variable missed their subprocesses. `scripts/test.sh` now links the tier into the prebuilt bd (`-X .../testtier.buildTier`); that surfaced four more offenders, moved the same way.

### Task 4: Integration tier target and require-mode

**Files:**
- Modify: `scripts/test.sh` (a `TEST_TAGS` knob appended to `gms_pure_go`)
- Modify: `Makefile` (`test-integration`; `make test` unchanged in meaning: the unit tier)
- Modify: `internal/testutil/testdoltcommon.go`, `internal/testutil/testdoltserver.go` (require-mode: under `BD_TEST_TIER=integration` an infra skip becomes `t.Fatal`, except a missing `dolt` binary)
- Test: `internal/testutil/require_mode_test.go`

`make test-integration` = `BEADS_TEST_ENV_RUN_DOLT=1 BEADS_TEST_EMBEDDED_DOLT=1 TEST_TAGS=integration TEST_TIMEOUT=45m ./scripts/test.sh ./...`.

- [x] Failing test for require-mode in the central helpers, implement, green. The server-unavailable and container-crashed skip sites outside the embedded and proxied stacks (140) go through `testutil.SkipOrFailUnavailable`.
- [x] Docs: `engdocs/TESTING.md` names the two tiers, the tripwire, and how to move a test.

### Task 5: Policy test

`TestUnitTierPolicy` (in `scripts/`) asserts the wiring: `make test` runs `scripts/test.sh` without opting into Dolt, `make test-integration` opts in, the runner links `testtier.buildTier` into the prebuilt bd, and that symbol exists (a `-X` on a missing symbol is silently ignored). `TestPrebuiltBDCarriesUnitTier` (cmd/bd) spawns the prebuilt bd with only `PATH` and `HOME` and requires `bd init` to be refused. The tripwire itself is the per-test policy: any new unit-tier test that opens a fresh migrated store fails with `ErrUnitTier`. Both tests were mutation-checked.

### Task 6: Measure, gate, review

- [ ] After: `make test` wall, `./cmd/bd` unit wall, `make test-integration` wall, all by exit code.
- [ ] `go build ./...`, `go vet ./...`, `make ci-pr-lint`.
- [ ] `om review -base origin/main`; fix blockers and majors.

## Out of scope on this branch

- A copied pre-initialized workspace for `bd init` subprocess tests. Each init mints a random `project_id` that is also the store's identity, so a byte copy would give unrelated tests one identity; fixing that means rewriting identity inside embedded stores, the stack be-c94.1 deletes. The integration tier's wall times decide any fixture work after that deletion.

- be-b23.1, the narrow public client interface and in-memory fake: an API design change that depends on D7 deleting the legacy verbs; doing it now collides with the delete branch.
- A production fresh-database fast path (one commit for a fresh schema, B2-13a): it changes the migration crash-recovery contract (#4566) and needs its own review.
- CI workflow consolidation to one `make gate` (be-b23.4 beyond the Makefile targets): CI-green work is paused per the D9 resolution.
