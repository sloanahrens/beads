package doltserver

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// serverCandidate is a running process that looked like a `dolt sql-server`
// from a coarse filter (cmdline substring match), along with enough
// identity data to judge whether it is leaked test debris.
type serverCandidate struct {
	pid int
	// cmdline is the process's command line, space-joined.
	cmdline string
	// cwd is the process's resolved working directory. Empty if unknown.
	cwd string
	// cwdDeleted is true when cwd names a directory that no longer exists
	// (e.g. Linux's /proc/<pid>/cwd symlink grew a " (deleted)" suffix
	// because something rm -rf'd the directory out from under the process).
	cwdDeleted bool
	// ownerPID is the PID recorded in cwd's TestOwnerPIDFileName marker, or
	// 0 when no such marker exists (always the case for a real shared
	// server — the marker is only ever written by test harnesses). A
	// nonzero value names the process that must stay alive for this server
	// to still be in legitimate use.
	ownerPID int
}

// readTestOwnerPID reads the test-owner marker (see TestOwnerPIDFileName)
// from a candidate's working directory, if present. Returns 0 when the
// marker is absent, unreadable, or does not contain a valid PID — all of
// which mean "no recorded owner" rather than an error.
func readTestOwnerPID(cwd string) int {
	if cwd == "" {
		return 0
	}
	data, err := os.ReadFile(filepath.Join(cwd, TestOwnerPIDFileName)) //nolint:gosec // G304: cwd is a candidate process's own resolved working directory (from ps/lsof or /proc), read-only, never user input
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// mergePIDs returns the union of a and b, preserving a's order and
// appending any of b's PIDs not already present.
func mergePIDs(a, b []int) []int {
	if len(b) == 0 {
		return a
	}
	seen := make(map[int]bool, len(a))
	for _, pid := range a {
		seen[pid] = true
	}
	out := a
	for _, pid := range b {
		if seen[pid] {
			continue
		}
		seen[pid] = true
		out = append(out, pid)
	}
	return out
}

// countOrphanCandidates reports how many candidates look like leaked test
// debris under the read-only rules used for visibility (bd doctor, gt dolt
// status): a deleted working directory, or a recorded test-owner PID (see
// TestOwnerPIDFileName) whose owner is confirmed dead. It never signals
// anything — see the platform-specific CountOrphanedTestServers wrappers.
func countOrphanCandidates(candidates []serverCandidate, isProcessAlive func(pid int) bool) int {
	pids := selectOrphanTestServerPIDs(candidates, nil)
	pids = mergePIDs(pids, selectDeadOwnerServerPIDs(candidates, isProcessAlive))
	return len(pids)
}

// selectDeadOwnerServerPIDs returns the PIDs of candidates that recorded an
// owner PID (see TestOwnerPIDFileName) whose owner process is confirmed
// dead. Unlike selectOrphanTestServerPIDs, this needs no suiteRoots and
// cannot mistake a live parallel suite's server for debris: it only ever
// acts on a candidate that itself named a specific PID as its owner, and
// only once isProcessAlive reports that exact PID gone. A production shared
// server never carries this marker, so it is categorically excluded.
func selectDeadOwnerServerPIDs(candidates []serverCandidate, isProcessAlive func(pid int) bool) []int {
	var pids []int
	for _, c := range candidates {
		if !isDoltServerCmdline(c.cmdline) {
			continue
		}
		if c.ownerPID <= 0 {
			continue
		}
		if isProcessAlive(c.ownerPID) {
			continue
		}
		pids = append(pids, c.pid)
	}
	return pids
}

// selectOrphanTestServerPIDs returns the PIDs of candidates that are safe to
// reap as leaked test debris. A candidate qualifies only when its cmdline
// names a dolt sql-server AND either:
//
//   - its working directory has been deleted (the temp dir it was serving
//     no longer exists — this is the signature of a SIGKILLed test run
//     whose t.TempDir() cleanup ran on top of a still-live server), or
//   - its working directory sits under one of suiteRoots.
//
// suiteRoots MUST be directories owned by the calling test suite alone
// (e.g. that suite's own testTempRoot) — never a shared/global temp dir
// such as os.TempDir(). A live (non-deleted-cwd) server is only reaped when
// its data dir is nested under a root the caller vouches for as its own;
// otherwise a parallel test run (scripts/test.sh -p N) would see every
// *other* suite's still-live server as debris, since virtually all suites'
// data dirs live somewhere under os.TempDir() too. Passing a global root
// here would turn this safety net into a cross-suite server killer.
//
// This is intentionally conservative in the "never kill production" sense:
// a real shared server's data directory is a persistent, non-temp path that
// still exists and is never one of a test suite's own scoped roots, so it
// matches neither condition and is left alone.
func selectOrphanTestServerPIDs(candidates []serverCandidate, suiteRoots []string) []int {
	var pids []int
	for _, c := range candidates {
		if !isDoltServerCmdline(c.cmdline) {
			continue
		}
		if c.cwdDeleted {
			pids = append(pids, c.pid)
			continue
		}
		if c.cwd == "" {
			continue
		}
		if underAnyRoot(c.cwd, suiteRoots) {
			pids = append(pids, c.pid)
		}
	}
	return pids
}

// isDoltServerCmdline reports whether cmdline looks like a dolt sql-server
// invocation. Mirrors the substring check in listDoltProcessPIDs (both
// "dolt" and "sql-server" must appear) rather than an exact match, since
// debug mode inserts flags between the binary name and the subcommand
// (e.g. `dolt --prof cpu --prof-path … sql-server …`).
func isDoltServerCmdline(cmdline string) bool {
	return strings.Contains(cmdline, "dolt") && strings.Contains(cmdline, "sql-server")
}

// underAnyRoot reports whether dir is equal to, or nested under, any of
// roots. Empty roots are ignored so callers can pass optional extras
// without filtering first.
func underAnyRoot(dir string, roots []string) bool {
	for _, root := range roots {
		if root == "" {
			continue
		}
		if isUnderDir(dir, root) {
			return true
		}
	}
	return false
}

// isUnderDir reports whether dir is root itself or a descendant of root.
// Both paths are compared as given (callers are expected to pass already
// resolved/absolute paths); this only does the string-prefix-with-boundary
// check, no filesystem access.
func isUnderDir(dir, root string) bool {
	root = strings.TrimRight(root, "/")
	if root == "" {
		return false
	}
	if dir == root {
		return true
	}
	return strings.HasPrefix(dir, root+"/")
}

// gatherPSCandidates parses the output of `ps -axo pid=,command=` and
// resolves the working directory of each dolt sql-server candidate. Darwin
// uses this path because it has no /proc filesystem.
//
// cwdForPID returns the resolved cwd, whether that cwd has been deleted, and
// whether it could be determined. Keeping the command execution outside this
// parser makes the safety-critical selection path deterministic to test.
func gatherPSCandidates(psOutput []byte, cwdForPID func(int) (string, bool, bool)) []serverCandidate {
	var candidates []serverCandidate
	for _, line := range strings.Split(string(psOutput), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		pidText, cmdline, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil || pid <= 0 {
			continue
		}
		cmdline = strings.TrimSpace(cmdline)
		if !isDoltServerCmdline(cmdline) {
			continue
		}

		cwd, deleted, ok := cwdForPID(pid)
		if !ok {
			continue
		}
		candidates = append(candidates, serverCandidate{
			pid:        pid,
			cmdline:    cmdline,
			cwd:        cwd,
			cwdDeleted: deleted,
		})
	}
	return candidates
}
