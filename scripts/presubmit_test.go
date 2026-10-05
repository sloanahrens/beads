package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// `gt done` runs `make presubmit` on the rebased branch, so the target has to
// exist and delegate to scripts/presubmit.sh. The rest of this file pins what
// that script decides: which packages the branch changed, which are skipped
// because they cannot run without infrastructure, and that a branch touching
// no Go file still stops after lint and build.

// TestPresubmitMakefileTargetWiresTheScript keeps `make presubmit` pointing at
// the script gt done runs; a renamed or dropped target would make gt done fall
// back to nothing at all.
func TestPresubmitMakefileTargetWiresTheScript(t *testing.T) {
	makefile := readRepoFile(t, "../Makefile")
	recipe := makeRecipe(t, makefile, "presubmit")
	if !strings.Contains(recipe, "./scripts/presubmit.sh") {
		t.Errorf("make presubmit recipe does not run scripts/presubmit.sh:\n%s", recipe)
	}
}

// TestPresubmitListSelectsChangedPackages drives the selection against a
// throwaway git repo: only the packages holding a changed Go file, minus the
// ones whose tests need a container or a Dolt server, and minus the examples
// (separate Go modules, outside the root module's ./...).
func TestPresubmitListSelectsChangedPackages(t *testing.T) {
	repo := newPresubmitFixtureRepo(t, presubmitFixtureOriginMain)
	out, err := runPresubmit(t, repo, nil, "--list")
	if err != nil {
		t.Fatalf("presubmit --list: %v\n%s", err, out)
	}

	for _, want := range []string{
		"presubmit: base origin/main",
		"presubmit: run .",
		"presubmit: run ./alpha",
		"presubmit: run ./integpkg",
		"presubmit: skip ./doltpkg: its tests need a Docker container or a Dolt server; the Forgejo gate covers it",
		"presubmit: skip ./examples/demo: separate Go module",
		"presubmit: base origin/main; 3 package(s) to test",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("presubmit --list output lacks %q:\n%s", want, out)
		}
	}
	// beta/notes.md changed, but no Go file in beta/ did.
	if strings.Contains(out, "beta") {
		t.Errorf("presubmit --list selected beta from a non-Go change:\n%s", out)
	}
}

// TestPresubmitListFallsBackToMergeBaseWithMain covers the checkout without a
// remote-tracking ref: the base becomes the merge base with the local main
// branch, and the same packages are selected.
func TestPresubmitListFallsBackToMergeBaseWithMain(t *testing.T) {
	repo := newPresubmitFixtureRepo(t, presubmitFixtureMergeBase)
	out, err := runPresubmit(t, repo, nil, "--list")
	if err != nil {
		t.Fatalf("presubmit --list: %v\n%s", err, out)
	}

	if !strings.Contains(out, "presubmit: base ") || strings.Contains(out, "presubmit: base origin/main") {
		t.Errorf("presubmit --list did not fall back to the merge base with main:\n%s", out)
	}
	for _, want := range []string{"presubmit: run ./alpha", "presubmit: skip ./doltpkg", "3 package(s) to test"} {
		if !strings.Contains(out, want) {
			t.Errorf("presubmit --list output lacks %q:\n%s", want, out)
		}
	}
}

// TestPresubmitRunsLintAndBuildWithoutGoChanges pins the no-Go-change branch of
// the contract: lint and build run, no test command does, and the target still
// exits 0.
func TestPresubmitRunsLintAndBuildWithoutGoChanges(t *testing.T) {
	repo := newPresubmitFixtureRepo(t, presubmitFixtureDocsOnly)
	fakeBin, logPath := presubmitFakeTools(t)
	out, err := runPresubmit(t, repo, presubmitFakeEnv(fakeBin, logPath))
	if err != nil {
		t.Fatalf("presubmit with no Go change: %v\n%s", err, out)
	}

	calls := readPresubmitCalls(t, logPath)
	for _, want := range []string{"make ci-pr-lint", "go build -tags gms_pure_go ./..."} {
		if !strings.Contains(calls, want) {
			t.Errorf("presubmit did not run %q:\n%s", want, calls)
		}
	}
	if strings.Contains(calls, "go test") {
		t.Errorf("presubmit ran a test command with no Go file changed:\n%s", calls)
	}
	if !strings.Contains(out, "no testable changed Go package") {
		t.Errorf("presubmit output does not say no Go package changed:\n%s", out)
	}
}

// TestPresubmitTestsOnlyChangedPackages is the end-to-end counterpart: the test
// command names the changed packages and nothing else, and the skipped package
// is reported.
func TestPresubmitTestsOnlyChangedPackages(t *testing.T) {
	repo := newPresubmitFixtureRepo(t, presubmitFixtureOriginMain)
	fakeBin, logPath := presubmitFakeTools(t)
	out, err := runPresubmit(t, repo, presubmitFakeEnv(fakeBin, logPath))
	if err != nil {
		t.Fatalf("presubmit: %v\n%s", err, out)
	}

	calls := readPresubmitCalls(t, logPath)
	if !strings.Contains(calls, "go test -tags gms_pure_go . ./alpha ./integpkg") {
		t.Errorf("presubmit did not test exactly the changed packages:\n%s", calls)
	}
	if strings.Contains(calls, "doltpkg") || strings.Contains(calls, "examples/demo") {
		t.Errorf("presubmit tested a skipped package:\n%s", calls)
	}
	if !strings.Contains(out, "presubmit: skip ./doltpkg") {
		t.Errorf("presubmit output does not report the skipped package:\n%s", out)
	}
}

type presubmitFixtureMode int

