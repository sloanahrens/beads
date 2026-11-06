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
	denied     map[string]bool // databases this login has no grant on: connecting to one is refused with 1044
	openErr    error           // when set, every connection attempt is refused with it
	createErr  error           // when set, CREATE DATABASE fails with it
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
	if s.denied[parsed.DBName] {
		return nil, &mysql.MySQLError{Number: 1044, Message: fmt.Sprintf("Access denied for user to database '%s'", parsed.DBName)}
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

// TestNewServerModeNoCreateRefusedOpenKeepsError: a refusal other than 1049
// keeps the text the old admin-connection probe gave it, "failed to check if
// database ... exists on server host:port: <server error>", and is never read
// as database-not-found or as a failed CREATE (none ran). The text is pinned
// whole, so a change to it fails here.
func TestNewServerModeNoCreateRefusedOpenKeepsError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"login refused", &mysql.MySQLError{Number: 1045, Message: "Access denied for user"}},
		{"connection refused", errors.New("dial tcp 127.0.0.1:3309: connect: connection refused")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeDoltServer{openErr: tc.err}
			cfg := fakeOpenConfig(t, false)

			db, _, _, err := openServerConnectionWithDriver(context.Background(), cfg, registerFakeDoltServer(t, server))
			if err == nil {
				_ = db.Close()
				t.Fatal("openServerConnectionWithDriver succeeded on a refused open, want error")
			}
			want := fmt.Sprintf("failed to check if database %q exists on server %s:%d: %v", cfg.Database, cfg.ServerHost, cfg.ServerPort, tc.err)
			if err.Error() != want {
				t.Errorf("error = %q, want %q", err.Error(), want)
			}
			if !errors.Is(err, tc.err) {
				t.Errorf("error %v does not wrap the server's refusal %v", err, tc.err)
			}
			if strings.Contains(err.Error(), "CREATE DATABASE") {
				t.Errorf("error %q names a CREATE that never ran", err)
			}
		})
	}
}

// TestNewServerModeNoCreateDoesNotRetryConnectionRefused: the no-create ping is
// one attempt. isRetryableError treats connection refused as transient, so a
// retry inside the create path's backoff would keep retrying for 10 s. The
// refusal falls through to the probe, which fails at once: two attempts in all
// (the ping, then the probe), never a backoff. The guard line makes sure the
// error used here is the kind the retry loop would retry.
func TestNewServerModeNoCreateDoesNotRetryConnectionRefused(t *testing.T) {
	refused := errors.New("dial tcp 127.0.0.1:3309: connect: connection refused")
	if !isRetryableError(refused) {
		t.Fatalf("isRetryableError(%v) = false; the test needs a retryable error", refused)
	}
	server := &fakeDoltServer{openErr: refused}
	cfg := fakeOpenConfig(t, false)

	db, _, _, err := openServerConnectionWithDriver(context.Background(), cfg, registerFakeDoltServer(t, server))
	if err == nil {
		_ = db.Close()
		t.Fatal("openServerConnectionWithDriver succeeded on a refused open, want error")
	}
	if got := server.openCount(); got != 2 {
		t.Errorf("connection attempts = %d, want 2 (one ping, one probe, no retry)", got)
	}
}

// TestNewServerModeNoCreatePrivilegeRefusalKeepsProbeText: a login with no
// grant on the database is refused with 1044, not 1049, so the ping alone
// cannot say "not found". The open falls through to the SHOW DATABASES probe,
// which does not list the database to this login, and gives the not-found
// text the old open gave.
func TestNewServerModeNoCreatePrivilegeRefusalKeepsProbeText(t *testing.T) {
	server := &fakeDoltServer{databases: map[string]bool{}, denied: map[string]bool{"proj_alpha": true}}
	cfg := fakeOpenConfig(t, false)

	db, _, _, err := openServerConnectionWithDriver(context.Background(), cfg, registerFakeDoltServer(t, server))
	if err == nil {
		_ = db.Close()
		t.Fatal("openServerConnectionWithDriver succeeded for a database the login cannot see, want error")
	}
	if want := databaseNotFoundError(cfg).Error(); err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	if n := server.statementCount("SHOW DATABASES"); n != 1 {
		t.Errorf("SHOW DATABASES sent %d times, want 1 (the probe answers after the 1044 refusal)", n)
	}
	if n := server.statementCount("CREATE DATABASE"); n != 0 {
		t.Errorf("CREATE DATABASE sent %d times, want 0", n)
	}
}

// TestNewServerModeIsUnknownDatabase: the 1049 mapping is by number when the
// driver returns a typed MySQL error, and by text when it does not. A 1044
// refusal is not the not-found answer, so it must not map.
func TestNewServerModeIsUnknownDatabase(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"typed 1049", &mysql.MySQLError{Number: 1049, Message: "Unknown database 'proj_alpha'"}, true},
		{"untyped text", errors.New("Error 1049: Unknown database 'proj_alpha'"), true},
		{"typed 1044 is not not-found", &mysql.MySQLError{Number: 1044, Message: "Access denied to database"}, false},
		{"connection refused", errors.New("dial tcp 127.0.0.1:3309: connect: connection refused"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUnknownDatabaseError(tc.err); got != tc.want {
				t.Errorf("isUnknownDatabaseError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
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
