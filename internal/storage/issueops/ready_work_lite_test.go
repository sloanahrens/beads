package issueops

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/steveyegge/beads/internal/types"
)

// fullColumnNames mirrors ScanIssueFrom's dests order (scan.go): the lite list
// with the six heavy TEXT columns put back where IssueSelectColumns has them.
func fullColumnNames() []string {
	return []string{
		"id", "content_hash", "title", "description", "design", "acceptance_criteria", "notes",
		"status",
		"priority", "issue_type", "assignee", "estimated_minutes",
		"created_at", "created_by", "owner", "updated_at", "started_at", "closed_at", "external_ref", "spec_id",
		"compaction_level", "compacted_at", "compacted_at_commit", "original_size", "source_repo", "close_reason", "closed_by_session",
		"sender", "ephemeral", "no_history", "wisp_type", "pinned", "is_template",
		"await_type", "await_id", "timeout_ns", "waiters",
		"mol_type",
		"event_kind", "actor", "target", "payload",
		"due_at", "defer_until",
		"work_type", "source_system", "metadata", "row_lock", "storage_class",
		"lease_expires_at", "heartbeat_at", "granted_node",
	}
}

// fullRow is fullColumnNames-width with only identity/status/priority/
// created_at populated and the six heavy columns NULL — enough for the scan,
// and deliberately textless so only the COLUMN COUNT separates it from a lite
// row.
func fullRow(id string, createdAt time.Time) []driver.Value {
	return []driver.Value{
		id, nil, id, "", "", "", "", // id, content_hash, title, description, design, acceptance_criteria, notes
		"open",              // status
		2, "task", nil, nil, // priority, issue_type, assignee, estimated_minutes
		createdAt.Format(time.RFC3339), nil, nil, nil, nil, nil, nil, nil,
		0, nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, // await_type, await_id, timeout_ns, waiters
		nil,                // mol_type
		nil, nil, nil, nil, // event_kind, actor, target, payload
		nil, nil, // due_at, defer_until
		nil, nil, nil, 0, nil, // work_type, source_system, metadata, row_lock, storage_class
		nil, nil, nil, // lease_expires_at, heartbeat_at, granted_node
	}
}

// TestGetReadyWorkInTx_LiteHydratesThroughTheLiteProjection pins the store
// backend's half of `bd ready`'s text default.
//
// The text rendering prints no free-form field, so cmd/bd asks this bare,
// UNCCOUNTED ready front for types.WorkFilter.Lite and never for the counts
// mega-query. Until this path honored the flag, that default would have been a
// silent no-op on this stack: the ids came back projected by luck of the
// renderer, but every body was selected and hydrated anyway.
//
// The mock returns IssueSelectColumnsLite-width rows, so a regression that
// reverted the hydration query to IssueSelectColumns fails on the scan rather
// than passing on a coincidence: the two lists have different widths.
func TestGetReadyWorkInTx_LiteHydratesThroughTheLiteProjection(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(deferredParentProbeRegex("issues")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(deferredParentProbeRegex("wisps")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT id FROM issues`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("bd-lite-ready"))
	// WispIDSetInTx's emptiness probe: no wisps, so every id routes to issues.
	mock.ExpectQuery(`SELECT 1 FROM wisps LIMIT 1`).WillReturnError(sql.ErrNoRows)
	// THE ASSERTION: the lite column list, not the full one. `status` follows
	// `title` here; in IssueSelectColumns it follows description/design/
	// acceptance_criteria/notes.
	mock.ExpectQuery(`(?s)SELECT id, content_hash, title,\s+status,.*FROM issues.*WHERE id IN`).
		WithArgs("bd-lite-ready").
		WillReturnRows(sqlmock.NewRows(liteColumnNames()).
			AddRow(liteMergeRow("bd-lite-ready", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))...))
	mock.ExpectQuery(`SELECT issue_id, label FROM labels WHERE issue_id IN`).
		WithArgs("bd-lite-ready").
		WillReturnRows(sqlmock.NewRows([]string{"issue_id", "label"}))
	// getReadyWispsInTx's own emptiness probe.
	mock.ExpectQuery(`SELECT 1 FROM wisps LIMIT 1`).WillReturnError(sql.ErrNoRows)

	got, err := GetReadyWorkInTx(context.Background(), tx, types.WorkFilter{Lite: true})
	if err != nil {
		t.Fatalf("GetReadyWorkInTx(Lite: true): %v", err)
	}
	if len(got) != 1 || got[0].ID != "bd-lite-ready" {
		t.Fatalf("GetReadyWorkInTx(Lite: true) = %v, want the one ready row", got)
	}
	if !got[0].IsLitePartial {
		t.Error("row IsLitePartial = false, want true: the lite projection must travel on the row, or a renderer cannot tell it apart from a textless issue")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

// TestGetReadyWorkInTx_FullRowByDefault is the negative control: the same
// query without the flag still selects the whole row. Without it, a hydration
// path hard-coded to the lite list would leave the test above green.
func TestGetReadyWorkInTx_FullRowByDefault(t *testing.T) {
	t.Parallel()

	_, mock, tx := beginMockTx(t)

	mock.ExpectQuery(deferredParentProbeRegex("issues")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(deferredParentProbeRegex("wisps")).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT id FROM issues`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("bd-full-ready"))
	mock.ExpectQuery(`SELECT 1 FROM wisps LIMIT 1`).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`(?s)SELECT id, content_hash, title, description,.*FROM issues.*WHERE id IN`).
		WithArgs("bd-full-ready").
		WillReturnRows(sqlmock.NewRows(fullColumnNames()).
			AddRow(fullRow("bd-full-ready", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))...))
	mock.ExpectQuery(`SELECT issue_id, label FROM labels WHERE issue_id IN`).
		WithArgs("bd-full-ready").
		WillReturnRows(sqlmock.NewRows([]string{"issue_id", "label"}))
	mock.ExpectQuery(`SELECT 1 FROM wisps LIMIT 1`).WillReturnError(sql.ErrNoRows)

	got, err := GetReadyWorkInTx(context.Background(), tx, types.WorkFilter{})
	if err != nil {
		t.Fatalf("GetReadyWorkInTx: %v", err)
	}
	if len(got) != 1 || got[0].ID != "bd-full-ready" {
		t.Fatalf("GetReadyWorkInTx = %v, want the one ready row", got)
	}
	if got[0].IsLitePartial {
		t.Error("row IsLitePartial = true, want false: the default must still hydrate the whole row")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
