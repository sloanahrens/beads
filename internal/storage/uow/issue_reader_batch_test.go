package uow

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/steveyegge/beads/internal/storage/domain/db"
	"github.com/steveyegge/beads/internal/types"
	publicops "github.com/steveyegge/beads/issueops"
)

// countingTx wraps a unit of work's transaction so every statement its runner
// sends is counted. The use cases and the batch read both reach the database
// through tx.Runner(), so the count covers every read the reader makes.
type countingTx struct {
	Tx
	statements *int
}

func (t countingTx) Runner() db.Runner {
	return countingRunner{Runner: t.Tx.Runner(), statements: t.statements}
}

type countingRunner struct {
	db.Runner
	statements *int
}

func (r countingRunner) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	*r.statements++
	return r.Runner.ExecContext(ctx, query, args...)
}

func (r countingRunner) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	*r.statements++
	return r.Runner.QueryContext(ctx, query, args...)
}

func (r countingRunner) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	*r.statements++
	return r.Runner.QueryRowContext(ctx, query, args...)
}

// countingProvider hands out units of work whose transaction counts statements.
// The transaction is swapped on the base unit of work before any statement
// runs, so the reader's own unit of work is the one that is counted.
type countingProvider struct {
	UnitOfWorkProvider
	statements *int
}

func (p countingProvider) NewUOW(ctx context.Context) (UnitOfWork, error) {
	uw, err := p.UnitOfWorkProvider.NewUOW(ctx)
	if err != nil {
		return nil, err
	}
	base, ok := unwrapUOW(uw).(*baseUOW)
	if !ok {
		return nil, fmt.Errorf("counting provider: %T is not a base unit of work", uw)
	}
	base.tx = countingTx{Tx: base.tx, statements: p.statements}
	return uw, nil
}

// TestReaderGetManyCostsTheSameAtAnyListLength is the cost half of the batch
// read on this seam: GetMany over 50 ids must issue the statements it issues
// for 10, because the answer is one statement list over the whole id set, not
// one detail read per id. Each id's answer must also equal Get's.
func TestReaderGetManyCostsTheSameAtAnyListLength(t *testing.T) {
	ctx := context.Background()
	const prefix = "gmc"
	base := newUOWRoleFixtureProvider(t, ctx, prefix)
	fixture := newUOWReaderFixture(t, base, prefix)

	statements := 0
	reader, err := NewIssueReader(countingProvider{UnitOfWorkProvider: base, statements: &statements})
	if err != nil {
		t.Fatalf("NewIssueReader: %v", err)
	}

	seedIssue := func(id string) {
		t.Helper()
		issue := &types.Issue{ID: id, Title: "batch " + id, Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}
		if err := fixture.CreateIssue(ctx, issue, "seed"); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	blocker := prefix + "-blocker"
	seedIssue(blocker)

	// One wisp in both lists, with an edge, so the wisp arms of the read are
	// non-empty for both sizes and the comparison is like for like.
	wisp := prefix + "-wisp"
	if err := fixture.CreateWisp(ctx, &types.Issue{
		ID: wisp, Title: "a wisp the batch must read", Status: types.StatusOpen,
		Priority: 2, IssueType: types.TypeTask, Ephemeral: true,
	}, "seed"); err != nil {
		t.Fatalf("create wisp: %v", err)
	}
	if err := fixture.AddDependency(ctx, &types.Dependency{
		IssueID: wisp, DependsOnID: blocker, Type: types.DepBlocks,
	}, "seed"); err != nil {
		t.Fatalf("wisp dependency: %v", err)
	}

	seedList := func(name string, n int) []string {
		t.Helper()
		ids := []string{wisp}
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("%s-%s-%03d", prefix, name, i)
			seedIssue(id)
			if err := fixture.AddDependency(ctx, &types.Dependency{
				IssueID: id, DependsOnID: blocker, Type: types.DepBlocks,
			}, "seed"); err != nil {
				t.Fatalf("dependency %s: %v", id, err)
			}
			if err := fixture.AddComment(ctx, id, "seed", "a comment for "+id); err != nil {
				t.Fatalf("comment %s: %v", id, err)
			}
			ids = append(ids, id)
		}
		return ids
	}
	small := seedList("small", 10)
	// The missing id proves the batch names a miss rather than failing the
	// whole list or dropping the id silently.
	missing := prefix + "-missing"
	large := append(seedList("large", 50), missing)

	run := func(t *testing.T, ids []string, req publicops.GetManyRequest) (publicops.GetManyResult, int) {
		t.Helper()
		req.IDs = ids
		statements = 0
		got, err := reader.GetMany(ctx, req)
		if err != nil {
			t.Fatalf("GetMany over %d ids: %v", len(ids), err)
		}
		return got, statements
	}

	for _, opts := range []struct {
		name string
		req  publicops.GetManyRequest
	}{
		{"counts only", publicops.GetManyRequest{}},
		{"every row list", publicops.GetManyRequest{IncludeDependents: true, IncludeComments: true}},
	} {
		t.Run(opts.name, func(t *testing.T) {
			_, smallStatements := run(t, small, opts.req)
			largeGot, largeStatements := run(t, large, opts.req)

			if smallStatements != largeStatements {
				t.Errorf("statements for %d ids = %d, for %d ids = %d: GetMany costs per id, not per list",
					len(small), smallStatements, len(large), largeStatements)
			}

			// The count only means something if the reads answered.
			if got, want := len(largeGot.Details), len(large)-1; got != want {
				t.Fatalf("GetMany over %d ids answered %d details, want %d", len(large), got, want)
			}
			if !slices.Equal(largeGot.NotFound, []string{missing}) {
				t.Errorf("NotFound = %v, want [%s]", largeGot.NotFound, missing)
			}

			for _, details := range largeGot.Details {
				want, err := fixture.Reader.Get(ctx, publicops.GetRequest{
					ID:                details.ID,
					IncludeDependents: opts.req.IncludeDependents,
					IncludeComments:   opts.req.IncludeComments,
					BriefDeps:         opts.req.BriefDeps,
				})
				if err != nil {
					t.Fatalf("Get(%s): %v", details.ID, err)
				}
				if got, wantJSON := mustJSON(t, details), mustJSON(t, want); got != wantJSON {
					t.Errorf("GetMany answer for %s differs from Get's:\n got  %s\n want %s", details.ID, got, wantJSON)
				}
			}
		})
	}
}

