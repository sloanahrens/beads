# D7 deletions: trackers and the non-server storage stack

> **For agentic workers:** execute inline, one commit per cluster; every commit
> must pass `go build ./...` and `go vet ./...` under `-tags=gms_pure_go`.

**Goal:** Delete the external tracker integrations (be-xu2.1) and every storage
path the town does not run (be-xu2.2), leaving bd a server-mode-only CLI.

**Architecture:** Compiler-driven deletion. Delete a cluster's files, then fix
each remaining reference until build and vet pass. Where a deleted mode was a
branch inside shared code, keep the server-mode arm and drop the rest. The cgo
build adopts the behaviour the nocgo build already has (server only).

**Tech stack:** Go, Dolt server mode, cobra.

**Spec:** beads `be-xu2.1`, `be-xu2.2` (epic `be-xu2`, wayfinder D7);
`~/.claude/docs/research/deep-review/beads-integrations.md` (Delete candidates
ranks 1-2), `beads-storage.md` (Delete candidates), `synthesis.md` section 5.

## Global constraints

- All six town databases run `dolt_mode: server`; `external_ref` is empty in all six.
- Keep anything a server-mode `cmd/bd` path the town uses still calls; record the caller.
- No bd install, no live Dolt (:3307), no `bd sync`, no rebase, push the branch only.
- No AI attribution in commits.

## Inventory and caller evidence

| # | Delete | Size src / test | Callers outside the cluster (grep) | Verdict |
|---|---|---|---|---|
| 1 | internal/{linear,ado,gitlab,jira,github,notion,tracker} | 17.1k / 26.2k | only cmd/bd tracker files, sync_push_pull, sync_flags, config.go (prefix list) | delete |
| 1 | cmd/bd {linear,ado,gitlab,jira,github,notion,sync_push_pull,sync_flags,federation,federation_nocgo}.go + tests | 6.6k / 6.6k | no gastown, formula, plugin, or ~/.claude caller of `bd <tracker>` or `bd federation` | delete |
| 1 | go.mod: html-to-markdown/v2, bluemonday, goldmark | | importers only inside cluster 1 | drop via `go mod tidy` |
| 2 | cmd/bd `*_proxied_server.go` (58), `proxied*.go`, proxied tests, init/migrate proxied-server modes | 10.9k / 32.7k | main.go builds `uowProvider` only when `proxiedServerMode` (main.go:1644) | delete |
| 2 | internal/storage/{uow,dbproxy,domain/db}, cmd/bd db_proxy_child.go, uow_factory external path | | **`bd serve`**: serve.go:377 `newSQLServerUOWProvider` -> `uow.NewExternalDoltServerUOWProvider`; internal/httpapi imports uow (5 files) | **keep for be-xu2.3** (httpapi + bd serve) |
| 3 | internal/storage/embeddeddolt + cmd/bd `*_embedded_test.go` | 6.4k / 18.2k + 28.5k | store_factory.go, init.go, main.go, bootstrap.go, doctor, legacy_upgrade_guard.go, beads_cgo.go | delete; cgo build becomes the nocgo server-only shape |
| 4 | internal/storage/backends, backendnames, `backend.Register/Deregister/Lookup/Registered/WorkspaceIsBeadsDir` | 0.3k | only `backend/backend.go:148`; no Register call anywhere | delete; backend/conformance stays (dolt contract tests use it) |
| 5 | `VersionControl.Checkout`, `DeleteBranch` | | zero non-test callers | delete |
| 5 | Iter methods: IterIssues, IterDependenciesWithMetadata, IterAllEventsSince, IterReadyWork, IterBlockedIssues, IterWisps, IterAllDependencyRecords | | zero callers outside telemetry wrapper and conformance | delete |
| 5 | IterEvents, IterDependentsWithMetadata, IterIssueComments | | **live**: history.go:139; workapi/detail.go:326,330 via dolt/issue_reader.go storereader (`bd show`) | **keep** |
| 5 | `RecoverPreV56DoltDir` auto-call (version_tracking.go:214) | | destructive, no pre-0.56 databases | delete call and function |
| 5 | stale `"events"` in `doltAddAndCommitInTx` table lists (CloseIssue issues.go:601 and siblings) | | events is dolt-ignored since migration 0062 | drop the entry |

## Tasks

### Task 1: Trackers (be-xu2.1)
- [ ] Delete the internal packages and cmd files in rows 1.
- [ ] config.go `allRecognizedConfigPrefixes`: drop the `tracker.List()` loop.
- [ ] yaml_config.go: drop the tracker secret keys and tracker prefixes.
- [ ] Remove command registrations and help groups that name deleted commands.
- [ ] `go build ./... && go vet ./...`; commit; `bd comments add be-xu2.1`.

### Task 2: Proxied CLI mode (be-xu2.2)
- [ ] Delete the 58 duals, proxied helpers, proxied tests.
- [ ] main.go: remove the proxied branch and every `uowProvider != nil` dual dispatch.
- [ ] init.go / migrate_dolt_mode.go: remove `--proxied-server`, `--team-server`, and proxied migrations; metadata with `dolt_mode: proxied-server` fails loudly.
- [ ] Keep uow_factory's server-mode topology and external provider for bd serve.
- [ ] Build, vet, commit, bead comment.

### Task 3: embeddeddolt
- [ ] Delete the package, cmd embedded tests, embedded arms of store_factory, init, main, doctor, bootstrap, legacy guard, root beads_cgo.go.
- [ ] Merge store_factory.go and store_factory_nocgo.go into one server-only factory.
- [ ] Build, vet, commit, bead comment.

### Task 4: Backend registry
- [ ] Delete backends, backendnames, the registry functions in backend/backend.go and their callers (backend_support.go, store_factory, beads_*.go, internal/beads, configfile).
- [ ] Build, vet, commit, bead comment.

### Task 5: Dead methods and stale entries
- [ ] Remove Checkout/DeleteBranch, seven Iter methods (interface, dolt impl, telemetry wrapper, conformance cases), RecoverPreV56DoltDir, stale "events" entries.
- [ ] Build, vet, commit, bead comment.

### Task 6: go.mod tidy, docs, gates
- [ ] `go mod tidy`; commit.
- [ ] Update docs that name deleted commands so docsync and doc-freshness pass.
- [ ] Gates by exit code: build, vet, `make ci-pr-lint`, tidy no-diff, `go test -timeout 45m` over touched packages + `./cmd/bd/...`.
- [ ] `om review -base origin/main`; fix blockers and majors.
- [ ] Attribution grep empty; `git push origin crew/sloan/be-delete`.
