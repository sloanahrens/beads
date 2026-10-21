package issueops

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

func TestResolveBatchDependencyRefs(t *testing.T) {
	t.Run("rewrites batch refs on both ends to the assigned IDs", func(t *testing.T) {
		a := &types.Issue{ID: "bd-a"}
		b := &types.Issue{ID: "bd-b", Dependencies: []*types.Dependency{
			{IssueID: "batch:1", DependsOnID: "batch:0", Type: types.DepBlocks},
		}}
		c := &types.Issue{ID: "bd-c", Dependencies: []*types.Dependency{
			{IssueID: "batch:2", DependsOnID: "batch:1", Type: types.DepBlocks},
		}}
		d := &types.Issue{ID: "bd-d", Dependencies: []*types.Dependency{
			{IssueID: "batch:0", DependsOnID: "bd-x-real", Type: types.DepBlocks},
		}}

		if err := ResolveBatchDependencyRefs([]*types.Issue{a, b, c, d}); err != nil {
			t.Fatalf("ResolveBatchDependencyRefs: %v", err)
		}

		if got := b.Dependencies[0]; got.IssueID != "bd-b" || got.DependsOnID != "bd-a" {
			t.Errorf("b dependency = %s -> %s, want bd-b -> bd-a", got.IssueID, got.DependsOnID)
		}
		if got := c.Dependencies[0]; got.IssueID != "bd-c" || got.DependsOnID != "bd-b" {
			t.Errorf("c dependency = %s -> %s, want bd-c -> bd-b", got.IssueID, got.DependsOnID)
		}
		if got := d.Dependencies[0]; got.IssueID != "bd-a" || got.DependsOnID != "bd-x-real" {
			t.Errorf("d dependency = %s -> %s, want bd-a -> bd-x-real", got.IssueID, got.DependsOnID)
		}
	})

	t.Run("leaves an issue without dependencies untouched", func(t *testing.T) {
		a := &types.Issue{ID: "bd-a"}
		if err := ResolveBatchDependencyRefs([]*types.Issue{a}); err != nil {
			t.Fatalf("ResolveBatchDependencyRefs: %v", err)
		}
		if a.ID != "bd-a" || a.Dependencies != nil {
			t.Errorf("issue changed: ID=%q Dependencies=%v", a.ID, a.Dependencies)
		}
	})

	badRefs := []string{"batch:3", "batch:x", "batch:-1", "batch:+1", "batch:"}
	for _, ref := range badRefs {
		t.Run("refuses "+ref, func(t *testing.T) {
			a := &types.Issue{ID: "bd-a"}
			b := &types.Issue{ID: "bd-b", Dependencies: []*types.Dependency{
				{IssueID: "batch:0", DependsOnID: "batch:1", Type: types.DepBlocks},
				{IssueID: "bd-b", DependsOnID: ref, Type: types.DepBlocks},
			}}
			issues := []*types.Issue{a, b}

			err := ResolveBatchDependencyRefs(issues)
			if err == nil {
				t.Fatalf("ResolveBatchDependencyRefs accepted %q", ref)
			}
			if !strings.Contains(err.Error(), ref) {
				t.Errorf("error %q does not name the ref %q", err, ref)
			}
			if got := b.Dependencies[0]; got.IssueID != "batch:0" || got.DependsOnID != "batch:1" {
				t.Errorf("a refused batch rewrote a dependency: %s -> %s", got.IssueID, got.DependsOnID)
			}
		})
	}

	t.Run("refuses a ref to an issue with no ID yet", func(t *testing.T) {
		a := &types.Issue{}
		b := &types.Issue{ID: "bd-b", Dependencies: []*types.Dependency{
			{IssueID: "bd-b", DependsOnID: "batch:0", Type: types.DepBlocks},
		}}
		if err := ResolveBatchDependencyRefs([]*types.Issue{a, b}); err == nil {
			t.Fatal("ResolveBatchDependencyRefs accepted a ref to an issue without an ID")
		}
		if got := b.Dependencies[0].DependsOnID; got != "batch:0" {
			t.Errorf("refused batch rewrote DependsOnID to %q", got)
		}
	})
}

