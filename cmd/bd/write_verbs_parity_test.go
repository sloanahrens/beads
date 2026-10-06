//go:build cgo

// Characterization ("parity") suite for the four write verbs — bd create,
// bd update, bd close, bd reopen — pinning the observable CLI contract as it
// stands BEFORE cmd/bd is rewired onto the issue-operations facade
// (internal/storage/issueops).
//
// Everything here is a statement about what the CLI does TODAY, not about what
// it ought to do. Several pinned behaviors are known bugs; four of them have
// already been adjudicated and WILL change during the rewire:
//
//	R1  bd create --id <occupied>  -> silent full-row upsert reporting success
//	                                  [FLIPPED to a refusal by the bd create rewire]
//	R2  compound bd update         -> one store call (= one hook firing) per
//	                                  field/label/parent edit, plus phantom
//	                                  label_added/label_removed events
//	                                  [FLIPPED to one atomic op by the bd update
//	                                  rewire]
//	R3  bd update --parent         -> removes only the FIRST parent edge
//	                                  [FLIPPED to replace-all by the bd update
//	                                  rewire]
//	R4  bd reopen on a non-done,
//	    non-open status            -> prints "↻ Reopened" and reports success
//	                                  [FLIPPED to "nothing to do" by the bd
//	                                  reopen rewire]
//
// Assertions covering those four are tagged `RULING Rn`. When a rewire commit
// flips one, the assertion is updated IN THAT COMMIT with a comment naming the
// ruling. Any OTHER assertion in this file changing is a regression, not a
// refactor. All four have now landed; every remaining assertion is unchanged
// from the pre-rewire CLI and must stay that way.
//
// Harness: the commands' RunE functions are invoked in-process against a real
// storage.DoltStorage, with stdout/stderr captured and the returned error
// mapped to the exit code main.go would produce. The store is wrapped in
// parityStore, a counting decorator shaped exactly like
// storage.HookFiringStore (embed + Unwrap), so mutation-call counts stand in
// for hook-firing counts: HookFiringStore fires exactly one hook per mutating
// store call, so "N store calls" is "N hook firings".

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/steveyegge/beads"
	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
	"github.com/steveyegge/beads/issueops"
)

// ===== counting store decorator =====

// parityStore counts the mutating store calls a command makes. It mirrors
// storage.HookFiringStore's shape (embedded interface for passthrough, inner
// for the real calls, Unwrap for storage.UnwrapStore) so the counts equal the
// number of hooks production would fire for the same command.
type parityStore struct {
	storage.DoltStorage
	inner storage.DoltStorage

	mu    sync.Mutex
	calls []string
}

func newParityStore(inner storage.DoltStorage) *parityStore {
	return &parityStore{DoltStorage: inner, inner: inner}
}

func (p *parityStore) Unwrap() storage.DoltStorage { return p.inner }

func (p *parityStore) record(name string) {
	p.mu.Lock()
	p.calls = append(p.calls, name)
	p.mu.Unlock()
}

func (p *parityStore) mutations() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

func (p *parityStore) reset() {
	p.mu.Lock()
	p.calls = nil
	p.mu.Unlock()
}

func (p *parityStore) CreateIssue(ctx context.Context, issue *types.Issue, actor string) error {
	p.record("CreateIssue")
	return p.inner.CreateIssue(ctx, issue, actor)
}

func (p *parityStore) UpdateIssue(ctx context.Context, id string, updates map[string]interface{}, actor string) error {
	p.record("UpdateIssue")
	return p.inner.UpdateIssue(ctx, id, updates, actor)
}

func (p *parityStore) UpdateIssueChecked(ctx context.Context, id string, updates map[string]interface{}, actor string, opts storage.UpdateIssueOptions) error {
	p.record("UpdateIssueChecked")
	return p.inner.UpdateIssueChecked(ctx, id, updates, actor, opts)
}

func (p *parityStore) UpdateIssueType(ctx context.Context, id, issueType, actor string) error {
	p.record("UpdateIssueType")
	return p.inner.UpdateIssueType(ctx, id, issueType, actor)
}

func (p *parityStore) ClaimIssue(ctx context.Context, id, actor string) error {
	p.record("ClaimIssue")
	return p.inner.ClaimIssue(ctx, id, actor)
}

