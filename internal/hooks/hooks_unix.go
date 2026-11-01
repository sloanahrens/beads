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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/steveyegge/beads/internal/types"
)

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
		if cmd.Process != nil {
			if err := r.killTimedOutHook(cmd.Process.Pid); err != nil {
				return err
			}
		}
		// Wait for the hook and its descendants to exit after the kill. cmd.Wait
		// returns only once the stdout/stderr pipes close, and a descendant that
		// inherited them keeps this wait open until it is dead too.
		<-done
		addHookOutputEvents(span, &stdout, &stderr)
		return ctx.Err()
	case err := <-done:
		addHookOutputEvents(span, &stdout, &stderr)
		if err != nil {
			return err
		}
		return nil
	}
}

// killTimedOutHook terminates the hook whose pid is pid and its descendants.
// It signals the hook's process group first, which takes the whole tree in one
// call. Some kernels refuse that call with EPERM even for a group the caller
// created: a polecat container's seccomp filter refuses every negative-pid
// signal. On EPERM the group's members are signalled one pid at a time from the
// process table; any other error is returned as before.
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
	if err := r.killGroupMembers(pid, signal); err != nil {
		return fmt.Errorf("kill process group members: %w", err)
	}
	return nil
}

// killGroupMembers sends SIGKILL to every live member of process group pgid,
// one pid at a time. signal takes a plain pid here and a negative group pid in
// the group call. A member that exits before it is signalled (ESRCH) is done,
// not an error. The member list comes from groupMembers when a test injects
// it, and from the process table otherwise.
func (r *Runner) killGroupMembers(pgid int, signal func(pid int, sig syscall.Signal) error) error {
	list := processGroupMembers
	if r.groupMembers != nil {
		list = r.groupMembers
	}
	members, err := list(pgid)
	if err != nil {
		return err
	}
	for _, pid := range members {
		if err := signal(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("kill member %d: %w", pid, err)
		}
	}
	return nil
}

// processGroupMembers lists the live pids in process group pgid. Linux reads
// the process table from /proc; the other Unixes have no procfs and ask ps,
// the same source the darwin sweep uses.
func processGroupMembers(pgid int) ([]int, error) {
	if runtime.GOOS == "linux" {
		return procGroupMembers("/proc", pgid)
	}
	return psGroupMembers(pgid)
}

// procGroupMembers lists the pids under procRoot whose process group is pgid.
// procRoot is a parameter so the parsing runs against a temporary tree in
// tests; production passes /proc.
func procGroupMembers(procRoot string, pgid int) ([]int, error) {
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
		stat, err := os.ReadFile(filepath.Join(procRoot, entry.Name(), "stat"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // exited while the table was being read
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
