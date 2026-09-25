#!/bin/bash
# Test suite for the pre-push hook: branch allowlist, polecat main-push
# refusal (gt-93od, ported from the gastown rig's hook, gt-ibt8), the
# off-branch guard, and the release-tag version check.
# Creates temporary git repos to simulate push scenarios.
#
# Usage: bash .githooks/pre-push_test.sh

set -euo pipefail

# The suite runs from inside a polecat or Refinery session as often as not,
# and the hook now refuses default-branch pushes from a polecat (gt-93od)
# while trusting GT_REFINERY as a Refinery signal (gt-9tf9) — so clear the
# ambient role/identity signals first and let each test state its own context
# explicitly. Without this, a suite run inside a Refinery session (which sets
# GT_REFINERY=1 at spawn, internal/refinery/manager.go) would leak that var
# into the self-grant cases and let a spoofed push through.
unset GT_ROLE GT_POLECAT GT_POLECAT_PATH GT_REFINERY 2>/dev/null || true
unset GT_REFINERY_MERGE GT_DONE_DIRECT_MERGE 2>/dev/null || true

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HOOK="$SCRIPT_DIR/pre-push"
PASS=0
FAIL=0
TMPDIR=""
DEFAULT_BRANCH=""

cleanup() {
  cd /tmp  # Ensure CWD exists before removing TMPDIR
  if [[ -n "$TMPDIR" && -d "$TMPDIR" ]]; then
    rm -rf "$TMPDIR"
  fi
  TMPDIR=""
}
trap cleanup EXIT

# setup_repos [local_subdir]
# The optional subdir places the "local" clone at a chosen relative path, so a
# test can run the hook from a polecat-shaped worktree
# (<town>/<rig>/polecats/<name>/<repo>) without any env vars set.
setup_repos() {
  local local_rel="${1:-local}"
  TMPDIR=$(mktemp -d)
  # Create a bare "remote" repo
  git init --bare "$TMPDIR/remote.git" >/dev/null 2>&1
  # Clone it as the "local" repo
  mkdir -p "$(dirname "$TMPDIR/$local_rel")"
  git clone "$TMPDIR/remote.git" "$TMPDIR/$local_rel" >/dev/null 2>&1
  cd "$TMPDIR/$local_rel"
  git config user.email "test@test.com"
  git config user.name "Test"
  # Initial commit
  echo "init" > file.txt
  git add file.txt
  git commit -m "initial" >/dev/null 2>&1
  # Detect the default branch name (main or master)
  DEFAULT_BRANCH=$(git branch --show-current)
  git push origin "$DEFAULT_BRANCH" >/dev/null 2>&1
  # Set up origin/HEAD so hook can detect default branch
  git remote set-head origin "$DEFAULT_BRANCH" >/dev/null 2>&1
  # Copy the hook
  cp "$HOOK" "$TMPDIR/$local_rel/.git/hooks/pre-push"
  chmod +x "$TMPDIR/$local_rel/.git/hooks/pre-push"
}

run_hook() {
  # Simulate pre-push stdin: local_ref local_sha remote_ref remote_sha
  local local_ref=$1 local_sha=$2 remote_ref=$3 remote_sha=$4
  echo "$local_ref $local_sha $remote_ref $remote_sha" | bash "$HOOK" "origin" 2>&1
}

run_hook_env() {
  # run_hook with extra environment: run_hook_env "VAR=value VAR2=value" <refs...>
  local envs=$1
  shift
  # shellcheck disable=SC2086
  local local_ref=$1 local_sha=$2 remote_ref=$3 remote_sha=$4
  # shellcheck disable=SC2086
  echo "$local_ref $local_sha $remote_ref $remote_sha" | env $envs bash "$HOOK" "origin" 2>&1
}

