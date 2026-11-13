package dolt

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	"github.com/steveyegge/beads/samplesops"
)

// samplesRetryDriver is an in-process samples table behind a database/sql
// driver, so the test drives the REAL retry boundary — DoltStore.Samples()
// wiring Add through withRetryTx — instead of a hand-written stand-in for it.
//
// The first transaction's INSERT fails with MySQL 1213 (serialization
// conflict), the one failure withRetryTx replays because it guarantees the
// server rolled the write back. The driver stages rows per transaction and
// only publishes them on Commit, so a rolled-back attempt leaves no row.
type samplesRetryDriver struct {
	mu        sync.Mutex
	committed map[string][]driver.Value
	attempts  int
	insertIDs []string
}

func (d *samplesRetryDriver) Driver() driver.Driver { return d }

func (d *samplesRetryDriver) Open(string) (driver.Conn, error) {
	return d.Connect(context.Background())
}

func (d *samplesRetryDriver) Connect(context.Context) (driver.Conn, error) {
	return &samplesRetryConn{driver: d}, nil
}

type samplesRetryConn struct {
	driver *samplesRetryDriver
	staged map[string][]driver.Value
}

func (c *samplesRetryConn) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("samples retry driver does not prepare statements")
}

func (c *samplesRetryConn) Close() error { return nil }

func (c *samplesRetryConn) Begin() (driver.Tx, error) {
	c.driver.mu.Lock()
	defer c.driver.mu.Unlock()
	c.driver.attempts++
	c.staged = map[string][]driver.Value{}
	return &samplesRetryTx{conn: c}, nil
}

func (c *samplesRetryConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if !strings.Contains(query, "INSERT INTO samples") {
		return nil, fmt.Errorf("samples retry driver does not execute %q", query)
	}
	c.driver.mu.Lock()
	defer c.driver.mu.Unlock()

	values := make([]driver.Value, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}
	id, ok := values[0].(string)
	if !ok || id == "" {
		return nil, fmt.Errorf("insert carried no string id: %#v", values)
	}
	c.driver.insertIDs = append(c.driver.insertIDs, id)

	if c.driver.attempts == 1 {
		// Serialization conflict: the server rolled this write back, so
		// nothing is staged for a commit.
		return nil, &mysql.MySQLError{Number: 1213, Message: "serialization failure: retry"}
	}
	c.staged[id] = values
	return driver.RowsAffected(1), nil
}

// QueryContext answers the read-back AddInTx runs after its insert, against the
// rows this transaction has staged and the rows an earlier attempt committed.
func (c *samplesRetryConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT kind, at, payload, created_at FROM samples WHERE id = ?") {
		return nil, fmt.Errorf("samples retry driver does not query %q", query)
	}
	id, ok := args[0].Value.(string)
	if !ok {
		return nil, fmt.Errorf("lookup carried no string id: %#v", args[0].Value)
	}
	c.driver.mu.Lock()
	defer c.driver.mu.Unlock()
	stored, ok := c.staged[id]
	if !ok {
		stored, ok = c.driver.committed[id]
	}
	rows := &samplesRetryRows{columns: []string{"kind", "at", "payload", "created_at"}}
	if ok {
		rows.values = [][]driver.Value{{stored[1], stored[2], stored[3], stored[4]}}
	}
	return rows, nil
}

type samplesRetryRows struct {
	columns []string
	values  [][]driver.Value
	next    int
}

func (r *samplesRetryRows) Columns() []string { return r.columns }
func (r *samplesRetryRows) Close() error      { return nil }
func (r *samplesRetryRows) Next(dest []driver.Value) error {
	if r.next >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.next])
	r.next++
	return nil
}

type samplesRetryTx struct{ conn *samplesRetryConn }

func (t *samplesRetryTx) Commit() error {
	t.conn.driver.mu.Lock()
	defer t.conn.driver.mu.Unlock()
	for id, values := range t.conn.staged {
		t.conn.driver.committed[id] = values
	}
	t.conn.staged = nil
	return nil
}

func (t *samplesRetryTx) Rollback() error {
	t.conn.driver.mu.Lock()
	defer t.conn.driver.mu.Unlock()
	t.conn.staged = nil
	return nil
}

// TestSamplesAddRetriesThroughWithRetryTx pins the retry clause at the
// production boundary: a serialization conflict makes withRetryTx replay the
// whole transaction, and because the row is PREPARED before that boundary both
// attempts carry the same id. Minting the id inside the closure would insert
// under two ids and turn this red on the equality check; a boundary that did
// not retry would leave attempts at one.
func TestSamplesAddRetriesThroughWithRetryTx(t *testing.T) {
	driver := &samplesRetryDriver{committed: map[string][]driver.Value{}}
	store := &DoltStore{db: sql.OpenDB(driver)}
	t.Cleanup(func() { _ = store.db.Close() })

	role, err := store.Samples()
	if err != nil {
		t.Fatalf("Samples: %v", err)
	}

	result, err := role.Add(context.Background(), samplesops.AddRequest{
		Kind:    "usage_record",
		At:      time.Date(2026, 11, 11, 9, 31, 0, 0, time.UTC),
		Payload: []byte(`{"used":1200}`),
	})
	if err != nil {
		t.Fatalf("Add through withRetryTx: %v", err)
	}

	driver.mu.Lock()
	defer driver.mu.Unlock()
	if driver.attempts != 2 {
		t.Fatalf("write boundary ran %d transactions, want 2 (one serialization replay)", driver.attempts)
	}
	if len(driver.insertIDs) != 2 {
		t.Fatalf("the insert ran %d times, want 2", len(driver.insertIDs))
	}
	if driver.insertIDs[0] != driver.insertIDs[1] {
		t.Fatalf("the replay inserted id %q after %q; the row must be prepared before the boundary so both attempts reuse one id",
			driver.insertIDs[1], driver.insertIDs[0])
	}
	if result.Sample.ID != driver.insertIDs[0] {
		t.Errorf("Add returned id %q, but the insert carried %q", result.Sample.ID, driver.insertIDs[0])
	}
	if got := len(driver.committed); got != 1 {
		t.Fatalf("a retried add committed %d rows, want 1", got)
	}
	if _, ok := driver.committed[result.Sample.ID]; !ok {
		t.Errorf("the committed row is not the id Add returned (%q)", result.Sample.ID)
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
