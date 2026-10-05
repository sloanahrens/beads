#!/usr/bin/env bash
#
# install-bd.sh — put the bd a landing just built in place of the installed bd.
#
# The beads rig runs this as its post_land_command: `make build` leaves a fresh
# ./bd in the landing worktree, and this script decides whether that build may
# replace the installed bd, the data plane every agent in the town talks to.
#
# It is deliberately conservative. It refuses, and escalates, when
#
#   * there is no installed bd, or the installed one records no commit;
#   * the installed commit is not an ancestor of HEAD — a downgrade or a
#     divergence (the gt rebuild crash loop, with a wider blast radius);
#   * the new binary's `version --json` differs from the installed one by more
#     than build, build_id and commit. A difference there is a schema or JSON
#     contract change, and moving the town's database across it needs a human;
#   * the pre-install smoke command fails.
#
# It does nothing, and says so, when no non-test Go file, go.mod or go.sum has
# changed since the installed commit: a rebuild of unchanged sources is
# byte-identical to the binary already installed.
#
# It installs atomically — stage a sibling file, then rename over bd, never copy
# over a running binary — keeps the previous binary as
# bd.bak-<commit>-<YYYYMMDD>, and moves that backup back if the post-install
# smoke fails.
#
# post_land_command reads a non-zero exit as a failed landing (the main goes red
# and the landing worker reverts it), so this script exits 0 on every outcome,
# including every refusal. Refusals are reported through ESCALATE_CMD.
#
# Inputs (all optional, all overridable from the environment):
#
#   INSTALL_DIR   directory holding the installed bd   (default $HOME/.local/bin)
#   BD_NEW        the freshly built binary to install  (default ./bd)
#   SMOKE_CMD     read-only command proving a bd works (default bd list --limit 1)
#   SMOKE_DIR     directory to run SMOKE_CMD in        (default $GT_TOWN_ROOT,
#                 else $HOME/gt, else the current directory)
#   ESCALATE_CMD  command run with the refusal message as its one argument
#                 (default gt escalate -s high)
#   BD_BACKUPS    how many bd.bak-* files to keep      (default 3)

set -uo pipefail

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BD_NEW="${BD_NEW:-$PWD/bd}"
SMOKE_CMD="${SMOKE_CMD:-bd list --limit 1}"
BD_BACKUPS="${BD_BACKUPS:-3}"
ESCALATE_CMD="${ESCALATE_CMD:-gt escalate -s high}"

if [ -z "${SMOKE_DIR:-}" ]; then
    SMOKE_DIR="${GT_TOWN_ROOT:-$HOME/gt}"
    [ -d "$SMOKE_DIR" ] || SMOKE_DIR="$PWD"
fi

# staged is the sibling the new binary is written to before the rename; setting
# it back to empty after the rename keeps the EXIT trap from deleting the
# installed binary. linked_bd_dir holds the symlink bd_path_dir may have made.
staged=""
linked_bd_dir=""

# shellcheck disable=SC2329  # runs from the EXIT trap below
cleanup() {
    if [ -n "$staged" ]; then
        rm -f -- "$staged"
    fi
    if [ -n "$linked_bd_dir" ]; then
        rm -f -- "$linked_bd_dir/bd"
        rmdir -- "$linked_bd_dir" 2>/dev/null || true
    fi
}
trap cleanup EXIT

log() {
    printf 'install-bd: %s\n' "$*"
}