# assert_live_block runs a REAL `git push` (not a hand-fed stdin) and fails if
# it was NOT refused. Running the push for real is what makes the polecat
# refusal a live proof rather than a matcher unit test (gt-ibt8, gt-93od).
assert_live_block() {
  # assert_live_block <name> <envs> <refspec>
  local test_name=$1 envs=$2 refspec=$3 out="" status=0
  # shellcheck disable=SC2086
  out=$(env $envs git push origin "$refspec" 2>&1) || status=$?
  if [[ $status -eq 0 ]]; then
    echo "  FAIL: $test_name (expected live push to be refused, but it succeeded)"
    FAIL=$((FAIL + 1))
  else
    echo "  PASS: $test_name"
    printf '%s\n' "$out" | sed 's/^/        | /'
    PASS=$((PASS + 1))
  fi
}

assert_live_pass() {
  # assert_live_pass <name> <envs> <refspec>
  local test_name=$1 envs=$2 refspec=$3 out="" status=0
  # shellcheck disable=SC2086
  out=$(env $envs git push origin "$refspec" 2>&1) || status=$?
  if [[ $status -eq 0 ]]; then
    echo "  PASS: $test_name"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $test_name (expected live push to succeed, but it was refused)"
    printf '%s\n' "$out" | sed 's/^/        | /'
    FAIL=$((FAIL + 1))
  fi
}

get_sha() {
  git rev-parse "$1"
}

assert_pass() {
  local test_name=$1
  shift
  if "$@" >/dev/null 2>&1; then
    echo "  PASS: $test_name"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $test_name (expected pass, got block)"
    FAIL=$((FAIL + 1))
  fi
}

assert_block() {
  local test_name=$1
  shift
  if "$@" >/dev/null 2>&1; then
    echo "  FAIL: $test_name (expected block, got pass)"
    FAIL=$((FAIL + 1))
  else
    echo "  PASS: $test_name"
    PASS=$((PASS + 1))
  fi
}

echo "=== Pre-push hook test suite (gt-93od) ==="
echo ""

# ---------------------------------------------------------------------------
# Allowlist and off-branch guard
# ---------------------------------------------------------------------------

# Test 1: Normal push to default branch by a non-polecat
echo "Test 1: Normal push to default branch (no polecat signals)"
setup_repos
cd "$TMPDIR/local"
remote_sha=$(get_sha HEAD)
echo "change1" >> file.txt
git add file.txt && git commit -m "normal change" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_pass "Crew push to default branch allowed" run_hook "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 2: Push to polecat/* branch
echo "Test 2: Push to polecat/* branch"
setup_repos
cd "$TMPDIR/local"
git checkout -b polecat/worker1 >/dev/null 2>&1
echo "polecat work" >> file.txt
git add file.txt && git commit -m "polecat work" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_pass "Polecat branch push allowed" run_hook "refs/heads/polecat/worker1" "$local_sha" "refs/heads/polecat/worker1" "0000000000000000000000000000000000000000"
cleanup

# Test 3: Push to integration/* branch
echo "Test 3: Push to integration/* branch"
setup_repos
cd "$TMPDIR/local"
git checkout -b integration/epic-1 >/dev/null 2>&1
echo "integration work" >> file.txt
git add file.txt && git commit -m "integration work" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_pass "Integration branch push allowed" run_hook "refs/heads/integration/epic-1" "$local_sha" "refs/heads/integration/epic-1" "0000000000000000000000000000000000000000"
cleanup

# Test 4: Push to feature/* without upstream remote (blocked)
echo "Test 4: Push to feature/* without upstream remote"
setup_repos
cd "$TMPDIR/local"
git checkout -b feature/thing >/dev/null 2>&1
echo "feature" >> file.txt
git add file.txt && git commit -m "feature" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_block "Feature branch blocked (no upstream)" run_hook "refs/heads/feature/thing" "$local_sha" "refs/heads/feature/thing" "0000000000000000000000000000000000000000"
cleanup

# Test 5: Push to feature/* with upstream remote (allowed)
echo "Test 5: Push to feature/* with upstream remote"
setup_repos
cd "$TMPDIR/local"
git remote add upstream "$TMPDIR/remote.git" >/dev/null 2>&1
git checkout -b feature/thing >/dev/null 2>&1
echo "feature" >> file.txt
git add file.txt && git commit -m "feature" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_pass "Feature branch allowed (upstream exists)" run_hook "refs/heads/feature/thing" "$local_sha" "refs/heads/feature/thing" "0000000000000000000000000000000000000000"
cleanup

