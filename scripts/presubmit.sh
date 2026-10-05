#!/usr/bin/env bash
# Fast presubmit: lint, build, and the tests of only the packages that changed.
#
# `gt done` runs this on the rebased branch before pushing it for landing, so
# it has to stay cheap: the whole unit tier takes 10+ minutes, and the Forgejo
# gate runs `make gate` (lint plus that full tier) on the candidate anyway.
# This target is a subset of that tier, never a substitute for it.
#
# Two rules keep it cheap and safe:
#
#   * Only the packages whose Go files differ from origin/main are tested. A
#     branch that changes no Go file runs lint and build only.
#   * Nothing here starts a Docker container or a Dolt sql-server. A package
#     whose tests cannot run without one is skipped, and the test run itself
#     uses the unit tier's hermetic environment (scripts/ci/lib/test-env.sh),
#     so a Dolt-backed test that slips past the static scan skips instead of
#     starting infrastructure.
#
# Usage: presubmit.sh [--list]
#   --list  print the base ref, the packages that would be tested and the ones
#           that would be skipped, then exit without running anything.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# PRESUBMIT_REPO_ROOT lets scripts/presubmit_test.go drive the selection
# against a throwaway git repo. Everything else (build flags, the hermetic
# test env) still comes from this checkout, so only the tree being diffed and
# tested moves.
REPO_ROOT="${PRESUBMIT_REPO_ROOT:-$(cd "$SCRIPT_DIR/.." && pwd)}"

# shellcheck source=../.buildflags
source "$SCRIPT_DIR/../.buildflags"
# shellcheck source=ci/lib/test-env.sh
source "$SCRIPT_DIR/ci/lib/test-env.sh"

list_only=0
case "${1:-}" in
    --list) list_only=1 ;;
    "") ;;
    *)
        printf 'usage: %s [--list]\n' "$0" >&2
        exit 2
        ;;
esac

cd "$REPO_ROOT"

# origin/main is the canonical comparison point: a polecat branch is rebased
# onto it before `gt done` runs this. A checkout that never fetched a remote
# falls back to the merge base with the local main branch. The diff is taken
# against the working tree, so uncommitted edits count too — an untracked file
# does not (add it first if it is part of the change).
base_ref=""
resolve_base_ref() {
    if git rev-parse --verify --quiet refs/remotes/origin/main >/dev/null; then
        base_ref="origin/main"
        return 0
    fi
    if git rev-parse --verify --quiet main >/dev/null; then
        base_ref="$(git merge-base main HEAD)"
        return 0
    fi
    return 1
}

if ! resolve_base_ref; then
    echo "presubmit: no origin/main and no main branch to diff against; cannot tell which packages changed" >&2
    exit 1
fi

# A changed file's package is the directory that holds it, so no `go list` (and
# no compilation) is needed to name the packages. `.` is the module root. -z
# keeps a path with a space or a quote in it one record long.
changed_go_dirs() {
    git diff --name-only -z "$base_ref" -- '*.go' | while IFS= read -r -d '' path; do
        [[ -n "$path" ]] || continue
        dirname "$path"
    done | sort -u
}

# A package is skipped when its own default-build test files wire up a Docker
# container or a local Dolt sql-server: a TestMain calling
# EnsureDoltContainerForTestMain, a direct doltserver.Start, or testcontainers.
# The unit tier this target runs under starts none of that, and a suite that
# brings its own server has nothing left to check here — the Forgejo gate runs
# the whole tier on the candidate. This is deliberately conservative: cmd/bd
# and the module root skip too, because their TestMains do build a container,
# and cmd/bd's suite is only fast when scripts/test.sh has prebuilt the bd
# binary it spawns — a bare `go test ./cmd/bd` is the pathological slow case
# this target exists to avoid.
#
# Test files behind //go:build integration are ignored: `make test` does not
# build them either, so a package that keeps its heavy tests there (and wires
# no container in the default build) is still tested.
#
# This is a text scan, not a parse: whole-line comments are dropped so a test
# that only documents the wiring (`// doltserver.Start() writes ...`) is not
# mistaken for one that starts a server.
package_needs_infra() {
    local file
    while IFS= read -r -d '' file; do
        if grep -qE '^//go:build .*\bintegration\b' "$file"; then
            continue
        fi
        if grep -vE '^[[:space:]]*//' "$file" |
            grep -qE 'EnsureDoltContainerForTestMain\(|doltserver\.Start\(|testcontainers'; then
            return 0
        fi
    done < <(find "$1" -maxdepth 1 -name '*_test.go' -print0)
    return 1
}

packages=()
while IFS= read -r dir; do
    pkg="."
    [[ "$dir" == "." ]] || pkg="./$dir"

    # The examples are their own Go modules, so neither this target's `./...`
    # build nor the gate's reaches them; naming one on the go command line
    # would just fail with "main module does not contain package". "." is this
    # module's root, whose go.mod is the expected one, not a nested module.
    if [[ "$dir" != "." && -f "$dir/go.mod" ]]; then
        echo "presubmit: skip $pkg: separate Go module, outside the root module's ./..."
        continue
    fi

    if package_needs_infra "$dir"; then
        echo "presubmit: skip $pkg: its tests need a Docker container or a Dolt server; the Forgejo gate covers it"
        continue
    fi

    packages+=("$pkg")
    echo "presubmit: run $pkg"
done < <(changed_go_dirs)

echo "presubmit: base $base_ref; ${#packages[@]} package(s) to test"

if ((list_only)); then
    exit 0
fi

echo "presubmit: lint (make ci-pr-lint)"
make ci-pr-lint

echo "presubmit: build (go build -tags $BEADS_BUILD_TAGS ./...)"
go build -tags "$BEADS_BUILD_TAGS" ./...

if ((${#packages[@]} == 0)); then
    echo "presubmit: no testable changed Go package; lint and build only"
    exit 0
fi

# The same hermetic environment `make test` uses: BEADS_TEST_SKIP=dolt plus
# BD_TEST_TIER=unit, so a test that reaches for a real store fails loudly
# instead of silently opening one, and an isolated HOME keeps the run from
# touching this host's beads state.
beads_test_env_enter

echo "presubmit: test (go test -tags $BEADS_BUILD_TAGS ${packages[*]})"
go test -tags "$BEADS_BUILD_TAGS" ${packages[@]+"${packages[@]}"}
