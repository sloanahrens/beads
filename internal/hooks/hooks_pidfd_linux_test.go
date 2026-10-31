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

// pidfdKernel is the three calls the exit wait makes, passed in as a value so
// a row drives them with fakes while the descendants test passes the host
// kernel. Nothing here is package state a test could swap.
type pidfdKernel struct {
	open  func(pid, flags int) (int, error)
	poll  func(fd int, timeout time.Duration) (bool, error)
	close func(fd int) error
}

// realPidfdKernel is the host kernel's pidfd_open, poll and close.
func realPidfdKernel() pidfdKernel {
	return pidfdKernel{open: unix.PidfdOpen, poll: pollPidfd, close: unix.Close}
}

// pidfdUnsupported reports whether err means the kernel has no pidfd_open
// (Linux before 5.3). That must skip the test with a message: the wait cannot
// be performed, and a refusal reported as a passed wait would assert nothing.
func pidfdUnsupported(err error) bool {
	return errors.Is(err, unix.ENOSYS)
}

// waitForChildExit opens a pidfd for pid and returns the wait on the child's
// exit. A pid that is already gone — exited and reaped before the fd could be
// opened — is reported by the wait as exited at once. closePidfd releases the
// descriptor. An open failure is returned, with ENOSYS left for the caller to
// recognise as an unsupported kernel.
func waitForChildExit(k pidfdKernel, pid int, timeout time.Duration) (wait func() (bool, error), closePidfd func(), err error) {
	fd, err := k.open(pid, 0)
	if err != nil {
		if errors.Is(err, unix.ESRCH) {
			// The process does not exist: it has already exited and been reaped.
			return func() (bool, error) { return true, nil }, func() {}, nil
		}
		return nil, nil, fmt.Errorf("pidfd_open %d: %w", pid, err)
	}
	return func() (bool, error) {
		return k.poll(fd, timeout)
	}, func() { _ = k.close(fd) }, nil
}

// pollPidfd is the single kernel wait: one poll for the child's exit
// notification.
func pollPidfd(fd int, timeout time.Duration) (bool, error) {
	return pollForReadiness(unix.Poll, fd, int(timeout.Milliseconds()))
}

// pollFunc is the poll syscall, passed in so a row can interrupt a wait
// without a signal and without a clock.
type pollFunc func(fds []unix.PollFd, timeoutMs int) (int, error)

// pollForReadiness reports whether fd became readable within timeoutMs: true
// when it is readable (the process exited), false when the timeout expires
// with the fd still silent, and an error when poll itself fails. EINTR means a
// signal interrupted the wait before the timeout, so the wait is resumed: an
// interruption is neither readability nor a timeout.
func pollForReadiness(poll pollFunc, fd int, timeoutMs int) (bool, error) {
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		n, err := poll(fds, timeoutMs)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("poll pidfd %d: %w", fd, err)
		}
		return n > 0, nil
	}
}

// TestAwaitChildExit pins every way the descendants test reads the wait it
// opened: the child exited, it was still running at the timeout, the wait
// failed, the kernel has no pidfd_open (the skip), the child was already
// reaped, and the open failed. Every call is injected, so no row uses a clock
// or a sleep.
func TestAwaitChildExit(t *testing.T) {
	errPoll := errors.New("poll failed")
	opened := func(int, int) (int, error) { return 7, nil }
	exited := func() (bool, error) { return true, nil }
	running := func() (bool, error) { return false, nil }

	tests := []struct {
		name        string
		open        func(pid, flags int) (int, error)
		poll        func(fd int, timeout time.Duration) (bool, error)
		state       func() (bool, error)
		wantStopped bool
		wantSkip    bool
		wantErr     error
		wantCloses  int
	}{
		{
			name:        "child exited",
			open:        opened,
			poll:        func(int, time.Duration) (bool, error) { return true, nil },
			state:       exited,
			wantStopped: true,
			wantCloses:  1,
		},
		{
			name:       "still running at the timeout",
			open:       opened,
			poll:       func(int, time.Duration) (bool, error) { return false, nil },
			state:      running,
			wantCloses: 1,
		},
		{
			name:       "wait failed",
			open:       opened,
			poll:       func(int, time.Duration) (bool, error) { return false, errPoll },
			state:      exited,
			wantErr:    errPoll,
			wantCloses: 1,
		},
		{
			name:       "state still reads running",
			open:       opened,
			poll:       func(int, time.Duration) (bool, error) { return true, nil },
			state:      running,
			wantCloses: 1,
		},
		{
			name:       "state unreadable",
			open:       opened,
			poll:       func(int, time.Duration) (bool, error) { return true, nil },
			state:      func() (bool, error) { return false, errPoll },
			wantErr:    errPoll,
			wantCloses: 1,
		},
		{
			name:     "kernel without pidfd_open skips",
			open:     func(int, int) (int, error) { return -1, unix.ENOSYS },
			wantSkip: true,
		},
		{
			name:        "child already reaped",
			open:        func(int, int) (int, error) { return -1, unix.ESRCH },
			state:       exited,
			wantStopped: true,
		},
		{
			name:    "open failed",
			open:    func(int, int) (int, error) { return -1, unix.EACCES },
			wantErr: unix.EACCES,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closed := 0
			k := pidfdKernel{
				open:  tt.open,
				poll:  tt.poll,
				close: func(int) error { closed++; return nil },
			}

			res := awaitChildExit(k, 42, time.Second, tt.state)

			if tt.wantSkip {
				if res.SkipReason == "" {
					t.Fatal("an unsupported kernel was not reported as a skip")
				}
				if res.Err != nil || res.Stopped {
					t.Errorf("skip result = %+v, want only a skip reason", res)
				}
				return
			}
			if res.SkipReason != "" {
				t.Fatalf("skip reason = %q, want none", res.SkipReason)
			}
			if tt.wantErr != nil {
				if !errors.Is(res.Err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", res.Err, tt.wantErr)
				}
			} else if res.Err != nil {
				t.Fatalf("error = %v, want nil", res.Err)
			}
			if res.Stopped != tt.wantStopped {
				t.Errorf("stopped = %v, want %v", res.Stopped, tt.wantStopped)
			}
			if closed != tt.wantCloses {
				t.Errorf("closed the pidfd %d times, want %d", closed, tt.wantCloses)
			}
		})
	}
}

