#!/usr/bin/env bash
# Fast presubmit: lint, build, and the tests of the packages a branch can
# affect.
#
# `gt done` runs this on the rebased branch before pushing it for landing, so
# it has to stay cheap: the whole unit tier takes 10+ minutes, and the Forgejo
# gate runs `make gate` (lint plus that full tier) on the candidate anyway.
# This target is a subset of that tier, never a substitute for it.
#
# Two rules keep it cheap and safe:
#
#   * The packages tested are the ones whose Go files differ from origin/main
#     plus the ones that depend on them, test files included. A change to a
#     package's production code can break a *dependent's* tests without
#     touching a byte of that dependent — `go build` does not compile test
#     files, so it stays green — and testing only the changed directories left
#     the candidate gate to find that (be-3osn). A branch that changes no Go
#     file runs lint and build only.
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

# A module-relative directory as a package on the go command line.
pkg_path() {
    if [[ "$1" == "." ]]; then
        printf '.'
    else
        printf './%s' "$1"
    fi
}

# Every package in the root module that depends on one of the given import
# paths, printing "<dir>|<changed import path it depends on>" per line.
#
# This is what makes a dependent's breakage visible locally: `go build` never
# compiles test files, so a change in one package can leave a dependent's tests
# uncompilable or its mock expectations wrong and still pass the build. `-test`
# adds each package's test imports to its dependency set, and `-deps` makes
# that set transitive, so one pass over the module finds every package whose
# build — including its tests — reaches a changed one.
#
# It resolves the import graph only; nothing is compiled, and a warm module
# cache makes it a few seconds. Under -test, `go list` prints three lines per
# package: the package, its test-augmented form ("pkg [pkg.test]"), and a
# synthetic test main ("pkg.test"). The bracket suffix is stripped and the
# synthetic main dropped so all three collapse onto the real package; the
# "_test" entry is the external test package, which has no directory to test.
#
# -e tolerates a package that fails to load — the tree may not compile yet,
# and a package that cannot be loaded contributes none of its own dependents
# to the selection rather than aborting it.
dependent_packages() {
    local graph
    graph="$(go list -e -test -deps \
        -f '{{.ImportPath}}|{{range .Deps}}{{.}} {{end}}' ./... 2>/dev/null)" || return 1
    # No packages at all means the module did not load; say so rather than
    # silently reporting that nothing depends on anything.
    [[ -n "$graph" ]] || return 1
    printf '%s\n' "$graph" | awk -F'|' -v module="$module_path" -v changed="$*" '
        BEGIN {
            n = split(changed, c, " ")
            for (i = 1; i <= n; i++) want[c[i]] = 1
        }
        $1 == "" { next }
        {
            id = $1
            if (match(id, / \[[^]]*\]$/)) {
                id = substr(id, 1, RSTART - 1)
            } else if (id ~ /\.test$/) {
                next
            }
            if (id ~ /_test$/) { next }
            m = split($2, deps, " ")
            for (i = 1; i <= m; i++) {
                if (deps[i] in want) {
                    if (!(id in via)) via[id] = deps[i]
                    break
                }
            }
        }
        END {
            for (id in via) {
                if (id != module && index(id, module "/") != 1) continue
                print rel_dir(id, module) "|" rel_dir(via[id], module)
            }
        }
        function rel_dir(path, module) {
            return (path == module) ? "." : substr(path, length(module) + 2)
        }
    ' | LC_ALL=C sort
}

changed_dirs=()
while IFS= read -r dir; do
    # The examples are their own Go modules, so neither this target's `./...`
    # build nor the gate's reaches them; naming one on the go command line
    # would just fail with "main module does not contain package". "." is this
    # module's root, whose go.mod is the expected one, not a nested module.
    if [[ "$dir" != "." && -f "$dir/go.mod" ]]; then
        echo "presubmit: skip $(pkg_path "$dir"): separate Go module, outside the root module's ./..."
        continue
    fi
    changed_dirs+=("$dir")
done < <(changed_go_dirs)

# The import path a changed directory's package is named by, so the dependency
# graph can be asked about it. Best effort: if `go list` cannot answer, the
# lint and build below still run and report the real problem, and the
# selection falls back to the changed packages alone rather than this step
# inventing a failure verdict of its own.
module_path=""
if ((${#changed_dirs[@]})); then
    module_path="$(go list -m -f '{{.Path}}')" || module_path=""
    if [[ -z "$module_path" ]]; then
        echo "presubmit: warn: could not read the module path from go.mod; testing only the changed packages" >&2
    fi
fi

packages=()
# The packages already selected, as " pkg1 pkg2 " so a member test needs no
# associative array (this script stays bash 3.2-safe like the rest of it).
selected=" "
changed_count=0
changed_imports=()

for dir in ${changed_dirs[@]+"${changed_dirs[@]}"}; do
    pkg="$(pkg_path "$dir")"

    # Every changed package is looked up in the dependency graph, including the
    # ones skipped below: the skip is about what can run here, not about what
    # the change can break.
    if [[ -n "$module_path" ]]; then
        if [[ "$dir" == "." ]]; then
            changed_imports+=("$module_path")
        else
            changed_imports+=("$module_path/$dir")
        fi
    fi

    if package_needs_infra "$dir"; then
        echo "presubmit: skip $pkg: its tests need a Docker container or a Dolt server; the Forgejo gate covers it"
        continue
    fi

    packages+=("$pkg")
    selected+="$pkg "
    changed_count=$((changed_count + 1))
    echo "presubmit: run $pkg"
done

if ((${#changed_imports[@]})); then
    if ! dependents="$(dependent_packages "${changed_imports[@]}")"; then
        echo "presubmit: warn: could not resolve the packages that depend on the changed ones; testing only the changed packages (run 'go list -e -test -deps ./...' to see why)" >&2
        dependents=""
    fi
    while IFS='|' read -r dir via; do
        [[ -n "$dir" ]] || continue
        pkg="$(pkg_path "$dir")"
        [[ "$selected" != *" $pkg "* ]] || continue
        if package_needs_infra "$dir"; then
            echo "presubmit: skip $pkg (depends on $(pkg_path "$via")): its tests need a Docker container or a Dolt server; the Forgejo gate covers it"
            continue
        fi
        packages+=("$pkg")
        selected+="$pkg "
        echo "presubmit: run $pkg (depends on $(pkg_path "$via"))"
    done <<<"$dependents"
fi

dependent_count=$((${#packages[@]} - changed_count))
if ((dependent_count > 0)); then
    echo "presubmit: base $base_ref; ${#packages[@]} package(s) to test ($changed_count changed, $dependent_count pulled in as dependents)"
else
    echo "presubmit: base $base_ref; ${#packages[@]} package(s) to test"
fi

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
