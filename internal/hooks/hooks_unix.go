//go:build unix

package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/steveyegge/beads/internal/types"
)

// hookWaitDelay bounds how long a timed-out hook's pipe wait may run after the
// kill. Wait returns only once every holder of the hook's stdout/stderr pipes
// exits, and a descendant that survived the kill holds one; without a bound a
// fallback that could not signal every member would block the caller past its
// own timeout. It is cleanup slop, not a second timeout.
var hookWaitDelay = 2 * time.Second

// runHook executes the hook and enforces a timeout, killing the process group
// on expiration to ensure descendant processes are terminated.
func (r *Runner) runHook(hookPath, event string, issue *types.Issue) (retErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Hooks are fire-and-forget so they have no parent span; we create a root span
	// to track execution time and errors for observability.
	tracer := otel.Tracer("github.com/steveyegge/beads/hooks")
	ctx, span := tracer.Start(ctx, "hook.exec",
		trace.WithAttributes(
			attribute.String("hook.event", event),
			attribute.String("hook.path", hookPath),
			attribute.String("bd.issue_id", issue.ID),
		),
	)
	defer func() {
		if retErr != nil {
			span.RecordError(retErr)
			span.SetStatus(codes.Error, retErr.Error())
		}
		span.End()
	}()

	// Prepare JSON data for stdin
	issueJSON, err := json.Marshal(issue)
	if err != nil {
		return err
	}

	// Create command: hook_script <issue_id> <event_type>
	// #nosec G204 -- hookPath is from controlled .beads/hooks directory
	cmd := exec.CommandContext(ctx, hookPath, issue.ID, event)
	// WaitDelay has to be set before Start, so it covers every run, not only a
	// timed-out one. It only matters when the hook has exited or its timeout
	// fired while a holder of the stdout/stderr pipes is still alive (a
	// descendant, or a hook that ignored the kill); returning after that
	// cleanup slop is better than blocking the caller behind a stray holder.
	cmd.WaitDelay = hookWaitDelay
	cmd.Stdin = bytes.NewReader(issueJSON)

	// Capture output for debugging (but don't block on it)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Start the hook so we can manage its process group and kill children on timeout.
	//
	// Rationale: scripts may spawn child processes (backgrounded or otherwise).
	// If we only kill the immediate process, descendants may survive and keep
	// the test (or caller) blocked — this was exposed by TestRunSync_Timeout and
	// validated by TestRunSync_KillsDescendants. Creating a process group (Setpgid)
	// and sending a negative PID to syscall.Kill ensures the entire group
	// (parent + children) are killed reliably on timeout. A kernel that refuses
	// the group signal falls back to killing the group's members one by one —
	// see killTimedOutHook.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		var killErr error
		if cmd.Process != nil {
			killErr = r.killTimedOutHook(cmd.Process.Pid)
		}
		// Wait for the hook and its descendants to exit after the kill, even
		// when the kill reported a failure: returning while the hook is still
		// running hands the caller a live process, the outcome the timeout
		// exists to prevent. cmd.Wait returns only once the stdout/stderr
		// pipes close, and a descendant that inherited them keeps this wait
		// open until it is dead too; WaitDelay bounds that to cleanup slop
		// when a descendant outlived the kill.
		<-done
		addHookOutputEvents(span, &stdout, &stderr)
		if killErr != nil {
			return killErr
		}
		return ctx.Err()
	case err := <-done:
		addHookOutputEvents(span, &stdout, &stderr)
		// A hook that exited 0 while a descendant still held its stdout or
		// stderr pipe makes Wait return ErrWaitDelay after hookWaitDelay. The
		// hook itself succeeded, so that is not a failure of the run.
		if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
			return err
		}
		return nil
	}
}

// killTimedOutHook terminates the hook whose pid is pid and its descendants.
// It signals the hook's process group first, which takes the whole tree in one
// call. Some kernels refuse that call with EPERM even for a group the caller
// created: a polecat container's seccomp filter refuses every negative-pid
// signal. On EPERM the hook itself is signaled with a plain pid, which the
// kernel accepts, and then the group's members are swept one pid at a time
// from the process table. Any other group error is returned as before.
func (r *Runner) killTimedOutHook(pid int) error {
	signal := r.signalKill
	if signal == nil {
		signal = syscall.Kill
	}
	err := signal(-pid, syscall.SIGKILL)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if !errors.Is(err, syscall.EPERM) {
		return fmt.Errorf("kill process group: %w", err)
	}
	var errs []error
	// Signal the hook itself first. A plain pid is not refused, so the hook
	// dies even when the member sweep below cannot run, and the tree's root
	// cannot fork again while that sweep reads the table.
	if sigErr := signal(pid, syscall.SIGKILL); sigErr != nil && !errors.Is(sigErr, syscall.ESRCH) {
		errs = append(errs, fmt.Errorf("kill hook %d: %w", pid, sigErr))
	}
	if memberErr := r.killGroupMembers(pid, signal); memberErr != nil {
		errs = append(errs, fmt.Errorf("kill process group members: %w", memberErr))
	}
	return errors.Join(errs...)
}

// maxMemberSweeps bounds how many times the fallback re-reads the process
// table. One sweep lists the group as the table shows it and signals every
// member; a second catches a descendant that forked while the first sweep was
// being signaled. The loop normally ends sooner, on the first sweep that finds
// no pid it has not already signaled.
const maxMemberSweeps = 3

