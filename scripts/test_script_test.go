package scripts_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	testScriptFakeGoLogEnv      = "BEADS_TEST_SCRIPT_FAKE_GO_LOG"
	testScriptExpectedBinaryEnv = "BEADS_TEST_SCRIPT_EXPECTED_BINARY"
	testScriptExpectedBaseEnv   = "BEADS_TEST_SCRIPT_EXPECTED_BASENAME"
	testScriptDriverEnv         = "BEADS_TEST_SCRIPT_DRIVER"
	testScriptNativeSuffixEnv   = "BEADS_TEST_SCRIPT_NATIVE_SUFFIX"
	testScriptLaunchProbeEnv    = "BEADS_TEST_SCRIPT_LAUNCH_PROBE"
	testScriptCoverLogEnv       = "BEADS_TEST_SCRIPT_FAKE_GO_COVER_LOG"
	testScriptCoverFailEnv      = "BEADS_TEST_SCRIPT_FAKE_GO_COVER_FAIL"
)

const testScriptFakeGo = `#!/usr/bin/env bash
set -euo pipefail

record() {
    printf '%s\n' "$1" >>"$BEADS_TEST_SCRIPT_FAKE_GO_LOG"
}

case "${1:-}" in
    env)
        record env
        if [[ $# -ne 2 || "$2" != "GOEXE" ]]; then
            printf 'fake go: unsupported env arguments: %s\n' "$*" >&2
            exit 90
        fi
        printf '%s\n' "$BEADS_TEST_SCRIPT_NATIVE_SUFFIX"
        ;;
    build)
        record build
        shift
        output=""
        while [[ $# -gt 0 ]]; do
            if [[ "$1" == "-o" ]]; then
                if [[ $# -lt 2 ]]; then
                    printf 'fake go: -o is missing its output\n' >&2
                    exit 90
                fi
                output="$2"
                shift 2
            else
                shift
            fi
        done
        if [[ -z "$output" || "$output" != "$BEADS_TEST_SCRIPT_EXPECTED_BINARY" ]]; then
            printf 'fake go: build output %q, want %q\n' "$output" "$BEADS_TEST_SCRIPT_EXPECTED_BINARY" >&2
            exit 90
        fi
        cp -f -- "$BEADS_TEST_SCRIPT_DRIVER" "$output"
        chmod +x "$output"
        ;;
    test)
        record test
        profile=""
        while [[ $# -gt 0 ]]; do
            if [[ "$1" == "-coverprofile" ]]; then
                profile="${2:-}"
                shift 2
            else
                shift
            fi
        done
        if [[ -n "$profile" ]]; then
            # Coverage mode: behave like a passing go test that wrote a
            # profile, so scripts/test.sh reaches its go-tool-cover step.
            if [[ -z "${BEADS_TEST_SCRIPT_FAKE_GO_COVER_LOG:-}" ]]; then
                printf 'fake go: coverage run without a profile log\n' >&2
                exit 90
            fi
            printf '%s\n' "$profile" >>"$BEADS_TEST_SCRIPT_FAKE_GO_COVER_LOG"
            printf 'total:\t(statements)\t100.0%%\n' >"$profile"
            if [[ "${BEADS_TEST_SCRIPT_FAKE_GO_COVER_FAIL:-0}" == "1" ]]; then
                printf 'fake go: simulated test failure\n' >&2
                exit 1
            fi
            exit 0
        fi
        "$BEADS_TEST_SCRIPT_DRIVER" \
            -test.run '^TestTestScriptPrebuiltBinaryLaunchProbe$' \
            -test.count=1
        ;;
    tool)
        # go tool cover -func=<profile>. Fail when the profile is gone, the
        # same way the real tool does after a torn or deleted profile.
        record tool
        profile=""
        for arg in "$@"; do
            case "$arg" in
                -func=*) profile="${arg#-func=}" ;;
            esac
        done
        if [[ ! -s "$profile" ]]; then
            printf 'fake go: cover profile %q is missing or empty\n' "$profile" >&2
            exit 1
        fi
        cat "$profile"
        ;;
    *)
        printf 'fake go: unsupported command: %s\n' "$*" >&2
        exit 90
        ;;
esac
`

func TestTestScriptPrebuiltBinaryContract(t *testing.T) {
	t.Run("generated path uses the native executable suffix and launches", func(t *testing.T) {
		commands := runTestScriptWithFakeGo(t, "")
		assertFakeGoCommands(t, commands, "env", "build", "test")
	})

	t.Run("caller supplied binary wins without a build", func(t *testing.T) {
		fixtureRoot := filepath.Join(t.TempDir(), "caller override with spaces")
		if err := os.MkdirAll(fixtureRoot, 0o755); err != nil {
			t.Fatalf("create caller fixture root: %v", err)
		}
		callerBinary := filepath.Join(fixtureRoot, "caller supplied bd"+nativeExecutableSuffix())
		copyCurrentTestExecutable(t, callerBinary)

		commands := runTestScriptWithFakeGo(t, callerBinary)
		assertFakeGoCommands(t, commands, "test")
	})
}

