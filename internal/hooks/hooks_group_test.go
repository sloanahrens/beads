//go:build unix

package hooks

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

// TestRunHookCleanExitWithLingeringPipeHolderSucceeds pins the success path:
// a hook that exits 0 while a backgrounded child still holds its stdout and
// stderr makes cmd.Wait return ErrWaitDelay after hookWaitDelay, and RunSync
// must report the hook's own success, not that bookkeeping error. The delay is
// shortened so the row stays fast, and the child is killed at cleanup.
func TestRunHookCleanExitWithLingeringPipeHolderSucceeds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook")
	}
	old := hookWaitDelay
	hookWaitDelay = 100 * time.Millisecond
	t.Cleanup(func() { hookWaitDelay = old })

	tmpDir := t.TempDir()
	hookPath := filepath.Join(tmpDir, HookOnCreate)
	pidFile := filepath.Join(tmpDir, "child.pid")
	// The child inherits the hook's stdout and stderr and outlives the hook,
	// which exits 0 at once.
	hookScript := `#!/bin/sh
sleep 30 &
echo $! > ` + pidFile + `
exit 0`
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatalf("write hook: %v", err)
	}
	t.Cleanup(func() {
		if b, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})

	runner := &Runner{hooksDir: tmpDir, timeout: 10 * time.Second}
	if err := runner.RunSync(EventCreate, &types.Issue{ID: "bd-test", Title: "Test"}); err != nil {
		t.Fatalf("RunSync = %v, want nil for a hook that exited 0", err)
	}
}

// TestRunHookKillsDescendantsWhenGroupSignalRefused pins the fallback a polecat
// container needs: its seccomp filter refuses every negative-pid signal with
// EPERM, even for a process group the hook created. The group's members must
// then be signalled one pid at a time, or the backgrounded descendant outlives
// the hook and RunSync never returns. The signal is injected, so the row does
// not depend on the host refusing the group call.
func TestRunHookKillsDescendantsWhenGroupSignalRefused(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the descendant check needs Linux /proc")
	}

	tmpDir := t.TempDir()
	hookPath := filepath.Join(tmpDir, HookOnCreate)
	pidFile := filepath.Join(tmpDir, "child.pid")

	// Hook starts a background sleep, writes its pid, and waits for it. The
	// hook shell, its subshell and the sleep are all in the hook's process
	// group, so the group signal would have taken all three.
	hookScript := `#!/bin/sh
(sleep 60 & echo $! > ` + pidFile + ` ; wait)`
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatalf("Failed to create hook file: %v", err)
	}

	var perMember []int
	runner := &Runner{
		hooksDir: tmpDir,
		timeout:  500 * time.Millisecond,
		signalKill: func(pid int, sig syscall.Signal) error {
			if pid < 0 {
				return syscall.EPERM // the container refuses the group signal
			}
			perMember = append(perMember, pid)
			return syscall.Kill(pid, sig)
		},
	}

	err := runner.RunSync(EventCreate, &types.Issue{ID: "bd-test", Title: "Test"})
	if err == nil {
		t.Fatal("RunSync returned nil for a hook that outlived its timeout")
	}

	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("Failed to read pid file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("Invalid pid in pid file: %v", err)
	}
	if !slices.Contains(perMember, pid) {
		t.Fatalf("backgrounded descendant %d was not signalled one pid at a time; signalled: %v", pid, perMember)
	}
	// RunSync returns once the hook's stdout pipe closes, which the killed
	// descendant does as it exits. Wait on that exit with the kernel, as
	// TestRunSync_KillsDescendants does, rather than spinning on the scheduler.
	res := awaitChildExit(pid, pidfdExitTimeout,
		func() (bool, error) { return childStopped(pid) })
	if res.SkipReason != "" {
		t.Skip(res.SkipReason)
	}
	if res.Err != nil {
		t.Fatalf("wait for descendant %d to exit: %v", pid, res.Err)
	}
	if !res.Stopped {
		t.Fatalf("backgrounded descendant %d is still running after RunSync returned", pid)
	}
}

