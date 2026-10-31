package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// showResolveTestStore answers `bd show --json`'s resolution out of memory:
// exact ids, the rows a partial id resolves to, and the lookups the partial-ID
// resolver runs. Every call that resolution makes is counted, so a test can
// tell a batched existence check from a per-id lookup.
type showResolveTestStore struct {
	storage.DoltStorage

	exact      map[string]struct{}
	issues     map[string]*types.Issue
	exactErr   error
	exactCalls int
	// searchCalls counts the per-id resolver's lookups: the exact-id search,
	// the prefix search and the row read all land here.
	searchCalls int
}

func (s *showResolveTestStore) ExactIssueIDs(_ context.Context, ids []string) (map[string]struct{}, error) {
	s.exactCalls++
	if s.exactErr != nil {
		return nil, s.exactErr
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := s.exact[id]; ok {
			set[id] = struct{}{}
		}
	}
	return set, nil
}

func (s *showResolveTestStore) SearchIssues(_ context.Context, _ string, filter types.IssueFilter) ([]*types.Issue, error) {
	s.searchCalls++
	var out []*types.Issue
	for _, id := range filter.IDs {
		if issue, ok := s.issues[id]; ok {
			out = append(out, issue)
		}
	}
	return out, nil
}

func (s *showResolveTestStore) SearchIssueIDs(_ context.Context, query string, filter types.IssueFilter) ([]string, error) {
	s.searchCalls++
	var out []string
	for id := range s.issues {
		switch {
		case filter.IDPrefix != "" && !strings.HasPrefix(id, filter.IDPrefix):
			continue
		case filter.IDPrefix == "" && query != "" && !strings.Contains(id, query):
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *showResolveTestStore) GetIssue(_ context.Context, id string) (*types.Issue, error) {
	s.searchCalls++
	if issue, ok := s.issues[id]; ok {
		return issue, nil
	}
	return nil, fmt.Errorf("%w: issue %s", storage.ErrNotFound, id)
}

func (s *showResolveTestStore) GetConfig(_ context.Context, key string) (string, error) {
	// The configured prefix is what turns "be-mix-2" into the hash-part search
	// the partial-ID resolver runs, so the fake carries the same one its ids do.
	if key == "issue_prefix" {
		return "be", nil
	}
	return "", nil
}

func (s *showResolveTestStore) GetAllConfig(_ context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func newShowResolveTestStore(ids ...string) *showResolveTestStore {
	s := &showResolveTestStore{
		exact:  make(map[string]struct{}, len(ids)),
		issues: make(map[string]*types.Issue, len(ids)),
	}
	for _, id := range ids {
		s.exact[id] = struct{}{}
		s.issues[id] = &types.Issue{ID: id, Title: "issue " + id, Status: types.StatusOpen}
	}
	return s
}

// canonicalAndFailures reduces resolutions to the two things the caller sees:
// the canonical ids that will be printed, in order, and the per-id failures.
func canonicalAndFailures(resolutions []showResolution) ([]string, []idOutcome) {
	var ids []string
	var failures []idOutcome
	for _, r := range resolutions {
		if r.failed {
			failures = append(failures, r.failure)
			continue
		}
		ids = append(ids, r.canonical)
	}
	return ids, failures
}

// TestExactLocalIDsResolvesFiveHundredIDsInOneLookup is the cost half of
// be-mfu5 at the command level: the ids the local store holds exactly must be
// settled by ONE existence lookup for the whole list, and none of them may
// reach the per-id resolver.
func TestExactLocalIDsResolvesFiveHundredIDsInOneLookup(t *testing.T) {
	ids := make([]string, 0, 500)
	for i := 0; i < 500; i++ {
		ids = append(ids, fmt.Sprintf("be-exact-%03d", i))
	}
	store := newShowResolveTestStore(ids...)
	ctx := context.Background()

	exact := exactLocalIDs(ctx, store, ids)
	if store.exactCalls != 1 {
		t.Errorf("exact lookups = %d, want 1 for %d ids", store.exactCalls, len(ids))
	}

	resolutions, open := resolveShowIDs(ctx, store, ids, exact)
	for _, r := range open {
		r.Close()
	}
	gotIDs, failures := canonicalAndFailures(resolutions)
	if len(failures) != 0 {
		t.Fatalf("failures = %d, want none", len(failures))
	}
	if len(gotIDs) != len(ids) {
		t.Fatalf("resolved %d ids, want %d", len(gotIDs), len(ids))
	}
	for i, id := range ids {
		if gotIDs[i] != id {
			t.Fatalf("resolved[%d] = %q, want %q", i, gotIDs[i], id)
		}
	}
	if store.searchCalls != 0 {
		t.Errorf("per-id resolver ran %d lookups for ids the batched lookup already settled", store.searchCalls)
	}
}

// TestShowExactIDSetSkipsCurrentPick pins the one caller-driven exception to
// the fast path: a --current pick keeps resolving per id, so the batched
// lookup must not run (and must not cost a query) for it.
func TestShowExactIDSetSkipsCurrentPick(t *testing.T) {
	store := newShowResolveTestStore("be-cur-1")
	ctx := context.Background()

	if set := showExactIDSet(ctx, store, []string{"be-cur-1"}, true); set != nil {
		t.Errorf("exact set for a --current pick = %v, want none", set)
	}
	if store.exactCalls != 0 {
		t.Errorf("exact lookups for a --current pick = %d, want 0", store.exactCalls)
	}

	set := showExactIDSet(ctx, store, []string{"be-cur-1"}, false)
	if _, ok := set["be-cur-1"]; !ok {
		t.Errorf("exact set for a named id = %v, want be-cur-1", set)
	}
	if store.exactCalls != 1 {
		t.Errorf("exact lookups for named ids = %d, want 1", store.exactCalls)
	}
}

// TestResolveShowIDsFallsBackWhenExactLookupFails drives the failure branch:
// an exact-id read that errors must not lose ids or change their outcome — it
// only costs the per-id resolver the fast path would have skipped.
func TestResolveShowIDsFallsBackWhenExactLookupFails(t *testing.T) {
	store := newShowResolveTestStore("be-fb-1", "be-fb-2bbb")
	store.exactErr = errors.New("store unavailable")
	ctx := context.Background()
	ids := []string{"be-fb-1", "be-fb-2", "be-zzz"}

	exact := exactLocalIDs(ctx, store, ids)
	if exact != nil {
		t.Fatalf("exact set after a failed lookup = %v, want none", exact)
	}
	if store.exactCalls != 1 {
		t.Errorf("exact lookups = %d, want the one that failed", store.exactCalls)
	}

	resolutions, open := resolveShowIDs(ctx, store, ids, exact)
	for _, r := range open {
		r.Close()
	}
	gotIDs, failures := canonicalAndFailures(resolutions)
	wantIDs := []string{"be-fb-1", "be-fb-2bbb"}
	if fmt.Sprint(gotIDs) != fmt.Sprint(wantIDs) {
		t.Errorf("ids = %v, want %v resolved per id", gotIDs, wantIDs)
	}
	if len(failures) != 1 || failures[0].ID != "be-zzz" {
		t.Errorf("failures = %+v, want just be-zzz", failures)
	}
	if store.searchCalls == 0 {
		t.Error("no per-id lookup ran after the batched lookup failed")
	}
}

// TestResolveShowIDsMixMatchesPerIDPath is the answer half of be-mfu5: an
// exact local id, a partial id, a missing id and an id of another rig — named
// in one list — must come back with the same canonical ids and the same
// per-id failure outcomes whether or not the batched fast path is used.
func TestResolveShowIDsMixMatchesPerIDPath(t *testing.T) {
	store := newShowResolveTestStore("be-mix-1", "be-mix-2wkz")
	ctx := context.Background()
	ids := []string{"be-mix-1", "be-mix-2", "be-zzz", "hr-1abc"}

	withFastPath, open := resolveShowIDs(ctx, store, ids, exactLocalIDs(ctx, store, ids))
	for _, r := range open {
		r.Close()
	}
	perIDOnly, open := resolveShowIDs(ctx, store, ids, nil)
	for _, r := range open {
		r.Close()
	}

	fastIDs, fastFailures := canonicalAndFailures(withFastPath)
	slowIDs, slowFailures := canonicalAndFailures(perIDOnly)

	// The partial id resolves to its full id on both paths; the exact ids stay
	// as named; the misses stay misses.
	wantIDs := []string{"be-mix-1", "be-mix-2wkz"}
	if fmt.Sprint(fastIDs) != fmt.Sprint(wantIDs) {
		t.Errorf("fast-path ids = %v, want %v", fastIDs, wantIDs)
	}
	if fmt.Sprint(slowIDs) != fmt.Sprint(wantIDs) {
		t.Errorf("per-id ids = %v, want %v", slowIDs, wantIDs)
	}
	if fmt.Sprint(fastFailures) != fmt.Sprint(slowFailures) {
		t.Errorf("fast-path failures = %+v, per-id failures = %+v", fastFailures, slowFailures)
	}
	if len(fastFailures) != 2 {
		t.Fatalf("failures = %d, want the missing id and the other rig's id", len(fastFailures))
	}
	if fastFailures[0].ID != "be-zzz" || fastFailures[0].Kind != kindNotFound {
		t.Errorf("first failure = %+v, want be-zzz not found", fastFailures[0])
	}
	if fastFailures[1].ID != "hr-1abc" || fastFailures[1].Kind != kindNotFound {
		t.Errorf("second failure = %+v, want hr-1abc not found", fastFailures[1])
	}
}

// TestResolveShowIDsKeepsCallerOrderAcrossFastAndSlowIds pins the output
// order: fast-path ids and per-id ids interleave in the caller's list, and the
// answer keeps the caller's order rather than grouping the fast ones first.
func TestResolveShowIDsKeepsCallerOrderAcrossFastAndSlowIds(t *testing.T) {
	store := newShowResolveTestStore("be-x1aaa", "be-x2bbb", "be-x3ccc")
	ctx := context.Background()
	// A partial id sits between two exact ones, and a miss after them: the
	// answer must still read in the caller's order.
	ids := []string{"be-x1aaa", "be-x2", "be-x2bbb", "be-zzz", "be-x3ccc"}

	resolutions, open := resolveShowIDs(ctx, store, ids, exactLocalIDs(ctx, store, ids))
	for _, r := range open {
		r.Close()
	}
	if len(resolutions) != len(ids) {
		t.Fatalf("resolutions = %d, want one per requested id (%d)", len(resolutions), len(ids))
	}

	// The resolutions stay parallel to the request: three fast hits in their
	// caller positions, one partial that resolved, one miss.
	wantRequested := ids
	wantCanonical := []string{"be-x1aaa", "be-x2bbb", "be-x2bbb", "", "be-x3ccc"}
	for i, r := range resolutions {
		if r.requested != wantRequested[i] {
			t.Errorf("resolutions[%d].requested = %q, want %q", i, r.requested, wantRequested[i])
		}
		if r.canonical != wantCanonical[i] {
			t.Errorf("resolutions[%d].canonical = %q, want %q", i, r.canonical, wantCanonical[i])
		}
		if wantFailed := wantCanonical[i] == ""; r.failed != wantFailed {
			t.Errorf("resolutions[%d].failed = %v, want %v", i, r.failed, wantFailed)
		}
	}

	gotIDs, failures := canonicalAndFailures(resolutions)
	wantIDs := []string{"be-x1aaa", "be-x2bbb", "be-x2bbb", "be-x3ccc"}
	if fmt.Sprint(gotIDs) != fmt.Sprint(wantIDs) {
		t.Errorf("ids = %v, want caller order %v", gotIDs, wantIDs)
	}
	if len(failures) != 1 || failures[0].ID != "be-zzz" {
		t.Errorf("failures = %+v, want just be-zzz", failures)
	}
}
