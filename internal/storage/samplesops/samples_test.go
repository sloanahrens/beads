package samplesops

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	publicops "github.com/steveyegge/beads/samplesops"
)

// samplesTable is an in-process model of the samples table behind a
// database/sql driver. AddInTx writes one row and then reads it back through the
// same runner, so the fake has to answer both against the same rows; a
// hand-written db.Runner cannot, because QueryRowContext hands back a concrete
// *sql.Row that only a driver can produce.
//
// What it models: the PRIMARY KEY (a repeated id without an ON DUPLICATE clause
// is a duplicate-key error), the idempotent duplicate branch (`id = id` leaves
// the stored row alone), and — for any other ON DUPLICATE clause — the outcome
// owl observed for the old statement on Dolt (be-8o10): a differing duplicate
// exits 0 with the stored key cleared instead of being refused. Modelling that
// last case is what lets a regression to the old statement turn the collision
// test red; the real-engine check is the overseer's.
type samplesTable struct {
	mu       sync.Mutex
	rows     map[string][]driver.Value // stored (id, kind, at, payload, created_at) by id
	inserted int
	// acceptedButNotStored models an insert the engine reports as successful
	// while leaving no row under the id, the shape of the NULL-key write. Only
	// a test that drives the read-back refusal sets it.
	acceptedButNotStored bool
	// readErr, when set, fails the read-back the way a broken connection would.
	// Only a test that drives the read-back error branch sets it.
	readErr error
}

func newSamplesTable(t *testing.T) (*samplesTable, *sql.DB) {
	t.Helper()
	table := &samplesTable{rows: map[string][]driver.Value{}}
	database := sql.OpenDB(table)
	t.Cleanup(func() { _ = database.Close() })
	return table, database
}

// row returns a copy of the stored values for id, or nil when no row has it.
func (t *samplesTable) row(id string) []driver.Value {
	t.mu.Lock()
	defer t.mu.Unlock()
	stored, ok := t.rows[id]
	if !ok {
		return nil
	}
	return append([]driver.Value(nil), stored...)
}

func (t *samplesTable) Driver() driver.Driver { return t }
func (t *samplesTable) Open(string) (driver.Conn, error) {
	return &samplesConn{table: t}, nil
}
func (t *samplesTable) Connect(context.Context) (driver.Conn, error) {
	return &samplesConn{table: t}, nil
}

type samplesConn struct{ table *samplesTable }

func (c *samplesConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("samples table fake: does not prepare statements")
}
func (c *samplesConn) Close() error              { return nil }
func (c *samplesConn) Begin() (driver.Tx, error) { return noopTx{}, nil }

func (c *samplesConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	id, ok := values[0].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("insert carried no string id: %#v", values)
	}

	c.table.mu.Lock()
	defer c.table.mu.Unlock()
	if c.table.acceptedButNotStored {
		return driver.RowsAffected(1), nil
	}
	stored, exists := c.table.rows[id]
	if !exists {
		c.table.rows[id] = values
		c.table.inserted++
		return driver.RowsAffected(1), nil
	}
	switch {
	case strings.Contains(query, "ON DUPLICATE KEY UPDATE id = id"):
		// The idempotent branch: the stored row is left exactly as it is and
		// the read-back decides whether this was a retry.
		return driver.RowsAffected(0), nil
	case strings.Contains(query, "ON DUPLICATE KEY UPDATE"):
		// Any other guarded form: model the Dolt behaviour the old statement
		// met on the real engine. A differing duplicate exits 0 with the
		// stored key cleared, so reading by id finds nothing.
		if !reflect.DeepEqual(stored, values) {
			delete(c.table.rows, id)
		}
		return driver.RowsAffected(0), nil
	default:
		return nil, fmt.Errorf("Error 1062: Duplicate entry %q for key 'PRIMARY'", id)
	}
}

func (c *samplesConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT kind, at, payload, created_at FROM samples WHERE id = ?") {
		return nil, fmt.Errorf("samples table fake does not query %q", query)
	}
	id, ok := args[0].Value.(string)
	if !ok {
		return nil, fmt.Errorf("lookup carried no string id: %#v", args[0].Value)
	}
	if c.table.readErr != nil {
		return nil, c.table.readErr
	}
	c.table.mu.Lock()
	defer c.table.mu.Unlock()
	stored, exists := c.table.rows[id]
	rows := &samplesRows{columns: []string{"kind", "at", "payload", "created_at"}}
	if exists {
		rows.values = [][]driver.Value{{stored[1], stored[2], stored[3], stored[4]}}
	}
	return rows, nil
}

type samplesRows struct {
	columns []string
	values  [][]driver.Value
	next    int
}

func (r *samplesRows) Columns() []string { return r.columns }
func (r *samplesRows) Close() error      { return nil }
func (r *samplesRows) Next(dest []driver.Value) error {
	if r.next >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.next])
	r.next++
	return nil
}