// TestReaderGetManyRefusesAUnitOfWorkWithoutAStatementRunner: the batch read
// runs on the open unit of work's runner. A unit of work that cannot expose one
// must refuse the call, not answer through the per-id reads the batch replaced.
func TestReaderGetManyRefusesAUnitOfWorkWithoutAStatementRunner(t *testing.T) {
	reader, err := NewIssueReader(&mockUnitOfWorkProvider{})
	if err != nil {
		t.Fatalf("NewIssueReader: %v", err)
	}
	if _, err := reader.GetMany(context.Background(), publicops.GetManyRequest{IDs: []string{"bd-1"}}); err == nil {
		t.Fatal("GetMany over a unit of work with no statement runner returned no error")
	}
}

// sqlmockTx is a unit of work's transaction over a sqlmock connection, so a
// test can make the batch read's first statement fail without a database.
type sqlmockTx struct {
	runner *sql.Tx
}

func (t sqlmockTx) Commit(context.Context, string) error    { return nil }
func (t sqlmockTx) Rollback(context.Context) error          { return t.runner.Rollback() }
func (t sqlmockTx) RollbackUnlessCommitted(context.Context) {}
func (t sqlmockTx) Runner() db.Runner                       { return t.runner }

// fixedProvider hands out one unit of work.
type fixedProvider struct {
	uw UnitOfWork
}

func (p fixedProvider) NewUOW(context.Context) (UnitOfWork, error) { return p.uw, nil }
func (p fixedProvider) Close(context.Context) error                { return nil }

// TestReaderGetManyFailsTheCallWhenTheBatchReadFails: a backend error in the
// batch read must fail the call. A shorter list would read as "those ids do not
// exist", which is the one answer a failed read must never give.
func TestReaderGetManyFailsTheCallWhenTheBatchReadFails(t *testing.T) {
	ctx := context.Background()
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close sqlmock connection: %v", err)
		}
	})
	mock.ExpectBegin()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// A generic error, not a missing table: the wisp probe must surface it
	// rather than read it as "no wisps".
	boom := errors.New("connection reset by peer")
	mock.ExpectQuery(".*").WillReturnError(boom)

	reader, err := NewIssueReader(fixedProvider{uw: &baseUOW{tx: sqlmockTx{runner: tx}}})
	if err != nil {
		t.Fatalf("NewIssueReader: %v", err)
	}
	got, err := reader.GetMany(ctx, publicops.GetManyRequest{IDs: []string{"bd-1", "bd-2"}})
	if !errors.Is(err, boom) {
		t.Fatalf("GetMany error = %v, want the batch read's error", err)
	}
	if len(got.Details) != 0 || len(got.NotFound) != 0 {
		t.Errorf("GetMany answered a list on a failed read: %+v", got)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
