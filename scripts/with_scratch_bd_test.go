package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// `make check-docs` needs a runnable bd, and where that binary is built decides
// whether the checkout keeps an executable bd at its root. A root bd is
// gitignored (/bd), so it lingers invisibly; it shadows the installed binary
// for anything that resolves ./bd (scripts/generate-cli-docs.sh prefers it);
// and in a Gas Town worktree it trips gt doctor's rig-bd-binary check, which
// reports every executable named bd anywhere under the worktree and leaves the
// operator chmod-ing it. with-scratch-bd.sh is what keeps the docs gate from
// putting one there: it builds outside the checkout and removes the directory
// when the wrapped command returns.

// TestCheckDocsDoesNotBuildIntoTheCheckout pins the wiring: the docs gate runs
// its checker through with-scratch-bd.sh and never builds into the checkout
// itself, while `make build` keeps writing $(BUILD_DIR)/bd — that root ./bd is
// the landing contract scripts/install-bd.sh reads as BD_NEW.
func TestCheckDocsDoesNotBuildIntoTheCheckout(t *testing.T) {
	makefile := readRepoFile(t, "../Makefile")

	recipe := makeRecipe(t, makefile, "check-docs")
	if !strings.Contains(recipe, "with-scratch-bd.sh") {
		t.Errorf("make check-docs no longer builds through scripts/with-scratch-bd.sh:\n%s", recipe)
	}
	if strings.Contains(recipe, "$(BUILD_DIR)/bd") || strings.Contains(recipe, "-o ./bd") {
		t.Errorf("make check-docs builds bd into the checkout again:\n%s", recipe)
	}

	if !strings.Contains(makefile, "-o $(BUILD_DIR)/bd ./cmd/bd") {
		t.Error("make build no longer writes $(BUILD_DIR)/bd; scripts/install-bd.sh reads that ./bd as BD_NEW")
	}
	if !strings.Contains(makefile, "\nBUILD_DIR := .\n") {
		t.Error("BUILD_DIR no longer defaults to the checkout root; the landing's ./bd contract moved")
	}
}

// TestWithScratchBDBuildsOutsideTheCheckout drives the script against a
// throwaway checkout and a fake `go`, so the assertion is on the real argv the
// build would receive: the -o target must sit outside the checkout, the wrapped
// command must be handed that same path, and the scratch directory must be gone
// once the command returns.
func TestWithScratchBDBuildsOutsideTheCheckout(t *testing.T) {
	repo, fakeGoBin, callLog := newWithScratchBDFixture(t)
	argFile := filepath.Join(t.TempDir(), "wrapped-arg")
	wrapped := filepath.Join(t.TempDir(), "record-arg.sh")
	writeWithScratchBDFile(t, wrapped, "#!/usr/bin/env bash\n"+
		"set -euo pipefail\n"+
		"if [ \"$#\" -ne 1 ]; then echo \"expected one argument, got $#\" >&2; exit 1; fi\n"+
		"printf '%s\\n' \"$1\" >\"$WITH_SCRATCH_ARG_FILE\"\n", 0o755)

	out, err := runWithScratchBD(t, repo, fakeGoBin, callLog, argFile, wrapped)
	if err != nil {
		t.Fatalf("with-scratch-bd.sh: %v\n%s", err, out)
	}

	built := builtPathFromGoCalls(t, readWithScratchBDLog(t, callLog))
	if !filepath.IsAbs(built) {
		t.Errorf("build output path is not absolute: %q", built)
	}
	if withinCheckout(built, repo) {
		t.Errorf("build output %q is inside the checkout %q; a root bd is what gt doctor flags", built, repo)
	}

	got, err := os.ReadFile(argFile)
	if err != nil {
		t.Fatalf("read wrapped command's argument: %v", err)
	}
	if strings.TrimSpace(string(got)) != built {
		t.Errorf("wrapped command got %q, want the built binary %q", strings.TrimSpace(string(got)), built)
	}

	if _, statErr := os.Stat(built); !os.IsNotExist(statErr) {
		t.Errorf("scratch binary %q survived the run; the scratch directory must be removed on exit", built)
	}
}