func (p *parityStore) ReopenIssue(ctx context.Context, id, reason, actor string) error {
	p.record("ReopenIssue")
	return p.inner.ReopenIssue(ctx, id, reason, actor)
}

func (p *parityStore) CloseIssue(ctx context.Context, id, reason, actor, session string) error {
	p.record("CloseIssue")
	return p.inner.CloseIssue(ctx, id, reason, actor, session)
}

func (p *parityStore) CloseIssueChecked(ctx context.Context, id, actor string, opts storage.CloseIssueOptions) (storage.CloseIssueResult, error) {
	p.record("CloseIssueChecked")
	return p.inner.CloseIssueChecked(ctx, id, actor, opts)
}

func (p *parityStore) AddLabel(ctx context.Context, issueID, label, actor string) error {
	p.record("AddLabel")
	return p.inner.AddLabel(ctx, issueID, label, actor)
}

func (p *parityStore) RemoveLabel(ctx context.Context, issueID, label, actor string) error {
	p.record("RemoveLabel")
	return p.inner.RemoveLabel(ctx, issueID, label, actor)
}

func (p *parityStore) AddDependency(ctx context.Context, dep *types.Dependency, actor string) error {
	p.record("AddDependency")
	return p.inner.AddDependency(ctx, dep, actor)
}

func (p *parityStore) RemoveDependency(ctx context.Context, issueID, dependsOnID, actor string) error {
	p.record("RemoveDependency")
	return p.inner.RemoveDependency(ctx, issueID, dependsOnID, actor)
}

// ===== counting issue-operations facade =====

// parityOps is parityStore's counterpart on the issue-operations facade, the
// surface the write verbs move onto. It records into the SAME call list, so
// `mutations()` stays one entry per hook firing however the verb reached the
// database — a verb still on a direct store call records "UpdateIssue", a
// rewired verb records "Update".
//
// The firing rules mirror beads.hookIssueOperations exactly, which is what
// makes the counts comparable across the rewire: Create, Update and Close fire
// their completion hook on any success; Reopen fires only when it changed
// something.
//
// A facade is required here because the lifecycle accessor never routes
// through the DoltStorage methods parityStore decorates — it builds operations
// straight off the concrete store — so once a verb is rewired, store-level
// counting goes blind and every "no store mutations" assertion would pass
// vacuously.
type parityOps struct {
	inner issueops.Lifecycle
	store *parityStore
}

func (o *parityOps) Create(ctx context.Context, request issueops.CreateRequest) (issueops.CreateResult, error) {
	result, err := o.inner.Create(ctx, request)
	if err == nil {
		o.store.record("Create")
	}
	return result, err
}

func (o *parityOps) Update(ctx context.Context, request issueops.UpdateRequest) (issueops.UpdateResult, error) {
	result, err := o.inner.Update(ctx, request)
	if err == nil {
		o.store.record("Update")
	}
	return result, err
}

func (o *parityOps) Close(ctx context.Context, request issueops.CloseRequest) (issueops.CloseResult, error) {
	result, err := o.inner.Close(ctx, request)
	if err == nil {
		o.store.record("Close")
	}
	return result, err
}

func (o *parityOps) Reopen(ctx context.Context, request issueops.ReopenRequest) (issueops.ReopenResult, error) {
	result, err := o.inner.Reopen(ctx, request)
	if err == nil && result.Changed {
		o.store.record("Reopen")
	}
	return result, err
}

// ===== harness =====

type parityEnv struct {
	t        *testing.T
	store    *parityStore
	beadsDir string
}

// newParityEnv wires the package globals the write-verb RunE functions read
// (store, rootCtx, actor, jsonOutput, quietFlag, readonlyMode) at a real Dolt
// store, points BEADS_DIR at its .beads dir so SetLastTouchedID is observable,
// and restores everything on cleanup.
//
// quietFlag is set so `bd create`'s random maybeShowTip line cannot appear on
// stdout mid-assertion. It does NOT suppress the "✓ Created issue:" lines:
// those go through debug.PrintNormal, which reads the debug package's own
// quiet flag (set by main.go's PersistentPreRun, which RunE-level tests skip).
// parityOwnerEmail pins the git identity create derives its owner field from.
const parityOwnerEmail = "parity-owner@test"

