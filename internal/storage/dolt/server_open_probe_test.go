package dolt

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	mysql "github.com/go-sql-driver/mysql"
)

// fakeDoltServer is a database/sql driver standing in for a dolt sql-server.
// It knows a fixed set of databases and refuses a connection to an unknown one
// with MySQL 1049, as the real handshake does. It records every statement a
// connection runs, so a test can assert what an open sent to the server.
type fakeDoltServer struct {
	mu         sync.Mutex
	databases  map[string]bool
	openErr    error // when set, every connection attempt is refused with it
	createErr  error // when set, CREATE DATABASE fails with it
	opens      int
	statements []string
}

func (s *fakeDoltServer) Open(dsn string) (driver.Conn, error) {
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opens++
	if s.openErr != nil {
		return nil, s.openErr
	}
	if parsed.DBName != "" && !s.databases[parsed.DBName] {
		return nil, &mysql.MySQLError{Number: 1049, Message: fmt.Sprintf("Unknown database '%s'", parsed.DBName)}
	}
	return &fakeDoltConn{server: s}, nil
}

func (s *fakeDoltServer) record(stmt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statements = append(s.statements, stmt)
}

func (s *fakeDoltServer) openCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opens
}

// statementCount returns how many recorded statements contain substr.
func (s *fakeDoltServer) statementCount(substr string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, stmt := range s.statements {
		if strings.Contains(stmt, substr) {
			n++
		}
	}
	return n
}

func (s *fakeDoltServer) databaseNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	for name := range s.databases {
		names = append(names, name)
	}
	return names
}

func (s *fakeDoltServer) createDatabase(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.databases == nil {
		s.databases = map[string]bool{}
	}
	s.databases[name] = true
}

type fakeDoltConn struct {
	server *fakeDoltServer
}

func (c *fakeDoltConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("fake dolt: prepared statements not supported: %s", query)
}

func (c *fakeDoltConn) Close() error { return nil }

func (c *fakeDoltConn) Begin() (driver.Tx, error) {
	return nil, errors.New("fake dolt: transactions not supported")
}

func (c *fakeDoltConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.server.record(query)
	if query != "SHOW DATABASES" {
		return nil, fmt.Errorf("fake dolt: unexpected query %q", query)
	}
	return &fakeDoltRows{names: c.server.databaseNames()}, nil
}

func (c *fakeDoltConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.server.record(query)
	if !strings.HasPrefix(query, "CREATE DATABASE") {
		return nil, fmt.Errorf("fake dolt: unexpected exec %q", query)
	}
	if c.server.createErr != nil {
		return nil, c.server.createErr
	}
	c.server.createDatabase(strings.Trim(strings.TrimPrefix(query, "CREATE DATABASE"), " `"))
	return driver.RowsAffected(0), nil
}

type fakeDoltRows struct {
	names []string
	next  int
}

func (r *fakeDoltRows) Columns() []string { return []string{"Database"} }

func (r *fakeDoltRows) Close() error { return nil }

func (r *fakeDoltRows) Next(dest []driver.Value) error {
	if r.next >= len(r.names) {
		return io.EOF
	}
	dest[0] = r.names[r.next]
	r.next++
	return nil
}

// registerFakeDoltServer registers the fake under a per-test driver name, since
// database/sql panics on duplicate registration, and returns that name.
func registerFakeDoltServer(t *testing.T, server *fakeDoltServer) string {
	t.Helper()
	name := fmt.Sprintf("fake-dolt-%s-%p", t.Name(), server)
	sql.Register(name, server)
	return name
}

// fakeOpenConfig targets a database name that is not a test-database name, so
// the production firewall does not apply, on a port the fake ignores.
func fakeOpenConfig(t *testing.T, createIfMissing bool) *Config {
	t.Helper()
	return &Config{
		Database:        "proj_alpha",
		BeadsDir:        t.TempDir(),
		ServerHost:      "127.0.0.1",
		ServerPort:      3309,
		CreateIfMissing: createIfMissing,
	}
}