// killGroupMembers sends SIGKILL to every live member of process group pgid,
// one pid at a time, re-reading the table until a sweep finds no member it has
// not signaled. A member may fork after the sweep that listed it, so the
// re-read is what keeps that child from outliving the hook; the hook itself is
// signaled before this sweep runs, so the tree's root cannot fork behind it.
// signal takes a plain pid here and a negative group pid in the group call. A
// member that exits before it is signaled (ESRCH) is done, not an error, and
// one refused member does not stop the rest of the sweep from being signaled:
// every failure is collected rather than returned at the first one. The member
// list comes from groupMembers when a test injects it, and from the process
// table otherwise. A listed pid that is not positive is refused and reported
// unsignalled: a negative pid is a process group, which is the call this
// fallback exists because the kernel refused, so it is never made from table
// data as if it were a member.
//
// Every signal in this fallback, the hook's own plain-pid kill included, names
// a pid that was read from the process table. A pid listed there and then
// reused by an unrelated process before the signal lands receives it. That
// window is an accepted limit of any pid-based sweep and this fallback does not
// narrow it: no kernel call signals "this pid, if it is still in group pgid"
// atomically, and re-reading the group id before each signal would only shrink
// the window while widening this change.
func (r *Runner) killGroupMembers(pgid int, signal func(pid int, sig syscall.Signal) error) error {
	list := processGroupMembers
	if r.groupMembers != nil {
		list = r.groupMembers
	}
	signaled := make(map[int]bool)
	var errs []error
	for sweep := 0; sweep < maxMemberSweeps; sweep++ {
		members, err := list(pgid)
		if err != nil {
			errs = append(errs, err)
			return errors.Join(errs...)
		}
		fresh := 0
		for _, pid := range members {
			if signaled[pid] {
				continue
			}
			signaled[pid] = true
			if pid <= 0 {
				errs = append(errs, fmt.Errorf("member list holds non-positive pid %d", pid))
				continue
			}
			fresh++
			if sigErr := signal(pid, syscall.SIGKILL); sigErr != nil && !errors.Is(sigErr, syscall.ESRCH) {
				errs = append(errs, fmt.Errorf("kill member %d: %w", pid, sigErr))
			}
		}
		if fresh == 0 {
			return errors.Join(errs...)
		}
	}
	errs = append(errs, fmt.Errorf("process group %d still has members after %d sweeps", pgid, maxMemberSweeps))
	return errors.Join(errs...)
}

// processGroupMembers lists the live pids in process group pgid. Linux reads
// the process table from /proc; the other Unixes have no procfs and ask ps,
// the same source the darwin sweep uses.
func processGroupMembers(pgid int) ([]int, error) {
	if runtime.GOOS == "linux" {
		return procGroupMembers("/proc", pgid, os.ReadFile)
	}
	return psGroupMembers(pgid)
}

// procGroupMembers lists the pids under procRoot whose process group is pgid.
// procRoot and readFile are parameters so the walk runs against a temporary
// tree with injected read failures in tests; production passes /proc and
// os.ReadFile.
func procGroupMembers(procRoot string, pgid int, readFile func(string) ([]byte, error)) ([]int, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", procRoot, err)
	}
	var members []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // not a process directory
		}
		stat, err := readFile(filepath.Join(procRoot, entry.Name(), "stat"))
		if err != nil {
			// A process that exits while the table is being read cannot be
			// signaled. The kernel reports its missing stat as ENOENT, or as
			// ESRCH when the pid was reaped just before the read. Such a pid is
			// no longer a member, and it must not abort the enumeration: the
			// rest of the group still has to be swept and killed. Any other
			// read failure is an error — a table that could not be read must
			// not pass as absent.
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
				continue
			}
			return nil, fmt.Errorf("read %s stat: %w", entry.Name(), err)
		}
		group, err := processGroupFromStat(stat)
		if err != nil {
			return nil, fmt.Errorf("parse %s stat: %w", entry.Name(), err)
		}
		if group == pgid {
			members = append(members, pid)
		}
	}
	return members, nil
}

// processGroupFromStat reads the process group id from /proc/<pid>/stat. The
// comm field may contain spaces and parentheses, so the fields after the last
// ')' are state, ppid, pgrp, ... and pgrp is the third of them.
func processGroupFromStat(stat []byte) (int, error) {
	closeParen := bytes.LastIndexByte(stat, ')')
	if closeParen < 0 {
		return 0, errors.New("stat has no comm terminator")
	}
	fields := strings.Fields(string(stat[closeParen+1:]))
	if len(fields) < 3 {
		return 0, fmt.Errorf("stat has %d fields after comm, want at least 3", len(fields))
	}
	group, err := strconv.Atoi(fields[2])
	if err != nil {
		return 0, fmt.Errorf("stat process group %q: %w", fields[2], err)
	}
	return group, nil
}

// psGroupMembers lists the pids ps reports in process group pgid. macOS has no
// procfs, so the process table comes from ps.
func psGroupMembers(pgid int) ([]int, error) {
	out, err := exec.Command("ps", "-axo", "pid=,pgid=").Output()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	return parsePSGroups(out, pgid)
}

// parsePSGroups reads pid/pgid pairs from `ps -o pid=,pgid=` output and returns
// the pids in process group pgid. A line that is not two integers is an error:
// a process the table could not be read for must not pass as absent, since the
// caller is killing exactly the members this table lists.
func parsePSGroups(out []byte, pgid int) ([]int, error) {
	var members []int
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("ps line %q: want pid and pgid", line)
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("ps pid %q: %w", fields[0], err)
		}
		group, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("ps pgid %q: %w", fields[1], err)
		}
		if group == pgid {
			members = append(members, pid)
		}
	}
	return members, nil
}