func newParityEnv(t *testing.T) *parityEnv {
	t.Helper()

	saveAndRestoreGlobals(t)
	ensureCleanGlobalState(t)
	initConfigForTest(t)

	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	raw := newTestStore(t, filepath.Join(beadsDir, "beads.db"))
	ps := newParityStore(raw)

	savedCtx, savedJSON, savedActor := rootCtx, jsonOutput, actor
	savedQuiet, savedReadonly := quietFlag, readonlyMode
	savedNewOps := newIssueOperations
	t.Cleanup(func() {
		rootCtx, jsonOutput, actor = savedCtx, savedJSON, savedActor
		quietFlag, readonlyMode = savedQuiet, savedReadonly
		newIssueOperations = savedNewOps
	})

	// Count the facade operations the write verbs perform. The real store is
	// unwrapped first so the counted lifecycle is the concrete store's,
	// whatever parityStore's own passthrough happens to promote.
	newIssueOperations = func(target beads.Storage) (issueops.Lifecycle, error) {
		if decorated, ok := target.(storage.DoltStorage); ok {
			target = storage.UnwrapStore(decorated)
		}
		inner, err := target.IssueLifecycle()
		if err != nil {
			return nil, err
		}
		return &parityOps{inner: inner, store: ps}, nil
	}

	store = ps
	rootCtx = context.Background()
	jsonOutput = false
	actor = "parity-actor"
	quietFlag = true
	readonlyMode = false

	t.Setenv("NO_COLOR", "1")
	// getOwner reads GIT_AUTHOR_EMAIL, then falls back to git config user.email, so
	// create's owner field — and therefore its --json key set — otherwise varies with
	// the ambient git identity. Pin it: CI commonly sets GIT_AUTHOR_EMAIL, a developer
	// shell usually does not, and the suite must render the same verdict in both.
	t.Setenv("GIT_AUTHOR_EMAIL", parityOwnerEmail)
	t.Setenv("BEADS_DIR", beadsDir)

	// Pin every config key the write verbs read. config.Initialize() merges
	// whatever config.yaml files happen to exist under the test HOME, and
	// other tests in this package mutate these keys globally — an inherited
	// value would either break these tests or, worse, silently weaken them
	// (output.title-length=0 makes formatFeedbackID drop the title, which
	// would turn the exact-line assertions into vacuous id-only comparisons).
	setParityConfig(t, map[string]any{
		"issue-prefix":               "", // fall through to the store's "test"
		"output.title-length":        255,
		"create.require-description": false,
		"validation.on-create":       "",
		"validation.on-close":        "",
		"routing.mode":               "",
		"routing.default":            "",
		"routing.maintainer":         "",
		"routing.contributor":        "",
	})

	// Styling must be inert or the exact-line assertions below are comparing
	// against ANSI-wrapped text. Fail loudly rather than silently mismatch.
	if got := ui.RenderPass("✓"); got != "✓" {
		t.Fatalf("parity harness requires unstyled output; ui.RenderPass(\"✓\") = %q", got)
	}
	// Guard the pin above: the human-output assertions are only meaningful
	// while formatFeedbackID actually interpolates the title.
	if got := formatFeedbackID("x-1", "T"); got != "x-1 — T" {
		t.Fatalf("parity harness requires title interpolation; formatFeedbackID = %q", got)
	}

	env := &parityEnv{t: t, store: ps, beadsDir: beadsDir}
	env.clearLastTouched()
	return env
}

// setParityConfig overrides config keys for the duration of the test. The
// enclosing initConfigForTest already registers config.ResetForTesting as
// cleanup, which drops these along with the rest of the viper state.
func setParityConfig(t *testing.T, kv map[string]any) {
	t.Helper()
	for key, value := range kv {
		config.Set(key, value)
	}
}

func (e *parityEnv) clearLastTouched() {
	e.t.Helper()
	_ = os.Remove(filepath.Join(e.beadsDir, lastTouchedFile))
}