func TestNewServerModeNoCreateSkipsAdminProbe(t *testing.T) {
	server := &fakeDoltServer{databases: map[string]bool{"proj_alpha": true}}
	cfg := fakeOpenConfig(t, false)

	db, _, facts, err := openServerConnectionWithDriver(context.Background(), cfg, registerFakeDoltServer(t, server))
	if err != nil {
		t.Fatalf("openServerConnectionWithDriver: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if got := server.openCount(); got != 1 {
		t.Errorf("connections opened = %d, want 1", got)
	}
	if n := server.statementCount("SHOW DATABASES"); n != 0 {
		t.Errorf("SHOW DATABASES sent %d times, want 0", n)
	}
	if !facts.alreadyExisted || facts.created {
		t.Errorf("facts = %+v, want alreadyExisted true and created false", facts)
	}
}

func TestNewServerModeNoCreateMissingDatabase(t *testing.T) {
	server := &fakeDoltServer{databases: map[string]bool{}}
	cfg := fakeOpenConfig(t, false)

	db, _, _, err := openServerConnectionWithDriver(context.Background(), cfg, registerFakeDoltServer(t, server))
	if err == nil {
		_ = db.Close()
		t.Fatal("openServerConnectionWithDriver succeeded for a missing database, want not-found error")
	}
	if want := databaseNotFoundError(cfg).Error(); err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if n := server.statementCount("SHOW DATABASES"); n != 0 {
		t.Errorf("SHOW DATABASES sent %d times, want 0", n)
	}
	if n := server.statementCount("CREATE DATABASE"); n != 0 {
		t.Errorf("CREATE DATABASE sent %d times, want 0", n)
	}
	if got := server.openCount(); got != 1 {
		t.Errorf("connections opened = %d, want 1", got)
	}
}

func TestNewServerModeNoCreateRefusedOpenKeepsError(t *testing.T) {
	server := &fakeDoltServer{openErr: &mysql.MySQLError{Number: 1044, Message: "Access denied for user"}}
	cfg := fakeOpenConfig(t, false)

	db, _, _, err := openServerConnectionWithDriver(context.Background(), cfg, registerFakeDoltServer(t, server))
	if err == nil {
		_ = db.Close()
		t.Fatal("openServerConnectionWithDriver succeeded on a refused open, want error")
	}
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1044 {
		t.Errorf("error = %v, want the server's 1044 refusal wrapped", err)
	}
	if err.Error() == databaseNotFoundError(cfg).Error() {
		t.Errorf("a non-1049 refusal was reported as database not found: %v", err)
	}
}

func TestNewServerModeCreateIfMissingExistingDatabaseKeepsProbe(t *testing.T) {
	server := &fakeDoltServer{databases: map[string]bool{"proj_alpha": true}}
	cfg := fakeOpenConfig(t, true)

	db, _, facts, err := openServerConnectionWithDriver(context.Background(), cfg, registerFakeDoltServer(t, server))
	if err != nil {
		t.Fatalf("openServerConnectionWithDriver: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if n := server.statementCount("SHOW DATABASES"); n != 1 {
		t.Errorf("SHOW DATABASES sent %d times, want 1 on the init connection", n)
	}
	if n := server.statementCount("CREATE DATABASE"); n != 0 {
		t.Errorf("CREATE DATABASE sent %d times for an existing database, want 0", n)
	}
	if !facts.alreadyExisted || facts.created {
		t.Errorf("facts = %+v, want alreadyExisted true and created false", facts)
	}
}

func TestNewServerModeCreateIfMissingCreateFailureUnchanged(t *testing.T) {
	server := &fakeDoltServer{databases: map[string]bool{}, createErr: errors.New("disk full")}
	cfg := fakeOpenConfig(t, true)

	db, _, _, err := openServerConnectionWithDriver(context.Background(), cfg, registerFakeDoltServer(t, server))
	if err == nil {
		_ = db.Close()
		t.Fatal("openServerConnectionWithDriver succeeded when CREATE DATABASE failed, want error")
	}
	if want := "failed to create database: disk full"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if n := server.statementCount("CREATE DATABASE"); n != 1 {
		t.Errorf("CREATE DATABASE sent %d times, want 1", n)
	}
}