// TestTestScriptCoverageProfile pins the be-4pc contract: a coverage run uses a
// profile of its own, removes it when the script exits, and leaves a caller's
// explicit TEST_COVERPROFILE alone.
func TestTestScriptCoverageProfile(t *testing.T) {
	t.Run("concurrent runs get distinct profiles and remove them", func(t *testing.T) {
		first := newTestScriptFixture(t, "")
		second := newTestScriptFixture(t, "")

		firstCmd := first.command("./cmd/bd")
		firstCmd.Env = first.withEnv("TEST_COVER=1")
		secondCmd := second.command("./cmd/bd")
		secondCmd.Env = second.withEnv("TEST_COVER=1")

		var firstOutput, secondOutput bytes.Buffer
		firstCmd.Stdout, firstCmd.Stderr = &firstOutput, &firstOutput
		secondCmd.Stdout, secondCmd.Stderr = &secondOutput, &secondOutput

		if err := firstCmd.Start(); err != nil {
			t.Fatalf("start first scripts/test.sh: %v", err)
		}
		if err := secondCmd.Start(); err != nil {
			t.Fatalf("start second scripts/test.sh: %v", err)
		}
		firstErr := firstCmd.Wait()
		secondErr := secondCmd.Wait()
		if firstErr != nil {
			t.Fatalf("first scripts/test.sh failed: %v\n%s", firstErr, firstOutput.String())
		}
		if secondErr != nil {
			t.Fatalf("second scripts/test.sh failed: %v\n%s", secondErr, secondOutput.String())
		}

		firstProfiles := first.coverProfiles()
		secondProfiles := second.coverProfiles()
		if len(firstProfiles) != 1 || len(secondProfiles) != 1 {
			t.Fatalf("coverprofile paths = %q and %q, want exactly one each", firstProfiles, secondProfiles)
		}
		if firstProfiles[0] == secondProfiles[0] {
			t.Fatalf("concurrent runs shared coverage profile %q", firstProfiles[0])
		}
		assertFakeGoCommands(t, first.commands(), "env", "build", "test", "tool")
		assertFakeGoCommands(t, second.commands(), "env", "build", "test", "tool")

		runs := []struct {
			output  string
			profile string
		}{
			{firstOutput.String(), firstProfiles[0]},
			{secondOutput.String(), secondProfiles[0]},
		}
		for _, run := range runs {
			want := "Total coverage: 100.0% (profile: " + run.profile + ")"
			if !strings.Contains(run.output, want) {
				t.Fatalf("scripts/test.sh output does not report %q:\n%s", want, run.output)
			}
			if _, err := os.Stat(run.profile); !os.IsNotExist(err) {
				t.Fatalf("per-run profile %q survived the run (stat error: %v)", run.profile, err)
			}
		}
	})

	t.Run("explicit TEST_COVERPROFILE is used as given and kept", func(t *testing.T) {
		fixture := newTestScriptFixture(t, "")
		explicit := filepath.Join(t.TempDir(), "caller chosen profile.out")
		cmd := fixture.command("./cmd/bd")
		cmd.Env = fixture.withEnv("TEST_COVER=1", "TEST_COVERPROFILE="+portableTestScriptPath(explicit))
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			t.Fatalf("scripts/test.sh with explicit TEST_COVERPROFILE failed: %v\n%s", runErr, output)
		}

		profiles := fixture.coverProfiles()
		if len(profiles) != 1 || profiles[0] != portableTestScriptPath(explicit) {
			t.Fatalf("coverprofile paths = %q, want [%q]", profiles, portableTestScriptPath(explicit))
		}
		if _, err := os.Stat(explicit); err != nil {
			t.Fatalf("explicit TEST_COVERPROFILE was removed: %v", err)
		}
	})

	t.Run("per-run profile is removed when the test run fails", func(t *testing.T) {
		fixture := newTestScriptFixture(t, "")
		cmd := fixture.command("./cmd/bd")
		cmd.Env = fixture.withEnv("TEST_COVER=1", testScriptCoverFailEnv+"=1")
		output, runErr := cmd.CombinedOutput()
		if runErr == nil {
			t.Fatalf("scripts/test.sh succeeded, want the failing go test to propagate:\n%s", output)
		}

		assertFakeGoCommands(t, fixture.commands(), "env", "build", "test")
		profiles := fixture.coverProfiles()
		if len(profiles) != 1 {
			t.Fatalf("coverprofile paths = %q, want exactly one", profiles)
		}
		if _, err := os.Stat(profiles[0]); !os.IsNotExist(err) {
			t.Fatalf("per-run profile %q survived the failed run (stat error: %v)", profiles[0], err)
		}
	})
}

