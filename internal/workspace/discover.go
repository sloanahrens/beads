package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/beads/internal/git"
	"github.com/steveyegge/beads/internal/utils"
)

// FollowFunc follows one .beads directory's redirect. Resolve uses the strict
// FollowRedirect; internal/beads passes a lenient variant that warns and falls
// back to the source directory, preserving its string-returning API.
type FollowFunc func(beadsDir string) (target string, redirected bool, err error)

// Discover finds the effective .beads directory as if discovery started from
// startDir, without consulting BEADS_DIR or the process working directory.
// It returns the .beads directory it found (source) and the directory in use
// after following that directory's redirect (resolved). Both are "" when no
// workspace is found. A redirect error on a directory that would be returned
// is returned as err and ends the walk: a broken redirect never lets an
// ancestor workspace stand in for the one it names. Redirect errors on
// directories only probed for a fallback database are treated as "no
// database there". Callers that pass a follower which never errors (as
// internal/beads does, to keep its warn-and-fall-back contract) get the old
// behavior of skipping such a directory and walking on.
//
// Order: walk up from startDir, at each .beads following its redirect and
// accepting it when the resolved directory holds workspace files, except that
// a git worktree root (or jj secondary workspace root) whose .beads owns no
// database defers to the shared/primary workspace that does. Then the git
// worktree shared fallback (<git-common-dir>/../.beads), then the jj primary.
func Discover(startDir string, follow FollowFunc) (source, resolved string, err error) {
	if startDir == "" {
		return "", "", nil
	}
	info, statErr := os.Stat(startDir)
	if statErr != nil || !info.IsDir() {
		return "", "", nil
	}
	if follow == nil {
		follow = FollowRedirect
	}
	probe := func(dir string) string {
		target, _, ferr := follow(dir)
		if ferr != nil {
			return dir
		}
		return target
	}

	startDir = utils.CanonicalizePath(startDir)

	// Git facts are needed only when a candidate .beads has no local
	// database (a worktree root carrying tracked metadata) or the walk comes
	// up empty. Resolve them lazily (two git execs, only then): config loading runs
	// discovery on every bd invocation, and a normal workspace owns its
	// database, so the common path spawns no git at all.
	var (
		gitLoaded        bool
		repoRoot         string
		fallbackBeadsDir string
		fallbackHasDB    bool
	)
	loadGit := func() {
		if gitLoaded {
			return
		}
		gitLoaded = true
		if out, err := gitOutput(startDir, "rev-parse", "--show-toplevel"); err == nil {
			repoRoot = utils.CanonicalizePath(out)
		}
		if repoRoot != "" {
			fallbackBeadsDir = worktreeFallbackBeadsDirForRepo(repoRoot)
		}
		if fallbackBeadsDir != "" && isDir(fallbackBeadsDir) {
			fallbackHasDB = HasDatabase(probe(fallbackBeadsDir))
		}
	}

	jjSecondaryRoot := ""
	jjPrimarySource, jjPrimaryResolved := "", ""
	jjPrimaryHasDB := false
	if root, ok := git.JJSecondaryWorkspaceRootFrom(startDir); ok {
		jjSecondaryRoot = utils.CanonicalizePath(root)
		if primaryRoot, jErr := git.GetJJPrimaryWorkspaceRootFrom(startDir); jErr == nil && primaryRoot != "" {
			primaryBeadsDir := filepath.Join(primaryRoot, ".beads")
			if isDir(primaryBeadsDir) {
				r := probe(primaryBeadsDir)
				if HasProjectFiles(r) {
					jjPrimarySource, jjPrimaryResolved = primaryBeadsDir, r
					jjPrimaryHasDB = HasDatabase(r)
				}
			}
		}
	}

	for dir := startDir; dir != "/" && dir != "."; {
		beadsDir := filepath.Join(dir, ".beads")
		if isDir(beadsDir) {
			target, _, ferr := follow(beadsDir)
			if ferr != nil {
				return beadsDir, "", ferr
			}
			hasDB := HasDatabase(target)
			isWorktreeRoot := false
			if !hasDB {
				loadGit()
				isWorktreeRoot = repoRoot != "" && utils.PathsEqual(dir, repoRoot)
			}
			isJJSecondaryRoot := jjSecondaryRoot != "" && utils.PathsEqual(dir, jjSecondaryRoot)
			switch {
			case isWorktreeRoot && fallbackHasDB && !hasDB:
				// A worktree root can carry tracked .beads metadata without
				// owning the ignored database directory: prefer the shared
				// worktree database.
			case isJJSecondaryRoot && jjPrimaryHasDB && !hasDB:
				// Same for a jj secondary workspace: prefer the primary's DB.
			case HasProjectFiles(target):
				return beadsDir, target, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	loadGit()
	if fallbackBeadsDir != "" && isDir(fallbackBeadsDir) {
		target, _, ferr := follow(fallbackBeadsDir)
		if ferr != nil {
			return fallbackBeadsDir, "", ferr
		}
		if HasProjectFiles(target) {
			return fallbackBeadsDir, target, nil
		}
	}

	if jjPrimaryResolved != "" {
		return jjPrimarySource, jjPrimaryResolved, nil
	}
	return "", "", nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// CanonicalizeBeadsDir makes beadsDir absolute and symlink-free, and when it
// sits in a detached snapshot worktree (megarepo refs/commits/<sha>) prefers
// the .beads of a branch worktree at the same commit.
func CanonicalizeBeadsDir(beadsDir string) string { return canonicalizeBeadsDirPath(beadsDir) }

func canonicalizeBeadsDirPath(beadsDir string) string {
	canonical := utils.CanonicalizePath(beadsDir)
	if stable := preferStableBranchWorktreeBeadsDir(canonical); stable != "" {
		return stable
	}
	return canonical
}

type worktreeInfo struct {
	Path     string
	Head     string
	Branch   string
	Detached bool
	Bare     bool
}

func preferStableBranchWorktreeBeadsDir(beadsDir string) string {
	if filepath.Base(beadsDir) != ".beads" {
		return ""
	}

	repoRoot := filepath.Dir(beadsDir)
	if !isDetachedCommitWorktreePath(repoRoot) {
		return ""
	}

	branch, err := gitOutput(repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || branch != "HEAD" {
		return ""
	}

	head, err := gitOutput(repoRoot, "rev-parse", "HEAD")
	if err != nil || head == "" {
		return ""
	}

	worktrees, err := listWorktrees(repoRoot)
	if err != nil {
		return ""
	}

	var candidates []worktreeInfo
	for _, wt := range worktrees {
		if wt.Bare || wt.Detached || wt.Branch == "" {
			continue
		}
		if wt.Head != head || utils.PathsEqual(wt.Path, repoRoot) {
			continue
		}
		candidates = append(candidates, wt)
	}

	if len(candidates) == 0 {
		return ""
	}

	sort.Slice(candidates, func(i, j int) bool {
		iStable := !isDetachedCommitWorktreePath(candidates[i].Path)
		jStable := !isDetachedCommitWorktreePath(candidates[j].Path)
		if iStable != jStable {
			return iStable
		}
		return candidates[i].Path < candidates[j].Path
	})

	stableBeadsDir := filepath.Join(candidates[0].Path, ".beads")
	if isDir(stableBeadsDir) {
		return utils.CanonicalizePath(stableBeadsDir)
	}
	return ""
}

// isDetachedCommitWorktreePath checks if a path follows the megarepo convention
// of placing detached worktrees under refs/commits/<sha>.
func isDetachedCommitWorktreePath(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/refs/commits/")
}

func gitOutput(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // args are internal, not user-supplied
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func listWorktrees(repoRoot string) ([]worktreeInfo, error) {
	output, err := gitOutput(repoRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var worktrees []worktreeInfo
	var current *worktreeInfo

	for _, line := range strings.Split(output, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current != nil {
				worktrees = append(worktrees, *current)
			}
			current = &worktreeInfo{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		case current == nil:
			continue
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "detached":
			current.Detached = true
		case line == "bare":
			current.Bare = true
		}
	}

	if current != nil {
		worktrees = append(worktrees, *current)
	}
	return worktrees, nil
}

// WorktreeFallbackBeadsDir returns <main checkout>/.beads for a git worktree
// at repoPath (derived from git-common-dir), or "" when repoPath is not a
// linked worktree.
func WorktreeFallbackBeadsDir(repoPath string) string {
	return worktreeFallbackBeadsDirForRepo(repoPath)
}

func worktreeFallbackBeadsDirForRepo(repoPath string) string {
	// --show-toplevel is deliberately not folded into this call: it fails
	// in contexts --git-dir answers (a bare repository), which would lose
	// the fallback entirely.
	out, err := gitOutput(repoPath, "rev-parse", "--git-dir", "--git-common-dir")
	if err != nil {
		return ""
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		return ""
	}
	gitDir := gitPathForRepo(repoPath, strings.TrimSpace(lines[0]))
	commonDir := gitPathForRepo(repoPath, strings.TrimSpace(lines[1]))
	if gitDir == "" || commonDir == "" || utils.PathsEqual(gitDir, commonDir) {
		return ""
	}
	if filepath.Base(commonDir) == ".git" {
		return filepath.Join(filepath.Dir(commonDir), ".beads")
	}
	return filepath.Join(commonDir, ".beads")
}

func gitPathForRepo(repoPath, path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoPath, path)
	}
	return utils.CanonicalizePath(path)
}
