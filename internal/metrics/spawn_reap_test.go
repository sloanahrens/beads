package metrics

import (
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestStartDetachedReapsExitedChild is the GH#5900 regression: a child that
// exits while the parent stays alive must be waitpid'd, not left as a zombie.
// The helper re-execs this test binary and exits when the parent closes its
// stdin; startDetached must reap it so `ps` never reports STAT Z under this
// process.
func TestStartDetachedReapsExitedChild(t *testing.T) {
	if os.Getenv("BD_TEST_FLUSHER_HELPER") == "1" {
		// Stay alive until the parent has observed a live pid and closes our
		// stdin. An instant-exit helper can vanish from `ps` before the first
		// poll, which looks identical to a successful reap and would
		// false-pass the old cmd.Process.Release() path.
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestStartDetachedReapsExitedChild$") //nolint:gosec // test binary re-exec
	cmd.Env = append(os.Environ(), "BD_TEST_FLUSHER_HELPER=1")
	cmd.Stdout = nil
	cmd.Stderr = nil
	release, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	reaped := make(chan struct{})
	pid, err := startDetachedNotify(cmd, func() { close(reaped) })
	if err != nil {
		t.Fatalf("startDetached: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("startDetached pid = %d, want > 0", pid)
	}

	if runtime.GOOS != "windows" {
		// Zombie STAT is a Unix process-table property. The child blocks on
		// stdin, so it must still be running here: a missing or zombie entry
		// now is a failure, not a lucky race.
		st, alive := unixProcStat(pid)
		if !alive {
			t.Fatalf("child pid %d never appeared in ps (alive=%v stat=%q)", pid, alive, st)
		}
		if strings.Contains(st, "Z") {
			t.Fatalf("child pid %d is a zombie before exit (stat=%q)", pid, st)
		}
	}

	if err := release.Close(); err != nil {
		t.Fatalf("closing child stdin: %v", err)
	}
	select {
	case <-reaped:
	case <-time.After(3 * time.Second):
		st, alive := unixProcStat(pid)
		t.Fatalf("child pid %d not reaped after 3s (alive=%v stat=%q); startDetached did not reap", pid, alive, st)
	}

	// Windows has no ps; reaching here still exercised startDetached's Wait path.
	if runtime.GOOS == "windows" {
		return
	}
	if st, alive := unixProcStat(pid); alive {
		t.Fatalf("child pid %d still in ps after reap (stat=%q); startDetached did not reap", pid, st)
	}
}

// unixProcStat returns the `ps` STAT field for pid. alive is false when ps
// finds no such process (reaped and gone). On Windows, ps is unavailable so
// this reports not-alive after a short grace; the Wait goroutine is still
// the resource-release path under test.
func unixProcStat(pid int) (stat string, alive bool) {
	if runtime.GOOS == "windows" {
		return "", false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		return "", false
	}
	st := strings.TrimSpace(string(out))
	if st == "" || strings.HasPrefix(st, "STAT") {
		return "", false
	}
	return st, true
}
