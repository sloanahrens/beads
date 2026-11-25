//go:build cgo

package issueops

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	_ "github.com/mattn/go-sqlite3"

	"github.com/steveyegge/beads/internal/storage/depid"
	"github.com/steveyegge/beads/internal/types"
)

// TestAddDependencyRefusesCrossPrefix locks the library's plain cross-prefix
// refusal: a plain target in another project is refused before anything is
// written, while a same-prefix target, an external: reference and an id with no
// prefix on either end keep their existing handling.
func TestAddDependencyRefusesCrossPrefix(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		target      string
		kind        DepTargetKind
		readsTarget bool
		wantRefused bool
	}{
		{name: "plain cross-prefix is refused", source: "gt-a", target: "om-b", wantRefused: true},
		{name: "same-prefix target is written", source: "gt-a", target: "gt-b", kind: DepTargetIssue, readsTarget: true},
		{name: "external target is written", source: "gt-a", target: "external:om:cap", kind: DepTargetExternal},
		{name: "prefix-less target is written", source: "gt-a", target: "plainid", kind: DepTargetIssue, readsTarget: true},
		{name: "prefix-less source is written", source: "plainid", target: "gt-b", kind: DepTargetIssue, readsTarget: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer func() { _ = db.Close() }()

			mock.ExpectBegin()
			tx, err := db.Begin()
			if err != nil {
				t.Fatalf("begin: %v", err)
			}

			kind := tt.kind
			dep := &types.Dependency{IssueID: tt.source, DependsOnID: tt.target, Type: types.DepRelated}
			opts := AddDependencyOpts{
				SourceTable:    "issues",
				TargetTable:    "issues",
				WriteTable:     "dependencies",
				TargetKind:     &kind,
				SkipCycleCheck: true,
			}

			if tt.wantRefused {
				// No query or exec is expected: the refusal must come before any
				// store access, so an unexpected call fails the add some other way.
				mock.ExpectRollback()
				_, err := AddDependencyInTx(context.Background(), tx, dep, "tester", opts)
				if !errors.Is(err, ErrCrossPrefixDependency) {
					t.Fatalf("AddDependencyInTx(%s -> %s) = %v, want ErrCrossPrefixDependency", tt.source, tt.target, err)
				}
				if !strings.Contains(err.Error(), "external:<project>:<capability>") {
					t.Errorf("refusal must name the supported form, got %q", err)
				}
				if err := tx.Rollback(); err != nil {
					t.Fatalf("rollback: %v", err)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatalf("unmet sqlmock expectations: %v", err)
				}
				return
			}

			issueType := func(id string) {
				mock.ExpectQuery(regexp.QuoteMeta("SELECT issue_type FROM issues WHERE id = ?")).
					WithArgs(id).
					WillReturnRows(sqlmock.NewRows([]string{"issue_type"}).AddRow("task"))
			}
			issueType(tt.source)
			if tt.readsTarget {
				issueType(tt.target)
			}
			mock.ExpectQuery(`SELECT type FROM dependencies`).
				WithArgs(tt.source, tt.target).
				WillReturnError(sql.ErrNoRows)
			mock.ExpectExec(regexp.QuoteMeta("INSERT INTO dependencies")).
				WithArgs(depid.New(tt.source, tt.target), tt.source, tt.target, string(types.DepRelated), "tester", "{}", "").
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()

			if _, err := AddDependencyInTx(context.Background(), tx, dep, "tester", opts); err != nil {
				t.Fatalf("AddDependencyInTx(%s -> %s) = %v, want the edge written", tt.source, tt.target, err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet sqlmock expectations: %v", err)
			}
		})
	}
}

// TestStoredCrossPrefixEdgeStillReads is the read-side regression for the
// cross-prefix refusal (be-39t0). A bare cross-prefix edge stored before the
// refusal — gt-a -> om-b, which ClassifyDepTarget writes to
// depends_on_external as a plain target — must still come back from the shared
// dependency read, so the edges already on disk stay listable for the by-hand
// conversion.
//
// The edge is seeded with a raw INSERT because AddDependencyInTx now refuses
// that shape. The read then runs against a real SQL engine (a sqlite file under
// t.TempDir(), the same driver the legacy-sqlite tests use), so the returned
// target is resolved from the stored row by the production projection
// (DepTargetExpr) instead of being restated by the test.
func TestStoredCrossPrefixEdgeStillReads(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "deps.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)

	// Only the columns the read touches. The three typed target columns are
	// mutually exclusive, as they are in the real schema.
	for _, stmt := range []string{
		`CREATE TABLE wisps (id TEXT PRIMARY KEY)`,
		`CREATE TABLE dependencies (
			id TEXT PRIMARY KEY,
			issue_id TEXT NOT NULL,
			depends_on_issue_id TEXT,
			depends_on_wisp_id TEXT,
			depends_on_external TEXT,
			type TEXT NOT NULL,
			created_at DATETIME,
			created_by TEXT NOT NULL,
			metadata TEXT,
			thread_id TEXT
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create schema: %v", err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	if _, err := tx.Exec(
		`INSERT INTO dependencies (id, issue_id, depends_on_external, type, created_by) VALUES (?, ?, ?, ?, ?)`,
		depid.New("gt-a", "om-b"), "gt-a", "om-b", string(types.DepRelated), "seeder",
	); err != nil {
		t.Fatalf("seed stored cross-prefix edge: %v", err)
	}

	got, err := GetDependencyRecordsForIssuesInTx(context.Background(), tx, []string{"gt-a"})
	if err != nil {
		t.Fatalf("GetDependencyRecordsForIssuesInTx: %v", err)
	}
	deps := got["gt-a"]
	if len(deps) != 1 {
		t.Fatalf("stored cross-prefix edge: got %d records, want 1 (all: %+v)", len(deps), got)
	}
	if deps[0].DependsOnID != "om-b" {
		t.Errorf("DependsOnID = %q, want %q", deps[0].DependsOnID, "om-b")
	}
	if deps[0].Type != types.DepRelated {
		t.Errorf("Type = %q, want %q", deps[0].Type, types.DepRelated)
	}
}