// TestRunHookWaitsForTheHookWhenTheKillFails pins the failure path: a fallback
// that cannot signal the group's members must not return while the hook's tree
// is still running. The hook backgrounds a subshell that holds the hook's
// stdout pipe and writes a marker after 500ms, and the member list is made
// unreadable so the sweep cannot signal it. RunSync may return only after that
// subshell exits — the marker exists by then — and it reports the enumeration
// error rather than the timeout.
func TestRunHookWaitsForTheHookWhenTheKillFails(t *testing.T) {
	skipWithoutShellHooks(t)

	dir := t.TempDir()
	marker := filepath.Join(dir, "descendant-exited")
	writeHook(t, dir, HookOnCreate,
		"#!/bin/sh\n(sleep 0.5; echo exited > \""+marker+"\") &\nwait\n")

	errEnum := errors.New("unreadable process table")
	runner := &Runner{
		hooksDir: dir,
		timeout:  200 * time.Millisecond,
		signalKill: func(pid int, sig syscall.Signal) error {
			if pid < 0 {
				return syscall.EPERM
			}
			return syscall.Kill(pid, sig)
		},
		groupMembers: func(int) ([]int, error) { return nil, errEnum },
	}

	err := runner.RunSync(EventCreate, &types.Issue{ID: "bd-test", Title: "Test"})
	if !errors.Is(err, errEnum) {
		t.Fatalf("RunSync error = %v, want the enumeration error wrapped", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("RunSync returned while the hook's descendant was still running: %v", statErr)
	}
}

// TestRunHookGroupSignalSucceeds pins the unchanged path: when the group signal
// reports success, no per-member signal is sent.
func TestRunHookGroupSignalSucceeds(t *testing.T) {
	skipWithoutShellHooks(t)

	dir := t.TempDir()
	writeHook(t, dir, HookOnCreate, "#!/bin/sh\nexec sleep 0.5\n")

	var perMember []int
	runner := &Runner{
		hooksDir: dir,
		timeout:  200 * time.Millisecond,
		signalKill: func(pid int, sig syscall.Signal) error {
			if pid < 0 {
				// Report success without depending on the host accepting a
				// negative pid: the hook is one process, so stopping it by pid
				// is the same kill the group call would make.
				return syscall.Kill(-pid, sig)
			}
			perMember = append(perMember, pid)
			return nil
		},
		groupMembers: func(int) ([]int, error) {
			return []int{42}, nil // a group a correct path never asks about
		},
	}

	if err := runner.RunSync(EventCreate, &types.Issue{ID: "bd-test", Title: "Test"}); err == nil {
		t.Fatal("RunSync returned nil for a hook that outlived its timeout")
	}
	if len(perMember) != 0 {
		t.Fatalf("a successful group signal still sent per-member signals: %v", perMember)
	}
}

// TestRunHookGroupSignalOtherErrorReturned pins that only EPERM takes the
// fallback: any other refusal is wrapped and returned exactly as before.
func TestRunHookGroupSignalOtherErrorReturned(t *testing.T) {
	skipWithoutShellHooks(t)

	dir := t.TempDir()
	writeHook(t, dir, HookOnCreate, "#!/bin/sh\nexec sleep 0.5\n")

	var perMember []int
	runner := &Runner{
		hooksDir: dir,
		timeout:  200 * time.Millisecond,
		signalKill: func(pid int, sig syscall.Signal) error {
			if pid < 0 {
				// Leave the hook alive: the error aborts the wait, and the
				// short-lived sleep exits on its own.
				return syscall.EACCES
			}
			perMember = append(perMember, pid)
			return nil
		},
		groupMembers: func(int) ([]int, error) {
			return []int{42}, nil // a group a correct path never asks about
		},
	}

	err := runner.RunSync(EventCreate, &types.Issue{ID: "bd-test", Title: "Test"})
	if !errors.Is(err, syscall.EACCES) {
		t.Fatalf("RunSync error = %v, want syscall.EACCES wrapped", err)
	}
	if !strings.Contains(err.Error(), "kill process group") {
		t.Errorf("RunSync error = %q, want the kill process group wrap", err)
	}
	if len(perMember) != 0 {
		t.Errorf("a non-EPERM group error still sent per-member signals: %v", perMember)
	}
}

// TestRunHookGroupSignalRefusedEnumerationError pins the refusal when the group
// signal is refused and the member list cannot be read: the hook itself is
// still signalled with the plain pid the kernel accepts, no pid is taken from
// an unknown list, and the enumeration error is returned wrapped, not
// swallowed.
func TestRunHookGroupSignalRefusedEnumerationError(t *testing.T) {
	skipWithoutShellHooks(t)

	dir := t.TempDir()
	writeHook(t, dir, HookOnCreate, "#!/bin/sh\nexec sleep 0.5\n")

	errEnum := errors.New("unreadable process table")
	var signaled []int
	var rootPid int
	runner := &Runner{
		hooksDir: dir,
		timeout:  200 * time.Millisecond,
		signalKill: func(pid int, sig syscall.Signal) error {
			if pid < 0 {
				return syscall.EPERM
			}
			signaled = append(signaled, pid)
			return syscall.Kill(pid, sig)
		},
		groupMembers: func(pgid int) ([]int, error) {
			rootPid = pgid
			return nil, errEnum
		},
	}

	err := runner.RunSync(EventCreate, &types.Issue{ID: "bd-test", Title: "Test"})
	if !errors.Is(err, errEnum) {
		t.Fatalf("RunSync error = %v, want the enumeration error wrapped", err)
	}
	if !strings.Contains(err.Error(), "kill process group members") {
		t.Errorf("RunSync error = %q, want the kill process group members wrap", err)
	}
	if !slices.Equal(signaled, []int{rootPid}) {
		t.Errorf("signaled %v, want only the hook's own pid %d", signaled, rootPid)
	}
}

// TestKillGroupMembers pins the member sweep's edges: every member is
// signalled, a member that already exited (ESRCH) is not an error, a member
// forked after the sweep that listed it is caught by the next sweep, one
// refused member does not stop the rest, a failed enumeration is returned, and
// a group that keeps producing members is reported rather than swept forever.
func TestKillGroupMembers(t *testing.T) {
	errEnum := errors.New("unreadable process table")
	errSignal := errors.New("signal refused")

	tests := []struct {
		name string
		// members is the table every sweep sees; lists, when set, is the table
		// the successive sweeps see instead, its last entry repeating.
		members []int
		lists   [][]int
		listErr error
		signal  func(pid int) error
		want    []int
		wantErr string
	}{
		{
			name:    "every member signalled",
			members: []int{11, 12},
			signal:  func(int) error { return nil },
			want:    []int{11, 12},
		},
		{
			name:    "a member that already exited",
			members: []int{11, 12},
			signal: func(pid int) error {
				if pid == 11 {
					return syscall.ESRCH
				}
				return nil
			},
			want: []int{11, 12},
		},
		{
			name:   "a member forked after the sweep that listed it",
			lists:  [][]int{{11}, {11, 12}, {11, 12}},
			signal: func(int) error { return nil },
			want:   []int{11, 12},
		},
		{
			name:    "a refused member does not stop the rest",
			members: []int{11, 12},
			signal: func(pid int) error {
				if pid == 11 {
					return errSignal
				}
				return nil
			},
			want:    []int{11, 12},
			wantErr: "kill member 11",
		},
		{
			name:    "members keep appearing",
			lists:   [][]int{{1}, {2}, {3}, {4}},
			signal:  func(int) error { return nil },
			want:    []int{1, 2, 3},
			wantErr: "still has members after 3 sweeps",
		},
		{
			name:    "enumeration failed",
			listErr: errEnum,
			signal:  func(int) error { return nil },
			wantErr: errEnum.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var signalled []int
			sweeps := 0
			runner := &Runner{
				groupMembers: func(int) ([]int, error) {
					if tt.listErr != nil {
						return nil, tt.listErr
					}
					if tt.lists == nil {
						return tt.members, nil
					}
					i := min(sweeps, len(tt.lists)-1)
					sweeps++
					return tt.lists[i], nil
				},
			}
			err := runner.killGroupMembers(4242, func(pid int, sig syscall.Signal) error {
				signalled = append(signalled, pid)
				return tt.signal(pid)
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("killGroupMembers error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("killGroupMembers returned an error: %v", err)
			}
			if !slices.Equal(signalled, tt.want) {
				t.Errorf("signalled %v, want %v", signalled, tt.want)
			}
		})
	}
}

// TestKillGroupMembersSurvivesRacingStatRead drives the real process-table walk
// through the sweep: a pid whose stat read fails with ESRCH was reaped while
// the table was being read, so it is no longer a member. The sweep must still
// signal the members that remain and must not report the vanished pid as an
// error — aborting the enumeration there is what leaves live members behind.
func TestKillGroupMembersSurvivesRacingStatRead(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"300/stat": "300 (hook) S 1 100 100 0",
		"301/stat": "301 (sleep) S 300 100 100 0",
		"302/stat": "302 (sleep) S 300 100 100 0",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	readFile := func(path string) ([]byte, error) {
		if filepath.Base(filepath.Dir(path)) == "301" {
			return nil, &fs.PathError{Op: "read", Path: path, Err: syscall.ESRCH}
		}
		return os.ReadFile(path)
	}

	var signalled []int
	runner := &Runner{
		groupMembers: func(pgid int) ([]int, error) {
			return procGroupMembers(root, pgid, readFile)
		},
	}
	err := runner.killGroupMembers(100, func(pid int, sig syscall.Signal) error {
		signalled = append(signalled, pid)
		return nil
	})
	if err != nil {
		t.Fatalf("killGroupMembers returned an error for a pid reaped mid-read: %v", err)
	}
	if !slices.Equal(signalled, []int{300, 302}) {
		t.Errorf("signalled %v, want the live members [300 302]", signalled)
	}
}

// TestProcGroupMembers pins the /proc table walk against a temporary tree:
// members are selected by process group, the comm field may hold spaces and
// parentheses, a pid that exits mid-read is skipped whether the kernel reports
// it missing (ENOENT) or reaped (ESRCH), and an unreadable or malformed table
// is an error rather than an empty member list.
func TestProcGroupMembers(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		dirs    []string
		readErr map[string]error
		want    []int
		wantErr string
	}{
		{
			name: "members of the group",
			files: map[string]string{
				"100/stat": "100 (hook) S 1 100 100 0 -1 0",
				"101/stat": "101 (sleep) S 100 100 100 0 -1 0",
				"102/stat": "102 (other) S 1 999 999 0 -1 0",
				"uptime":   "123.45 67.89",
			},
			want: []int{100, 101},
		},
		{
			name: "comm with spaces and parentheses",
			files: map[string]string{
				"200/stat": "200 (weird ) name) S 1 100 100 0",
			},
			want: []int{200},
		},
		{
			name: "pid exits while the table is read",
			files: map[string]string{
				"300/stat": "300 (hook) S 1 100 100 0",
			},
			dirs: []string{"301"},
			want: []int{300},
		},
		{
			name: "pid reaped while the table is read",
			files: map[string]string{
				"310/stat": "310 (hook) S 1 100 100 0",
				"311/stat": "311 (sleep) S 310 100 100 0",
			},
			readErr: map[string]error{"311": syscall.ESRCH},
			want:    []int{310},
		},
		{
			name:    "stat without a comm terminator",
			files:   map[string]string{"400/stat": "400 hook S 1 100 100"},
			wantErr: "no comm terminator",
		},
		{
			name:    "stat with too few fields",
			files:   map[string]string{"500/stat": "500 (hook) S 1"},
			wantErr: "want at least 3",
		},
		{
			name:    "stat with a non-numeric group",
			files:   map[string]string{"600/stat": "600 (hook) S 1 nope 100"},
			wantErr: "process group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}
			for name, body := range tt.files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatalf("mkdir %s: %v", filepath.Dir(name), err)
				}
				if err := os.WriteFile(path, []byte(body), 0644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			readFile := func(path string) ([]byte, error) {
				if rerr, ok := tt.readErr[filepath.Base(filepath.Dir(path))]; ok {
					return nil, &fs.PathError{Op: "read", Path: path, Err: rerr}
				}
				return os.ReadFile(path)
			}
			got, err := procGroupMembers(root, 100, readFile)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("procGroupMembers error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("procGroupMembers returned an error: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("procGroupMembers = %v, want %v", got, tt.want)
			}
		})
	}

	if _, err := procGroupMembers(filepath.Join(t.TempDir(), "absent"), 100, os.ReadFile); err == nil {
		t.Fatal("procGroupMembers returned no error for an unreadable process table")
	}
}

// TestParsePSGroups pins the ps table parser: members are selected by group,
// blank lines are skipped, and a line that is not two integers is an error.
func TestParsePSGroups(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		pgid    int
		want    []int
		wantErr string
	}{
		{name: "members", out: "  100   100\n  101   100\n  102   999\n", pgid: 100, want: []int{100, 101}},
		{name: "blank lines", out: "\n  200   200 \n\n", pgid: 200, want: []int{200}},
		{name: "a line with one field", out: "300\n", pgid: 300, wantErr: "want pid and pgid"},
		{name: "non-numeric pid", out: "x 300\n", pgid: 300, wantErr: "ps pid"},
		{name: "non-numeric pgid", out: "300 x\n", pgid: 300, wantErr: "ps pgid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePSGroups([]byte(tt.out), tt.pgid)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parsePSGroups error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePSGroups returned an error: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("parsePSGroups = %v, want %v", got, tt.want)
			}
		})
	}
}
