# Fork Fixes

Fixes applied to the beads fork that are not yet upstream.

## 2026-02-04: Fix flaky tests on macOS

**Commit:** `aefe3be4`
**Files changed:** 2 test files (no production code)

### GitLab Context Cancellation Tests

**Problem:** `TestFetchIssues_ContextCancellation` and `TestFetchIssuesSince_ContextCancellation` in `internal/gitlab/client_test.go` fail consistently.

The tests verify that pagination loops respect `context.WithCancel`. They spin up a mock HTTP server that always returns `X-Next-Page: 2` (infinite pagination), cancel the context after 50ms, and assert the loop stops with `context.Canceled`.

However, the mock server responds instantly — no network latency, no I/O wait. The pagination loop completes all 1000 iterations (the `MaxPages` safety limit) in under 50ms, before the goroutine's `time.Sleep(50ms)` fires the cancellation. The loop hits the pagination limit first, returning `"pagination limit exceeded"` instead of `context.Canceled`.

**Fix:** Added `time.Sleep(100 * time.Microsecond)` per request in the mock server handler. With 100µs per request, ~500 requests fit in 50ms — well under the 1000-page limit. Context cancellation now consistently wins the race. The tests still run in ~50ms total.

### Syncbranch Path Comparison Test

**Problem:** `TestFreshCloneScenario` in `internal/syncbranch/ensure_worktree_test.go` fails on macOS.

The test creates a temporary worktree and compares the path returned by `EnsureWorktree()` against `getBeadsWorktreePath()`. On macOS, `/var` is a symlink to `/private/var`. One function returns the symlink path (`/var/folders/...`), the other returns the resolved path (`/private/var/folders/...`). Direct string comparison fails.

**Fix:** Added `filepath.EvalSymlinks()` to canonicalize both paths before comparison. Falls back to the original path if `EvalSymlinks` errors (e.g., path doesn't exist yet).