// TestTestScriptPrebuiltBinaryLaunchProbe is selected only by the fake go test
// process above. Keeping the os/exec probe in a normal test avoids claiming the
// package-wide TestMain authority needed by other script-selection contracts.
func TestTestScriptPrebuiltBinaryLaunchProbe(t *testing.T) {
	if os.Getenv(testScriptLaunchProbeEnv) != "1" {
		t.Skip("re-exec probe runs only under the test.sh fake-go driver")
	}

	prebuilt := os.Getenv("BEADS_TEST_BD_BINARY")
	expected := os.Getenv(testScriptExpectedBinaryEnv)
	if prebuilt == "" || expected == "" || !sameTestScriptFile(prebuilt, expected) {
		t.Fatalf("exported prebuilt binary %q is not expected file %q", prebuilt, expected)
	}
	if want := os.Getenv(testScriptExpectedBaseEnv); filepath.Base(prebuilt) != want {
		t.Fatalf("exported prebuilt basename = %q, want %q", filepath.Base(prebuilt), want)
	}

	command := exec.Command(prebuilt, "-test.run=^$")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("launch exported prebuilt binary through os/exec: %v\n%s", err, output)
	}
}

// testScriptFixture is one hermetic scripts/test.sh run: a private fake `go`,
// fixture-local HOME/TMPDIR, and logs of the fake go's calls and of every
// -coverprofile path the script handed it.
type testScriptFixture struct {
	t        *testing.T
	callLog  string
	coverLog string
	env      []string
	bash     string
	repoRoot string
}

func newTestScriptFixture(t *testing.T, callerBinary string) *testScriptFixture {
	t.Helper()

	// A directory with spaces exercises quoting, and MkdirTemp makes each
	// fixture unique so one test can run two of them at once.
	root, err := os.MkdirTemp(t.TempDir(), "test script root with spaces ")
	if err != nil {
		t.Fatalf("create fixture root: %v", err)
	}
	fakeBin := filepath.Join(root, "fake go bin")
	testEnvRoot := filepath.Join(root, "isolated test environment")
	tempRoot := filepath.Join(root, "temporary files")
	for _, path := range []string{fakeBin, testEnvRoot, tempRoot} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("create fixture directory %s: %v", path, err)
		}
	}

	fakeGo := filepath.Join(fakeBin, "go")
	if err := os.WriteFile(fakeGo, []byte(testScriptFakeGo), 0o755); err != nil {
		t.Fatalf("write fake go: %v", err)
	}
	callLog := filepath.Join(root, "fake go calls")
	if err := os.WriteFile(callLog, nil, 0o600); err != nil {
		t.Fatalf("initialize fake-go call log: %v", err)
	}
	coverLog := filepath.Join(root, "fake go coverage profiles")
	if err := os.WriteFile(coverLog, nil, 0o600); err != nil {
		t.Fatalf("initialize fake-go coverage log: %v", err)
	}

	expected := callerBinary
	if expected == "" {
		expected = filepath.Join(testEnvRoot, "prebuilt-bd", "bd"+nativeExecutableSuffix())
	}

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("bash is required to exercise scripts/test.sh: %v", err)
	}
	repoRoot := sourceRepoRoot(t)
	env := testScriptEnvironment(testEnvRoot, tempRoot, expected, callerBinary, coverLog)
	fakeBinShellPath := shellPathUnderEnv(t, bash, fakeBin, env)
	fakeGoShellPath := shellPathUnderEnv(t, bash, fakeGo, env)
	driverShellPath := shellPathUnderEnv(t, bash, currentTestExecutable(t), env)
	callLogShellPath := shellPathUnderEnv(t, bash, callLog, env)
	env = append(env,
		"BEADS_TEST_COMMAND_PATH="+fakeBinShellPath+":/usr/bin:/bin",
		testScriptDriverEnv+"="+driverShellPath,
		testScriptFakeGoLogEnv+"="+callLogShellPath,
	)
	requireShellCommandPath(t, bash, repoRoot, env, "go", fakeGoShellPath)

	return &testScriptFixture{
		t:        t,
		callLog:  callLog,
		coverLog: coverLog,
		env:      env,
		bash:     bash,
		repoRoot: repoRoot,
	}
}

