package workapi

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// ---------------------------------------------------------------------------
// The batch assembly against the per-id assembly, over one fixture.
//
// These cases pin the half of be-2wkz that lives in this package: the batch
// answer is BUILDIssueDetails's answer, not a second view that happens to agree
// on the fields someone remembered to compare. The statement-level half — that
// the store really reads the list as one query set — is pinned at the store,
// where the SQL is.
// ---------------------------------------------------------------------------

// detailBatchFromFixture builds the raw material a store would hand back for
// ids out of the fixture: labels hydrated into the issue rows, the outgoing
// edges, the shallow dependents, the comment rows and the three counts, each
// present only when the option asks for the rows that carry it.
func detailBatchFromFixture(fx *detailFixture, ids []string, opts DetailOptions) *storage.IssueDetailsBatch {
	batch := &storage.IssueDetailsBatch{
		Issues:           make(map[string]*types.Issue, len(ids)),
		Dependencies:     make(map[string][]*types.IssueWithDependencyMetadata, len(ids)),
		Dependents:       make(map[string][]*types.IssueWithDependencyMetadata, len(ids)),
		Comments:         make(map[string][]*types.Comment, len(ids)),
		DependencyCounts: make(map[string]int64, len(ids)),
		DependentCounts:  make(map[string]int64, len(ids)),
		CommentCounts:    make(map[string]int64, len(ids)),
	}
	for _, id := range ids {
		issue, ok := fx.issues[id]
		if !ok {
			issue, ok = fx.wisps[id]
		}
		if !ok {
			continue
		}
		// A copy: the store hydrates labels into the row it returns, and the
		// fixture's rows are shared with every other case here.
		row := *issue
		row.Labels = append([]string(nil), fx.labels[id]...)
		batch.Issues[id] = &row
		batch.Dependencies[id] = fx.deps[id]
		batch.DependencyCounts[id] = int64(len(fx.deps[id]))
		batch.DependentCounts[id] = int64(len(fx.dependents[id]))
		batch.CommentCounts[id] = int64(len(fx.comments[id]))
		if opts.IncludeDependents {
			for _, dep := range fx.dependents[id] {
				// The store carries created_at on the row so the assembly can
				// order it; the published shallow view drops it.
				row := shallowDep(dep)
				row.Issue.CreatedAt = dep.Issue.CreatedAt
				batch.Dependents[id] = append(batch.Dependents[id], row)
			}
		}
		if opts.IncludeComments {
			batch.Comments[id] = fx.comments[id]
		}
	}
	return batch
}

func detailJSON(t *testing.T, id string, details *types.IssueDetails) string {
	t.Helper()
	encoded, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("marshal the detail view of %s: %v", id, err)
	}
	return string(encoded)
}

// TestBuildIssueDetailsBatchMatchesPerID is the equality the batch exists to
// preserve: for every id and every option combination, the JSON the batch
// assembly publishes is the JSON the per-id assembly publishes.
func TestBuildIssueDetailsBatchMatchesPerID(t *testing.T) {
	fx := newDetailFixture()
	storeSrc, _ := fixtureSources(fx)
	ctx := context.Background()

	ids := []string{"bd-1", "bd-epic", "bd-chat", "bd-w1"}
	opts := []struct {
		name string
		opts DetailOptions
	}{
		{"default", DetailOptions{}},
		{"brief deps", DetailOptions{BriefDeps: true}},
		{"comments", DetailOptions{IncludeComments: true}},
		{"dependents", DetailOptions{IncludeDependents: true}},
		{"every option", DetailOptions{IncludeDependents: true, IncludeComments: true, BriefDeps: true}},
	}
	for _, o := range opts {
		t.Run(o.name, func(t *testing.T) {
			want := make([]string, 0, len(ids))
			for _, id := range ids {
				issue, err := storeSrc.GetIssue(ctx, id)
				if err != nil {
					t.Fatalf("GetIssue(%s): %v", id, err)
				}
				isWisp := fx.isWisp(id)
				details, err := BuildIssueDetails(ctx, storeSrc, issue, isWisp, o.opts)
				if err != nil {
					t.Fatalf("BuildIssueDetails(%s): %v", id, err)
				}
				want = append(want, detailJSON(t, id, details))
			}

			got, notFound := BuildIssueDetailsBatch(detailBatchFromFixture(fx, ids, o.opts), ids, o.opts)
			if len(notFound) != 0 {
				t.Errorf("notFound = %v, want none: every id is in the fixture", notFound)
			}
			if len(got) != len(ids) {
				t.Fatalf("batch answered %d views, want %d", len(got), len(ids))
			}
			for i, details := range got {
				if details.ID != ids[i] {
					t.Errorf("batch position %d = %q, want request order (%q)", i, details.ID, ids[i])
				}
				if json := detailJSON(t, ids[i], details); json != want[i] {
					t.Errorf("batch view of %s differs from the per-id view:\n batch:  %s\n per-id: %s", ids[i], json, want[i])
				}
			}
		})
	}
}

