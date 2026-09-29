// Package workspace is the one place bd decides which .beads directory a
// command runs against, and derives every path that hangs off it: the
// project config.yaml, the metadata.json that names the database, and the
// workspace gate file.
//
// Before this package, the config walk (internal/config), database discovery
// (internal/beads) and BEADS_DIR handling were separate code with different
// rules. The config walk ignored .beads/redirect, so a git worktree whose
// .beads only redirects to its rig read an unrelated ancestor's config.yaml
// while writing the rig database (be-h0k, deep review B3-07).
//
// This package sits below internal/config and internal/beads (it imports
// neither), so both can use it.
package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/beads/internal/workspacegate"
)

// ErrNoWorkspace is returned by Resolve when neither BEADS_DIR nor discovery
// yields a .beads directory.
var ErrNoWorkspace = errors.New("no beads workspace found")

// EnvBeadsDir is the environment variable that selects a workspace
// explicitly. FromEnv is the only function that interprets it.
const EnvBeadsDir = "BEADS_DIR"

// Env looks up an environment variable; production callers pass os.Getenv.
type Env func(key string) string

// Workspace is a resolved beads workspace.
type Workspace struct {
	// BeadsDir is the .beads directory in use, after one redirect hop.
	BeadsDir string
	// SourceDir is the .beads directory selected before its redirect was
	// followed (BEADS_DIR's value, or the directory discovery found).
	SourceDir string
	// Redirected is true when SourceDir's redirect was followed.
	Redirected bool
	// FromEnv is true when BEADS_DIR selected the workspace.
	FromEnv bool
	// ConfigPath is BeadsDir/config.yaml. It may not exist.
	ConfigPath string
	// MetadataPath is BeadsDir/metadata.json, which names the database
	// (backend, server database, data directory). It may not exist.
	MetadataPath string
	// GatePath is the workspace gate file guarding BeadsDir
	// (workspacegate.ForWorkspace). Empty if the gate cannot be derived,
	// e.g. BeadsDir's parent does not exist yet.
	GatePath string
}

// FromEnv interprets BEADS_DIR: trimmed, canonicalized, redirect followed one
// hop. set is false when BEADS_DIR is unset or blank. On a redirect error it
// returns the canonical BEADS_DIR itself as beadsDir alongside the error, so
// lenient callers can keep the historical warn-and-use-source behavior.
func FromEnv(env Env) (beadsDir string, set bool, err error) {
	ws, set, err := fromEnv(env)
	return ws.BeadsDir, set, err
}

func fromEnv(env Env) (Workspace, bool, error) {
	if env == nil {
		return Workspace{}, false, nil
	}
	raw := strings.TrimSpace(env(EnvBeadsDir))
	if raw == "" {
		return Workspace{}, false, nil
	}
	source := canonicalizeBeadsDirPath(raw)
	target, redirected, err := FollowRedirect(source)
	if err != nil {
		return newWorkspace(source, source, false, true), true, fmt.Errorf("%s=%s: %w", EnvBeadsDir, raw, err)
	}
	return newWorkspace(source, target, redirected, true), true, nil
}

// Resolve selects the workspace for a command started in cwd.
//
// Precedence, applied here and nowhere else:
//  1. BEADS_DIR, when set: that directory (redirect followed), whether or not
//     it holds a workspace yet, so `bd init` can target it and config never
//     leaks in from cwd's ancestors.
//  2. Discover(cwd): walk up from cwd following each .beads/redirect, then
//     git worktree and jj fallbacks.
//
// Redirects follow exactly one hop; a loop, a missing target, or a target
// without workspace files is an error naming the redirect file.
func Resolve(cwd string, env Env) (Workspace, error) {
	if ws, set, err := fromEnv(env); set {
		if err != nil {
			return Workspace{}, err
		}
		return ws, nil
	}
	source, resolved, err := Discover(cwd, FollowRedirect)
	if err != nil {
		return Workspace{}, err
	}
	if resolved == "" {
		return Workspace{}, fmt.Errorf("%w from %s", ErrNoWorkspace, cwd)
	}
	return newWorkspace(source, resolved, source != resolved, false), nil
}

func newWorkspace(source, beadsDir string, redirected, fromEnv bool) Workspace {
	ws := Workspace{
		BeadsDir:     beadsDir,
		SourceDir:    source,
		Redirected:   redirected,
		FromEnv:      fromEnv,
		ConfigPath:   filepath.Join(beadsDir, "config.yaml"),
		MetadataPath: filepath.Join(beadsDir, "metadata.json"),
	}
	if g, err := workspacegate.ForWorkspace(beadsDir); err == nil {
		ws.GatePath = g.Path()
	}
	return ws
}