# Test 6: Tag push — allowed (not a branch, allowlist does not apply)
echo "Test 6: Tag push"
setup_repos
cd "$TMPDIR/local"
local_sha=$(get_sha HEAD)
assert_pass "Tag push allowed" run_hook "refs/tags/v1.0.0" "$local_sha" "refs/tags/v1.0.0" "0000000000000000000000000000000000000000"
cleanup

# Test 7: Off-branch push — HEAD on a session branch, pushing the default
# branch (the classic `git push origin main` from a feature branch).
# HEAD mismatch — BLOCKED.
echo "Test 7: Off-branch push (HEAD on session branch, pushing default)"
setup_repos
cd "$TMPDIR/local"
git checkout -b session/x >/dev/null 2>&1
echo "session work" >> file.txt
git add file.txt && git commit -m "session work" >/dev/null 2>&1
default_sha=$(get_sha "$DEFAULT_BRANCH")
unset GT_ALLOW_OFFBRANCH_PUSH 2>/dev/null || true
assert_block "Off-branch default push blocked (HEAD mismatch)" run_hook "refs/heads/$DEFAULT_BRANCH" "$default_sha" "refs/heads/$DEFAULT_BRANCH" "$default_sha"
cleanup

# Test 8: Off-branch push with GT_ALLOW_OFFBRANCH_PUSH=1 — ALLOWED (override).
echo "Test 8: Off-branch push with GT_ALLOW_OFFBRANCH_PUSH=1"
setup_repos
cd "$TMPDIR/local"
git checkout -b session/y >/dev/null 2>&1
echo "session work" >> file.txt
git add file.txt && git commit -m "session work" >/dev/null 2>&1
default_sha=$(get_sha "$DEFAULT_BRANCH")
GT_ALLOW_OFFBRANCH_PUSH=1 assert_pass "Off-branch push allowed with override" run_hook "refs/heads/$DEFAULT_BRANCH" "$default_sha" "refs/heads/$DEFAULT_BRANCH" "$default_sha"
cleanup

# ---------------------------------------------------------------------------
# Polecat main-push refusal (gt-93od, ported from gastown gt-ibt8)
# ---------------------------------------------------------------------------

# Test 9: Push to default branch from a polecat context (GT_ROLE) — BLOCKED
echo "Test 9: Default branch push from polecat (GT_ROLE=polecat)"
setup_repos
cd "$TMPDIR/local"
remote_sha=$(get_sha HEAD)
echo "polecat main push" >> file.txt
git add file.txt && git commit -m "polecat main push" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_block "Polecat default push blocked (GT_ROLE=polecat)" \
  run_hook_env "GT_ROLE=polecat" "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 10: Push to default branch from a polecat (compound GT_ROLE) — BLOCKED
echo "Test 10: Default branch push from polecat (GT_ROLE=<rig>/polecats/x)"
setup_repos
cd "$TMPDIR/local"
remote_sha=$(get_sha HEAD)
echo "polecat main push" >> file.txt
git add file.txt && git commit -m "polecat main push" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_block "Polecat default push blocked (compound GT_ROLE)" \
  run_hook_env "GT_ROLE=beads/polecats/chrome" "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 11: Push to default branch from a polecat cwd, no GT_ROLE — BLOCKED
echo "Test 11: Default branch push from polecat cwd (no GT_ROLE)"
setup_repos "$TMPDIR/town/beads/polecats/chrome/beads"
cd "$TMPDIR/town/beads/polecats/chrome/beads"
remote_sha=$(get_sha HEAD)
echo "polecat main push" >> file.txt
git add file.txt && git commit -m "polecat main push" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_block "Polecat default push blocked (cwd only)" \
  run_hook "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# commit_new adds a fresh commit so a following `git push` is a REAL push, not
# an up-to-date no-op. git feeds pre-push EMPTY stdin for up-to-date pushes, so
# a no-op "push" would exercise neither guard and the live assertions would
# pass vacuously; a real push sends the ref and exercises the actual check.
commit_new() {
  echo "$(date +%s%N)" >> file.txt
  git add file.txt
  git commit -m "commit_new" >/dev/null 2>&1
}

