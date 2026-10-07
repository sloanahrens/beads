#!/usr/bin/env bash
# with-scratch-bd.sh — run a command against a bd built outside the checkout.
#
# `make check-docs` needs a runnable bd, and the checkout root is the wrong
# place to put one:
#
#   * /bd is gitignored, so a root bd lingers invisibly and outlives the run
#     that produced it — exactly what AGENT_INSTRUCTIONS.md warns about when
#     it tells a reader that a stale binary in the working directory shadows
#     the installed bd;
#   * anything that resolves ./bd finds the stale one first, and the docs
#     generator does exactly that (scripts/generate-cli-docs.sh prefers
#     $PROJECT_ROOT/bd over building its own);
#   * in a Gas Town worktree, gt doctor's rig-bd-binary check flags an
#     executable named bd anywhere under the worktree, so a root build turns
#     every checkout that runs that check into an operator chore.
#
# So the binary is built into a scratch directory the operating system owns,
# the wrapped command is run against that binary, and the directory is removed
# when the command returns. `make build` is untouched: $(BUILD_DIR)/bd at the
# checkout root is the landing contract (scripts/install-bd.sh reads it as
# BD_NEW), not a general-purpose build product.
#
# The binary keeps the name bd (bd.exe on Windows): the docs tools derive
# usage text and subcommand names from it.
#
# Usage: with-scratch-bd.sh <command> [args...]
#   <command> runs with the scratch binary's absolute path appended as its last
#   argument, and its exit status is this script's. The scratch directory is
#   removed on exit whether the build or the command succeeded or failed.
#
#   BEADS_SCRATCH_BD_LDFLAGS  optional -ldflags value for the build. The docs
#                             gate passes the checkout's Build and Commit so
#                             `bd version` reads the same as `make build`.
#
# Exit: 2 with no command to run; otherwise the build's or the command's status.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# shellcheck source=../.buildflags
source "$PROJECT_ROOT/.buildflags"

if [ "$#" -eq 0 ]; then
    echo "usage: $0 <command> [args...]" >&2
    exit 2
fi

# bd.exe on Windows, bd elsewhere; `go env GOEXE` is the check the rest of the
# repo's scripts use (scripts/test.sh). A `go` that cannot answer it leaves the
# name as bd, which is right for every non-Windows toolchain.
bd_name="bd"
if [ "$(go env GOEXE 2>/dev/null || true)" = ".exe" ]; then
    bd_name="bd.exe"
fi

scratch="$(mktemp -d "${TMPDIR:-/tmp}/beads-scratch-bd-XXXXXX")"
cleanup() {
    rm -rf -- "$scratch"
}
trap cleanup EXIT

bd="$scratch/$bd_name"

# CGO_ENABLED=0 matches the docs pipeline's canonical build
# (scripts/generate-cli-docs.sh, scripts/check-cli-docs-drift.sh): the docs
# tools only read the CLI tree, and a pure-Go build needs no ICU or cgo
# toolchain. -o outside the checkout, not $(BUILD_DIR)/bd.
if [ -n "${BEADS_SCRATCH_BD_LDFLAGS:-}" ]; then
    (cd "$PROJECT_ROOT" && CGO_ENABLED=0 go build \
        -tags "$BEADS_BUILD_TAGS" \
        -ldflags "$BEADS_SCRATCH_BD_LDFLAGS" \
        -o "$bd" ./cmd/bd)
else
    (cd "$PROJECT_ROOT" && CGO_ENABLED=0 go build \
        -tags "$BEADS_BUILD_TAGS" \
        -o "$bd" ./cmd/bd)
fi

"$@" "$bd"
