package dolt

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ansiEscapeSeq matches ANSI SGR color codes. `dolt log` emits these even
// when stdout is not a terminal (be-aru), so output must be scrubbed before
// the commit hash can be parsed reliably.
var ansiEscapeSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")

// parseDoltLogHash extracts the commit hash from the first line of `dolt log`
// output ("commit <hash>", optionally ANSI-colored, and — observed on dolt
// 2.3.2 — sometimes followed by the commit message on the same line once
// color codes are stripped, since the color reset butts directly against the
// message with no separating space in the raw output).
func parseDoltLogHash(out []byte) (string, error) {
	firstLine := strings.SplitN(string(out), "\n", 2)[0]
	firstLine = strings.TrimSpace(ansiEscapeSeq.ReplaceAllString(firstLine, ""))
	const prefix = "commit "
	if !strings.HasPrefix(firstLine, prefix) {
		return "", fmt.Errorf("unexpected `dolt log` output: %q", firstLine)
	}
	rest := strings.TrimSpace(strings.TrimPrefix(firstLine, prefix))
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", fmt.Errorf("unexpected `dolt log` output: %q", firstLine)
	}
	return fields[0], nil
}

// localBranchHash reads the current commit hash of branch in the on-disk
// Dolt directory dir via `dolt log`, without going through any sql-server
// connection. It is the local-filesystem half of verifying that a CLI-routing
// target directory really is the store a connected sql-server has open,
// rather than an unrelated or stale directory that merely contains a `.dolt`
// folder (be-aru).
func localBranchHash(ctx context.Context, dir, branch string) (string, error) {
	logCtx, cancel := withFSCKTimeout(ctx)
	defer cancel()
	cmd := exec.CommandContext(logCtx, "dolt", "log", "-n", "1", branch) // #nosec G204 -- fixed command; branch is the store's own configured ref
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("dolt log -n 1 %s failed in %s: %s: %w", branch, dir, strings.TrimSpace(string(out)), err)
	}
	return parseDoltLogHash(out)
}

// withFSCKTimeout bounds a local, read-only dolt CLI call against a directory
// that is expected to already be on disk (no network transfer): the same
// budget prePushFSCK uses for its own local-only `dolt fsck` call.
func withFSCKTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, fsckTimeoutDuration())
}

// staleLocalCLIDirError reports whether a CLI-routing directory should be
// refused because it does not match the connected server's branch head, and
// if so builds the explicit, both-sides-named error the be-aru deliverable
// requires. Pure and side-effect free so it can be unit tested without any
// dolt process or sql-server.
//
// A CLI-routed push/pull shells `dolt` out to run directly against dir,
// operating on whatever chunk store lives there on disk — NOT through the
// sql-server connection. In the healthy case dir IS the exact physical
// directory the connected sql-server has open, so serverHash and localHash
// are the same store's same branch and must be identical, not merely
// related by ancestry. A mismatch means dir is a different (often stale,
// leftover-from-before-server-mode) Dolt repository that happens to also
// satisfy "has a .dolt folder" — pushing/pulling through it would silently
// operate on the wrong history (be-aru: a July embedded copy overwrote a
// remote's September history this way).
func staleLocalCLIDirError(cliDir, database, branch, serverHash, localHash string) error {
	if serverHash != "" && localHash != "" && serverHash == localHash {
		return nil
	}
	return fmt.Errorf(
		"refusing to use local directory %s for a CLI-routed Dolt transfer: "+
			"it does not match the connected server (database %q, branch %q).\n"+
			"  server branch head:  %s\n"+
			"  local directory head: %s\n"+
			"This looks like a stale or unrelated local Dolt directory left over from before "+
			"this workspace used server mode. Before moving it: confirm it is NOT the data "+
			"directory the running sql-server currently has open — moving that out from under "+
			"a live server can corrupt or lose its state. Once confirmed safe, move it aside "+
			"before retrying, e.g.:\n"+
			"  mv %s %s.STALE-do-not-use",
		cliDir, database, branch, displayLogHash(serverHash), displayLogHash(localHash), cliDir, cliDir)
}

// displayLogHash renders a possibly-empty hash for the staleLocalCLIDirError
// message so "no branch/no commit" reads as an explicit state, not a blank.
func displayLogHash(hash string) string {
	if hash == "" {
		return "(none)"
	}
	return hash
}

// verifyCLIDirIsServerStore is the I/O half of the be-aru guard: it reads the
// connected server's branch head over SQL and the candidate CLI directory's
// branch head via a local `dolt log`, then delegates the comparison to
// staleLocalCLIDirError. Called at the top of doltCLIPush/doltCLIPull so
// every CLI-routing caller (git-protocol, credential, cloud-auth, and
// local-remote routing) is covered from one choke point.
//
// The server-head and local-head reads are not atomic: a legitimate commit
// can land on the server between them, making a genuinely-synced directory
// look momentarily stale (om-editorial review of be-wisp-u88). On a mismatch
// this re-reads the server head once and accepts if the local hash matches
// either reading, rather than refusing (and telling the operator to move
// data aside) on what may be nothing but a race.
func (s *DoltStore) verifyCLIDirIsServerStore(ctx context.Context, cliDir string) error {
	serverHash, err := s.branchHash(ctx, s.branch)
	if err != nil {
		return fmt.Errorf("reading server branch %q hash to verify CLI directory %s: %w", s.branch, cliDir, err)
	}
	localHash, err := localBranchHash(ctx, cliDir, s.branch)
	if err != nil {
		return fmt.Errorf("reading local directory %s branch %q hash to verify it matches the connected server: %w", cliDir, s.branch, err)
	}
	if staleLocalCLIDirError(cliDir, s.database, s.branch, serverHash, localHash) == nil {
		return nil
	}

	serverHash2, err := s.branchHash(ctx, s.branch)
	if err != nil {
		return fmt.Errorf("reading server branch %q hash to verify CLI directory %s: %w", s.branch, cliDir, err)
	}
	if serverHash2 == localHash {
		return nil
	}
	return staleLocalCLIDirError(cliDir, s.database, s.branch, serverHash2, localHash)
}