# Test 12: Live push from a polecat cwd to the default branch — REFUSED
echo "Test 12: LIVE polecat push to default branch"
setup_repos "$TMPDIR/town/beads/polecats/chrome/beads"
cd "$TMPDIR/town/beads/polecats/chrome/beads"
commit_new
assert_live_block "Live polecat default push refused (cwd)" "" "HEAD:$DEFAULT_BRANCH"
cleanup

# Test 13: Live push from a detached HEAD with GT_ROLE=polecat — REFUSED
# (the exact gt-ibt8 granite incident shape: HEAD:main from a detached HEAD)
echo "Test 13: LIVE polecat push from detached HEAD"
setup_repos
cd "$TMPDIR/local"
git checkout --detach >/dev/null 2>&1
commit_new
assert_live_block "Live polecat detached-HEAD default push refused" "GT_ROLE=polecat" "HEAD:$DEFAULT_BRANCH"
cleanup

# Test 14: Live push of a plain branch ref while standing elsewhere — REFUSED
echo "Test 14: LIVE polecat plain-branch push while on another branch"
setup_repos "$TMPDIR/town/beads/polecats/chrome/beads"
cd "$TMPDIR/town/beads/polecats/chrome/beads"
# Advance the default branch with a real commit (so pushing it is a real push,
# not an up-to-date no-op), then stand on a session branch and push the default
# branch ref - the off-branch guard's exact shape.
commit_new
git checkout -b session/wip >/dev/null 2>&1
assert_live_block "Live polecat \$DEFAULT_BRANCH push from session branch refused" "" "$DEFAULT_BRANCH"
cleanup

# Test 14b: Live push of the default branch by a NON-polecat (env clean,
# non-polecat cwd) — ALLOWED. This is the crew/Refinery path the allowlist is
# built around: the default branch is explicitly on the allowlist for every
# caller. It is also the live regression guard for the hook's own shebang:
# without `#!/usr/bin/env bash` git runs the hook under /bin/sh, where the
# `< <(...)` process-substitution re-feed at the bottom is a syntax error and
# the hook dies, refusing EVERY push (discovered porting gt-ibt8, gt-93od).
echo "Test 14b: LIVE non-polecat default push allowed (shebang regression guard)"
setup_repos
cd "$TMPDIR/local"
# setup_repos already pushed the initial commit, so HEAD:main would be
# up-to-date; add a real commit so git performs a live push and exercises the
# hook the same way tests 12-14 do.
echo "second" >> file.txt
git add file.txt
git commit -m "second" >/dev/null 2>&1
assert_live_pass "Live non-polecat default push allowed" "" "HEAD:$DEFAULT_BRANCH"
cleanup

# Test 15: GT_DONE_DIRECT_MERGE=1 from a polecat — ALLOWED (deliberate gt done
# direct-merge landing, no separate identity required)
echo "Test 15: Default branch push with GT_DONE_DIRECT_MERGE=1 (polecat)"
setup_repos
cd "$TMPDIR/local"
remote_sha=$(get_sha HEAD)
echo "direct merge" >> file.txt
git add file.txt && git commit -m "direct merge" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_pass "GT_DONE_DIRECT_MERGE=1 allowed for polecat" \
  run_hook_env "GT_ROLE=polecat GT_DONE_DIRECT_MERGE=1" "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 16: GT_REFINERY_MERGE=1 with Refinery corroboration (GT_REFINERY=1) —
# ALLOWED even when GT_ROLE looks polecat-ish; corroboration from the
# GT_REFINERY spawn var wins over a bare flag.
echo "Test 16: GT_REFINERY_MERGE=1 + GT_REFINERY=1 corroboration"
setup_repos
cd "$TMPDIR/local"
remote_sha=$(get_sha HEAD)
echo "refinery merge" >> file.txt
git add file.txt && git commit -m "refinery merge" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_pass "GT_REFINERY_MERGE=1 + GT_REFINERY=1 allowed" \
  run_hook_env "GT_REFINERY=1 GT_REFINERY_MERGE=1" "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 17: GT_REFINERY_MERGE=1 + GT_ROLE=refinery corroboration — ALLOWED