// withEnv returns the fixture environment plus extra KEY=VALUE pairs.
func (f *testScriptFixture) withEnv(pairs ...string) []string {
	env := append([]string(nil), f.env...)
	return append(env, pairs...)
}

// command builds a scripts/test.sh invocation carrying the fixture environment.
func (f *testScriptFixture) command(packagePath string) *exec.Cmd {
	f.t.Helper()
	cmd := exec.Command(
		f.bash,
		"--noprofile",
		"--norc",
		"-c",
		`PATH="$BEADS_TEST_COMMAND_PATH"; export PATH; exec "$BASH" --noprofile --norc "$@"`,
		"test-script",
		shellPathUnderEnv(f.t, f.bash, filepath.Join(f.repoRoot, "scripts", "test.sh"), f.env),
		packagePath,
	)
	cmd.Dir = f.repoRoot
	cmd.Env = append([]string(nil), f.env...)
	return cmd
}

// commands returns the fake-go commands the script ran, in order.
func (f *testScriptFixture) commands() []string {
	f.t.Helper()
	content, err := os.ReadFile(f.callLog)
	if err != nil {
		f.t.Fatalf("read fake-go call log: %v", err)
	}
	return strings.Fields(string(content))
}

// coverProfiles returns the -coverprofile paths the script passed to go test.
// One path per line: fixture paths contain spaces, so fields would split them.
func (f *testScriptFixture) coverProfiles() []string {
	f.t.Helper()
	content, err := os.ReadFile(f.coverLog)
	if err != nil {
		f.t.Fatalf("read fake-go coverage log: %v", err)
	}
	var profiles []string
	for _, line := range strings.Split(string(content), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			profiles = append(profiles, line)
		}
	}
	return profiles
}

func runTestScriptWithFakeGo(t *testing.T, callerBinary string) []string {
	t.Helper()

	fixture := newTestScriptFixture(t, callerBinary)
	output, runErr := fixture.command("./cmd/bd").CombinedOutput()
	if runErr != nil {
		t.Fatalf("scripts/test.sh failed: %v\n%s", runErr, output)
	}
	return fixture.commands()
}

func testScriptEnvironment(testEnvRoot string, tempRoot string, expected string, callerBinary string, coverLog string) []string {
	home := filepath.Join(testEnvRoot, "home")
	env := []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + portableTestScriptPath(home),
		"USERPROFILE=" + portableTestScriptPath(home),
		"TMPDIR=" + portableTestScriptPath(tempRoot),
		"TEMP=" + portableTestScriptPath(tempRoot),
		"TMP=" + portableTestScriptPath(tempRoot),
		"LC_ALL=C",
		"LANG=C",
		"BASH_ENV=",
		"ENV=",
		"CGO_ENABLED=1",
		"GOFLAGS=",
		"BEADS_TEST_ENV_ACTIVE=1",
		"BEADS_TEST_ENV_ROOT=" + portableTestScriptPath(testEnvRoot),
		testScriptExpectedBinaryEnv + "=" + portableTestScriptPath(expected),
		testScriptExpectedBaseEnv + "=" + filepath.Base(expected),
		testScriptNativeSuffixEnv + "=" + nativeExecutableSuffix(),
		testScriptLaunchProbeEnv + "=1",
		testScriptCoverLogEnv + "=" + portableTestScriptPath(coverLog),
	}
	if callerBinary != "" {
		env = append(env, "BEADS_TEST_BD_BINARY="+portableTestScriptPath(callerBinary))
	}
	for _, key := range []string{"SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func assertFakeGoCommands(t *testing.T, commands []string, want ...string) {
	t.Helper()
	if strings.Join(commands, " ") != strings.Join(want, " ") {
		t.Fatalf("fake-go commands = %q, want %q", commands, want)
	}
}

func copyCurrentTestExecutable(t *testing.T, destination string) {
	t.Helper()
	input, err := os.Open(currentTestExecutable(t))
	if err != nil {
		t.Fatalf("open current test executable: %v", err)
	}
	defer input.Close()

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		t.Fatalf("create native test executable: %v", err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		t.Fatalf("copy native test executable: %v", err)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("close native test executable: %v", err)
	}
}

func currentTestExecutable(t *testing.T) string {
	t.Helper()
	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve current test executable: %v", err)
	}
	return path
}

func sameTestScriptFile(first string, second string) bool {
	firstInfo, firstErr := os.Stat(first)
	secondInfo, secondErr := os.Stat(second)
	return firstErr == nil && secondErr == nil && os.SameFile(firstInfo, secondInfo)
}

func nativeExecutableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func portableTestScriptPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
