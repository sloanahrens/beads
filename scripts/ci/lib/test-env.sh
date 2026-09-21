#!/usr/bin/env bash
# Shared hermetic environment setup for broad local/CI test wrappers.

if [[ -n "${BEADS_CI_TEST_ENV_SH_LOADED:-}" ]]; then
    return 0
fi
BEADS_CI_TEST_ENV_SH_LOADED=1

beads_test_env_enter() {
    if [[ "${BEADS_TEST_ENV_DISABLE:-0}" == "1" ]]; then
        return 0
    fi
    if [[ "${BEADS_TEST_ENV_ACTIVE:-0}" == "1" ]]; then
        return 0
    fi

    local root
    root="$(mktemp -d "${TMPDIR:-/tmp}/beads-test-env-XXXXXX")"
    export BEADS_TEST_ENV_ROOT="$root"
    export BEADS_TEST_ENV_ACTIVE=1

    if [[ -z "${GOCACHE:-}" ]]; then
        local go_cache
        go_cache="$(go env GOCACHE 2>/dev/null || true)"
        if [[ -n "$go_cache" ]]; then
            export GOCACHE="$go_cache"
        fi
    fi
    if [[ -z "${GOMODCACHE:-}" ]]; then
        local go_mod_cache
        go_mod_cache="$(go env GOMODCACHE 2>/dev/null || true)"
        if [[ -n "$go_mod_cache" ]]; then
            export GOMODCACHE="$go_mod_cache"
        fi
    fi

    mkdir -p "$root/home" "$root/xdg-config" "$root/dolt-root"
    : >"$root/gitconfig"

    export HOME="$root/home"
    export USERPROFILE="$root/home"
    export XDG_CONFIG_HOME="$root/xdg-config"
    export DOLT_ROOT_PATH="$root/dolt-root"
    export GIT_CONFIG_NOSYSTEM=1
    export GIT_CONFIG_GLOBAL="$root/gitconfig"
    export BEADS_TEST_IGNORE_REPO_CONFIG=1
    if [[ "${BEADS_TEST_ENV_RUN_DOLT:-0}" != "1" ]]; then
        beads_test_env_add_skip "dolt"
    fi

    unset BEADS_DIR
    unset BEADS_DB
    unset BD_DB
    unset BD_JSON
    unset BD_NO_DB
    unset BD_NO_DAEMON
    unset BD_ACTOR
    unset BEADS_ACTOR
    unset GT_ROOT
    unset BEADS_DOLT_SHARED_SERVER
    unset BEADS_DOLT_SERVER_MODE
    unset BEADS_DOLT_AUTO_START
    unset BEADS_DOLT_SERVER_HOST
    unset BEADS_DOLT_SERVER_PORT
    unset BEADS_DOLT_PORT
    unset BEADS_DOLT_SERVER_DATABASE
    unset BEADS_DOLT_SERVER_SOCKET
    unset BEADS_DOLT_PASSWORD

    # be-9yi: GT_DOLT_PORT is Gas Town's own env var (not read anywhere in
    # this repo's Go code), pointing agent shells at a live, shared,
    # production dolt sql-server. It is not BD_-prefixed, so the sweep below
    # cannot catch it. A 2026-09-09 incident had it set in a test-runner's
    # environment; because nothing here unset it explicitly, a downstream
    # tool inferred a real server port from it and auto-migrated the town's
    # `hq` database mid test-run. Always strip it before tests run, whether
    # or not this repo ever comes to read it directly.
    unset GT_DOLT_PORT

    # Sweep every OTHER exported BD_-prefixed env var, not just the
    # hand-enumerated ones above. internal/config/config.go binds
    # v.SetEnvPrefix("BD") + v.AutomaticEnv(), so ANY "BD_<KEY>" env var can
    # silently override ANY config key (dolt.port -> BD_DOLT_PORT,
    # dolt.auto-commit -> BD_DOLT_AUTO_COMMIT, the remote-migrate escape
    # hatch BD_ALLOW_REMOTE_MIGRATE, etc.) without a corresponding line ever
    # being added here. The enumerated unsets above predate this sweep and
    # stayed missing GT_DOLT_PORT's BD_ siblings until be-9yi; a wildcard
    # sweep cannot drift out of sync with new config keys the same way.
    local bd_var
    for bd_var in "${!BD_@}"; do
        unset "$bd_var"
    done

    # be-9zm: with HOME pointed at an empty sandbox there is no user config,
    # so every bd the suites spawn would resolve telemetry ENABLED: fork the
    # platform machine-id probe (ioreg on macOS) on the cold cache, write
    # event files under $HOME/.beads/eventsData, and spawn detached
    # send-metrics children aimed at the real endpoint. Set the two switches
    # AFTER the sweep above (which would otherwise remove BD_DISABLE_METRICS).
    export BD_DISABLE_METRICS=1
    export BEADS_TEST_MODE=1

    if command -v dolt >/dev/null 2>&1; then
        dolt config --global --add user.name "beads-test" >/dev/null 2>&1 || true
        dolt config --global --add user.email "test@beads.local" >/dev/null 2>&1 || true
    fi

    trap beads_test_env_cleanup EXIT
}

