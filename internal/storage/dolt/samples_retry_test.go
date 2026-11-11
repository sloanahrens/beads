package dolt

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/storage/domain/db"
	"github.com/steveyegge/beads/samplesops"
)

// retrySampleTable is an in-process model of the samples table. It enforces
// the PRIMARY KEY the way Dolt does: a repeated id is a duplicate-key error
// unless the statement carries ON DUPLICATE KEY UPDATE.
type retrySampleTable struct{ rows map[string][]any }

func newRetrySampleTable() *retrySampleTable {
	return &retrySampleTable{rows: map[string][]any{}}
}

func (r *retrySampleTable) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	id, _ := args[0].(string)
	if _, exists := r.rows[id]; exists {
		if !strings.Contains(query, "ON DUPLICATE KEY UPDATE") {
			return nil, fmt.Errorf("Error 1062: Duplicate entry %q for key 'PRIMARY'", id)
		}
		return retryResult(0), nil
	}
	r.rows[id] = args
	return retryResult(1), nil
}

func (r *retrySampleTable) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, fmt.Errorf("retrySampleTable: QueryContext is not used by Add")
}

func (r *retrySampleTable) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

type retryResult int64

func (retryResult) LastInsertId() (int64, error)   { return 0, nil }
func (r retryResult) RowsAffected() (int64, error) { return int64(r), nil }

// TestSamplesAddLandsOneRowAcrossARetriedWrite pins acceptance's retry clause
// at the accessor, where the id is minted: the write boundary replays the
// whole transaction, and because the row was prepared BEFORE the boundary both
// attempts carry the same id. Minting inside the transaction would leave two
// rows and a duplicate-key error here.
//
// Production's withRetryTx refuses to replay an indeterminate commit; this
// test pins the stronger property that a boundary which does replay cannot
// double-apply either.
func TestSamplesAddLandsOneRowAcrossARetriedWrite(t *testing.T) {
	table := newRetrySampleTable()
	attempts := 0
	accessor := &samples{runWriteTx: func(ctx context.Context, fn func(db.Runner) error) error {
		for {
			attempts++
			if err := fn(table); err != nil {
				return err
			}
			if attempts == 1 {
				// The first attempt committed, but its result was lost, so
				// the boundary replays the whole transaction.
				continue
			}
			return nil
		}
	}}

	result, err := accessor.Add(context.Background(), samplesops.AddRequest{
		Kind:    "usage_record",
		At:      time.Date(2026, 11, 11, 9, 31, 0, 0, time.UTC),
		Payload: []byte(`{"used":1200}`),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("write boundary ran %d attempts, want 2", attempts)
	}
	if got := len(table.rows); got != 1 {
		t.Fatalf("a retried add stored %d rows, want 1", got)
	}
	if result.Sample.ID == "" {
		t.Error("Add returned no id")
	}
	if _, ok := table.rows[result.Sample.ID]; !ok {
		t.Errorf("Add returned id %q, which is not the row stored", result.Sample.ID)
	}
}

// TestSamplesAddRefusesAnUnwiredWriteBoundary covers the refusal branch: an
// accessor built without its transaction boundary must not report a write.
func TestSamplesAddRefusesAnUnwiredWriteBoundary(t *testing.T) {
	accessor := &samples{}
	_, err := accessor.Add(context.Background(), samplesops.AddRequest{
		Kind:    "usage_record",
		At:      time.Now(),
		Payload: []byte(`{"used":1}`),
	})
	if err == nil {
		t.Fatal("Add on an accessor with no write boundary returned nil, want a refusal")
	}
}
