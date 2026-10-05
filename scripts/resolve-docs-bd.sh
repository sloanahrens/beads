#!/bin/bash
# resolve-docs-bd.sh — Resolve the bd binary the docs pipeline must use.
#
# Reads the release pin from docs/cli-docs.pin. When the pin names a tag,
# builds (and caches) a canonical pure-Go bd from that tag's source and
# prints the binary's absolute path on stdout. When the pin is HEAD, the pin
# file is absent, or BD_DOCS_IGNORE_PIN=1 is set, prints nothing and exits 0:
# callers fall back to their current-checkout behavior.
#
# A pin that names a tag this repository cannot resolve is a hard failure
# (exit 1), never an empty success. Every caller reads empty stdout as "no
# pin, validate the current checkout", so reporting an unresolvable pin that
# way would quietly check the wrong binary — exactly the outcome the pin
# exists to prevent. The failure names the pin file, the pin value, and why
# the tag is missing, so the diagnostic is actionable without reading this
# script.
#
# The build matches CI's canonical docs build (CGO_ENABLED=0, -tags
# gms_pure_go), so regenerated docs are reproducible in any environment.
# The binary is cached at build/docs-bd/<pin>/bd (gitignored via /build/).
#
# Usage: scripts/resolve-docs-bd.sh
#   stdout: absolute path to the pinned bd binary, or empty when unpinned
#   diagnostics go to stderr
#   exit:   0 when the pin resolves or no pin is set; 1 when it cannot resolve

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
PIN_FILE="$PROJECT_ROOT/docs/cli-docs.pin"

if [ ! -f "$PIN_FILE" ]; then
    exit 0
fi

PIN="$(grep -v '^[[:space:]]*#' "$PIN_FILE" | grep -m1 '[^[:space:]]' | tr -d '[:space:]' || true)"
if [ -z "$PIN" ] || [ "$PIN" = "HEAD" ]; then
    exit 0
fi

# BD_DOCS_IGNORE_PIN=1 is the documented escape hatch: validate against the
# current checkout rather than the pinned release. Honored here too, not only
# in the gates that call this script, so the advice this script's own failure
# prints works wherever it is read.
if [ "${BD_DOCS_IGNORE_PIN:-0}" = "1" ]; then
    echo "docs pin: BD_DOCS_IGNORE_PIN=1 — resolving no pinned bd; callers validate the current checkout." >&2
    exit 0
fi

CACHE_DIR="$PROJECT_ROOT/build/docs-bd/$PIN"
CACHED_BD="$CACHE_DIR/bd"
if [ -x "$CACHED_BD" ]; then
    echo "$CACHED_BD"
    exit 0
fi

# Why origin cannot supply the pin. The fetch's own error — "couldn't find
# remote ref refs/tags/<pin>" — names neither the pin nor whether origin
# serves tags at all, and the two need different fixes: a mirror serving no
# tags wants them pushed, a stale pin wants bumping. Callers drop this into
# the failure's cause line.
pin_fetch_cause() {
    local origin_url remote_tags ref found=0
    origin_url="$(git -C "$PROJECT_ROOT" remote get-url origin 2>/dev/null || printf 'origin')"

    if ! remote_tags="$(GIT_TERMINAL_PROMPT=0 git -C "$PROJECT_ROOT" ls-remote --tags origin 2>&1)"; then
        printf 'origin (%s) could not be reached to list its tags: %s' \
            "$origin_url" "${remote_tags%%$'\n'*}"
        return
    fi
    if [ -z "$remote_tags" ]; then
        printf 'origin (%s) serves no tags at all — git ls-remote --tags origin lists nothing' "$origin_url"
        return
    fi

    # Exact string comparison, not a pattern: the pin is a tag name, and a
    # regex would read its dots as wildcards. A here-string, not a pipe: grep
    # exits at the first match, and under `set -o pipefail` the writer's
    # SIGPIPE would turn a match into a failed pipeline — reporting "origin
    # does not serve this tag" for a tag it serves, which is the one cause
    # that sends the reader to bump the pin instead of fixing the fetch.
    while read -r _ ref; do
        if [ "$ref" = "refs/tags/$PIN" ]; then
            found=1
            break
        fi
    done <<<"$remote_tags"

    if [ "$found" -eq 1 ]; then
        printf 'origin (%s) serves refs/tags/%s, but fetching it failed' "$origin_url" "$PIN"
    else
        printf 'origin (%s) serves tags, but not refs/tags/%s' "$origin_url" "$PIN"
    fi
}

