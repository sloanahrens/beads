package samplesops

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	publicops "github.com/steveyegge/beads/samplesops"
)

// tableRunner is an in-process model of the samples table. It enforces the
// PRIMARY KEY the way Dolt does: a repeated id is a duplicate-key error unless
// the statement carries ON DUPLICATE KEY UPDATE. It also models the guarded
// duplicate branch AddInTx writes: a duplicate whose fields all match the
// stored row is the retry no-op, while a duplicate that differs makes the
// statement assign NULL to the NOT NULL id column, which the engine refuses.
type tableRunner struct {
	rows     map[string][]any
	inserted int
}

func newTableRunner() *tableRunner { return &tableRunner{rows: map[string][]any{}} }

func (r *tableRunner) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	id, ok := args[0].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("insert carried no id: %#v", args)
	}
	stored, exists := r.rows[id]
	if !exists {
		r.rows[id] = args
		r.inserted++
		return fakeResult{affected: 1}, nil
	}
	if !strings.Contains(query, "ON DUPLICATE KEY UPDATE") {
		return nil, fmt.Errorf("Error 1062: Duplicate entry %q for key 'PRIMARY'", id)
	}
	if !strings.Contains(query, "IF(kind = VALUES(kind)") {
		// The unguarded `ON DUPLICATE KEY UPDATE id = id` Dolt accepts: it
		// swallows every collision, including one with a different row.
		return fakeResult{affected: 0}, nil
	}
	if !reflect.DeepEqual(stored, args) {
		return nil, fmt.Errorf("Error 1048 (23000): Column 'id' cannot be null")
	}
	return fakeResult{affected: 0}, nil
}

func (r *tableRunner) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, fmt.Errorf("tableRunner: QueryContext is not used by AddInTx")
}

func (r *tableRunner) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

type fakeResult struct{ affected int64 }

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (r fakeResult) RowsAffected() (int64, error) {
	return r.affected, nil
}

// TestAddInTxLandsOneRowWhenARetryReusesThePreparedRow pins the retry
// contract: a committed insert whose result was lost is replayed with the
// SAME Prepared row, and the secondary attempt adds nothing. Removing the
// idempotent clause from the insert turns this red with a duplicate-key error.
func TestAddInTxLandsOneRowWhenARetryReusesThePreparedRow(t *testing.T) {
	ctx := context.Background()
	table := newTableRunner()
	row := Prepare(publicops.AddRequest{
		Kind:    "usage_record",
		At:      time.Date(2026, 11, 11, 9, 31, 0, 0, time.UTC),
		Payload: []byte(`{"used":1200}`),
	})

	first, err := AddInTx(ctx, table, row)
	if err != nil {
		t.Fatalf("first AddInTx: %v", err)
	}
	// The retry: the same prepared row, after an ambiguous first result.
	second, err := AddInTx(ctx, table, row)
	if err != nil {
		t.Fatalf("retried AddInTx: %v", err)
	}

	if got := len(table.rows); got != 1 {
		t.Fatalf("a retried add stored %d rows, want 1", got)
	}
	if first.Sample != second.Sample {
		t.Errorf("retry returned a different row:\n first: %#v\nsecond: %#v", first.Sample, second.Sample)
	}
	if table.inserted != 1 {
		t.Errorf("inserted %d rows into the model, want 1", table.inserted)
	}
}

// TestAddInTxRefusesAPrimaryKeyCollisionWithADifferentRow pins the guard on
// the duplicate branch: an id already held by a DIFFERENT sample is a refusal,
// not a silent success that hands back somebody else's row. A bare
// `ON DUPLICATE KEY UPDATE id = id` swallows this case, so removing the IF
// guard from the statement turns this test red.
func TestAddInTxRefusesAPrimaryKeyCollisionWithADifferentRow(t *testing.T) {
	ctx := context.Background()
	table := newTableRunner()
	first := Prepare(publicops.AddRequest{
		Kind:    "usage_record",
		At:      time.Date(2026, 11, 11, 9, 31, 0, 0, time.UTC),
		Payload: []byte(`{"used":1200}`),
	})
	if _, err := AddInTx(ctx, table, first); err != nil {
		t.Fatalf("first AddInTx: %v", err)
	}

	// A different observation that collides on the minted id.
	collision := first
	collision.Payload = `{"used":9999}`
	_, err := AddInTx(ctx, table, collision)
	if err == nil {
		t.Fatal("AddInTx with a colliding id returned nil, want a refusal")
	}
	if !strings.Contains(err.Error(), first.ID) {
		t.Errorf("refusal %q does not name the colliding id %q", err, first.ID)
	}
	if got, want := table.rows[first.ID][3], `{"used":1200}`; got != want {
		t.Errorf("stored payload = %v, want the original row %q", got, want)
	}
	if table.inserted != 1 {
		t.Errorf("inserted %d rows, want 1 (a collision must not add a row)", table.inserted)
	}
}

// TestPrepareCopiesTheRequest pins that the row a caller holds across a retry
// does not alias the caller's buffer and carries every minted field.
func TestPrepareCopiesTheRequest(t *testing.T) {
	payload := []byte(`{"plan":"max"}`)
	request := publicops.AddRequest{Kind: "claude_plan", At: time.Now(), Payload: payload}
	row := Prepare(request)

	if row.ID == "" {
		t.Fatal("Prepare returned an empty id")
	}
	if row.CreatedAt.IsZero() {
		t.Error("Prepared created_at is the zero time")
	}
	payload[0] = 'X' // caller reuses its buffer after handing it over
	if row.Payload != `{"plan":"max"}` {
		t.Errorf("Prepared payload = %q, want the value as given", row.Payload)
	}
}