beads_test_env_add_skip() {
    local service="$1"
    local current=",${BEADS_TEST_SKIP:-},"
    if [[ "$current" != *",$service,"* ]]; then
        if [[ -n "${BEADS_TEST_SKIP:-}" ]]; then
            export BEADS_TEST_SKIP="${BEADS_TEST_SKIP},${service}"
        else
            export BEADS_TEST_SKIP="$service"
        fi
    fi
}

# Coverage boundary of this environment (be-1kk).
#
# Some suites OPT IN on an env var. When that var is unset they skip every
# test and `go test` still prints `ok <pkg> <duration>` — indistinguishable
# from a package that ran and passed. A green from such a package is not
# evidence about it, which is how main sat red (be-bz4) behind a gate
# reporting `ok internal/storage/embeddeddolt 265.768s`.
#
# Every var below is therefore NOT an accidental omission: the gate declines
# to set it, deliberately, and scripts/test.sh prints the result as a
# COVERAGE GAP block so no reader (refinery, witness, om, a human) can take
# the green for evidence. See engdocs/TESTING.md.
#
#   BEADS_TEST_EMBEDDED_DOLT — embedded-Dolt suites. Cannot be exported here
#     even though it looks like a one-line fix. The var is package-scoped in
#     intent and process-wide in effect: setting it also switches on cmd/bd's
#     184 TestEmbedded* functions, which CI only fits by sharding them across
#     20 jobs (.github/workflows/main.yml). Inside `make test` those land in
#     cmd/bd's single package run, already measured at 1003.7s and up to
#     1533s under load against scripts/test.sh's 1500s per-package deadline
#     (be-128), so the export would trade a false green for a guaranteed
#     false red. internal/storage/embeddeddolt alone costs 993.5s / ~1007s
#     wall with the var set (measured 2026-09-21, ~34% headroom on that same
#     deadline), so real coverage needs a package-scoped pass rather than an
#     export. That pass is blocked on be-bz4: the suite is red on main today
#     (TestGetIssue/missing_leases_table_is_an_error_not_absent), so adding
#     it to the gate would halt the merge queue on a pre-existing failure
#     until be-bz4 lands.
#     Until then, an MR touching internal/storage/embeddeddolt,
#     internal/doltserver or cmd/bd needs env-gated evidence in its
#     verification rather than a green `make test` — scripts/conformance.sh
#     runs the storage suite and already fails loudly on a silent skip.
beads_test_env_coverage_gaps() {
    # One "<gate-var>|<package>|<why>" line per suite whose gate var is unset
    # here, i.e. that this run skips while still reporting the package "ok".
    if [[ "${BEADS_TEST_EMBEDDED_DOLT:-}" != "1" ]]; then
        printf '%s\n' \
            "BEADS_TEST_EMBEDDED_DOLT|internal/storage/embeddeddolt/|embedded-Dolt storage suite: 240 assertions, 267 of 363 tests skipped without it (be-1kk)" \
            "BEADS_TEST_EMBEDDED_DOLT|cmd/bd/|TestEmbedded* subprocess suite: 184 top-level tests, sharded 20 ways in CI so they cannot fit this package's budget (be-128)"
    fi
}

beads_test_env_cleanup() {
    if [[ "${BEADS_TEST_ENV_KEEP:-0}" == "1" ]]; then
        return 0
    fi
    if [[ -n "${BEADS_TEST_ENV_ROOT:-}" ]]; then
        rm -rf "$BEADS_TEST_ENV_ROOT"
        unset BEADS_TEST_ENV_ROOT
    fi
}