// TestBuildIssueDetailsBatchNamesMisses pins the miss half of the contract: a
// batch names the ids it holds no row for, keeps every other answer in request
// order, and does not turn a miss into a failure.
func TestBuildIssueDetailsBatchNamesMisses(t *testing.T) {
	fx := newDetailFixture()
	ids := []string{"bd-1", "bd-absent", "bd-w1"}
	batch := detailBatchFromFixture(fx, ids, DetailOptions{})

	details, notFound := BuildIssueDetailsBatch(batch, ids, DetailOptions{})
	if len(notFound) != 1 || notFound[0] != "bd-absent" {
		t.Fatalf("notFound = %v, want just the absent id", notFound)
	}
	if len(details) != 2 || details[0].ID != "bd-1" || details[1].ID != "bd-w1" {
		t.Fatalf("details = %d rows, want the two found ids in request order", len(details))
	}
}

// TestBuildIssueDetailsBatchIsSensitiveToADroppedField is the tripwire under
// the equality above. A comparison test that passes because both sides are
// missing the same thing proves nothing, so this drops ONE field from the batch
// input and insists the published JSON changes: labels, a dependency row, a
// dependent row, a comment row and a count each fail the view on their own.
func TestBuildIssueDetailsBatchIsSensitiveToADroppedField(t *testing.T) {
	fx := newDetailFixture()
	ids := []string{"bd-1", "bd-chat"}
	everyOption := DetailOptions{IncludeDependents: true, IncludeComments: true, BriefDeps: true}
	baseline := func() string {
		details, _ := BuildIssueDetailsBatch(detailBatchFromFixture(fx, ids, everyOption), ids, everyOption)
		return detailJSON(t, ids[0], details[0])
	}()

	for _, dropped := range []struct {
		what string
		drop func(*storage.IssueDetailsBatch)
	}{
		{"labels", func(b *storage.IssueDetailsBatch) { b.Issues["bd-1"].Labels = nil }},
		{"dependencies", func(b *storage.IssueDetailsBatch) { b.Dependencies["bd-1"] = nil }},
		{"dependents", func(b *storage.IssueDetailsBatch) { b.Dependents["bd-1"] = nil }},
		{"dependency count", func(b *storage.IssueDetailsBatch) { delete(b.DependencyCounts, "bd-1") }},
		{"dependent count", func(b *storage.IssueDetailsBatch) { delete(b.DependentCounts, "bd-1") }},
	} {
		t.Run(dropped.what, func(t *testing.T) {
			batch := detailBatchFromFixture(fx, ids, everyOption)
			dropped.drop(batch)
			details, _ := BuildIssueDetailsBatch(batch, ids, everyOption)
			if got := detailJSON(t, ids[0], details[0]); got == baseline {
				t.Errorf("dropping %s did not change the published view, so the equality case cannot see it: %s", dropped.what, got)
			}
		})
	}

	// A comment row and its count live on the other id in this fixture.
	t.Run("comments", func(t *testing.T) {
		batch := detailBatchFromFixture(fx, ids, everyOption)
		batch.Comments["bd-chat"] = nil
		details, _ := BuildIssueDetailsBatch(batch, ids, everyOption)
		withRows := detailJSON(t, ids[1], details[1])
		whole, _ := BuildIssueDetailsBatch(detailBatchFromFixture(fx, ids, everyOption), ids, everyOption)
		if withRows == detailJSON(t, ids[1], whole[1]) {
			t.Errorf("dropping the comment rows did not change the view: %s", withRows)
		}
	})
}

