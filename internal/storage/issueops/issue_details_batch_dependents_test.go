package issueops

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// TestReadBatchDependentsKeepsThePerIssueOrderAndKey pins the two halves of the
// batch dependent read that make it answer like IterDependentsWithMetadata:
// the statement keeps that iterator's ORDER BY created_at ASC and gains no
// tiebreaker of its own (the pinned SQL test in internal/storage/dolt would
// have to be loosened for one), and the row's created_at still reaches the
// assembly, which is where equal timestamps are ordered (be-2wkz).
func TestReadBatchDependentsKeepsThePerIssueOrderAndKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	const targetID = "be-target"
	created := "2026-10-01 00:00:00"
	mock.ExpectQuery(`ORDER BY created_at ASC$`).
		WithArgs(targetID, targetID, targetID, targetID, targetID, targetID).
		WillReturnRows(sqlmock.NewRows([]string{
			"target_id", "id", "status", "issue_type", "priority", "title", "dep_type", "created_at",
		}).AddRow(targetID, "be-dependent", "open", "task", 2, "A dependent", "blocks", created))

	out := &storage.IssueDetailsBatch{Dependents: map[string][]*types.IssueWithDependencyMetadata{}}
	if err := readBatchDependents(context.Background(), db, []string{targetID}, out); err != nil {
		t.Fatalf("readBatchDependents: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("statement expectations: %v", err)
	}

	deps := out.Dependents[targetID]
	if len(deps) != 1 {
		t.Fatalf("read %d dependents for %s, want 1", len(deps), targetID)
	}
	if got := deps[0].Issue.CreatedAt; !got.Equal(ParseTimeString(created)) {
		t.Errorf("dependent created_at = %v, want %v: the assembly cannot order ties without it",
			got, ParseTimeString(created))
	}
	if got, want := deps[0].Issue.ID, "be-dependent"; got != want {
		t.Errorf("dependent id = %q, want %q", got, want)
	}
	if got, want := deps[0].DependencyType, types.DepBlocks; got != want {
		t.Errorf("dependent type = %q, want %q", got, want)
	}
}