type noopTx struct{}

func (noopTx) Commit() error   { return nil }
func (noopTx) Rollback() error { return nil }

// TestAddInTxLandsOneRowWhenARetryReusesThePreparedRow pins the retry
// contract: a committed insert whose result was lost is replayed with the
// SAME Prepared row, and the second attempt adds nothing. Dropping the
// idempotent clause from the insert turns this red with a duplicate-key error.
func TestAddInTxLandsOneRowWhenARetryReusesThePreparedRow(t *testing.T) {
	ctx := context.Background()
	table, runner := newSamplesTable(t)
	row := Prepare(publicops.AddRequest{
		Kind:    "usage_record",
		At:      time.Date(2026, 11, 11, 9, 31, 0, 0, time.UTC),
		Payload: []byte(`{"used":1200}`),
	})

	first, err := AddInTx(ctx, runner, row)
	if err != nil {
		t.Fatalf("first AddInTx: %v", err)
	}
	// The retry: the same prepared row, after an ambiguous first result.
	second, err := AddInTx(ctx, runner, row)
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

// TestAddInTxRefusesAPrimaryKeyCollisionWithADifferentRow pins the collision
// guard: an id already held by a DIFFERENT sample is a refusal naming the id,
// not a silent success that hands back somebody else's row, and the stored row
// is left whole. The read-back comparison is the whole guard, so deleting it
// turns this red; so does restoring the statement be-8o10 removed, because on
// the modelled engine that statement clears the stored row's key instead of
// refusing.
func TestAddInTxRefusesAPrimaryKeyCollisionWithADifferentRow(t *testing.T) {
	ctx := context.Background()
	table, runner := newSamplesTable(t)
	first := Prepare(publicops.AddRequest{
		Kind:    "usage_record",
		At:      time.Date(2026, 11, 11, 9, 31, 0, 0, time.UTC),
		Payload: []byte(`{"used":1200}`),
	})
	if _, err := AddInTx(ctx, runner, first); err != nil {
		t.Fatalf("first AddInTx: %v", err)
	}
	stored := table.row(first.ID)

	// A different observation that collides on the minted id.
	collision := first
	collision.Payload = `{"used":9999}`
	_, err := AddInTx(ctx, runner, collision)
	if err == nil {
		t.Fatal("AddInTx with a colliding id returned nil, want a refusal")
	}
	if !strings.Contains(err.Error(), first.ID) {
		t.Errorf("refusal %q does not name the colliding id %q", err, first.ID)
	}
	if got := table.row(first.ID); !reflect.DeepEqual(got, stored) {
		t.Errorf("a refused collision changed the stored row:\n got: %#v\nwant: %#v", got, stored)
	}
	if table.inserted != 1 {
		t.Errorf("inserted %d rows, want 1 (a collision must not add a row)", table.inserted)
	}
}

// TestAddInTxRefusesAnInsertThatLeavesNoRowForTheID covers the other way the
// read-back can fail: the insert reports success yet no row holds the id (the
// shape of the NULL-key write be-8o10 removes). Reading that as "no collision"
// would report an observation as stored that is not.
func TestAddInTxRefusesAnInsertThatLeavesNoRowForTheID(t *testing.T) {
	ctx := context.Background()
	table, runner := newSamplesTable(t)
	table.acceptedButNotStored = true
	row := Prepare(publicops.AddRequest{
		Kind:    "usage_record",
		At:      time.Date(2026, 11, 11, 9, 31, 0, 0, time.UTC),
		Payload: []byte(`{"used":1200}`),
	})

	_, err := AddInTx(ctx, runner, row)
	if err == nil {
		t.Fatal("AddInTx with no stored row returned nil, want a refusal")
	}
	if !strings.Contains(err.Error(), row.ID) {
		t.Errorf("refusal %q does not name the sample id %q", err, row.ID)
	}
}

// TestAddInTxSurfacesAReadBackFailure pins that a failed read-back is reported
// as a failure: the add cannot be confirmed, so the caller must not be told the
// observation was stored.
func TestAddInTxSurfacesAReadBackFailure(t *testing.T) {
	ctx := context.Background()
	table, runner := newSamplesTable(t)
	table.readErr = errors.New("read refused")
	row := Prepare(publicops.AddRequest{
		Kind:    "usage_record",
		At:      time.Date(2026, 11, 11, 9, 31, 0, 0, time.UTC),
		Payload: []byte(`{"used":1200}`),
	})

	_, err := AddInTx(ctx, runner, row)
	if err == nil {
		t.Fatal("AddInTx with a failing read-back returned nil, want a refusal")
	}
	if !errors.Is(err, table.readErr) {
		t.Errorf("error %v does not wrap the read-back failure %v", err, table.readErr)
	}
	if !strings.Contains(err.Error(), row.ID) {
		t.Errorf("refusal %q does not name the sample id %q", err, row.ID)
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