// TestBuildIssueDetailsOrdersDependentsByCreatedAtThenID pins the order the
// per-id path publishes dependents in. The store's statement orders by
// created_at alone (dependentsIterSQL), which leaves equal timestamps to the
// plan, and the batch read is a different statement over a whole id list. The
// shared assembly breaks the tie by id, so neither path's output depends on a
// plan.
func TestBuildIssueDetailsOrdersDependentsByCreatedAtThenID(t *testing.T) {
	fx := newDetailFixture()
	early := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	late := early.Add(time.Second)
	// Arrival order is the one that must NOT survive: id-descending within the
	// tie, and the newest row first.
	fx.dependents["bd-epic"] = []*types.IssueWithDependencyMetadata{
		{Issue: types.Issue{ID: "bd-c", CreatedAt: late}, DependencyType: types.DepParentChild},
		{Issue: types.Issue{ID: "bd-b", CreatedAt: early}, DependencyType: types.DepParentChild},
		{Issue: types.Issue{ID: "bd-a", CreatedAt: early}, DependencyType: types.DepParentChild},
	}
	storeSrc, _ := fixtureSources(fx)

	details, err := BuildIssueDetails(context.Background(), storeSrc, fx.issues["bd-epic"], false, DetailOptions{IncludeDependents: true})
	if err != nil {
		t.Fatalf("BuildIssueDetails(bd-epic): %v", err)
	}
	var got []string
	for _, dep := range details.Dependents {
		got = append(got, dep.ID)
	}
	want := []string{"bd-a", "bd-b", "bd-c"}
	if !slices.Equal(got, want) {
		t.Errorf("dependents = %v, want %v (created_at, then id)", got, want)
	}
}

// TestBuildIssueDetailsBatchOrdersDependentsLikeThePerIDPath is the order half
// of the equality case: the same dependent rows arriving in a different order,
// as one UNION ALL over many ids would hand them back, must publish the per-id
// path's order.
func TestBuildIssueDetailsBatchOrdersDependentsLikeThePerIDPath(t *testing.T) {
	fx := newDetailFixture()
	early := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	late := early.Add(time.Second)
	fx.dependents["bd-1"] = []*types.IssueWithDependencyMetadata{
		{Issue: types.Issue{ID: "bd-y", Title: "Y", CreatedAt: early}, DependencyType: types.DepBlocks},
		{Issue: types.Issue{ID: "bd-x", Title: "X", CreatedAt: early}, DependencyType: types.DepBlocks},
		{Issue: types.Issue{ID: "bd-w", Title: "W", CreatedAt: late}, DependencyType: types.DepBlocks},
	}
	ids := []string{"bd-1"}
	opts := DetailOptions{IncludeDependents: true}

	storeSrc, _ := fixtureSources(fx)
	perID, err := BuildIssueDetails(context.Background(), storeSrc, fx.issues["bd-1"], false, opts)
	if err != nil {
		t.Fatalf("BuildIssueDetails(bd-1): %v", err)
	}

	batch := detailBatchFromFixture(fx, ids, opts)
	slices.Reverse(batch.Dependents["bd-1"]) // arrival order must not matter
	got, notFound := BuildIssueDetailsBatch(batch, ids, opts)
	if len(notFound) != 0 || len(got) != 1 {
		t.Fatalf("batch answered %d views (notFound %v), want 1", len(got), notFound)
	}
	if want := detailJSON(t, "bd-1", perID); detailJSON(t, "bd-1", got[0]) != want {
		t.Errorf("batch order differs from the per-id order:\n batch:  %s\n per-id: %s", detailJSON(t, "bd-1", got[0]), want)
	}
}
