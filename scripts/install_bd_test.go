package scripts_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// installBdScript is the script under test, relative to the repository root
// that copyInstallBdScript copies it out of.
const installBdScript = "scripts/install-bd.sh"

// installBdFixture is one throwaway world for install-bd.sh: a temporary git
// repository holding a copy of the script, a directory standing in for the
// installed bd, and stub binaries for the build under test, the smoke command
// and the escalation.
//
// Every path it hands the script is inside a t.TempDir(). Nothing here is the
// host's ~/.local/bin, ~/go/bin, real bd, or the town's Dolt — this script
// replaces the data plane for every agent in a town, so a test that reached the
// real one by accident would be a town-wide outage, not a red test.
type installBdFixture struct {
	t          *testing.T
	bash       string
	repoDir    string // git repository the script runs in: its cwd and HEAD
	installDir string // INSTALL_DIR, holding the stand-in installed bd
	binDir     string // the smoke and escalate stubs
	stateDir   string // what those stubs record
	homeDir    string // HOME, so a default INSTALL_DIR can never be the real one
	smokeDir   string // SMOKE_DIR, which must never be the real town root
	newDir     string // BD_NEW, the freshly "built" binary

	commitA string // the fixture's first commit: what the installed bd reports
	branch  string // the branch that commit lives on
	goEdits int    // makes each goChange a distinct diff
}

type installBdRun struct {
	output string
	err    error
}

func (r installBdRun) exitCode() int {
	if r.err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(r.err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// installBdVersionJSON is the shape `bd version --json` prints. The three build
// stamps are what the script may ignore; version, db_schema_version,
// schema_ceiling and contract_version are the schema and contract surface it
// must not cross.
func installBdVersionJSON(commit, version string, schema int) string {
	return fmt.Sprintf(
		`{"branch":"main","build":"dev","build_id":%q,"commit":%q,"contract_version":1,"db_schema_version":%d,"schema_ceiling":{"ignored":0,"main":%d},"version":%q}`,
		commit, commit, schema, schema, version)
}

func newInstallBdFixture(t *testing.T, smokeFailOn int) *installBdFixture {
	t.Helper()

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("install-bd.sh is a bash script: %v", err)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("install-bd.sh reads the landing worktree's history: %v", err)
	}

	root := t.TempDir()
	f := &installBdFixture{
		t:          t,
		bash:       bash,
		repoDir:    filepath.Join(root, "repo"),
		installDir: filepath.Join(root, "install"),
		binDir:     filepath.Join(root, "bin"),
		stateDir:   filepath.Join(root, "state"),
		homeDir:    filepath.Join(root, "home"),
		smokeDir:   filepath.Join(root, "smoke"),
		newDir:     filepath.Join(root, "new"),
	}
	for _, dir := range []string{
		f.repoDir, filepath.Join(f.repoDir, "scripts"), f.installDir,
		f.binDir, f.stateDir, f.homeDir, f.smokeDir, f.newDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	f.copyScript()
	f.initRepo()
	f.writeSmokeStub(smokeFailOn)
	f.writeEscalateStub()
	return f
}

func (f *installBdFixture) copyScript() {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(sourceRepoRoot(f.t), filepath.FromSlash(installBdScript)))
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.repoDir, "scripts", "install-bd.sh"), data, 0o755); err != nil {
		f.t.Fatal(err)
	}
}

func (f *installBdFixture) initRepo() {
	f.t.Helper()
	f.git("init", "-q", ".")
	f.writeFile("go.mod", "module installbd.test\n\ngo 1.21\n")
	f.writeFile("go.sum", "")
	f.writeFile("main.go", "package main\n")
	f.git("add", "-A")
	f.commit("base")
	f.commitA = f.gitOut("rev-parse", "HEAD")
	f.branch = f.gitOut("symbolic-ref", "--short", "HEAD")
}

