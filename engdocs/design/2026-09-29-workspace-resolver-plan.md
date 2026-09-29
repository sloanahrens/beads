# Workspace Resolver Implementation Plan (be-h0k)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One workspace resolver, `workspace.Resolve(cwd, env)`, that config loading, database discovery and gate-path lookup share, so a git worktree whose `.beads/redirect` points at a rig reads the rig's `config.yaml` and not an unrelated ancestor's.

**Architecture:** A new leaf package `internal/workspace` owns redirect semantics, the workspace-marker predicates, explicit-directory discovery (moved from `beads.FindBeadsDirFrom`) and the single interpretation of `BEADS_DIR`. `internal/config` cannot import `internal/beads` (beads -> configfile -> config), which is why the resolver lives below both. `internal/beads` keeps its exported API as thin wrappers.

**Tech Stack:** Go, viper, git CLI (for worktree fallback), `internal/workspacegate` for gate paths.

**Spec:** bead be-h0k; deep review B3-07 and "Refactor candidates" (`~/.claude/docs/research/deep-review/beads-domain-core.md`); B5-10 (`beads-cross-cutting-contract.md`).

## Findings that shape the plan

- `config.Initialize` walks up from cwd for the first `.beads/config.yaml` and never reads `redirect`. In a polecat worktree (tracked `config.yaml` hidden, `.beads/redirect -> ../../../mayor/rig/.beads`) the walk reaches the town HQ `~/gt/.beads/config.yaml` (`issue-prefix: hq`). Reproduced below in a unit test at the `config.Initialize` level.
- In the CLI, `prepareSelectedCommandContext` later sets `BEADS_DIR` to the selected (redirect-followed) directory and re-runs `config.Initialize`, which masks the wrong walk for post-selection reads. Pre-selection reads (json, readonly, db, actor, dolt.auto-commit) and library consumers still see the wrong file.
- The live symptom named in the bead, `bd config get issue-prefix` printing "(not set)", has a second, separate cause: `config get` looks the hyphen key up in the database, where the prefix is stored as `issue_prefix`. It prints "(not set)" inside `mayor/rig` too. `bd create` reads YAML `issue-prefix` then DB `issue_prefix`; `config get issue-prefix` must report that same effective value.

## Global Constraints

- Do not change gastown. Do not touch live town dirs except read-only.
- Redirect relative paths resolve against the directory that CONTAINS `.beads` (the project root). This is the existing documented rule, and the live redirect `../../../mayor/rig/.beads` only works under it. Resolving against `.beads` itself would break every existing redirect.
- One hop only. A self-redirect or a redirect whose target redirects back to the source is a loop and an error. A missing, non-directory or marker-less target is an error. A non-loop chain follows one hop and warns (existing behavior).
- `BEADS_DIR` is interpreted in exactly one function, `workspace.FromEnv`; `FindBeadsDir`, `FindDatabasePath` and `config.Initialize` all call it.
- No new exported API is removed from `internal/beads`; callers keep compiling.

## Resolver contract

```go
package workspace

type Env func(key string) string // os.Getenv in production

type Workspace struct {
    BeadsDir     string // effective .beads dir, after one redirect hop
    SourceDir    string // .beads dir discovery found before the redirect ("" when from BEADS_DIR)
    Redirected   bool
    FromEnv      bool   // BEADS_DIR selected it
    ConfigPath   string // BeadsDir/config.yaml (may not exist)
    MetadataPath string // BeadsDir/metadata.json: database identity lives here
    GatePath     string // workspacegate.ForWorkspace(BeadsDir).Path()
}

func Resolve(cwd string, env Env) (Workspace, error)
func FromEnv(env Env) (beadsDir string, set bool, err error)
func FollowRedirect(beadsDir string) (target string, redirected bool, err error)
type FollowFunc func(beadsDir string) (target string, redirected bool, err error)
func Discover(startDir string, follow FollowFunc) (source, resolved string, err error)
func CanonicalizeBeadsDir(dir string) string; func WorktreeFallbackBeadsDir(repoPath string) string
func HasProjectFiles(dir string) bool; func HasWorkspaceMarker(dir string) bool; func HasDatabase(dir string) bool
```

Precedence: `BEADS_DIR` (canonicalized, redirect-followed) > discovery from cwd (walk up, following each `.beads/redirect`, with git worktree and jj fallbacks) > none. `ErrNoWorkspace` when nothing is found.

## Task 1: `internal/workspace` package with redirect rules and Resolve

**Files:** Create `internal/workspace/workspace.go`, `internal/workspace/redirect.go`, `internal/workspace/discover.go`, `internal/workspace/workspace_test.go`.

