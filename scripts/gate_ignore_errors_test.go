package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Markers the stub gate stages print. Asserting on them separates "the stage
// started" from "the guard stopped the gate before any stage".
const (
	gateStubLintMarker = "STUB-GATE-LINT"
	gateStubTestMarker = "STUB-GATE-TEST"
)

// TestGateRefusesIgnoreErrors pins the guard on the Makefile's gate target.
// Under -i, --ignore-errors or MAKEFLAGS=i GNU make ignores a failing recipe,
// so gate-lint could fail, gate-test would still run, and make gate would exit
// 0 over a red tree.
//
// Every case runs make against the real Makefile. The refusals use dry runs
// (-n expands the recipe, so the $(error) guard fires; the absent gate-lint and
// gate-test commands show no stage started) and real runs in a stub directory
// (see gateStubDir) that swaps both stages for markers. The real lint and test
// suites never run. Removing the guard drops the refusal cases to exit 0 and
// fails this test.
func TestGateRefusesIgnoreErrors(t *testing.T) {
	repo := gateRepoRoot(t)

	for _, tc := range []struct {
		name string
		args []string
		env  []string
	}{
		{name: "-i", args: []string{"-n", "-i", "gate"}},
		{name: "--ignore-errors", args: []string{"-n", "--ignore-errors", "gate"}},
		{name: "MAKEFLAGS=i", args: []string{"-n", "gate"}, env: []string{"MAKEFLAGS=i"}},
		{name: "-i split out by -j", args: []string{"-n", "-j8", "-i", "gate"}},
	} {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			out, code := gateRunMake(t, repo, gateEnv(tc.env...), tc.args...)
			if code == 0 {
				t.Fatalf("make %s exited 0; the gate must refuse ignore-errors mode:\n%s",
					strings.Join(tc.args, " "), out)
			}
			if !strings.Contains(out, "-i") {
				t.Errorf("refusal does not name -i:\n%s", out)
			}
			for _, stage := range []string{"gate-lint", "gate-test"} {
				if strings.Contains(out, stage) {
					t.Errorf("refusal came after %s started:\n%s", stage, out)
				}
			}
		})
	}

	t.Run("runs both stages otherwise", func(t *testing.T) {
		for _, args := range [][]string{
			{"-n", "gate"},
			{"-n", "-k", "gate"},
		} {
			out, code := gateRunMake(t, repo, gateEnv(), args...)
			if code != 0 {
				t.Fatalf("make %s exited %d:\n%s", strings.Join(args, " "), code, out)
			}
			lint, test := strings.Index(out, "gate-lint"), strings.Index(out, "gate-test")
			switch {
			case lint < 0 || test < 0:
				t.Errorf("make %s does not run both gate stages:\n%s", strings.Join(args, " "), out)
			case lint > test:
				t.Errorf("make %s runs gate-test before gate-lint:\n%s", strings.Join(args, " "), out)
			}
		}
	})

	// Words that contain an "i" without being the packed flag word must not
	// trip the guard: a command-line variable lands in MAKEFLAGS whole, and a
	// long option like --no-print-directory lands in the first word.
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "command-line variable", args: []string{"-n", "gate", "TEST_TAGS=integration"}},
		{name: "long option", args: []string{"-n", "--no-print-directory", "gate"}},
	} {
		t.Run(tc.name+" is not a flag", func(t *testing.T) {
			out, code := gateRunMake(t, repo, gateEnv(), tc.args...)
			if code != 0 {
				t.Fatalf("make %s exited %d; the guard read a non-flag word as flags:\n%s",
					strings.Join(tc.args, " "), code, out)
			}
		})
	}

	stub := gateStubDir(t, repo, 1)
	for _, tc := range []struct {
		name string
		args []string
		env  []string
	}{
		{name: "-i", args: []string{"-i", "gate"}},
		{name: "MAKEFLAGS=i", args: []string{"gate"}, env: []string{"MAKEFLAGS=i"}},
	} {
		t.Run("refuses "+tc.name+" before any stage runs", func(t *testing.T) {
			out, code := gateRunMake(t, stub, gateEnv(tc.env...), tc.args...)
			if code == 0 {
				t.Fatalf("make %s exited 0 over a failing stub:\n%s", strings.Join(tc.args, " "), out)
			}
			if strings.Contains(out, gateStubLintMarker) || strings.Contains(out, gateStubTestMarker) {
				t.Errorf("a gate stage ran despite ignore-errors mode:\n%s", out)
			}
		})
	}

	t.Run("stubbed lint failure stops the gate", func(t *testing.T) {
		out, code := gateRunMake(t, stub, gateEnv(), "gate")
		if code == 0 {
			t.Fatalf("make gate exited 0 though the stubbed gate-lint failed:\n%s", out)
		}
		if !strings.Contains(out, gateStubLintMarker) {
			t.Errorf("stubbed gate-lint never ran:\n%s", out)
		}
		if strings.Contains(out, gateStubTestMarker) {
			t.Errorf("gate-test ran after gate-lint failed:\n%s", out)
		}
	})

	t.Run("stubbed lint failure still stops the gate under -k", func(t *testing.T) {
		out, code := gateRunMake(t, stub, gateEnv(), "-k", "gate")
		if code == 0 {
			t.Fatalf("make -k gate exited 0 though the stubbed gate-lint failed:\n%s", out)
		}
	})
}

// gateRepoRoot returns the repository root, the parent of this package.
func gateRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Makefile")); err != nil {
		t.Fatalf("repository root %s has no Makefile: %v", root, err)
	}
	return root
}

// gateStubDir writes a Makefile in a fresh temp directory that includes the
// real Makefile and then replaces gate-lint and gate-test with stubs: a marker
// on stdout plus lintExit as the status. Recursive make looks for "Makefile" in
// the working directory, so the $(MAKE) gate-lint inside the gate recipe finds
// the stubs rather than the real suites.
func gateStubDir(t *testing.T, repo string, lintExit int) string {
	t.Helper()
	dir := t.TempDir()
	stub := strings.Join([]string{
		"include " + filepath.ToSlash(filepath.Join(repo, "Makefile")),
		"",
		"gate-lint:",
		"\t@echo " + gateStubLintMarker,
		"\t@exit " + strconv.Itoa(lintExit),
		"",
		"gate-test:",
		"\t@echo " + gateStubTestMarker,
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(stub), 0o644); err != nil {
		t.Fatalf("write stub Makefile: %v", err)
	}
	return dir
}

// gateEnv returns the process environment without make's flag variables, so a
// MAKEFLAGS inherited from the outer "make test" cannot reach the runs below,
// plus extra.
func gateEnv(extra ...string) []string {
	env := make([]string, 0, len(os.Environ())+len(extra))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "MAKEFLAGS=") || strings.HasPrefix(kv, "MFLAGS=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, extra...)
}

// gateRunMake runs make in dir and returns its combined output and exit code.
func gateRunMake(t *testing.T, dir string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("make", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run make %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out), exitErr.ExitCode()
}