// TestWithScratchBDCarriesTheBuildTag guards the ICU policy for the new build
// path: check-build-tags.sh accepts a script that sources .buildflags, so the
// tag arrives through BEADS_BUILD_TAGS rather than spelled out on the command.
func TestWithScratchBDCarriesTheBuildTag(t *testing.T) {
	repo, fakeGoBin, callLog := newWithScratchBDFixture(t)
	argFile := filepath.Join(t.TempDir(), "wrapped-arg")
	wrapped := filepath.Join(t.TempDir(), "noop.sh")
	writeWithScratchBDFile(t, wrapped, "#!/usr/bin/env bash\nexit 0\n", 0o755)

	if out, err := runWithScratchBD(t, repo, fakeGoBin, callLog, argFile, wrapped); err != nil {
		t.Fatalf("with-scratch-bd.sh: %v\n%s", err, out)
	}
	if calls := readWithScratchBDLog(t, callLog); !strings.Contains(calls, "-tags gms_pure_go") {
		t.Errorf("the scratch build did not carry -tags gms_pure_go:\n%s", calls)
	}
}

// newWithScratchBDFixture assembles the layout the script expects: itself under
// <repo>/scripts (it resolves PROJECT_ROOT from its own location), the
// checkout's .buildflags beside it, a cmd/bd directory to name, and a fake `go`
// that records its argv instead of compiling.
func newWithScratchBDFixture(t *testing.T) (repo, fakeGoBin, callLog string) {
	t.Helper()
	repo = t.TempDir()

	script := readRepoFile(t, "with-scratch-bd.sh")
	writeWithScratchBDFile(t, filepath.Join(repo, "scripts", "with-scratch-bd.sh"), script, 0o755)
	writeWithScratchBDFile(t, filepath.Join(repo, ".buildflags"), readRepoFile(t, "../.buildflags"), 0o644)
	if err := os.MkdirAll(filepath.Join(repo, "cmd", "bd"), 0o755); err != nil {
		t.Fatal(err)
	}

	fakeGoBin = t.TempDir()
	callLog = filepath.Join(t.TempDir(), "go-calls.log")
	writeWithScratchBDFile(t, filepath.Join(fakeGoBin, "go"), "#!/usr/bin/env bash\n"+
		"set -euo pipefail\n"+
		"printf 'go %s\\n' \"$*\" >>\"$WITH_SCRATCH_CALL_LOG\"\n", 0o755)
	return repo, fakeGoBin, callLog
}

func runWithScratchBD(t *testing.T, repo, fakeGoBin, callLog, argFile, wrapped string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(repo, "scripts", "with-scratch-bd.sh"), wrapped)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"PATH="+fakeGoBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"WITH_SCRATCH_CALL_LOG="+callLog,
		"WITH_SCRATCH_ARG_FILE="+argFile,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func readWithScratchBDLog(t *testing.T, callLog string) string {
	t.Helper()
	data, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read %s: %v", callLog, err)
	}
	return string(data)
}

// builtPathFromGoCalls returns the argument following the build's -o flag.
func builtPathFromGoCalls(t *testing.T, calls string) string {
	t.Helper()
	for _, line := range strings.Split(calls, "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			if field == "-o" && i+1 < len(fields) {
				return fields[i+1]
			}
		}
	}
	t.Fatalf("no `go build -o <path>` call recorded:\n%s", calls)
	return ""
}

// withinCheckout reports whether path is the checkout or sits under it, by path
// element rather than string prefix (a sibling named <repo>-other is not inside).
func withinCheckout(path, repo string) bool {
	rel, err := filepath.Rel(repo, path)
	if err != nil {
		return false
	}
	return rel == "." || filepath.IsLocal(rel)
}

func writeWithScratchBDFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}
