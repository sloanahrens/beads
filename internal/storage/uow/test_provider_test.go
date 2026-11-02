package uow

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"github.com/steveyegge/beads/internal/storage/doltutil"
	"github.com/steveyegge/beads/internal/testutil"
)

// newTestUOWProvider is the real-provider fixture for every contract test in
// this package. It starts a plain `dolt sql-server` in a temp directory and
// opens the uow backend against it — no proxy, no proxied child process.
func newTestUOWProvider(t *testing.T) UnitOfWorkProvider {
	t.Helper()
	server := startPlainDoltServer(t)
	ctx := context.Background()
	endpoint := "tcp:" + net.JoinHostPort(server.host, strconv.Itoa(server.port))

	initDB, err := openDB(ctx, plainServerDSN(server, ""))
	require.NoError(t, err, "open the database-less connection")
	initProvider := &doltSQLProvider{
		defaultBranch:  defaultBranch,
		db:             initDB,
		serverEndpoint: endpoint,
	}
	if err := initProvider.initSchema(ctx, "beads"); err != nil {
		_ = initDB.Close()
		t.Fatalf("uow: init schema: %v", err)
	}
	require.NoError(t, initDB.Close(), "close the database-less connection")

	dbConn, err := openDB(ctx, plainServerDSN(server, "beads"))
	require.NoError(t, err, "open the beads connection")

	// teamServer, expectedProjectID and preview are the zero values
	// openAndInitSchema gives a plain local server, so only the fields that
	// differ from the zero value are set here.
	provider := &doltSQLProvider{
		defaultBranch:  defaultBranch,
		db:             dbConn,
		serverEndpoint: endpoint,
	}
	t.Cleanup(func() { _ = provider.Close(context.Background()) })
	return provider
}

// plainServer is the address of a dolt sql-server a test started itself.
type plainServer struct {
	host string
	port int
}

const plainServerReadyLine = "Server ready. Accepting connections."

// startPlainDoltServer runs `dolt sql-server` on a free loopback port in a
// fresh temp directory, waits for its readiness line under a context deadline,
// and stops it when the test ends.
func startPlainDoltServer(t *testing.T) plainServer {
	t.Helper()
	testutil.RequireDoltBinary(t)
	doltBin, err := exec.LookPath("dolt")
	require.NoError(t, err)

	port, err := freePlainServerPort()
	require.NoError(t, err)

	rootDir := t.TempDir()
	cfgPath := filepath.Join(rootDir, "config.yaml")
	cfg := fmt.Sprintf("log_level: info\nlistener:\n  host: 127.0.0.1\n  port: %d\n", port)
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0o600))

	cmd := exec.Command(doltBin, "sql-server", "--config", cfgPath)
	cmd.Dir = rootDir
	logs := newReadyLogWriter(plainServerReadyLine)
	cmd.Stdout = logs
	cmd.Stderr = logs

	require.NoError(t, cmd.Start())
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() { stopPlainDoltServer(t, cmd, exited) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	select {
	case <-logs.ready:
	case <-exited:
		t.Fatalf("dolt sql-server exited before it became ready\nserver log:\n%s", logs.output())
	case <-ctx.Done():
		t.Fatalf("dolt sql-server did not become ready: %v\nserver log:\n%s", ctx.Err(), logs.output())
	}
	return plainServer{host: "127.0.0.1", port: port}
}

// stopPlainDoltServer asks the server to shut down and bounds the wait for it.
func stopPlainDoltServer(t *testing.T, cmd *exec.Cmd, exited <-chan struct{}) {
	t.Helper()
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Not every platform can deliver SIGTERM; kill instead.
		_ = cmd.Process.Kill()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	select {
	case <-exited:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-exited
	}
}

// freePlainServerPort reserves a loopback port by listening on it and releasing
// it, so the server can bind the address the test dials.
func freePlainServerPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("reserve loopback port: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		return 0, fmt.Errorf("release loopback port: %w", err)
	}
	return port, nil
}

// plainServerDSN builds the connection string the uow backend uses against a
// plain server, mirroring the fields buildDSN sets.
func plainServerDSN(server plainServer, database string) string {
	cfg := mysql.Config{
		User:                 "root",
		Net:                  "tcp",
		Addr:                 net.JoinHostPort(server.host, strconv.Itoa(server.port)),
		DBName:               database,
		ParseTime:            true,
		MultiStatements:      true,
		Timeout:              5 * time.Second,
		AllowNativePasswords: true,
		ClientFoundRows:      true,
		MaxAllowedPacket:     doltutil.DefaultMaxAllowedPacket,
		TLSConfig:            "false",
	}
	return cfg.FormatDSN()
}

// readyLogWriter accumulates a child's output and closes ready once the line
// appears, so readiness is a condition rather than a poll loop. Writes arrive
// from the stdout and stderr copy goroutines, so they are serialized here.
type readyLogWriter struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	line      string
	signalled bool
	ready     chan struct{}
}

func newReadyLogWriter(line string) *readyLogWriter {
	return &readyLogWriter{line: line, ready: make(chan struct{})}
}

func (w *readyLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	if !w.signalled && strings.Contains(w.buf.String(), w.line) {
		w.signalled = true
		close(w.ready)
	}
	return n, err
}

func (w *readyLogWriter) output() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestReadyLogWriterSignalsOnSplitReadinessLine pins the two properties the
// readiness wait relies on: a partial write that does not yet hold the line
// leaves ready open, and the write that completes it closes ready exactly
// once. A subprocess's output arrives in arbitrary chunks, so the line can
// straddle two Writes.
func TestReadyLogWriterSignalsOnSplitReadinessLine(t *testing.T) {
	w := newReadyLogWriter("Server ready. Accepting connections.")

	if _, err := w.Write([]byte("Starting server with Config HP=\"127.0.0.1:1\"\nServer rea")); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	select {
	case <-w.ready:
		t.Fatal("ready closed before the readiness line was complete")
	default:
	}

	if _, err := w.Write([]byte("dy. Accepting connections.\n")); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	select {
	case <-w.ready:
	default:
		t.Fatal("ready still open after the readiness line was written")
	}

	if got, want := w.output(), "Starting server with Config HP=\"127.0.0.1:1\"\nServer ready. Accepting connections.\n"; got != want {
		t.Fatalf("output() = %q, want %q", got, want)
	}
}