- [ ] Write failing tests: temp tree `town/.beads/config.yaml (issue-prefix: hq)`, `town/rig/mayor/rig/.beads/{config.yaml (issue-prefix: zz), metadata.json}`, `town/rig/polecats/amber/rig/.beads/redirect = ../../../mayor/rig/.beads`. Assert `Resolve(worktree)` gives BeadsDir, ConfigPath, MetadataPath and GatePath all under the rig `.beads`. Add loop, missing-target, marker-less target, comment-line and BEADS_DIR precedence cases. Repeat with a real `git worktree add` so the worktree fallback branch runs.
- [ ] Run `go test ./internal/workspace/` and see it fail (package missing).
- [ ] Move `canonicalizeBeadsDirPath`, `preferStableBranchWorktreeBeadsDir`, `listWorktrees`, `gitOutput`, `worktreeFallbackBeadsDirForRepo`, marker predicates and `FindBeadsDirFrom`'s body into the package; add strict `FollowRedirect`, `FromEnv`, `Resolve`.
- [ ] Tests pass. Commit `feat(workspace): one resolver for beads dir, config, metadata and gate paths (be-h0k)`.

## Task 2: `internal/beads` delegates

**Files:** Modify `internal/beads/beads.go`.

- [ ] `FollowRedirect(dir) string` wraps `workspace.FollowRedirect`, keeping its warn-and-fall-back contract. `HasBeadsProjectFiles`, `HasWorkspaceMarker`, `hasBeadsDatabase`, `FindBeadsDirFrom`, `canonicalizeBeadsDirPath` delegate. `FindBeadsDir` and `FindDatabasePath` take BEADS_DIR from `workspace.FromEnv`.
- [ ] Add a test: from inside the redirected worktree, `FindBeadsDir()` equals `workspace.Resolve(...).BeadsDir` and `FindDatabasePath()` lies under it.
- [ ] `go test ./internal/beads/` passes. Commit.

## Task 3: config loading uses the resolver

**Files:** Modify `internal/config/config.go`; test `internal/config/config_redirect_test.go`.

- [ ] Failing test first: chdir into the redirected worktree, unset BEADS_DIR, `Initialize()`, expect `GetString("issue-prefix") == "zz"` and `ConfigFileUsed()` under the rig. Fails on main with "hq".
- [ ] Replace the cwd walk, the worktree fallback and the separate BEADS_DIR block with one `workspace.Resolve`. Keep the `BEADS_TEST_IGNORE_REPO_CONFIG` ignore set and the module-root boundary (be-yjp4z): a resolved workspace outside the module root is ignored under that flag. A resolve error is returned from Initialize after defaults are set; every caller prints it and continues. A discovered workspace with a broken redirect loads no project config; a BEADS_DIR with a broken redirect loads BEADS_DIR's own config.yaml, where internal/beads also falls back for the database. Config keeps the caller's path spelling when no redirect was followed (ConfigFileUsed and external_projects have always used it).
- [ ] `go test ./internal/config/` passes. Commit.

## Task 4: `bd config get issue-prefix` reports the effective prefix

**Files:** Modify `cmd/bd/config.go`; test `cmd/bd/config_redirect_embedded_test.go` (cgo, embedded Dolt, no Docker).

- [ ] Failing test first: `bd init --prefix zz` in a rig dir, HQ `config.yaml` with `issue-prefix: hq` above it, a worktree dir whose `.beads` holds only `redirect`. `bd config get issue-prefix` from the worktree prints `zz`; `--json` carries value `zz`. Also from the rig itself.
- [ ] `issue-prefix` resolves as YAML `issue-prefix` then DB `issue_prefix`, the same order `bd create` uses. `issue_prefix` keeps its raw DB meaning.
- [ ] Test passes. Commit.

## Task 5: gates, review, push

- [ ] `go build ./...`, `make ci-pr-lint` or `go vet ./...`, `go test` for workspace, beads, config, workspacegate and `./cmd/bd/...` (touched packages plus importers), by exit code.
- [ ] `om review -base origin/main`; fix blockers and majors.
- [ ] Manual read-only check from `~/gt/gastown/polecats/amber/gastown` with the scratch binary.
- [ ] Attribution grep empty; `git push origin crew/sloan/be-resolver`.

## Out of scope

- Converging `FindBeadsDir`'s process-cwd walk (stops at the git root) with `Discover`'s explicit-dir walk (walks to `/`). Both now share redirect rules, markers and env handling; their walk bounds still differ and are covered by existing tests. Follow-up: be-8ff.
- Gastown's own redirect reader (B5-10) and its skip-worktree workaround (gt-y3pgh.8).