// TestPollForReadiness pins the one kernel wait's mapping: readable, timed out,
// interrupted and resumed, and failed — through an injected poll, with no
// signal, no clock and no sleep.
func TestPollForReadiness(t *testing.T) {
	errPoll := errors.New("poll failed")

	tests := []struct {
		name      string
		poll      func(calls int) (int, error)
		wantRead  bool
		wantErr   error
		wantCalls int
	}{
		{
			name:      "readable",
			poll:      func(int) (int, error) { return 1, nil },
			wantRead:  true,
			wantCalls: 1,
		},
		{
			name:      "timed out",
			poll:      func(int) (int, error) { return 0, nil },
			wantRead:  false,
			wantCalls: 1,
		},
		{
			name: "interrupted then readable",
			poll: func(calls int) (int, error) {
				if calls == 1 {
					return -1, unix.EINTR
				}
				return 1, nil
			},
			wantRead:  true,
			wantCalls: 2,
		},
		{
			name:      "poll failed",
			poll:      func(int) (int, error) { return -1, errPoll },
			wantRead:  false,
			wantErr:   errPoll,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			var polled []unix.PollFd
			polledTimeout := -1
			poll := func(fds []unix.PollFd, timeoutMs int) (int, error) {
				calls++
				polled = fds
				polledTimeout = timeoutMs
				return tt.poll(calls)
			}

			read, err := pollForReadiness(poll, 7, 1000)

			if calls != tt.wantCalls {
				t.Errorf("polled %d times, want %d", calls, tt.wantCalls)
			}
			if len(polled) != 1 || polled[0].Fd != 7 || polled[0].Events != unix.POLLIN {
				t.Errorf("polled %+v, want one descriptor 7 for POLLIN", polled)
			}
			if polledTimeout != 1000 {
				t.Errorf("poll timeout = %d, want 1000", polledTimeout)
			}
			if read != tt.wantRead {
				t.Errorf("readable = %v, want %v", read, tt.wantRead)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
		})
	}
}

// TestPidfdUnsupported pins the skip condition the descendants test relies on:
// a kernel without pidfd_open is told apart from every other open failure, so
// the test skips with a message instead of passing without having waited.
func TestPidfdUnsupported(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"kernel without pidfd_open", fmt.Errorf("pidfd_open 42: %w", unix.ENOSYS), true},
		{"permission denied", fmt.Errorf("pidfd_open 42: %w", unix.EACCES), false},
		{"no process", fmt.Errorf("pidfd_open 42: %w", unix.ESRCH), false},
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

// TestPollPidfd_ReportsAReadableDescriptor drives pollPidfd's real binding:
// a pipe whose read end is already readable is reported readable at once, with
// no wait. Each end is closed exactly once, by this test.
func TestPollPidfd_ReportsAReadableDescriptor(t *testing.T) {
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	t.Cleanup(func() { _ = unix.Close(fds[0]) })
	t.Cleanup(func() { _ = unix.Close(fds[1]) })

	if _, err := unix.Write(fds[1], []byte{1}); err != nil {
		t.Fatalf("write pipe: %v", err)
	}

	read, err := pollPidfd(fds[0], time.Second)
	if err != nil {
		t.Fatalf("pollPidfd error = %v, want nil", err)
	}
	if !read {
		t.Fatal("a readable descriptor was reported as still silent")
	}
}