# quote_word renders its argument as one single-quoted shell word, so a refusal
# message reaches ESCALATE_CMD as a single argument whatever it contains.
quote_word() {
    printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

# refuse reports a refusal through ESCALATE_CMD and stops. See the header for
# why this must exit 0.
refuse() {
    log "$1"
    eval "$ESCALATE_CMD $(quote_word "$1")" || log "escalate command failed: $ESCALATE_CMD"
    exit 0
}

# json_commit prints the "commit" field of a `bd version --json` line, or
# nothing. This is the grep/sed pair Makefile's check-forward-only already uses:
# the landing host does not guarantee jq.
json_commit() {
    printf '%s' "$1" |
        grep -o '"commit"[[:space:]]*:[[:space:]]*"[^"]*"' |
        sed 's/.*"\([^"]*\)"$/\1/'
}

# normalize_version_json blanks the three build stamps — build, build_id and
# commit — so two builds of the same source compare equal, and leaves the rest
# (version, db_schema_version, schema_ceiling, contract_version, and a JSON
# envelope's schema_version) in place. What remains is the schema and JSON
# contract surface the installed bd speaks, which a silent install must not
# cross. `"build"` cannot match inside `"build_id"`: the pattern requires the
# closing quote immediately after build.
normalize_version_json() {
    printf '%s' "$1" | sed -E \
        -e 's/("build"[[:space:]]*:[[:space:]]*)"[^"]*"/\1""/g' \
        -e 's/("build_id"[[:space:]]*:[[:space:]]*)"[^"]*"/\1""/g' \
        -e 's/("commit"[[:space:]]*:[[:space:]]*)"[^"]*"/\1""/g'
}

short_commit() {
    printf '%.12s' "$1"
}

# bd_path_dir prints a directory in which a bare `bd` resolves to the given
# binary, so SMOKE_CMD can name the tool the way an operator would. A binary
# already named bd is its own directory; anything else gets a private directory
# holding a symlink, removed by the EXIT trap.
bd_path_dir() {
    local bin="$1"
    if [ "$(basename "$bin")" = "bd" ]; then
        dirname "$bin"
        return 0
    fi
    linked_bd_dir="$(mktemp -d "${TMPDIR:-/tmp}/install-bd.XXXXXX")" || return 1
    ln -s "$bin" "$linked_bd_dir/bd" || return 1
    printf '%s' "$linked_bd_dir"
}

# run_smoke runs SMOKE_CMD from the smoke directory with the given directory
# first on PATH (pass an empty string to smoke the installed bd). The default
# command is a read-only `bd list`; nothing here writes. Output is held back on
# success and printed on failure.
run_smoke() {
    local path_dir="$1" out rc
    out="$(
        cd "$SMOKE_DIR" || exit 1
        if [ -n "$path_dir" ]; then
            PATH="$path_dir:$PATH" eval "$SMOKE_CMD" 2>&1
        else
            eval "$SMOKE_CMD" 2>&1
        fi
    )"
    rc=$?
    if [ "$rc" -ne 0 ]; then
        log "smoke command failed (exit $rc): $SMOKE_CMD"
        printf '%s\n' "$out" | sed 's/^/install-bd:   /'
    fi
    return "$rc"
}

# prune_backups keeps the newest BD_BACKUPS bd.bak-* files. The names carry only
# a date, so "newest" is mtime order — which is also the order the rollback path
# relies on. It runs from inside INSTALL_DIR so that only the backup's own name
# is ever parsed, never the directory it lives in.
prune_backups() {
    local path
    (
        cd "$INSTALL_DIR" || exit 0
        # shellcheck disable=SC2012  # backup names are ours: bd.bak-<hash>-<date>
        ls -1t bd.bak-* 2>/dev/null | tail -n "+$((BD_BACKUPS + 1))" |
            while IFS= read -r path; do
                [ -n "$path" ] || continue
                rm -f -- "$path"
            done
    )
}

