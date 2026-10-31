//go:build linux

package hooks

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// pidfdExitTimeout bounds the one kernel wait in TestRunSync_KillsDescendants.
// It is generous on purpose: the group kill has already been sent when RunSync
// returns, so a child still running at the end of it is a failed assertion,
// not one worth waiting on longer.
const pidfdExitTimeout = 10 * time.Second

// pidfdKernel is the calls the exit wait makes on the kernel, passed in as a
// value so a row drives them with fakes while awaitChildExit passes the host
// kernel. Nothing here is package state a test could swap.
type pidfdKernel struct {
	open  func(pid, flags int) (int, error)
	poll  func(fd int, timeout time.Duration) (bool, error)
	close func(fd int) error
}

// realPidfdKernel is the host kernel: pidfd_open, poll and close.
func realPidfdKernel() pidfdKernel {
	return pidfdKernel{
		open: unix.PidfdOpen,
		poll: func(fd int, timeout time.Duration) (bool, error) {
			return pollForReadiness(unix.Poll, time.Now, fd, timeout)
		},
		close: unix.Close,
	}
}

// awaitChildExit waits on pid's exit and reports the state that exit buys.
func awaitChildExit(pid int, timeout time.Duration, state func() (bool, error)) childExit {
	return awaitChildExitKernel(realPidfdKernel(), pid, timeout, state)
}

// awaitChildExitKernel is awaitChildExit with the kernel injected: it opens a
// pidfd for pid and makes one bounded wait on the child's exit, then reads the
// state that exit buys. A pid that is already gone — exited and reaped before
// the fd could be opened — is reported as exited at once. Opening right after
// RunSync returns cannot reach a reused pid: this process has not reaped the
// child, and the kernel does not release a pid for reuse until it is reaped,
// so its number still names that child (if it was reaped elsewhere the open
// fails with ESRCH). A kernel without pidfd_open (Linux before 5.3) is a skip
// reason, not a pass: the wait cannot be made, and a refusal must not read as
// a completed wait.
func awaitChildExitKernel(k pidfdKernel, pid int, timeout time.Duration, state func() (bool, error)) childExit {
	fd, err := k.open(pid, 0)
	if err != nil {
		if pidfdUnsupported(err) {
			return childExit{SkipReason: fmt.Sprintf(
				"pidfd_open is unavailable (Linux before 5.3); cannot wait for the child's exit: %v", err)}
		}
		if errors.Is(err, unix.ESRCH) {
			// Already exited and reaped: there is no child left to wait on.
			stopped, stateErr := waitForStopped(func() (bool, error) { return true, nil }, state)
			return childExit{Stopped: stopped, Err: stateErr}
		}
		return childExit{Err: fmt.Errorf("pidfd_open %d: %w", pid, err)}
	}
	defer func() { _ = k.close(fd) }()

	stopped, err := waitForStopped(func() (bool, error) { return k.poll(fd, timeout) }, state)
	return childExit{Stopped: stopped, Err: err}
}

// pidfdUnsupported reports whether err means the kernel has no pidfd_open
// (Linux before 5.3). That must skip the test with a message: the wait cannot
// be performed, and a refusal reported as a passed wait would assert nothing.
func pidfdUnsupported(err error) bool {
	return errors.Is(err, unix.ENOSYS)
}

// pollFunc is the poll syscall, passed in so a row can interrupt a wait
// without a signal and without a clock.
type pollFunc func(fds []unix.PollFd, timeoutMs int) (int, error)

// pollForReadiness reports whether fd became readable within timeout: true
// when it is readable (the process exited), false when the deadline expires
// with the fd still silent, and an error when poll itself fails. EINTR means a
// signal interrupted the wait, so the wait is resumed — but against one
// deadline recomputed from now, so a stream of interruptions cannot stretch
// the kernel wait past timeout. now is injected so no row needs a real clock.
func pollForReadiness(poll pollFunc, now func() time.Time, fd int, timeout time.Duration) (bool, error) {
	deadline := now().Add(timeout)
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	for {
		remaining := deadline.Sub(now())
		if remaining <= 0 {
			return false, nil
		}
		n, err := poll(fds, int(remaining.Milliseconds()))
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
	sawExit := func(int, time.Duration) (bool, error) { return true, nil }
	sawRunning := func(int, time.Duration) (bool, error) { return false, nil }

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
			poll:        sawExit,
			state:       exited,
			wantStopped: true,
			wantCloses:  1,
		},
		{
			name:       "still running at the timeout",
			open:       opened,
			poll:       sawRunning,
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

			res := awaitChildExitKernel(k, 42, time.Second, tt.state)

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

// TestPollForReadiness pins the one kernel wait's mapping — readable, timed
// out, interrupted and resumed, interrupted past the deadline, and failed —
// through an injected poll and clock, with no signal, no sleep and no real
// clock. The "interrupted until the deadline expires" row is the one that goes
// red when the deadline is removed: its poll fails after three interrupted
// calls, so an unbounded retry reports that error instead of timing out.
func TestPollForReadiness(t *testing.T) {
	t0 := time.Unix(1_000_000, 0)
	errPoll := errors.New("poll failed")
	errPastDeadline := errors.New("polled past the deadline")

	tests := []struct {
		name         string
		step         time.Duration // clock advance per poll call
		poll         func(calls int) (int, error)
		wantRead     bool
		wantErr      error
		wantCalls    int
		wantTimeouts []int
	}{
		{
			name:         "readable",
			poll:         func(int) (int, error) { return 1, nil },
			wantRead:     true,
			wantCalls:    1,
			wantTimeouts: []int{1000},
		},
		{
			name:         "timed out",
			poll:         func(int) (int, error) { return 0, nil },
			wantCalls:    1,
			wantTimeouts: []int{1000},
		},
		{
			name: "interrupted then readable",
			step: 400 * time.Millisecond,
			poll: func(calls int) (int, error) {
				if calls == 1 {
					return -1, unix.EINTR
				}
				return 1, nil
			},
			wantRead:     true,
			wantCalls:    2,
			wantTimeouts: []int{1000, 600},
		},
		{
			name: "interrupted until the deadline expires",
			step: time.Second,
			poll: func(calls int) (int, error) {
				if calls > 3 {
					return -1, errPastDeadline
				}
				return -1, unix.EINTR
			},
			wantCalls:    1,
			wantTimeouts: []int{1000},
		},
		{
			name:         "poll failed",
			poll:         func(int) (int, error) { return -1, errPoll },
			wantErr:      errPoll,
			wantCalls:    1,
			wantTimeouts: []int{1000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := t0
			calls := 0
			var timeouts []int
			var polled []unix.PollFd
			poll := func(fds []unix.PollFd, timeoutMs int) (int, error) {
				calls++
				polled = fds
				timeouts = append(timeouts, timeoutMs)
				now = now.Add(tt.step)
				return tt.poll(calls)
			}

			read, err := pollForReadiness(poll, func() time.Time { return now }, 7, time.Second)

			if calls != tt.wantCalls {
				t.Errorf("polled %d times, want %d", calls, tt.wantCalls)
			}
			if !slices.Equal(timeouts, tt.wantTimeouts) {
				t.Errorf("poll timeouts = %v, want %v", timeouts, tt.wantTimeouts)
			}
			if len(polled) != 1 || polled[0].Fd != 7 || polled[0].Events != unix.POLLIN {
				t.Errorf("polled %+v, want one descriptor 7 for POLLIN", polled)
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
