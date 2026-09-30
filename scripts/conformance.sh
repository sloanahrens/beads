#!/usr/bin/env bash
#
# Single conformance entrypoint. CI runs this verbatim; run it locally the same way:
#
#   ./scripts/conformance.sh
#
# Tier 1 exercises the storage conformance contract in-process against the Dolt
# server backend. Tier 2, the real-binary CLI corpus, awaits a server-mode
# reference profile.
#
set -euo pipefail
cd "$(dirname "$0")/.."

TAGS="gms_pure_go"

# assert_conformance_passed LOGFILE LABEL
# Fail the gate unless the top-level TestConformance in LOGFILE ran and PASSED. The
# checks MUST anchor to column-0 result lines: `go test -v` indents subtest results, and
# RunAll legitimately skips backend-inapplicable subtests, so an unanchored `--- SKIP: TestConformance`
# grep would match an indented subtest and false-fail the gate even though the top-level
# suite passed. Deletes LOGFILE either way.
assert_conformance_passed() {
  local log="$1" label="$2"
  if grep -qE '^--- SKIP: TestConformance' "$log" || ! grep -qE '^--- PASS: TestConformance' "$log"; then
    rm -f "$log"
    echo "FATAL: $label conformance (top-level TestConformance) skipped or did not run; storage-parity gate is not enforced" >&2
    exit 1
  fi
  rm -f "$log"
}

echo "==> Tier 1: in-process store conformance against the Dolt server backend"
# The Dolt server backend runs the full backend-agnostic suite (conformance.RunAll).
# Its TestMain needs a Dolt container and fails loudly without one (be-r18), so a
# missing container cannot turn this gate green. Run it with -v and fail loudly if
# the top-level TestConformance skips or reports no pass.
# (assert_conformance_passed explains why the skip/pass checks anchor to column-0 lines.)
dolt_log="$(mktemp)"
CGO_ENABLED=1 go test -tags "$TAGS" -v \
  -timeout 30m ./internal/storage/dolt/ -run '^TestConformance$' | tee "$dolt_log"
assert_conformance_passed "$dolt_log" "Dolt server"

# Tier 2 (test/conformance, the real-binary CLI corpus) used embedded Dolt as its
# only reference profile. Embedded Dolt was removed, so the corpus has no
# reference to diff against until it gains a server-mode profile.
echo "==> Tier 2: skipped (test/conformance has no server-mode reference profile yet)"

echo "==> conformance OK"