# Report an unresolvable pin and exit 1. $1 is the cause; $2 is the failed
# fetch's output, quoted so the raw git error stays visible under the
# explanation rather than replacing it.
pin_unresolved() {
    local cause="$1" fetch_error="${2:-}"

    {
        echo ""
        echo "Error: the docs pipeline cannot run — docs/cli-docs.pin names a tag this repository cannot resolve."
        echo ""
        echo "  pin file:  docs/cli-docs.pin"
        echo "  pin value: $PIN"
        echo "  cause:     $cause"
        if [ -n "$fetch_error" ]; then
            echo ""
            echo "  git fetch --depth=1 origin refs/tags/$PIN:refs/tags/$PIN failed:"
            printf '%s\n' "$fetch_error" | sed 's/^/    /'
        fi
        echo ""
        echo "The docs describe the pinned release, so validating against this checkout's"
        echo "build would check the wrong binary; nothing here falls back to it. To make"
        echo "the docs pipeline runnable, push tag $PIN to origin, point"
        echo "docs/cli-docs.pin at a tag origin serves, or set BD_DOCS_IGNORE_PIN=1 to"
        echo "validate against the current checkout deliberately."
    } >&2
    exit 1
}

# Make sure the pinned tag exists locally; shallow CI checkouts may not have
# fetched tags. GIT_TERMINAL_PROMPT=0 keeps a gate from blocking on a
# credential prompt it can never answer — a hang emits no diagnostic at all.
if ! git -C "$PROJECT_ROOT" rev-parse --verify --quiet "refs/tags/$PIN^{commit}" >/dev/null; then
    echo "docs pin: fetching tag $PIN from origin..." >&2
    if ! FETCH_ERROR="$(GIT_TERMINAL_PROMPT=0 git -C "$PROJECT_ROOT" fetch --depth=1 origin "refs/tags/$PIN:refs/tags/$PIN" 2>&1)"; then
        pin_unresolved "$(pin_fetch_cause)" "$FETCH_ERROR"
    fi
    # The fetch's own report ("From <url>", "[new tag]") is progress, not a
    # diagnostic: it stays on stderr where it was before the capture, so a
    # successful fetch looks the same whether or not it had to explain itself.
    if [ -n "$FETCH_ERROR" ]; then
        printf '%s\n' "$FETCH_ERROR" >&2
    fi
    if ! git -C "$PROJECT_ROOT" rev-parse --verify --quiet "refs/tags/$PIN^{commit}" >/dev/null; then
        pin_unresolved "the fetch reported success, but refs/tags/$PIN still does not resolve to a commit here"
    fi
fi

echo "docs pin: building bd from tag $PIN (CGO_ENABLED=0 -tags gms_pure_go)..." >&2
WT="$(mktemp -d)"
cleanup() {
    git -C "$PROJECT_ROOT" worktree remove --force "$WT" >/dev/null 2>&1 || true
    rm -rf "$WT"
}
trap cleanup EXIT

git -C "$PROJECT_ROOT" worktree add --detach --quiet "$WT" "refs/tags/$PIN"
mkdir -p "$CACHE_DIR"
(cd "$WT" && CGO_ENABLED=0 go build -tags gms_pure_go -o "$CACHED_BD" ./cmd/bd/) >&2

echo "$CACHED_BD"