func (e *parityEnv) lastTouched() string {
	e.t.Helper()
	data, err := os.ReadFile(filepath.Join(e.beadsDir, lastTouchedFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// runResult is everything a shell would observe from one command invocation.
type runResult struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
}

// run invokes cmd.RunE directly (as the other RunE-level tests in this package
// do), capturing both streams and mapping the returned error to the exit code
// main.go's run() would produce for it.
func (e *parityEnv) run(cmd *cobra.Command, args ...string) runResult {
	e.t.Helper()

	stdioMutex.Lock()
	defer stdioMutex.Unlock()

	oldOut, oldErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		e.t.Fatalf("os.Pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		e.t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout, os.Stderr = outW, errW

	var outBuf, errBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = io.Copy(&outBuf, outR) }()
	go func() { defer wg.Done(); _, _ = io.Copy(&errBuf, errR) }()

	runErr := cmd.RunE(cmd, args)

	_ = outW.Close()
	_ = errW.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	wg.Wait()
	_ = outR.Close()
	_ = errR.Close()

	return runResult{
		stdout:   outBuf.String(),
		stderr:   errBuf.String(),
		exitCode: parityExitCode(runErr),
		err:      runErr,
	}
}

// parityExitCode mirrors main.go's error→exit-status mapping.
func parityExitCode(err error) int {
	if err == nil {
		return 0
	}
	if code, ok := exitCodeFromError(err); ok {
		return code
	}
	return 1
}

// setFlags sets flags on cmd and registers a cleanup that returns every flag
// on the command to its declared default. The command objects are package
// globals shared by the whole test binary, so leaking a set flag silently
// corrupts unrelated tests.
func (e *parityEnv) setFlags(cmd *cobra.Command, kv map[string]string) {
	e.t.Helper()
	e.t.Cleanup(func() { resetCommandFlagsToDefaults(cmd) })
	for name, value := range kv {
		if err := cmd.Flags().Set(name, value); err != nil {
			e.t.Fatalf("set --%s=%q: %v", name, value, err)
		}
	}
}

func resetCommandFlagsToDefaults(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		switch v := f.Value.(type) {
		case *closeReasonFlagValue:
			// Accumulating flag: Set appends, so it cannot be reset by Set.
			v.values = nil
		case pflag.SliceValue:
			_ = v.Replace(nil)
		default:
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
}

// seed creates an issue directly through the store (bypassing the CLI) so
// fixtures never depend on the behavior under test.
func (e *parityEnv) seed(id, title string, mutate func(*types.Issue)) *types.Issue {
	e.t.Helper()
	issue := &types.Issue{
		ID:        id,
		Title:     title,
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
		CreatedBy: "parity-seed",
	}
	if mutate != nil {
		mutate(issue)
	}
	if err := e.store.inner.CreateIssue(rootCtx, issue, "parity-seed"); err != nil {
		e.t.Fatalf("seed %s: %v", id, err)
	}
	e.store.reset()
	return issue
}

func (e *parityEnv) get(id string) *types.Issue {
	e.t.Helper()
	issue, err := e.store.inner.GetIssue(rootCtx, id)
	if err != nil {
		e.t.Fatalf("GetIssue(%s): %v", id, err)
	}
	return issue
}

func (e *parityEnv) eventTypes(id string) []string {
	e.t.Helper()
	events, err := e.store.inner.GetEvents(rootCtx, id, 0)
	if err != nil {
		e.t.Fatalf("GetEvents(%s): %v", id, err)
	}
	out := make([]string, 0, len(events))
	for _, ev := range events {
		out = append(out, string(ev.EventType))
	}
	return out
}

func countOf(values []string, want string) int {
	n := 0
	for _, v := range values {
		if v == want {
			n++
		}
	}
	return n
}

// decodeJSONObject asserts stdout is exactly one pretty-printed JSON object
// and returns it as a map (key order is not preserved; use rawJSONKeyOrder for
// that).
func decodeJSONObject(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var obj map[string]any
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&obj); err != nil {
		t.Fatalf("stdout is not a single JSON object: %v\nstdout:\n%s", err, stdout)
	}
	if rest, _ := io.ReadAll(dec.Buffered()); strings.TrimSpace(string(rest)) != "" {
		t.Fatalf("trailing content after JSON object: %q\nstdout:\n%s", rest, stdout)
	}
	return obj
}

func decodeJSONArray(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var arr []map[string]any
	dec := json.NewDecoder(strings.NewReader(stdout))
	if err := dec.Decode(&arr); err != nil {
		t.Fatalf("stdout is not a JSON array: %v\nstdout:\n%s", err, stdout)
	}
	return arr
}

func parityJSONKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Sorted so the failure message is stable and diffable.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func assertKeySet(t *testing.T, obj map[string]any, want []string) {
	t.Helper()
	got := parityJSONKeys(obj)
	wantSorted := append([]string(nil), want...)
	for i := 1; i < len(wantSorted); i++ {
		for j := i; j > 0 && wantSorted[j] < wantSorted[j-1]; j-- {
			wantSorted[j], wantSorted[j-1] = wantSorted[j-1], wantSorted[j]
		}
	}
	if strings.Join(got, ",") != strings.Join(wantSorted, ",") {
		t.Errorf("JSON key set mismatch\n got: %v\nwant: %v", got, wantSorted)
	}
}

// rawJSONKeyOrder returns the top-level keys of a JSON object in the order
// they appear in the byte stream. Key ORDER is part of the byte shape: an
// object built by re-marshaling a map has sorted keys, one marshaled straight
// from a struct has struct-field order.
func rawJSONKeyOrder(t *testing.T, raw string) []string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("read JSON: %v", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		t.Fatalf("expected object, got %v", tok)
	}
	var keys []string
	depth := 0
	for dec.More() || depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("read JSON: %v", err)
		}
		if delim, ok := tok.(json.Delim); ok {
			switch delim {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth < 0 {
					return keys
				}
			}
			continue
		}
		if depth == 0 {
			key, ok := tok.(string)
			if !ok {
				t.Fatalf("expected object key, got %T %v", tok, tok)
			}
			keys = append(keys, key)
			// Consume the value.
			vtok, err := dec.Token()
			if err != nil {
				t.Fatalf("read JSON value: %v", err)
			}
			if delim, ok := vtok.(json.Delim); ok && (delim == '{' || delim == '[') {
				depth++
			}
		}
	}
	return keys
}