const (
	// presubmitFixtureOriginMain is the normal shape: a change set on a
	// detached HEAD plus a refs/remotes/origin/main base.
	presubmitFixtureOriginMain presubmitFixtureMode = iota
	// presubmitFixtureMergeBase has no remote-tracking ref; only a local main
	// branch at the base commit.
	presubmitFixtureMergeBase
	// presubmitFixtureDocsOnly changes a non-Go file and nothing else.
	presubmitFixtureDocsOnly
)

// presubmitChangedFiles are the second-commit edits: two packages that can be
// tested, one that needs a Dolt container, one whose only container test is
// behind //go:build integration, an example in its own Go module, the module
// root, and a markdown file whose package has no Go change.
var presubmitChangedFiles = []string{
	"root.go",
	"alpha/alpha.go",
	"alpha/alpha_test.go",
	"beta/notes.md",
	"doltpkg/main.go",
	"integpkg/main.go",
	"examples/demo/demo.go",
}

// The container bootstrap and Dolt server call that make a package need
// infrastructure. Assembled from two pieces so this file does not itself look
// like such a package: it lives in scripts/, a directory presubmit scans like
// any other.
const (
	presubmitContainerBootstrap = "testutil.EnsureDolt" + "ContainerForTestMain()"
	presubmitDoltServerStart    = "doltserver." + "Start("
)

var presubmitBaseFiles = map[string]string{
	"go.mod":              "module example.com/fixture\n\ngo 1.21\n",
	"root.go":             "package fixture\n",
	"alpha/alpha.go":      "package alpha\n",
	"alpha/alpha_test.go": "package alpha\n\nimport \"testing\"\n\nfunc TestAlpha(t *testing.T) {}\n",
	"beta/beta.go":        "package beta\n",
	"beta/notes.md":       "# notes\n",
	"doltpkg/main.go":     "package doltpkg\n",
	"integpkg/main.go":    "package integpkg\n",

	"examples/demo/go.mod":  "module example.com/demo\n\ngo 1.21\n",
	"examples/demo/demo.go": "package demo\n",

	// A package-level Dolt container: no environment can run these tests
	// without one, so presubmit must skip the package rather than build it.
	"doltpkg/testmain_test.go": `package doltpkg

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := ` + presubmitContainerBootstrap + `; err != nil {
		os.Exit(1)
	}
	os.Exit(m.Run())
}
`,

	// Behind the integration tag, so the unit tier presubmit runs never builds
	// it — the package itself stays runnable.
	"integpkg/heavy_integration_test.go": `//go:build integration

package integpkg

import "testing"

func TestHeavy(t *testing.T) {
	_ = ` + presubmitDoltServerStart + `t.TempDir())
}
`,
}

// newPresubmitFixtureRepo builds a git repo with one committed change set and
// returns its path.
func newPresubmitFixtureRepo(t *testing.T, mode presubmitFixtureMode) string {
	t.Helper()
	repo := t.TempDir()

	for path, content := range presubmitBaseFiles {
		writePresubmitFile(t, filepath.Join(repo, filepath.FromSlash(path)), content)
	}
	runPresubmitGit(t, repo, "init", "--initial-branch=main")
	runPresubmitGit(t, repo, "add", "-A")
	runPresubmitGit(t, repo, "commit", "-m", "base")
	base := runPresubmitGit(t, repo, "rev-parse", "HEAD")

	// Detach so the change commit never moves the local main branch.
	runPresubmitGit(t, repo, "checkout", "--detach")

	changed := presubmitChangedFiles
	if mode == presubmitFixtureDocsOnly {
		changed = []string{"beta/notes.md"}
	}
	for _, path := range changed {
		full := filepath.Join(repo, filepath.FromSlash(path))
		writePresubmitFile(t, full, readPresubmitFile(t, full)+"// changed\n")
	}
	runPresubmitGit(t, repo, "add", "-A")
	runPresubmitGit(t, repo, "commit", "-m", "change")

	if mode != presubmitFixtureMergeBase {
		runPresubmitGit(t, repo, "update-ref", "refs/remotes/origin/main", base)
	}
	return repo
}

// runPresubmit runs scripts/presubmit.sh against the given repo. A nil extraEnv
// lets the real tools run; a fake PATH keeps the run to writing its call log.
func runPresubmit(t *testing.T, repo string, extraEnv []string, args ...string) (string, error) {
	t.Helper()
	script, err := filepath.Abs("presubmit.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = repo
	cmd.Env = append(append(os.Environ(), "PRESUBMIT_REPO_ROOT="+repo), extraEnv...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// presubmitFakeTools returns a PATH directory holding stub `go` and `make`
// binaries that append their argv to a log, so a test can assert what the
// script ran without compiling anything.
func presubmitFakeTools(t *testing.T) (binDir, logPath string) {
	t.Helper()
	binDir = t.TempDir()
	logPath = filepath.Join(t.TempDir(), "calls.log")
	for _, name := range []string{"go", "make"} {
		writePresubmitFile(t, filepath.Join(binDir, name), "#!/usr/bin/env bash\n"+
			"set -euo pipefail\n"+
			"printf '%s %s\\n' \""+name+"\" \"$*\" >>\"$PRESUBMIT_CALL_LOG\"\n")
		if err := os.Chmod(filepath.Join(binDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return binDir, logPath
}

func presubmitFakeEnv(binDir, logPath string) []string {
	return []string{
		"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"PRESUBMIT_CALL_LOG=" + logPath,
	}
}

func readPresubmitCalls(t *testing.T, logPath string) string {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	return string(data)
}

func runPresubmitGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=presubmit test",
		"GIT_AUTHOR_EMAIL=presubmit@example.com",
		"GIT_COMMITTER_NAME=presubmit test",
		"GIT_COMMITTER_EMAIL=presubmit@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writePresubmitFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readPresubmitFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