echo "Test 17: GT_REFINERY_MERGE=1 + GT_ROLE=refinery corroboration"
setup_repos
cd "$TMPDIR/local"
remote_sha=$(get_sha HEAD)
echo "refinery merge" >> file.txt
git add file.txt && git commit -m "refinery merge" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_pass "GT_REFINERY_MERGE=1 + GT_ROLE=refinery allowed" \
  run_hook_env "GT_ROLE=refinery GT_REFINERY_MERGE=1" "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 18: GT_REFINERY_MERGE=1 self-granted by a polecat (no GT_REFINERY) —
# REFUSED (gt-9tf9: a polecat-shaped GT_ROLE disqualifies the claim)
echo "Test 18: GT_REFINERY_MERGE=1 self-grant by polecat"
setup_repos
cd "$TMPDIR/local"
remote_sha=$(get_sha HEAD)
echo "spoofed merge" >> file.txt
git add file.txt && git commit -m "spoofed merge" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_block "GT_REFINERY_MERGE=1 without Refinery corroboration blocked" \
  run_hook_env "GT_ROLE=polecat GT_REFINERY_MERGE=1" "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 19: GT_REFINERY_MERGE=1 self-granted from a polecat cwd (no GT_ROLE) —
# REFUSED (the cwd signal alone still disqualifies the claim)
echo "Test 19: GT_REFINERY_MERGE=1 self-grant from polecat cwd"
setup_repos "$TMPDIR/town/beads/polecats/chrome/beads"
cd "$TMPDIR/town/beads/polecats/chrome/beads"
remote_sha=$(get_sha HEAD)
echo "spoofed merge" >> file.txt
git add file.txt && git commit -m "spoofed merge" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_block "GT_REFINERY_MERGE=1 from polecat cwd blocked" \
  run_hook_env "GT_REFINERY_MERGE=1" "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 20: BOTH allow flags set by a polecat — REFUSED (contradictory/tampered
# env fails closed, gt-9tf9)
echo "Test 20: Both GT_REFINERY_MERGE=1 and GT_DONE_DIRECT_MERGE=1"
setup_repos
cd "$TMPDIR/local"
remote_sha=$(get_sha HEAD)
echo "double flag" >> file.txt
git add file.txt && git commit -m "double flag" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_block "Both flags set blocked (contradictory env)" \
  run_hook_env "GT_ROLE=polecat GT_REFINERY_MERGE=1 GT_DONE_DIRECT_MERGE=1" "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 21: Crew (no polecat signals) default push with GT_REFINERY_MERGE set —
# still ALLOWED (the allow-flag gate only applies to polecat callers; a crew
# caller is not flagged and the flag is ignored)
echo "Test 21: Non-polecat default push with stray allow flags"
setup_repos
cd "$TMPDIR/local"
remote_sha=$(get_sha HEAD)
echo "crew change" >> file.txt
git add file.txt && git commit -m "crew change" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_pass "Non-polecat default push unaffected by allow flags" \
  run_hook_env "GT_REFINERY_MERGE=1 GT_DONE_DIRECT_MERGE=1" "refs/heads/$DEFAULT_BRANCH" "$local_sha" "refs/heads/$DEFAULT_BRANCH" "$remote_sha"
cleanup

# Test 22: Polecat push to polecat/* with allow flags — unaffected, ALLOWED
echo "Test 22: Polecat polecat/* push is unaffected by the main-push check"
setup_repos "$TMPDIR/town/beads/polecats/chrome/beads"
cd "$TMPDIR/town/beads/polecats/chrome/beads"
git checkout -b polecat/chrome/be-xyz >/dev/null 2>&1
echo "work" >> file.txt
git add file.txt && git commit -m "work" >/dev/null 2>&1
local_sha=$(get_sha HEAD)
assert_pass "Polecat working-branch push still allowed" \
  run_hook "refs/heads/polecat/chrome/be-xyz" "$local_sha" "refs/heads/polecat/chrome/be-xyz" "0000000000000000000000000000000000000000"
cleanup

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]]
