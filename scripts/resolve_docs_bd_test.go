package scripts_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// resolveDocsBDScript is the script under test, relative to the repository
// root newResolvePinFixture copies it out of.
const resolveDocsBDScript = "scripts/resolve-docs-bd.sh"

// resolvePinTag is the tag the fixture's docs/cli-docs.pin names — the shape
// of a release tag a real checkout pins.
const resolvePinTag = "v1.2.2"

// resolvePinFixture is a throwaway world for resolve-docs-bd.sh: a temporary
// git repository holding a copy of the script, a bare origin it fetches tags
// from, and a stub `go` standing in for the pinned build.
//
// Every path is inside a t.TempDir(). The script fetches tags and builds a
// binary, so a fixture reaching the host's real repository or toolchain would
// do work outside the test; the stub exists so the real toolchain is never
// needed at all.
type resolvePinFixture struct {
	t         *testing.T
	bash      string
	repoDir   string // the clone the script runs in: its cwd and PROJECT_ROOT
	originDir string // bare origin, the tag source
	binDir    string // the stub `go`
	homeDir   string // HOME, so no global git config is read
}

type resolvePinRun struct {
	stdout string
	stderr string
	err    error
}

func (r resolvePinRun) exitCode() int {
	if r.err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(r.err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func newResolvePinFixture(t *testing.T) *resolvePinFixture {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("resolve-docs-bd.sh is a bash script: %v", err)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("resolve-docs-bd.sh resolves the pin through git: %v", err)
	}

	root := t.TempDir()
	f := &resolvePinFixture{
		t:         t,
		bash:      bash,
		repoDir:   filepath.Join(root, "repo"),
		originDir: filepath.Join(root, "origin.git"),
		binDir:    filepath.Join(root, "bin"),
		homeDir:   filepath.Join(root, "home"),
	}
	for _, dir := range []string{
		f.repoDir, filepath.Join(f.repoDir, "scripts"), f.originDir, f.binDir, f.homeDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	script, err := os.ReadFile(filepath.Join(sourceRepoRoot(t), filepath.FromSlash(resolveDocsBDScript)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.repoDir, filepath.FromSlash(resolveDocsBDScript)), script, 0o755); err != nil {
		t.Fatal(err)
	}

	// A stub go: the pinned build is not this test's subject and the fixture
	// has no bd source. It writes the binary the script asked for so the
	// success path can end where the real one would.
	writeExecutable(t, filepath.Join(f.binDir, "go"), `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
    if [ "$1" = "-o" ]; then
        out="$2"
        shift 2
    else
        shift
    fi
done
[ -n "$out" ] || { echo "stub go: no -o" >&2; exit 64; }
mkdir -p "$(dirname "$out")"
printf '#!/bin/sh\n' >"$out"
chmod +x "$out"
`)

	f.gitIn(f.repoDir, "init", "-q", ".")
	f.gitIn(f.originDir, "init", "-q", "--bare", ".")
	f.gitIn(f.repoDir, "remote", "add", "origin", f.originDir)
	f.writeFile("README.md", "fixture\n")
	f.gitIn(f.repoDir, "add", "-A")
	f.gitIn(f.repoDir, "commit", "-q", "-m", "base")
	// The origin is a clone's origin, not an empty shell: it has commits. A
	// real Forgejo mirror missing only its tags looks exactly like this.
	f.gitIn(f.repoDir, "push", "-q", "origin", "HEAD")
	return f
}

func (f *resolvePinFixture) gitIn(dir string, args ...string) string {
	f.t.Helper()
	// Identity and signing have to be supplied per command: HOME is a sandbox
	// with no config, and the system config is off, so the host's own
	// gitconfig (a tag.gpgSign=true, say) cannot make this fixture fail.
	prefix := []string{
		"-c", "user.name=resolve-docs-bd test",
		"-c", "user.email=resolve-docs-bd@example.invalid",
		"-c", "commit.gpgsign=false",
	}
	cmd := exec.Command("git", append(prefix, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+f.homeDir, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *resolvePinFixture) writeFile(name, body string) {
	f.t.Helper()
	path := filepath.Join(f.repoDir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// setPin writes docs/cli-docs.pin in the real file's shape: a comment block
// above the value, which the script has to skip.
func (f *resolvePinFixture) setPin(pin string) {
	f.t.Helper()
	f.writeFile("docs/cli-docs.pin", "# Pins the bd version the docs are generated from.\n#\n# Set to HEAD to track the current checkout instead.\n"+pin+"\n")
}

func (f *resolvePinFixture) run() resolvePinRun {
	f.t.Helper()
	return f.runWithEnv()
}

// runWithEnv runs the script with extra KEY=VALUE entries, for the cases that
// turn on an environment switch (BD_DOCS_IGNORE_PIN).
func (f *resolvePinFixture) runWithEnv(extra ...string) resolvePinRun {
	f.t.Helper()

	var stdout, stderr strings.Builder
	cmd := exec.Command(f.bash, "--noprofile", "--norc", resolveDocsBDScript)
	cmd.Dir = f.repoDir
	cmd.Env = append([]string{
		"PATH=" + shellPath(f.t, f.binDir) + ":" + bashPathList(f.t, os.Getenv("PATH")) + ":/usr/bin:/bin",
		"HOME=" + shellPath(f.t, f.homeDir),
		"LC_ALL=C",
		"LANG=C",
		"BASH_ENV=",
		"ENV=",
	}, extra...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return resolvePinRun{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

// cachedBDPath is where the script caches the binary it builds for the pin.
func (f *resolvePinFixture) cachedBDPath() string {
	return filepath.Join(f.repoDir, "build", "docs-bd", resolvePinTag, "bd")
}

// stubFailingFetch puts a git on the script's PATH that answers everything
// except `fetch`, which fails. Origin listing the pin in ls-remote while the
// fetch of that same ref fails is a real state (a mirror with a broken
// object, a ref the server refuses to send) that no local remote reproduces
// on demand, and it is the one diagnostic branch whose wording the reader
// acts on differently: "serves it, the fetch broke" is not "bump the pin".
func (f *resolvePinFixture) stubFailingFetch() {
	f.t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		f.t.Fatalf("locate git: %v", err)
	}
	writeExecutable(f.t, filepath.Join(f.binDir, "git"), fmt.Sprintf(`#!/bin/sh
for arg in "$@"; do
    if [ "$arg" = "fetch" ]; then
        echo "fatal: simulated fetch failure" >&2
        exit 128
    fi
done
exec %s "$@"
`, shSingleQuote(realGit)))
}

// TestResolveDocsBDFailsOnTaglessOrigin is the be-4f9 gate: on a clone whose
// origin serves no tags, the script must not report "unpinned". Callers read
// empty stdout plus exit 0 as "no pin, validate the current checkout", so
// answering that way would silently check the wrong binary — the exact thing
// the release pin exists to prevent. Before the fix the script died on the
// fetch's raw "fatal: couldn't find remote ref", which named neither the pin
// file, nor the pin value, nor the missing tag.
func TestResolveDocsBDFailsOnTaglessOrigin(t *testing.T) {
	f := newResolvePinFixture(t)
	f.setPin(resolvePinTag)

	run := f.run()

	if run.exitCode() == 0 {
		t.Fatalf("unresolvable pin exited 0; callers would fall back to the current checkout.\nstdout:\n%s\nstderr:\n%s",
			run.stdout, run.stderr)
	}
	if strings.TrimSpace(run.stdout) != "" {
		t.Errorf("failing pin printed %q to stdout; a caller takes non-empty stdout as the bd to use", run.stdout)
	}
	for _, want := range []string{
		"docs/cli-docs.pin",
		resolvePinTag,
		"serves no tags at all",
		"refs/tags/" + resolvePinTag,
	} {
		if !strings.Contains(run.stderr, want) {
			t.Errorf("diagnostic does not name %q:\n%s", want, run.stderr)
		}
	}
	if _, err := os.Stat(f.cachedBDPath()); err == nil {
		t.Error("a bd was built for an unresolvable pin")
	}
}

// TestResolveDocsBDSaysWhichTagIsMissing pins the other half of the cause
// line: an origin that serves tags but not this one is a stale pin, fixed by
// bumping the pin, while an origin serving none at all wants the tags pushed.
// Reporting "no tags" for both would send the reader to the wrong fix.
func TestResolveDocsBDSaysWhichTagIsMissing(t *testing.T) {
	f := newResolvePinFixture(t)
	f.setPin(resolvePinTag)
	f.gitIn(f.repoDir, "tag", "v9.9.9")
	f.gitIn(f.repoDir, "push", "-q", "origin", "refs/tags/v9.9.9")

	run := f.run()

	if run.exitCode() == 0 {
		t.Fatalf("unresolvable pin exited 0:\nstdout:\n%s\nstderr:\n%s", run.stdout, run.stderr)
	}
	if want := "serves tags, but not refs/tags/" + resolvePinTag; !strings.Contains(run.stderr, want) {
		t.Errorf("diagnostic does not report %q:\n%s", want, run.stderr)
	}
	if strings.Contains(run.stderr, "serves no tags at all") {
		t.Errorf("diagnostic claims origin serves no tags though v9.9.9 is there:\n%s", run.stderr)
	}
}

// TestResolveDocsBDSaysTheTagIsServedButUnfetchable pins the third cause: the
// pin is on origin, so "bump the pin" would be wrong advice — the fetch is
// what broke. The check is an exact tag-name comparison, and this is the case
// that shows it: the pin is served, and a regex or a pipe-shortened match
// would fall through to the "serves tags, but not this one" branch.
func TestResolveDocsBDSaysTheTagIsServedButUnfetchable(t *testing.T) {
	f := newResolvePinFixture(t)
	f.setPin(resolvePinTag)
	f.gitIn(f.repoDir, "tag", resolvePinTag)
	f.gitIn(f.repoDir, "push", "-q", "origin", "refs/tags/"+resolvePinTag)
	// The local tag would short-circuit the fetch that has to fail.
	f.gitIn(f.repoDir, "tag", "-d", resolvePinTag)
	f.stubFailingFetch()

	run := f.run()

	if run.exitCode() == 0 {
		t.Fatalf("failed fetch exited 0:\nstdout:\n%s\nstderr:\n%s", run.stdout, run.stderr)
	}
	if want := "serves refs/tags/" + resolvePinTag + ", but fetching it failed"; !strings.Contains(run.stderr, want) {
		t.Errorf("diagnostic does not report %q:\n%s", want, run.stderr)
	}
	if strings.Contains(run.stderr, "but not refs/tags/") {
		t.Errorf("diagnostic claims origin does not serve the pin it serves:\n%s", run.stderr)
	}
}

// TestResolveDocsBDReportsAnUnreachableOrigin covers the unreachable-origin
// cause. An
// origin that cannot be reached at all is not a missing tag — "push the tag"
// would be wrong advice — and the fetch's own error is the only thing that
// says so.
func TestResolveDocsBDReportsAnUnreachableOrigin(t *testing.T) {
	f := newResolvePinFixture(t)
	f.setPin(resolvePinTag)
	f.gitIn(f.repoDir, "remote", "set-url", "origin", filepath.Join(f.repoDir, "no-such-remote.git"))

	run := f.run()

	if run.exitCode() == 0 {
		t.Fatalf("unreachable origin exited 0:\nstdout:\n%s\nstderr:\n%s", run.stdout, run.stderr)
	}
	if want := "could not be reached to list its tags"; !strings.Contains(run.stderr, want) {
		t.Errorf("diagnostic does not report %q:\n%s", want, run.stderr)
	}
	if strings.Contains(run.stderr, "serves no tags at all") {
		t.Errorf("diagnostic claims origin serves no tags though it is unreachable:\n%s", run.stderr)
	}
}

// TestResolveDocsBDFetchesThePinnedTag is the success half of the acceptance
// criteria: with the tag available, the script resolves it (fetching when the
// clone lacks it) and prints the built binary's path on stdout.
func TestResolveDocsBDFetchesThePinnedTag(t *testing.T) {
	f := newResolvePinFixture(t)
	f.setPin(resolvePinTag)
	f.gitIn(f.repoDir, "tag", resolvePinTag)
	f.gitIn(f.repoDir, "push", "-q", "origin", "refs/tags/"+resolvePinTag)
	// Without this the local tag short-circuits the fetch and the fetch path
	// — the one that failed on the Forgejo mirror — never runs.
	f.gitIn(f.repoDir, "tag", "-d", resolvePinTag)

	run := f.run()

	if run.exitCode() != 0 {
		t.Fatalf("resolvable pin exited %d:\nstdout:\n%s\nstderr:\n%s", run.exitCode(), run.stdout, run.stderr)
	}
	got := strings.TrimSpace(run.stdout)
	if want := filepath.ToSlash(filepath.Join("build", "docs-bd", resolvePinTag, "bd")); !strings.HasSuffix(filepath.ToSlash(got), want) {
		t.Errorf("stdout = %q, want a path ending in %q", got, want)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("stdout %q is not an existing binary: %v", got, err)
	}
	if want := "fetching tag " + resolvePinTag + " from origin"; !strings.Contains(run.stderr, want) {
		t.Errorf("stderr does not report %q:\n%s", want, run.stderr)
	}
}

// TestResolveDocsBDStaysSilentWhenUnpinned pins the other exit-0 path: no pin
// file, or a pin of HEAD, means "track the current checkout" and must print
// nothing at all, since callers read non-empty stdout as a pinned binary.
func TestResolveDocsBDStaysSilentWhenUnpinned(t *testing.T) {
	for _, tc := range []struct {
		name    string
		writeFn func(*resolvePinFixture)
	}{
		{"no pin file", func(*resolvePinFixture) {}},
		{"pin is HEAD", func(f *resolvePinFixture) { f.setPin("HEAD") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newResolvePinFixture(t)
			tc.writeFn(f)

			run := f.run()

			if run.exitCode() != 0 {
				t.Fatalf("unpinned run exited %d:\nstderr:\n%s", run.exitCode(), run.stderr)
			}
			if run.stdout != "" {
				t.Errorf("unpinned run printed %q to stdout, want nothing", run.stdout)
			}
		})
	}
}

// TestResolveDocsBDHonorsTheIgnorePinEscape pins BD_DOCS_IGNORE_PIN=1 as the
// script's own escape, not just its callers'. The failure this script prints
// names that switch as the way to carry on deliberately, and a reader who
// sets it while calling the script directly — as the drift check's
// regeneration step does — must get the current-checkout behavior, not the
// error again.
func TestResolveDocsBDHonorsTheIgnorePinEscape(t *testing.T) {
	f := newResolvePinFixture(t)
	f.setPin(resolvePinTag)

	run := f.runWithEnv("BD_DOCS_IGNORE_PIN=1")

	if run.exitCode() != 0 {
		t.Fatalf("BD_DOCS_IGNORE_PIN=1 exited %d:\nstderr:\n%s", run.exitCode(), run.stderr)
	}
	if run.stdout != "" {
		t.Errorf("BD_DOCS_IGNORE_PIN=1 printed %q to stdout, want nothing", run.stdout)
	}
}

// TestResolveDocsBDUsesTheCachedBinary protects the ordering: the cache check
// sits above the fetch, so a pinned binary that was already built keeps the
// docs gates runnable even when origin cannot serve its tag. Flipping the
// order would make an unreachable tag fatal for a checkout that is fine.
func TestResolveDocsBDUsesTheCachedBinary(t *testing.T) {
	f := newResolvePinFixture(t)
	f.setPin(resolvePinTag)
	if err := os.MkdirAll(filepath.Dir(f.cachedBDPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, f.cachedBDPath(), "#!/bin/sh\nexit 0\n")

	run := f.run()

	if run.exitCode() != 0 {
		t.Fatalf("cached pin exited %d:\nstderr:\n%s", run.exitCode(), run.stderr)
	}
	if got := strings.TrimSpace(run.stdout); got != f.cachedBDPath() {
		// The script prints PROJECT_ROOT as bash resolves it, which can differ
		// from Go's path through a symlinked temp dir (macOS /var). Compare by
		// suffix and confirm the file is the one that exists.
		want := filepath.ToSlash(filepath.Join("build", "docs-bd", resolvePinTag, "bd"))
		if !strings.HasSuffix(filepath.ToSlash(got), want) {
			t.Errorf("stdout = %q, want the cached binary %q", got, f.cachedBDPath())
		}
	}
}