main() {
    case "$BD_BACKUPS" in
        '' | *[!0-9]* | 0)
            refuse "BD_BACKUPS must be a positive integer, got '$BD_BACKUPS'"
            ;;
    esac

    local installed_bd="$INSTALL_DIR/bd"

    # (1) The installed binary is the baseline for every later check.
    if [ ! -x "$installed_bd" ]; then
        refuse "no executable bd at $installed_bd; there is nothing to replace"
    fi
    local installed_json installed_commit
    installed_json="$("$installed_bd" version --json 2>/dev/null)"
    installed_commit="$(json_commit "$installed_json")"
    if [ -z "$installed_commit" ]; then
        refuse "the installed bd at $installed_bd reports no commit; cannot tell what it was built from"
    fi

    # (2) Forward only: the installed commit has to be behind HEAD.
    if ! git rev-parse --git-dir >/dev/null 2>&1; then
        refuse "not inside a git repository, so cannot tell whether HEAD contains $(short_commit "$installed_commit")"
    fi
    if ! git cat-file -e "$installed_commit^{commit}" 2>/dev/null; then
        refuse "the installed bd reports commit $installed_commit, which this repository does not contain"
    fi
    if ! git merge-base --is-ancestor "$installed_commit" HEAD 2>/dev/null; then
        refuse "the installed bd's commit $(short_commit "$installed_commit") is not an ancestor of HEAD; refusing a downgrade or a diverged build"
    fi

    # (3) A build of untouched Go sources is byte-identical to what is already
    # installed, so there is nothing to do. Test-only edits cannot change the
    # binary; everything else (.go, go.mod, go.sum) can.
    local changed head_commit
    head_commit="$(git rev-parse HEAD)"
    if ! changed="$(git diff --name-only "$installed_commit" HEAD 2>/dev/null)"; then
        refuse "cannot list what changed between $(short_commit "$installed_commit") and HEAD"
    fi
    changed="$(printf '%s\n' "$changed" |
        grep -E '(^|/)go\.(mod|sum)$|\.go$' |
        grep -v -- '_test\.go$')"
    if [ -z "$changed" ]; then
        log "no Go change between $(short_commit "$installed_commit") and $(short_commit "$head_commit"); nothing to install"
        exit 0
    fi

    # (4) Same schema, same JSON contract. Anything else means the town's
    # database would be handed to a binary that speaks a different language.
    if [ ! -x "$BD_NEW" ]; then
        refuse "no built bd at $BD_NEW; run make build in the landing worktree first"
    fi
    local new_json
    if ! new_json="$("$BD_NEW" version --json 2>/dev/null)"; then
        refuse "the build at $BD_NEW cannot report its version"
    fi
    if [ "$(normalize_version_json "$installed_json")" != "$(normalize_version_json "$new_json")" ]; then
        refuse "the new bd's version JSON differs from the installed bd's beyond build, build_id and commit; that is a schema or contract change and needs a human"
    fi

    # (5) Prove the new binary works before anything is replaced. The new
    # binary runs first on PATH, so a failure here is the new build's, not the
    # installed one's.
    local new_dir
    if ! new_dir="$(bd_path_dir "$BD_NEW")"; then
        refuse "cannot stage $BD_NEW for the smoke command"
    fi
    if ! run_smoke "$new_dir"; then
        refuse "the build at $BD_NEW failed the smoke command ($SMOKE_CMD); refusing to install"
    fi

    # (6) Back up what is about to be replaced. Plain cp, not cp -p: the
    # backup's mtime is when it was taken, which is what "newest backup" means.
    local backup
    backup="$INSTALL_DIR/bd.bak-$installed_commit-$(date +%Y%m%d)"
    if ! cp "$installed_bd" "$backup"; then
        refuse "cannot write the backup $backup; refusing to install without one"
    fi
    prune_backups

    # (7) Install atomically: a sibling file, then a rename. Copying straight
    # over a bd that agents are executing can leave a truncated binary.
    staged="$INSTALL_DIR/.bd.new.$$"
    if ! cp "$BD_NEW" "$staged"; then
        refuse "cannot stage the new bd at $staged"
    fi
    if ! chmod 755 "$staged"; then
        refuse "cannot make the staged bd executable"
    fi
    if ! "$staged" version >/dev/null 2>&1; then
        refuse "the staged bd at $staged does not run"
    fi
    if ! mv -f -- "$staged" "$installed_bd"; then
        refuse "cannot move the staged bd over $installed_bd"
    fi
    staged=""
    log "installed $(short_commit "$head_commit") at $installed_bd; previous binary kept as $(basename "$backup")"

    # (8) The smoke that matters: the installed binary, as agents will find it.
    # One that fails takes the town down, so put the backup back.
    if ! run_smoke "$INSTALL_DIR"; then
        if mv -f -- "$backup" "$installed_bd"; then
            refuse "the installed bd failed the smoke command ($SMOKE_CMD); restored $(basename "$backup")"
        fi
        refuse "the installed bd failed the smoke command and restoring $(basename "$backup") failed; $installed_bd is broken"
    fi

    log "install complete: $installed_bd passes $SMOKE_CMD"
    exit 0
}

main "$@"