func TestCheckBatchDependencyRefsReadsNoIDs(t *testing.T) {
	// No IDs are set: the check must judge the refs by position alone.
	a := &types.Issue{}
	b := &types.Issue{Dependencies: []*types.Dependency{
		{IssueID: "batch:1", DependsOnID: "batch:0", Type: types.DepBlocks},
	}}
	if err := CheckBatchDependencyRefs([]*types.Issue{a, b}); err != nil {
		t.Fatalf("CheckBatchDependencyRefs refused valid positions: %v", err)
	}
	b.Dependencies[0].DependsOnID = "batch:2"
	if err := CheckBatchDependencyRefs([]*types.Issue{a, b}); err == nil {
		t.Fatal("CheckBatchDependencyRefs accepted batch:2 in a two-issue batch")
	}
}

func TestAssignBatchIssueIDs(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Two identical issues: same title, description, actor and CreatedAt, so the
	// hash gives both the same first candidate. The second must move on to the
	// next nonce, because the first is already taken in this batch even though
	// no row holds it yet. The fixed issue carries its ID and generates nothing.
	created := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	same := func() *types.Issue {
		return &types.Issue{Title: "same", Description: "same", Status: types.StatusOpen, IssueType: types.TypeTask, CreatedAt: created}
	}
	fixed := &types.Issue{ID: "bd-k1", Title: "fixed", Status: types.StatusOpen, IssueType: types.TypeTask, CreatedAt: created}
	first, second := same(), same()

	// Expected in order. Each generation reads the counter mode first (off, so
	// the hash generator runs), then checks each candidate against the table.
	// Every candidate is free in the table; the batch's taken set is what
	// rejects the duplicate.
	counterMode := func() {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT value FROM config WHERE `key` = ?")).
			WithArgs("issue_id_mode").
			WillReturnError(sql.ErrNoRows)
	}
	candidateFree := func() {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM issues WHERE id = ?")).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	}
	counterMode() // first issue
	candidateFree()
	counterMode() // second issue: nonce 0 is taken in the batch
	candidateFree()
	candidateFree() // nonce 1

	bc := &BatchContext{ConfigPrefix: "bd", Opts: storage.BatchCreateOptions{SkipPrefixValidation: true}}
	batch := []*types.Issue{first, fixed, second}
	taken := map[string]bool{}
	if err := AssignBatchIssueIDsInTx(context.Background(), db, bc, batch, "tester", taken); err != nil {
		t.Fatalf("AssignBatchIssueIDsInTx: %v", err)
	}

	if first.ID == "" || second.ID == "" {
		t.Fatalf("generated IDs empty: first=%q second=%q", first.ID, second.ID)
	}
	if fixed.ID != "bd-k1" {
		t.Errorf("pre-set ID changed to %q", fixed.ID)
	}
	if first.ID == second.ID {
		t.Errorf("identical issues in one batch share ID %q", first.ID)
	}
	if !strings.HasPrefix(first.ID, "bd-") || !strings.HasPrefix(second.ID, "bd-") {
		t.Errorf("generated IDs lack the bd- prefix: %q, %q", first.ID, second.ID)
	}
	for _, id := range []string{first.ID, second.ID, "bd-k1"} {
		if !taken[id] {
			t.Errorf("taken does not record %q", id)
		}
	}
}

func TestAssignBatchIssueIDsRefusesBadPrefixBeforeWriting(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer func() { _ = db.Close() }()

	// No query or exec is expected: a bad pre-set ID must be refused before the
	// generator runs for any issue in the batch.
	bc := &BatchContext{ConfigPrefix: "bd", Opts: storage.BatchCreateOptions{}}
	batch := []*types.Issue{
		{Title: "generated", Status: types.StatusOpen, IssueType: types.TypeTask},
		{ID: "other-9", Title: "bad", Status: types.StatusOpen, IssueType: types.TypeTask},
	}
	err = AssignBatchIssueIDsInTx(context.Background(), db, bc, batch, "tester", map[string]bool{})
	if err == nil {
		t.Fatal("AssignBatchIssueIDsInTx accepted a pre-set ID outside the prefix")
	}
	if batch[0].ID != "" {
		t.Errorf("issue generated an ID (%q) before the refusal", batch[0].ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unexpected database calls: %v", err)
	}
}