func (f *installBdFixture) git(args ...string) string {
	f.t.Helper()
	// The wrapper (scripts/ci/lib/test-env.sh) points HOME at an empty sandbox
	// and clears the global git config, so identity and signing have to be
	// supplied per command.
	prefix := []string{
		"-c", "user.name=install-bd test",
		"-c", "user.email=install-bd@example.invalid",
		"-c", "commit.gpgsign=false",
	}
	cmd := exec.Command("git", append(prefix, args...)...)
	cmd.Dir = f.repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitOut runs a git command but returns its stdout only, for the few calls
// whose informational stderr (an init hint, a detached-HEAD notice) is not an
// error and must not end up inside a hash.
func (f *installBdFixture) gitOut(args ...string) string {
	f.t.Helper()
	prefix := []string{
		"-c", "user.name=install-bd test",
		"-c", "user.email=install-bd@example.invalid",
		"-c", "commit.gpgsign=false",
	}
	cmd := exec.Command("git", append(prefix, args...)...)
	cmd.Dir = f.repoDir
	out, err := cmd.Output()
	if err != nil {
		f.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *installBdFixture) commit(message string) {
	f.t.Helper()
	f.git("commit", "-q", "-m", message)
}

func (f *installBdFixture) writeFile(name, body string) {
	f.t.Helper()
	path := filepath.Join(f.repoDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// goChange commits a change to a non-test Go file and returns the new HEAD:
// exactly the input that has to make the script install.
func (f *installBdFixture) goChange() string {
	f.t.Helper()
	f.goEdits++
	f.writeFile("main.go", fmt.Sprintf("package main\n\n// edit %d\n", f.goEdits))
	f.git("add", "-A")
	f.commit("go change")
	return f.gitOut("rev-parse", "HEAD")
}

// testOnlyChange commits a test file and a doc, and returns the new HEAD:
// nothing there can change the binary, so the script must install nothing.
func (f *installBdFixture) testOnlyChange() string {
	f.t.Helper()
	f.goEdits++
	f.writeFile(fmt.Sprintf("main_%d_test.go", f.goEdits), "package main\n")
	f.writeFile("README.md", "docs\n")
	f.git("add", "-A")
	f.commit("test and doc change")
	return f.gitOut("rev-parse", "HEAD")
}

// divergedCommit branches off the fixture's first commit, commits there, and
// returns to the fixture's branch. The result is in the repository but is not
// an ancestor of HEAD — a build someone made from a side branch.
func (f *installBdFixture) divergedCommit() string {
	f.t.Helper()
	f.git("checkout", "-q", "-b", "diverged", f.commitA)
	f.goEdits++
	f.writeFile("main.go", fmt.Sprintf("package main\n\n// divergent %d\n", f.goEdits))
	f.git("add", "-A")
	f.commit("divergent")
	commit := f.gitOut("rev-parse", "HEAD")
	f.git("checkout", "-q", f.branch)
	return commit
}

// writeFakeBD writes a stand-in bd: `version --json` prints json, any other
// invocation prints marker and exits smokeExit. The script runs nothing else
// against a bd, so anything else is a stub bug worth seeing.
func (f *installBdFixture) writeFakeBD(path, marker, json string, smokeExit int) {
	f.t.Helper()
	writeExecutable(f.t, path, fmt.Sprintf(`#!/bin/sh
case "$1 $2" in
  "version --json")
    printf '%%s\n' %s
    ;;
  *)
    printf '%%s\n' %s
    exit %d
    ;;
esac
`, shSingleQuote(json), shSingleQuote(marker), smokeExit))
}

func (f *installBdFixture) installedBDPath() string {
	return filepath.Join(f.installDir, "bd")
}

func (f *installBdFixture) newBDPath() string {
	return filepath.Join(f.newDir, "bd")
}

// writeInstalledBD stands in for the bd already on the host.
func (f *installBdFixture) writeInstalledBD(marker, json string) {
	f.t.Helper()
	f.writeFakeBD(f.installedBDPath(), marker, json, 0)
}

// writeNewBD stands in for the binary `make build` just left in the landing
// worktree.
func (f *installBdFixture) writeNewBD(marker, json string) {
	f.t.Helper()
	f.writeFakeBD(f.newBDPath(), marker, json, 0)
}

// writeSmokeStub writes the SMOKE_CMD stand-in. It records which bd it
// resolved — so a test can prove the pre-install smoke ran the new build and
// the post-install smoke ran the installed one — and fails from call failOn
// on, so a test can make one of the two fail without failing the other.
func (f *installBdFixture) writeSmokeStub(failOn int) {
	f.t.Helper()
	if failOn < 1 {
		failOn = 1
	}
	writeExecutable(f.t, filepath.Join(f.binDir, "smoke-probe"), fmt.Sprintf(`#!/bin/sh
count=0
if [ -s "$SMOKE_COUNT" ]; then IFS= read -r count <"$SMOKE_COUNT"; fi
count=$((count + 1))
printf '%%s\n' "$count" >"$SMOKE_COUNT"
printf '%%s\n' "$(command -v bd)" >>"$SMOKE_LOG"
if [ "$count" -ge %d ]; then
  printf 'smoke-probe: refusing on call %%s\n' "$count" >&2
  exit 1
fi
`, failOn))
}

// writeEscalateStub records the refusal message the script escalates with.
func (f *installBdFixture) writeEscalateStub() {
	f.t.Helper()
	writeExecutable(f.t, filepath.Join(f.binDir, "escalate-stub"), `#!/bin/sh
printf '%s\n' "$*" >>"$ESCALATE_LOG"
`)
}

func (f *installBdFixture) shellPath(path string) string {
	f.t.Helper()
	return shellPathUnderEnv(f.t, f.bash, path, shellPathEnv())
}

// shellFile resolves a path for the script's environment even when the file
// itself does not exist yet: shellPathUnderEnv chdirs to the path, so it can
// only convert directories, and stat'ing a not-yet-written file would send it
// looking for a directory that is not there.
func (f *installBdFixture) shellFile(path string) string {
	f.t.Helper()
	return f.shellPath(filepath.Dir(path)) + "/" + filepath.Base(path)
}

// run executes the script from the fixture's repository with a fully replaced
// environment, so no host variable or host PATH entry reaches it. overrides
// replace individual variables by name.
func (f *installBdFixture) run(overrides ...string) installBdRun {
	f.t.Helper()

	commandPath := f.shellPath(f.binDir) + ":" + bashPathList(f.t, os.Getenv("PATH")) + ":/usr/bin:/bin"
	if runtime.GOOS == "windows" {
		commandPath = f.shellPath(f.binDir) + ":/usr/bin:/bin"
	}

	env := []string{
		"PATH=" + commandPath,
		"HOME=" + f.shellPath(f.homeDir),
		"INSTALL_DIR=" + f.shellPath(f.installDir),
		"BD_NEW=" + f.shellFile(f.newBDPath()),
		"SMOKE_CMD=smoke-probe",
		"SMOKE_DIR=" + f.shellPath(f.smokeDir),
		"ESCALATE_CMD=" + shSingleQuote(f.shellFile(filepath.Join(f.binDir, "escalate-stub"))),
		"BD_BACKUPS=3",
		"SMOKE_COUNT=" + f.shellFile(filepath.Join(f.stateDir, "smoke-count")),
		"SMOKE_LOG=" + f.shellFile(filepath.Join(f.stateDir, "smoke-log")),
		"ESCALATE_LOG=" + f.shellFile(filepath.Join(f.stateDir, "escalate-log")),
		"LC_ALL=C",
		"LANG=C",
		"BASH_ENV=",
		"ENV=",
	}
	env = installBdWithOverrides(f.t, env, overrides)

	cmd := exec.Command(f.bash, "--noprofile", "--norc", installBdScript)
	cmd.Dir = f.repoDir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return installBdRun{output: string(out), err: err}
}

func installBdWithOverrides(t *testing.T, env []string, overrides []string) []string {
	t.Helper()
	out := append([]string{}, env...)
	for _, override := range overrides {
		key, _, ok := strings.Cut(override, "=")
		if !ok {
			t.Fatalf("override %q is not KEY=VALUE", override)
		}
		replaced := false
		for i, entry := range out {
			if strings.HasPrefix(entry, key+"=") {
				out[i] = override
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, override)
		}
	}
	return out
}

// requireExitZero asserts the script reported success. Every refusal is a
// success: post_land_command reads a non-zero exit as a failed landing and
// reverts the merge, so exiting non-zero over a refusal is itself the bug.
func (r installBdRun) requireExitZero(t *testing.T) {
	t.Helper()
	if code := r.exitCode(); code != 0 {
		t.Fatalf("install-bd.sh exited %d, want 0 (a refusal must still exit 0)\n%s", code, r.output)
	}
}

func (f *installBdFixture) installedBD(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.installedBDPath())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func (f *installBdFixture) backups(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(f.installDir, "bd.bak-*"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, filepath.Base(match))
	}
	return names
}

// stageLeftovers lists the sibling files a successful install must have
// renamed away, and a refused one must never have left behind.
func (f *installBdFixture) stageLeftovers(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(f.installDir, ".bd.new.*"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func (f *installBdFixture) lines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.stateDir, name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func (f *installBdFixture) escalations(t *testing.T) []string {
	t.Helper()
	return f.lines(t, "escalate-log")
}

func (f *installBdFixture) smokeLog(t *testing.T) []string {
	t.Helper()
	return f.lines(t, "smoke-log")
}

// oldBackup plants a backup as if an earlier install had made it.
func (f *installBdFixture) oldBackup(t *testing.T, name string, mtime time.Time) {
	t.Helper()
	path := filepath.Join(f.installDir, name)
	if err := os.WriteFile(path, []byte("old backup\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

// TestInstallBdScriptIsValidBash covers the one acceptance check the script
// cannot be trusted to make about itself. bash -n parses without running, so
// it catches a syntax slip that a refusal path might otherwise hide behind a
// case branch no test reached.
func TestInstallBdScriptIsValidBash(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skipf("install-bd.sh is a bash script: %v", err)
	}
	path := filepath.Join(sourceRepoRoot(t), filepath.FromSlash(installBdScript))

	out, err := exec.Command(bash, "-n", path).CombinedOutput()
	if err != nil {
		t.Fatalf("bash -n %s: %v\n%s", installBdScript, err, out)
	}

	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix executable bits")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("%s is not executable", installBdScript)
	}
}

// TestInstallBdInstallsNothingWhenNoGoSourceChanged pins the no-op: a landing
// that touched only tests and docs cannot have changed the binary, so the
// script must leave the installed bd alone and, since nothing is wrong, not
// escalate about it either.
func TestInstallBdInstallsNothingWhenNoGoSourceChanged(t *testing.T) {
	f := newInstallBdFixture(t, 999)
	f.writeInstalledBD("installed-v1", installBdVersionJSON(f.commitA, "1.2.2", 5))
	installed := f.installedBD(t)
	f.writeNewBD("new-v1", installBdVersionJSON(f.commitA, "1.2.2", 5))
	f.testOnlyChange()

	run := f.run()
	run.requireExitZero(t)

	if got := f.installedBD(t); got != installed {
		t.Errorf("installed bd changed; the script must install nothing\n%s", run.output)
	}
	if backups := f.backups(t); len(backups) != 0 {
		t.Errorf("backups = %v, want none: nothing was replaced", backups)
	}
	if escalations := f.escalations(t); len(escalations) != 0 {
		t.Errorf("escalations = %v, want none: an unchanged tree is not a refusal", escalations)
	}
	if !strings.Contains(run.output, "nothing to install") {
		t.Errorf("output does not report the no-op:\n%s", run.output)
	}
}

// TestInstallBdRefusesContractChange pins the schema gate: the version JSON is
// the handshake between a bd and the database it opens, so a difference beyond
// the build stamps means the town's Dolt would be handed to a binary that
// speaks a different language. Nothing may be installed, and a human has to
// decide — so the refusal escalates exactly once.
func TestInstallBdRefusesContractChange(t *testing.T) {
	tests := []struct {
		name          string
		installedJSON string
		newJSON       string
	}{
		{
			name:          "schema version",
			installedJSON: installBdVersionJSON("COMMIT", "1.2.2", 5),
			newJSON:       installBdVersionJSON("COMMIT", "1.2.2", 6),
		},
		{
			name:          "released version",
			installedJSON: installBdVersionJSON("COMMIT", "1.2.2", 5),
			newJSON:       installBdVersionJSON("COMMIT", "1.3.0", 5),
		},
		{
			name:          "json contract",
			installedJSON: strings.Replace(installBdVersionJSON("COMMIT", "1.2.2", 5), `"contract_version":1`, `"contract_version":2`, 1),
			newJSON:       installBdVersionJSON("COMMIT", "1.2.2", 5),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newInstallBdFixture(t, 999)
			f.writeInstalledBD("installed-v1", strings.ReplaceAll(test.installedJSON, "COMMIT", f.commitA))
			installed := f.installedBD(t)
			head := f.goChange()
			f.writeNewBD("new-v1", strings.ReplaceAll(test.newJSON, "COMMIT", head))

			run := f.run()
			run.requireExitZero(t)

			if got := f.installedBD(t); got != installed {
				t.Errorf("installed bd changed despite a contract difference\n%s", run.output)
			}
			if backups := f.backups(t); len(backups) != 0 {
				t.Errorf("backups = %v, want none", backups)
			}
			escalations := f.escalations(t)
			if len(escalations) != 1 {
				t.Fatalf("escalations = %d, want exactly 1:\n%v\n%s", len(escalations), escalations, run.output)
			}
			if !strings.Contains(escalations[0], "schema or contract change") {
				t.Errorf("escalation does not name the reason: %q", escalations[0])
			}
		})
	}
}

// TestInstallBdRefusesInstalledCommitOutsideHEAD pins the forward-only check.
// Installing a build whose base is not behind HEAD is a downgrade or a
// divergence — the shape that put gastown's gt binary into a rebuild crash
// loop — and for bd it would replace the data plane under every agent.
func TestInstallBdRefusesInstalledCommitOutsideHEAD(t *testing.T) {
	t.Run("diverged", func(t *testing.T) {
		f := newInstallBdFixture(t, 999)
		diverged := f.divergedCommit()
		f.writeInstalledBD("installed-v1", installBdVersionJSON(diverged, "1.2.2", 5))
		installed := f.installedBD(t)
		head := f.goChange()
		f.writeNewBD("new-v1", installBdVersionJSON(head, "1.2.2", 5))

		run := f.run()
		run.requireExitZero(t)

		if got := f.installedBD(t); got != installed {
			t.Errorf("installed bd changed although its base is not an ancestor of HEAD\n%s", run.output)
		}
		escalations := f.escalations(t)
		if len(escalations) != 1 {
			t.Fatalf("escalations = %d, want exactly 1:\n%v\n%s", len(escalations), escalations, run.output)
		}
		if !strings.Contains(escalations[0], "not an ancestor of HEAD") {
			t.Errorf("escalation does not name the reason: %q", escalations[0])
		}
		if got := f.smokeLog(t); len(got) != 0 {
			t.Errorf("smoke ran before the ancestry check passed: %v", got)
		}
	})

	t.Run("unrelated commit", func(t *testing.T) {
		f := newInstallBdFixture(t, 999)
		f.writeInstalledBD("installed-v1", installBdVersionJSON("1111111111111111111111111111111111111111", "1.2.2", 5))
		installed := f.installedBD(t)
		head := f.goChange()
		f.writeNewBD("new-v1", installBdVersionJSON(head, "1.2.2", 5))

		run := f.run()
		run.requireExitZero(t)

		if got := f.installedBD(t); got != installed {
			t.Errorf("installed bd changed although its base is unknown to this repository\n%s", run.output)
		}
		if escalations := f.escalations(t); len(escalations) != 1 {
			t.Fatalf("escalations = %v, want exactly 1\n%s", escalations, run.output)
		}
	})
}

// TestInstallBdRefusesWithoutAnInstalledBaseline covers the first check: with
// no installed bd, or one that cannot say what it was built from, there is
// nothing to compare against and nothing safe to replace.
func TestInstallBdRefusesWithoutAnInstalledBaseline(t *testing.T) {
	t.Run("no installed binary", func(t *testing.T) {
		f := newInstallBdFixture(t, 999)
		head := f.goChange()
		f.writeNewBD("new-v1", installBdVersionJSON(head, "1.2.2", 5))

		run := f.run()
		run.requireExitZero(t)
		if escalations := f.escalations(t); len(escalations) != 1 {
			t.Fatalf("escalations = %v, want exactly 1\n%s", escalations, run.output)
		}
	})

	t.Run("installed binary without a commit", func(t *testing.T) {
		f := newInstallBdFixture(t, 999)
		f.writeInstalledBD("installed-v1", installBdVersionJSON("", "1.2.2", 5))
		installed := f.installedBD(t)
		head := f.goChange()
		f.writeNewBD("new-v1", installBdVersionJSON(head, "1.2.2", 5))

		run := f.run()
		run.requireExitZero(t)
		if got := f.installedBD(t); got != installed {
			t.Errorf("installed bd changed although it reports no commit\n%s", run.output)
		}
		if escalations := f.escalations(t); len(escalations) != 1 {
			t.Fatalf("escalations = %v, want exactly 1\n%s", escalations, run.output)
		}
	})
}

// TestInstallBdInstallsAndPrunesBackups is the happy path, and the only proof
// that the swap is real: the installed file is byte-for-byte the build, the
// previous binary survives as bd.bak-<commit>-<date>, the staging file is gone
// (a leftover would mean the rename never happened), and only the newest
// BD_BACKUPS backups are kept.
func TestInstallBdInstallsAndPrunesBackups(t *testing.T) {
	f := newInstallBdFixture(t, 999)
	f.writeInstalledBD("installed-v1", installBdVersionJSON(f.commitA, "1.2.2", 5))
	installed := f.installedBD(t)
	head := f.goChange()
	f.writeNewBD("new-v1", installBdVersionJSON(head, "1.2.2", 5))
	newBD := f.installedBDBytes(t, f.newBDPath())

	base := time.Now().Add(-72 * time.Hour)
	f.oldBackup(t, "bd.bak-cafe0001-20240101", base)
	f.oldBackup(t, "bd.bak-cafe0002-20240102", base.Add(time.Hour))
	f.oldBackup(t, "bd.bak-cafe0003-20240103", base.Add(2*time.Hour))
	f.oldBackup(t, "bd.bak-cafe0004-20240104", base.Add(3*time.Hour))

	run := f.run()
	run.requireExitZero(t)

	if got := f.installedBDBytes(t, f.installedBDPath()); string(got) != string(newBD) {
		t.Errorf("installed bd is not the build that landed\n%s", run.output)
	}
	if leftovers := f.stageLeftovers(t); len(leftovers) != 0 {
		t.Errorf("staging files left behind: %v", leftovers)
	}
	if escalations := f.escalations(t); len(escalations) != 0 {
		t.Errorf("escalations = %v, want none on a successful install", escalations)
	}

	fresh := fmt.Sprintf("bd.bak-%s-%s", f.commitA, time.Now().Format("20060102"))
	backupPath := filepath.Join(f.installDir, fresh)
	if got, err := os.ReadFile(backupPath); err != nil {
		t.Fatalf("backup %s: %v", fresh, err)
	} else if string(got) != installed {
		t.Errorf("backup %s does not hold the previous binary", fresh)
	}

	// BD_BACKUPS is 3: the fresh backup plus the two newest old ones.
	want := map[string]bool{fresh: true, "bd.bak-cafe0003-20240103": true, "bd.bak-cafe0004-20240104": true}
	got := f.backups(t)
	if len(got) != len(want) {
		t.Fatalf("backups = %v, want %d of %v", got, len(want), want)
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected surviving backup %s (want %v)", name, want)
		}
	}

	// The pre-install smoke must have run the build, the post-install smoke the
	// installed file: that ordering is what makes each of them mean anything.
	smoke := f.smokeLog(t)
	if len(smoke) != 2 {
		t.Fatalf("smoke ran %d times, want 2: %v", len(smoke), smoke)
	}
	if !strings.HasSuffix(smoke[0], f.shellFile(f.newBDPath())) {
		t.Errorf("pre-install smoke resolved %q, want the build at %q", smoke[0], f.newBDPath())
	}
	if !strings.HasSuffix(smoke[1], f.shellFile(f.installedBDPath())) {
		t.Errorf("post-install smoke resolved %q, want the installed bd at %q", smoke[1], f.installedBDPath())
	}
}

func (f *installBdFixture) installedBDBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestInstallBdRefusesWhenPreInstallSmokeFails pins the pre-install smoke: a
// build that cannot answer a read-only query must never reach the installed
// path. The installed bd is untouched, and no backup is written, because
// nothing was replaced.
func TestInstallBdRefusesWhenPreInstallSmokeFails(t *testing.T) {
	f := newInstallBdFixture(t, 1)
	f.writeInstalledBD("installed-v1", installBdVersionJSON(f.commitA, "1.2.2", 5))
	installed := f.installedBD(t)
	head := f.goChange()
	f.writeNewBD("new-v1", installBdVersionJSON(head, "1.2.2", 5))

	run := f.run()
	run.requireExitZero(t)

	if got := f.installedBD(t); got != installed {
		t.Errorf("installed bd changed although the build failed the smoke command\n%s", run.output)
	}
	if backups := f.backups(t); len(backups) != 0 {
		t.Errorf("backups = %v, want none: nothing was replaced", backups)
	}
	if leftovers := f.stageLeftovers(t); len(leftovers) != 0 {
		t.Errorf("staging files left behind: %v", leftovers)
	}
	escalations := f.escalations(t)
	if len(escalations) != 1 {
		t.Fatalf("escalations = %d, want exactly 1:\n%v\n%s", len(escalations), escalations, run.output)
	}
	if !strings.Contains(escalations[0], "failed the smoke command") {
		t.Errorf("escalation does not name the reason: %q", escalations[0])
	}
}

// TestInstallBdRollsBackWhenPostInstallSmokeFails pins the last line of
// defence. The install has already happened, so the town is running a binary
// that fails its own smoke check; the previous one has to come back before
// anyone notices, and the escalation has to say that it did.
func TestInstallBdRollsBackWhenPostInstallSmokeFails(t *testing.T) {
	f := newInstallBdFixture(t, 2)
	f.writeInstalledBD("installed-v1", installBdVersionJSON(f.commitA, "1.2.2", 5))
	installed := f.installedBD(t)
	head := f.goChange()
	f.writeNewBD("new-v1", installBdVersionJSON(head, "1.2.2", 5))

	run := f.run()
	run.requireExitZero(t)

	if got := f.installedBD(t); got != installed {
		t.Fatalf("installed bd was not rolled back to the previous binary\n%s", run.output)
	}
	if leftovers := f.stageLeftovers(t); len(leftovers) != 0 {
		t.Errorf("staging files left behind: %v", leftovers)
	}
	// The rollback moves the backup back, so the fresh backup is gone. Any
	// other backup is a leftover from another test's fixture and means the
	// script wrote outside its own directory.
	if backups := f.backups(t); len(backups) != 0 {
		t.Errorf("backups = %v, want none: the fresh one was consumed by the rollback", backups)
	}
	escalations := f.escalations(t)
	if len(escalations) != 1 {
		t.Fatalf("escalations = %d, want exactly 1:\n%v\n%s", len(escalations), escalations, run.output)
	}
	if !strings.Contains(escalations[0], "restored") {
		t.Errorf("escalation does not report the rollback: %q", escalations[0])
	}

	// The second smoke has to have been the one that failed: it is the
	// post-install run of the installed file that the rollback answers.
	smoke := f.smokeLog(t)
	if len(smoke) != 2 {
		t.Fatalf("smoke ran %d times, want 2: %v", len(smoke), smoke)
	}
	if !strings.HasSuffix(smoke[1], f.shellFile(f.installedBDPath())) {
		t.Errorf("post-install smoke resolved %q, want the installed bd at %q", smoke[1], f.installedBDPath())
	}
}
