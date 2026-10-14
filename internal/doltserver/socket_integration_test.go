//go:build integration && !windows

package doltserver_test

import (
	"bytes"
	"database/sql"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/steveyegge/beads/internal/storage/doltutil"
	"github.com/steveyegge/beads/internal/testutil/integration"
)

// doltReadyLogLine is what dolt sql-server logs, at info level, once it is
// accepting connections.
const doltReadyLogLine = "Server ready. Accepting connections."

// logSignal is an io.Writer that closes seen the first time the written
// stream contains marker, even when the marker is split across writes.
type logSignal struct {
	marker []byte
	seen   chan struct{}

	mu   sync.Mutex
	tail []byte
	done bool
}

func newLogSignal(marker string) *logSignal {
	return &logSignal{marker: []byte(marker), seen: make(chan struct{})}
}

func (l *logSignal) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.done {
		return len(p), nil
	}
	l.tail = append(l.tail, p...)
	if bytes.Contains(l.tail, l.marker) {
		l.done = true
		close(l.seen)
		l.tail = nil
		return len(p), nil
	}
	if keep := len(l.marker) - 1; len(l.tail) > keep {
		l.tail = l.tail[len(l.tail)-keep:]
	}
	return len(p), nil
}

// TestUnixSocket_ConnectAndQuery starts a Dolt server with --socket,
// connects via the unix socket DSN, and executes SQL to verify end-to-end
// unix socket connectivity. Covers the full path from GH#2939.
func TestUnixSocket_ConnectAndQuery(t *testing.T) {
	doltBin := integration.RequireDolt(t)

	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	doltDir := filepath.Join(beadsDir, "dolt")
	if err := os.MkdirAll(doltDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Initialize dolt database.
	configureDoltTestIdentity(t, doltBin, tmpDir)
	initCmd := exec.Command(doltBin, "init")
	initCmd.Dir = doltDir
	initCmd.Env = doltTestEnv(tmpDir)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("dolt init: %v\n%s", err, out)
	}

	socketPath := filepath.Join(tmpDir, "dolt.sock")
	portListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate TCP port: %v", err)
	}
	tcpPort := portListener.Addr().(*net.TCPAddr).Port
	_ = portListener.Close()

	// Start dolt sql-server with --socket. Current Dolt requires a valid TCP
	// listener port even when the test connects exclusively over the socket.
	serverCmd := exec.Command(doltBin, "sql-server",
		"--socket", socketPath,
		"--loglevel=info", // the readiness line is logged at info level
		"-H", "127.0.0.1",
		"-P", strconv.Itoa(tcpPort),
	)
	serverCmd.Dir = doltDir
	serverCmd.Env = doltTestEnv(tmpDir)
	serverCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	logFile := filepath.Join(tmpDir, "dolt-server.log")
	lf, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create log file: %v", err)
	}
	// The server announces readiness in its log once its listeners (TCP and
	// socket) are up, so the test blocks on that line instead of polling.
	ready := newLogSignal(doltReadyLogLine)
	logOut := io.MultiWriter(lf, ready)
	serverCmd.Stdout = logOut
	serverCmd.Stderr = logOut

	if err := serverCmd.Start(); err != nil {
		lf.Close()
		t.Fatalf("dolt sql-server start: %v", err)
	}
	exited := make(chan struct{})
	go func() {
		_ = serverCmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = serverCmd.Process.Signal(syscall.SIGTERM)
		<-exited
		lf.Close()
	})

	select {
	case <-ready.seen:
	case <-exited:
		logs, _ := os.ReadFile(logFile)
		t.Fatalf("dolt sql-server exited before it was ready\nServer logs:\n%s", logs)
	case <-time.After(30 * time.Second):
		logs, _ := os.ReadFile(logFile)
		t.Fatalf("dolt sql-server never logged readiness\nServer logs:\n%s", logs)
	}

	// Connect via unix socket DSN.
	dsn := doltutil.ServerDSN{
		Socket:   socketPath,
		User:     "root",
		Database: "dolt",
	}.String()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	// Verify connectivity.
	if err := db.Ping(); err != nil {
		logs, _ := os.ReadFile(logFile)
		t.Fatalf("db.Ping via socket failed: %v\nServer logs:\n%s", err, logs)
	}

	// Create, insert, query — prove the socket carries real MySQL traffic.
	if _, err := db.Exec("CREATE TABLE socket_test (id INT PRIMARY KEY, val VARCHAR(100))"); err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	if _, err := db.Exec("INSERT INTO socket_test VALUES (1, 'socket-works')"); err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	var val string
	if err := db.QueryRow("SELECT val FROM socket_test WHERE id = 1").Scan(&val); err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if val != "socket-works" {
		t.Errorf("expected 'socket-works', got %q", val)
	}
}
