package beads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/git"
	"github.com/steveyegge/beads/internal/workspace"
)

// be-h0k: database discovery and workspace.Resolve must agree on a worktree
// whose .beads holds only a redirect to its rig, even with an unrelated
// ancestor workspace above both.
func TestFindBeadsDir_AgreesWithWorkspaceResolveThroughRedirect(t *testing.T) {
	town, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rigBeads := filepath.Join(town, "rig", "mayor", "rig", ".beads")
	worktree := filepath.Join(town, "rig", "polecats", "amber", "rig")
	write(filepath.Join(town, ".beads", "config.yaml"), "issue-prefix: hq\n")
	write(filepath.Join(town, ".beads", "metadata.json"), `{"backend":"dolt"}`)
	write(filepath.Join(rigBeads, "config.yaml"), "issue-prefix: zz\n")
	write(filepath.Join(rigBeads, "metadata.json"), `{"backend":"dolt"}`)
	if err := os.MkdirAll(filepath.Join(rigBeads, "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(worktree, ".beads", RedirectFileName), "../../../mayor/rig/.beads\n")

	t.Setenv("BEADS_DIR", "")
	t.Setenv("BEADS_DB", "")
	t.Chdir(worktree)
	git.ResetCaches()
	t.Cleanup(git.ResetCaches)

	ws, err := workspace.Resolve(worktree, os.Getenv)
	if err != nil {
		t.Fatalf("workspace.Resolve: %v", err)
	}
	if ws.BeadsDir != rigBeads {
		t.Fatalf("Resolve.BeadsDir = %q, want %q", ws.BeadsDir, rigBeads)
	}
	if got := FindBeadsDir(); got != ws.BeadsDir {
		t.Errorf("FindBeadsDir() = %q, want %q", got, ws.BeadsDir)
	}
	if got := FindBeadsDirFrom(worktree); got != ws.BeadsDir {
		t.Errorf("FindBeadsDirFrom() = %q, want %q", got, ws.BeadsDir)
	}
	if got := FindDatabasePath(); !strings.HasPrefix(got, rigBeads+string(filepath.Separator)) {
		t.Errorf("FindDatabasePath() = %q, want a path under %q", got, rigBeads)
	}

	// BEADS_DIR naming the worktree's .beads is interpreted the same way.
	t.Setenv("BEADS_DIR", filepath.Join(worktree, ".beads"))
	if got := FindBeadsDir(); got != rigBeads {
		t.Errorf("FindBeadsDir() with BEADS_DIR = %q, want %q", got, rigBeads)
	}
	if got := FindDatabasePath(); !strings.HasPrefix(got, rigBeads+string(filepath.Separator)) {
		t.Errorf("FindDatabasePath() with BEADS_DIR = %q, want a path under %q", got, rigBeads)
	}
}
