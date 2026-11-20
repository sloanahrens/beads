package dolt

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	mysql "github.com/go-sql-driver/mysql"

	"github.com/steveyegge/beads/internal/types"
)

// errCreateRolledBack is injected where a create's SQL transaction fails. A
// plain error is neither a MySQL response nor a connection loss, so withRetryTx
// refuses to replay it and the caller sees a definite rollback.
var errCreateRolledBack = errors.New("create rollback ids test: injected non-retryable failure")

const (
	createRollbackPrefix = "test"
	createRollbackTarget = "test-target"

	configValueSelect = "SELECT value FROM config WHERE `key` = ?"
	wispActiveSelect  = "SELECT 1 FROM wisps WHERE id = ? LIMIT 1"
)

// TestCreateIssueRollbackIDs pins GH#8457 on the single-issue path: a create
// whose SQL transaction does not land hands the caller's issue back with the ID
// the create minted cleared, so a retry of the same struct mints a fresh ID
// instead of upserting a row a peer may now own. A commit whose result is
// indeterminate may have landed, so its IDs are kept.
func TestCreateIssueRollbackIDs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		commitErr error
		keepIDs   bool
	}{
		{
			// The server rejected the commit, so the transaction definitely did
			// not land and the minted IDs address rows that do not exist.
			name:      "rejected commit empties the minted IDs",
			commitErr: &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'test' for key 'issues.PRIMARY'"},
		},
		{
			name:      "indeterminate commit keeps the minted IDs",
			commitErr: testConnectionLoss,
			keepIDs:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock := newCreateRollbackStore(t)
			expectInfraTypeRead(mock)
			mock.ExpectBegin()
			expectBatchContext(mock)
			expectGeneratedIssueID(mock, "issues")
			expectIssueRowInsert(mock, "issues", "events")
			expectCommentInsert(mock, "comments")
			mock.ExpectCommit().WillReturnError(tc.commitErr)

			comment := &types.Comment{
				Author:    "tester",
				Text:      "created with the issue",
				CreatedAt: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
			}
			issue := &types.Issue{
				Title:     "create rollback ids",
				Status:    types.StatusOpen,
				Priority:  2,
				IssueType: types.TypeTask,
				Comments:  []*types.Comment{comment},
			}

			err := store.CreateIssue(context.Background(), issue, "tester")
			if !errors.Is(err, tc.commitErr) {
				t.Fatalf("CreateIssue() error = %v, want %v", err, tc.commitErr)
			}
			if tc.keepIDs {
				if issue.ID == "" {
					t.Errorf("issue.ID is empty after an indeterminate commit; want the minted ID kept")
				}
				if comment.ID == "" {
					t.Errorf("comment.ID is empty after an indeterminate commit; want the minted ID kept")
				}
			} else {
				if issue.ID != "" {
					t.Errorf("issue.ID = %q after a rolled-back create; want the empty ID the caller passed in", issue.ID)
				}
				if comment.ID != "" {
					t.Errorf("comment.ID = %q after a rolled-back create; want \"\"", comment.ID)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sqlmock expectations: %v", err)
			}
		})
	}
}

