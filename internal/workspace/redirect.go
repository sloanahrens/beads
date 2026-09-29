package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/beads/internal/utils"
)

// RedirectFileName is the file inside a .beads directory that points at the
// .beads directory actually in use (git worktrees and polecats share their
// rig's database this way).
const RedirectFileName = "redirect"

var (
	// ErrRedirectTarget marks a redirect whose target is missing, is not a
	// directory, or holds no beads workspace files.
	ErrRedirectTarget = errors.New("invalid redirect target")
	// ErrRedirectLoop marks a redirect that points back at its own
	// directory, directly or through the target's redirect.
	ErrRedirectLoop = errors.New("redirect loop")
)

// RedirectError describes a redirect that cannot be followed. It unwraps to
// ErrRedirectTarget or ErrRedirectLoop.
type RedirectError struct {
	File   string // the redirect file
	Target string // the resolved target it names
	// Missing is true when the target does not exist or is not a directory
	// (as opposed to existing without workspace files).
	Missing bool
	kind    error
	detail  string
}

func (e *RedirectError) Error() string {
	return fmt.Sprintf("%s: %s -> %s: %s", e.kind, e.File, e.Target, e.detail)
}

func (e *RedirectError) Unwrap() error { return e.kind }

// readRedirect reads beadsDir/redirect and returns its target resolved to an
// absolute, canonical path. present is false when there is no redirect file
// or it holds no path line.
//
// Format: the first line that is neither blank nor a '#' comment, trimmed.
// A relative path is resolved against the directory that CONTAINS the .beads
// directory (the project root), not against .beads itself. Gas Town writes
// polecat redirects as "../../../mayor/rig/.beads" from
// <rig>/polecats/<name>/<rig>/.beads, which only lands on the rig under this
// rule; it is also the rule documented under "Database Redirects".
func readRedirect(beadsDir string) (target string, present bool, err error) {
	data, err := os.ReadFile(filepath.Join(beadsDir, RedirectFileName)) //nolint:gosec // G304: reading the workspace redirect file is the purpose
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		// An unreadable redirect is not "no redirect": silently ignoring it
		// would bind the caller to whatever the ancestor walk finds next.
		return "", false, fmt.Errorf("reading %s: %w", filepath.Join(beadsDir, RedirectFileName), err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !filepath.IsAbs(line) {
			line = filepath.Join(filepath.Dir(beadsDir), line)
		}
		return canonicalizeBeadsDirPath(line), true, nil
	}
	return "", false, nil
}

// FollowRedirect follows beadsDir/redirect exactly one hop.
//
// With no redirect (or an empty one) it returns beadsDir unchanged and
// redirected=false. Otherwise it returns the canonical target, or a
// *RedirectError when:
//   - the target is missing or not a directory (ErrRedirectTarget, Missing),
//   - the target holds no workspace files, i.e. neither metadata.json,
//     config.yaml nor a database (ErrRedirectTarget; gastownhall/beads#4692),
//   - the target is beadsDir itself, or the target's own redirect points
//     back at beadsDir (ErrRedirectLoop).
//
// A target that redirects somewhere else is a chain; chains are not
// followed further, so the first hop's target is returned. Callers that
// want to warn about it can check the target for a redirect file.
func FollowRedirect(beadsDir string) (target string, redirected bool, err error) {
	target, present, err := readRedirect(beadsDir)
	if err != nil || !present {
		return beadsDir, false, err
	}
	file := filepath.Join(beadsDir, RedirectFileName)
	fail := func(kind error, missing bool, detail string) (string, bool, error) {
		return beadsDir, false, &RedirectError{File: file, Target: target, Missing: missing, kind: kind, detail: detail}
	}

	if utils.PathsEqual(target, canonicalizeBeadsDirPath(beadsDir)) {
		return fail(ErrRedirectLoop, false, "the redirect points at its own directory")
	}
	info, statErr := os.Stat(target)
	if statErr != nil || !info.IsDir() {
		return fail(ErrRedirectTarget, true, "target does not exist or is not a directory")
	}
	if !HasProjectFiles(target) {
		return fail(ErrRedirectTarget, false, "target has no database, metadata.json or config.yaml; fix or delete the redirect file")
	}
	if back, ok, _ := readRedirect(target); ok && utils.PathsEqual(back, canonicalizeBeadsDirPath(beadsDir)) {
		return fail(ErrRedirectLoop, false, "the target redirects back to this directory")
	}
	return target, true, nil
}

// HasChainedRedirect reports whether dir (a redirect target) carries its own
// redirect file, which FollowRedirect deliberately does not follow.
func HasChainedRedirect(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, RedirectFileName))
	return err == nil
}