func assertRecentUTCTimestamp(t *testing.T, label, value string) {
	t.Helper()
	if !strings.HasSuffix(value, "Z") {
		t.Errorf("%s = %q; want a UTC (Z-suffixed) timestamp", label, value)
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("%s = %q is not RFC3339Nano: %v", label, value, err)
	}
	if ts.IsZero() {
		t.Errorf("%s is the zero time", label)
	}
	if delta := time.Since(ts); delta < -time.Minute || delta > 10*time.Minute {
		t.Errorf("%s = %q is %v away from now; want a freshly stamped time", label, value, delta)
	}
}

// ===== bd create =====

// closeRowParityExclusions lists the types.Issue fields a row-for-row
// comparison of `bd update -s closed` against `bd close` must skip, and why.
// Everything else has to match: the two verbs reach the same done state, so any
// other divergence is the update funnel failing to close the way close closes.
var closeRowParityExclusions = map[string]string{
	"ID":          "the two verbs act on two different issues",
	"Title":       "the two verbs act on two different issues",
	"ContentHash": "derives from ID and Title",
	"CreatedAt":   "wall-clock stamp from two different seeds",
	"UpdatedAt":   "wall-clock stamp from two different writes",
	"ClosedAt":    "wall-clock stamp; asserted non-nil on both instead",
	"RowVersion":  "freshRowLock() is regenerated per write by design",
	"CloseReason": "cmd/bd/close.go resolveCloseReasons defaults `bd close`'s " +
		"reason to \"Closed\" at the CLI layer, and `bd update` has no reason " +
		"flag; the funnel-level default is asserted separately below",
	// ga-ktn9pe.4.14 owns pin behavior in the update funnels — issueops
	// auto-clears `pinned` on a status change and domain/db does not. Comparing
	// it here would either bless that divergence or drag its fix into ga-kjkv1.
	"Pinned": "ga-ktn9pe.4.14 owns pin behavior in the update funnels",
}

func firstLine(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func isSortedStrings(s []string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] < s[i-1] {
			return false
		}
	}
	return true
}
