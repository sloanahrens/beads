//go:build linux

package hooks

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// pidfdExitTimeout bounds the one kernel wait in TestRunSync_KillsDescendants.
// It is generous on purpose: the group kill has already been sent when RunSync
// returns, so a child still running at the end of it is a failed assertion,
// not one worth waiting on longer.
const pidfdExitTimeout = 10 * time.Second

// pidfdOpen is the syscall, in a variable so a row can drive the wait through
// a pipe in an environment where pidfd_open is unavailable (a container's
// seccomp policy answers ENOSYS on a kernel that has the call).
var pidfdOpen = unix.PidfdOpen

// pidfdUnsupported reports whether err means the kernel has no pidfd_open
// (Linux before 5.3). That must skip the test with a message: the wait cannot
// be performed, and a refusal reported as a passed wait would assert nothing.
func pidfdUnsupported(err error) bool {
	return errors.Is(err, unix.ENOSYS)
}

// pidfdExitWait waits on pid's exit with the kernel instead of a spin. A pidfd
// becomes readable when the process terminates, so one poll on it is the whole
// wait; the kernel carries it, not a Gosched loop in this process. A pid that
// is already gone — exited and reaped before the fd could be opened — reports
// exited at once. The returned close releases the pidfd.
func pidfdExitWait(pid int, timeout time.Duration) (wait func() (bool, error), closePidfd func(), err error) {
	fd, err := pidfdOpen(pid, 0)
	if err != nil {
		if errors.Is(err, unix.ESRCH) {
			// The process does not exist: it has already exited and been reaped.
			return func() (bool, error) { return true, nil }, func() {}, nil
		}
		return nil, nil, fmt.Errorf("pidfd_open %d: %w", pid, err)
	}
	return func() (bool, error) {
		return pollPidfd(fd, timeout)
	}, func() { _ = unix.Close(fd) }, nil
}

// pollPidfd is the single kernel wait: one poll for pid's exit notification.
// It reports true when the fd is readable (the process exited), false when the
// timeout expires with the process still running, and an error when poll
// itself fails. EINTR means a signal interrupted the wait before the timeout,
// so the wait is resumed: an interruption is neither an exit nor a timeout.
func pollPidfd(fd int, timeout time.Duration) (bool, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := unix.Poll(fds, int(timeout.Milliseconds()))
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("poll pidfd %d: %w", fd, err)
		}
		return n > 0, nil
	}
}

// TestPidfdUnsupported_SkipsOlderKernels pins the skip condition: a kernel
// without pidfd_open is told apart from a real open failure, so the descendants
// test skips with a message instead of passing without having waited.
func TestPidfdUnsupported_SkipsOlderKernels(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"kernel without pidfd_open", fmt.Errorf("pidfd_open 42: %w", unix.ENOSYS), true},
		{"other open failure", fmt.Errorf("pidfd_open 42: %w", unix.EPERM), false},
		{"no failure", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pidfdUnsupported(tt.err); got != tt.want {
				t.Errorf("pidfdUnsupported(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestPidfdExitWait_ReportsTheExit pins the readable-means-exited mapping the
// descendants test relies on, through the opener seam so it runs without a
// pidfd: the fd becomes readable, so the wait reports the child's exit.
func TestPidfdExitWait_ReportsTheExit(t *testing.T) {
	readFd := readablePipe(t)
	pidfdOpener(t, readFd, nil)

	wait, closePidfd, err := pidfdExitWait(42, time.Second)
	if err != nil {
		t.Fatalf("pidfdExitWait error = %v, want nil", err)
	}
	defer closePidfd()

	exited, err := wait()
	if err != nil {
		t.Fatalf("wait error = %v, want nil", err)
	}
	if !exited {
		t.Fatal("a readable fd — the child's exit — was reported as still running")
	}
}

// TestPidfdExitWait_TimesOut pins the other side: an fd that never becomes
// readable (the child still running) is reported as a timeout, not as an exit.
func TestPidfdExitWait_TimesOut(t *testing.T) {
	readFd := openPipe(t)
	pidfdOpener(t, readFd, nil)

	wait, closePidfd, err := pidfdExitWait(42, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("pidfdExitWait error = %v, want nil", err)
	}
	defer closePidfd()

	exited, err := wait()
	if err != nil {
		t.Fatalf("wait error = %v, want nil", err)
	}
	if exited {
		t.Fatal("a wait that timed out was reported as the child's exit")
	}
}

// TestPidfdExitWait_ReportsAProcessAlreadyGone pins the race the descendants
// test can lose: the child exited and was reaped before its pidfd could be
// opened, which the kernel reports as ESRCH and which is an exit, not a
// failure to wait.
func TestPidfdExitWait_ReportsAProcessAlreadyGone(t *testing.T) {
	pidfdOpener(t, -1, unix.ESRCH)

	wait, closePidfd, err := pidfdExitWait(42, time.Second)
	if err != nil {
		t.Fatalf("pidfdExitWait error = %v, want nil", err)
	}
	defer closePidfd()

	exited, err := wait()
	if err != nil {
		t.Fatalf("wait error = %v, want nil", err)
	}
	if !exited {
		t.Fatal("a pid already gone — exited and reaped — was reported as still running")
	}
}

// TestPidfdExitWait_ReportsAnOpenFailure pins that a real open failure is
// returned rather than read as an exit or as an unsupported kernel.
func TestPidfdExitWait_ReportsAnOpenFailure(t *testing.T) {
	pidfdOpener(t, -1, unix.EACCES)

	wait, closePidfd, err := pidfdExitWait(42, time.Second)
	if !errors.Is(err, unix.EACCES) {
		t.Fatalf("pidfdExitWait error = %v, want EACCES", err)
	}
	if pidfdUnsupported(err) {
		t.Fatal("a permission failure was reported as an unsupported kernel")
	}
	if wait != nil || closePidfd != nil {
		t.Fatal("a failed open returned a wait or a close")
	}
}

// pidfdOpener points the opener seam at fakeFd for one test, reporting err
// (with fakeFd -1 for an error). It restores the real syscall when the test
// ends.
func pidfdOpener(t *testing.T, fakeFd int, err error) {
	t.Helper()
	previous := pidfdOpen
	pidfdOpen = func(int, int) (int, error) { return fakeFd, err }
	t.Cleanup(func() { pidfdOpen = previous })
}

// readablePipe returns a pipe whose read end is already readable — a byte
// written and the writer released — so a poll on it returns at once.
func readablePipe(t *testing.T) int {
	t.Helper()
	readFd, writeFd := pipeEnds(t)
	if _, err := unix.Write(writeFd, []byte{1}); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	return readFd
}

// openPipe returns a pipe's read end whose writer is still open and silent, so
// a poll on it can only time out.
func openPipe(t *testing.T) int {
	t.Helper()
	readFd, _ := pipeEnds(t)
	return readFd
}

// pipeEnds creates a pipe and releases both ends when the test ends. A row
// that hands the read end to the wait also closes it through closePidfd; that
// second close returns EBADF and touches nothing else, so the cleanup is safe
// whether or not the row got that far.
func pipeEnds(t *testing.T) (readFd, writeFd int) {
	t.Helper()
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = unix.Close(fds[0])
		_ = unix.Close(fds[1])
	})
	return fds[0], fds[1]
}