// TestCreateIssuesRollbackIDs pins GH#8457 on the batch path: when the
// transaction fails, both issues' minted IDs, the dependency IssueID the
// persist pass filled, and the comment ID the create minted are handed back
// empty so a retry of the same structs mints fresh IDs. The all-wisps fast path
// and the durable versioning path each get their own subtest.
func TestCreateIssuesRollbackIDs(t *testing.T) {
	for _, tc := range []struct {
		name         string
		issueTable   string
		eventTable   string
		commentTable string
		ephemeral    bool
	}{
		{
			name:         "all-wisps path",
			issueTable:   "wisps",
			eventTable:   "wisp_events",
			commentTable: "wisp_comments",
			ephemeral:    true,
		},
		{
			name:         "durable path",
			issueTable:   "issues",
			eventTable:   "events",
			commentTable: "comments",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock := newCreateRollbackStore(t)
			mock.ExpectBegin()
			expectBatchContext(mock)
			expectGeneratedIssueID(mock, tc.issueTable)
			expectGeneratedIssueID(mock, tc.issueTable)
			expectIssueRowInsert(mock, tc.issueTable, tc.eventTable)
			expectCommentInsert(mock, tc.commentTable)
			expectIssueRowInsert(mock, tc.issueTable, tc.eventTable)
			// The dependency pass fills the empty IssueID from its owning issue
			// before it reads anything; the target lookup then fails.
			if tc.ephemeral {
				// The source is a wisp row written in this transaction.
				mock.ExpectQuery(regexp.QuoteMeta(wispActiveSelect)).
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(1))
			} else {
				mock.ExpectQuery(regexp.QuoteMeta(wispActiveSelect)).
					WillReturnError(sql.ErrNoRows)
			}
			mock.ExpectQuery(regexp.QuoteMeta(wispActiveSelect)).
				WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM issues WHERE id = ?")).
				WillReturnError(errCreateRolledBack)
			mock.ExpectRollback()

			comment := &types.Comment{
				Author:    "tester",
				Text:      "created with the batch",
				CreatedAt: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
			}
			items := []*types.Issue{
				{
					Title:     "first batch issue",
					Status:    types.StatusOpen,
					Priority:  2,
					IssueType: types.TypeTask,
					Ephemeral: tc.ephemeral,
					Dependencies: []*types.Dependency{
						{DependsOnID: createRollbackTarget, Type: types.DepRelated},
					},
					Comments: []*types.Comment{comment},
				},
				{
					Title:     "second batch issue",
					Status:    types.StatusOpen,
					Priority:  2,
					IssueType: types.TypeTask,
					Ephemeral: tc.ephemeral,
				},
			}

			err := store.CreateIssues(context.Background(), items, "tester")
			if !errors.Is(err, errCreateRolledBack) {
				t.Fatalf("CreateIssues() error = %v, want %v", err, errCreateRolledBack)
			}
			for i, issue := range items {
				if issue.ID != "" {
					t.Errorf("issues[%d].ID = %q after a rolled-back batch; want \"\"", i, issue.ID)
				}
			}
			if got := items[0].Dependencies[0].IssueID; got != "" {
				t.Errorf("Dependency.IssueID = %q after a rolled-back batch; want \"\"", got)
			}
			if got := comment.ID; got != "" {
				t.Errorf("Comment.ID = %q after a rolled-back batch; want \"\"", got)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sqlmock expectations: %v", err)
			}
		})
	}
}

// newCreateRollbackStore returns a DoltStore over sqlmock, ordered to match the
// create's SQL calls in the order the store makes them.
func newCreateRollbackStore(t *testing.T) (*DoltStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &DoltStore{db: db}, mock
}

// expectConfigMissing records a config read that finds no row: the callers
// either fall back to their defaults or ignore the miss.
func expectConfigMissing(mock sqlmock.Sqlmock, key string) {
	mock.ExpectQuery(regexp.QuoteMeta(configValueSelect)).
		WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"value"}))
}

// expectBatchContext records the config reads NewBatchContext performs.
func expectBatchContext(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT name, category FROM custom_statuses ORDER BY name")).
		WillReturnRows(sqlmock.NewRows([]string{"name", "category"}))
	expectConfigMissing(mock, "status.custom")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT name FROM custom_types ORDER BY name")).
		WillReturnRows(sqlmock.NewRows([]string{"name"}))
	expectConfigMissing(mock, "types.custom")
	mock.ExpectQuery(regexp.QuoteMeta(configValueSelect)).
		WithArgs("issue_prefix").
		WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(createRollbackPrefix))
	expectConfigMissing(mock, "allowed_prefixes")
}

// expectInfraTypeRead records the read-only transaction IsInfraTypeCtx opens on
// first use; no configured infra types means the built-in defaults apply.
func expectInfraTypeRead(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	expectConfigMissing(mock, "types.infra")
	mock.ExpectRollback()
}

// expectGeneratedIssueID records one hash-ID mint: the counter-mode probe (only
// for the issues table), the adaptive length and config reads, and the
// collision probe.
func expectGeneratedIssueID(mock sqlmock.Sqlmock, table string) {
	if table == "issues" {
		expectConfigMissing(mock, "issue_id_mode")
	}
	mock.ExpectQuery(`SELECT COUNT\(\*\)\s+FROM ` + table + `\s+WHERE id LIKE`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	expectConfigMissing(mock, "max_collision_prob")
	expectConfigMissing(mock, "min_hash_length")
	expectConfigMissing(mock, "max_hash_length")
	mock.ExpectQuery(countByIDQuery(table)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
}

// expectIssueRowInsert records one issue's row write and its created event, as
// the per-issue create performs them: the cross-table collision probe, the
// same-table existence probe, the row UPSERT, then the event insert.
func expectIssueRowInsert(mock sqlmock.Sqlmock, issueTable, eventTable string) {
	sibling := "wisps"
	if issueTable == "wisps" {
		sibling = "issues"
	}
	mock.ExpectQuery(countByIDQuery(sibling)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(countByIDQuery(issueTable)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO " + issueTable)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM " + eventTable)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO " + eventTable)).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

// expectCommentInsert records a create-time comment's dedup read (no
// same-content row exists) and its insert.
func expectCommentInsert(mock sqlmock.Sqlmock, commentTable string) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM " + commentTable)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO " + commentTable)).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func countByIDQuery(table string) string {
	return regexp.QuoteMeta("SELECT COUNT(*) FROM " + table + " WHERE id = ?")
}
