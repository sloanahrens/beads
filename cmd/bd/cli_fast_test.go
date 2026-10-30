//go:build cgo && integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/steveyegge/beads/internal/git"
	"github.com/steveyegge/beads/internal/storage/dolt"
)

// Fast CLI tests converted from scripttest suite
// These use in-process testing (calling rootCmd.Execute directly) for speed
// A few tests still use exec.Command for end-to-end validation
//
// Performance improvement (bd-ky74):
//   - Before: exec.Command() tests took 2-4 seconds each (~40s total)
//   - After: in-process tests take <1 second each, ~10x faster
//   - End-to-end test (TestCLI_EndToEnd) still validates binary with exec.Command

var (
	inProcessMutex sync.Mutex // Protects concurrent access to rootCmd and global state
)

// setupCLITestDB creates a fresh bd workspace whose data lives in a Dolt
// database of its own on the test server.
//
// In server mode the data lives on the shared Dolt server under the database
// name in metadata.json, so a workspace initialized once and then reused points
// every test at the same database and tests see each other's issues. Each call
// therefore initializes a new workspace against a database whose name carries a
// fresh random suffix (newCLITestDatabaseName), and drops exactly that database
// in cleanup, so each test starts from and leaves an empty database of its own.
//
// The workspace is initialized instead of copied from a template because bd
// init refuses a workspace that already holds a database (checkExistingBeadsData
// in init.go), so in server mode there is nothing to save by copying: the
// database work is the cost, and any fix pays one bd init per test.
func setupCLITestDB(t *testing.T) string {
	t.Helper()
	return setupCLITestDBFor(t, t.Name())
}

// setupCLITestDBFor is setupCLITestDB with the database named for name instead
// of the test name, for a test that needs more than one isolated workspace. Two
// calls never share a database, even with the same name.
func setupCLITestDBFor(t *testing.T, name string) string {
	t.Helper()
	if testDoltServerPort == 0 {
		t.Skip("skipping: Dolt test container not available")
	}
	dbName := newCLITestDatabaseName(t, name)
	tmpDir := createTempDirWithCleanup(t)
	initCLITestDatabase(t, tmpDir, dbName)
	// Registered only after init succeeds, so the drop names a database this
	// call created. If init reported that the name already existed, the test
	// stops before this line and nothing is dropped. A database init left
	// half-built is swept by the cliTestDatabaseNamePrefix sweep.
	t.Cleanup(func() {
		if err := dropDatabaseOnServer(dbName, testDoltServerPort, false); err != nil {
			t.Errorf("Failed to drop test database %s: %v", dbName, err)
		}
	})
	return tmpDir
}

const (
	// cliTestDatabaseNameMaxLen is the longest name dolt.ValidateDatabaseName accepts.
	cliTestDatabaseNameMaxLen = 64
	// cliTestDatabaseNamePrefix keeps per-test databases inside the namespace
	// the storage layer reserves for tests (testDatabasePrefixes in
	// internal/storage/dolt). bd init refuses to create a database on a server
	// that already serves others unless the name is reserved for tests or the
	// caller opts in with --allow-new-database, and bd dolt clean-databases
	// sweeps leftovers by this prefix.
	cliTestDatabaseNamePrefix = "testdb_clitest_"
	// cliTestDatabaseSuffixBytes is the random part of a database name. Eight
	// bytes is 16 hex digits: 64 bits, so runs sharing one server do not collide.
	cliTestDatabaseSuffixBytes = 8
	// cliTestInitTimeout bounds one bd init subprocess, so a hung server fails
	// the test instead of stalling the whole run.
	cliTestInitTimeout = 2 * time.Minute
)

// newCLITestDatabaseName returns a database name for one setupCLITestDB call.
// The suffix comes from crypto/rand, so no two calls share a name, including
// two calls for the same test and calls from concurrent runs on one server.
func newCLITestDatabaseName(t *testing.T, testName string) string {
	t.Helper()
	raw := make([]byte, cliTestDatabaseSuffixBytes)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("Failed to generate a test database suffix: %v", err)
	}
	return cliTestDatabaseName(testName, hex.EncodeToString(raw))
}

// cliTestDatabaseName builds a database name from a test name and a suffix. The
// result is valid for dolt.ValidateDatabaseName and is in the test-database
// namespace. The same inputs give the same name; a different suffix gives a
// different name even for the same test. A test name too long for the limit is
// cut, so two long names that share a prefix need different suffixes to differ.
func cliTestDatabaseName(testName, suffix string) string {
	readable := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, testName)
	maxReadable := cliTestDatabaseNameMaxLen - len(cliTestDatabaseNamePrefix) - 1 - len(suffix)
	if len(readable) > maxReadable {
		readable = readable[:maxReadable]
	}
	return cliTestDatabaseNamePrefix + readable + "_" + suffix
}

// workspaceEnvVars are the variables that point bd at a workspace or database
// other than the one in the working directory.
var workspaceEnvVars = []string{"BEADS_DIR", "BEADS_DB", "BD_DB"}

// withoutWorkspaceEnv returns environ without the workspace variables, so a
// value an earlier in-process run left behind cannot reach a bd init child.
func withoutWorkspaceEnv(environ []string) []string {
	kept := make([]string, 0, len(environ))
	for _, kv := range environ {
		drop := false
		for _, key := range workspaceEnvVars {
			if strings.HasPrefix(kv, key+"=") {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, kv)
		}
	}
	return kept
}

// TestWithoutWorkspaceEnvDropsTheWorkspaceVariables checks that only the
// variables that name a workspace are removed.
func TestWithoutWorkspaceEnvDropsTheWorkspaceVariables(t *testing.T) {
	in := []string{"BEADS_DIR=/gone/.beads", "PATH=/usr/bin", "BEADS_DB=/gone/db", "BD_DB=/gone/db"}
	got := withoutWorkspaceEnv(in)
	if len(got) != 1 || got[0] != "PATH=/usr/bin" {
		t.Errorf("withoutWorkspaceEnv(%q) = %q, want only PATH", in, got)
	}
}

// TestRunBDInProcessRestoresTheWorkspaceVariables checks that a run leaves the
// workspace variables as it found them: a set value comes back, an unset
// variable stays unset. It goes red if the restore in runBDInProcess is removed.
func TestRunBDInProcessRestoresTheWorkspaceVariables(t *testing.T) {
	t.Setenv("BEADS_DIR", "/restore/me/.beads")
	t.Setenv("BEADS_DB", "/restore/me/db")
	os.Unsetenv("BD_DB")

	runBDInProcess(t, t.TempDir(), "version")

	if got := os.Getenv("BEADS_DIR"); got != "/restore/me/.beads" {
		t.Errorf("BEADS_DIR after the run = %q, want it restored", got)
	}
	if got := os.Getenv("BEADS_DB"); got != "/restore/me/db" {
		t.Errorf("BEADS_DB after the run = %q, want it restored", got)
	}
	if v, ok := os.LookupEnv("BD_DB"); ok {
		t.Errorf("BD_DB after the run = %q, want it still unset", v)
	}
}

// TestRunBDInProcessAllowErrorRestoresTheWorkspaceVariables is the check above
// for the runner that does not fail on error. It goes red if the restore is
// removed from runBDInProcessAllowError.
func TestRunBDInProcessAllowErrorRestoresTheWorkspaceVariables(t *testing.T) {
	t.Setenv("BEADS_DIR", "/restore/me/.beads")
	t.Setenv("BEADS_DB", "/restore/me/db")
	os.Unsetenv("BD_DB")

	runBDInProcessAllowError(t, t.TempDir(), "version")

	if got := os.Getenv("BEADS_DIR"); got != "/restore/me/.beads" {
		t.Errorf("BEADS_DIR after the run = %q, want it restored", got)
	}
	if got := os.Getenv("BEADS_DB"); got != "/restore/me/db" {
		t.Errorf("BEADS_DB after the run = %q, want it restored", got)
	}
	if v, ok := os.LookupEnv("BD_DB"); ok {
		t.Errorf("BD_DB after the run = %q, want it still unset", v)
	}
}

// TestRunBDInProcessClearsFlagsAfterTheRun checks that a run leaves no flag
// armed for the next one: --json is set by this run and must read as unset
// once runBDInProcess returns. It goes red if the flag reset after Execute is
// removed.
func TestRunBDInProcessClearsFlagsAfterTheRun(t *testing.T) {
	runBDInProcess(t, t.TempDir(), "version", "--json")

	flag := rootCmd.PersistentFlags().Lookup("json")
	if flag == nil {
		t.Fatal("rootCmd has no persistent --json flag")
	}
	if flag.Changed {
		t.Errorf("--json still marked Changed after the run")
	}
	if got := flag.Value.String(); got != "false" {
		t.Errorf("--json after the run = %q, want the default false", got)
	}
}

// TestResetCommandFlagsRestoresDefaults checks that resetCommandTreeFlags puts
// a string flag, a bool flag and string-slice flags back to their defaults,
// clears Changed on each, and walks into subcommands. It goes red if the reset
// stops restoring a flag, or if a slice flag is left holding the values set
// before the reset.
func TestResetCommandFlagsRestoresDefaults(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	sub := &cobra.Command{Use: "sub"}
	root.AddCommand(sub)
	sub.Flags().String("title", "draft", "")
	sub.Flags().Bool("force", false, "")
	sub.Flags().StringSlice("label", nil, "")
	sub.Flags().StringSlice("ids", []string{"one", "two"}, "")

	for name, value := range map[string]string{"title": "final", "force": "true", "label": "x"} {
		if err := sub.Flags().Set(name, value); err != nil {
			t.Fatalf("Set(%s) = %v", name, err)
		}
	}
	if err := sub.Flags().Set("ids", "three"); err != nil {
		t.Fatalf("Set(ids) = %v", err)
	}

	if err := resetCommandTreeFlags(root); err != nil {
		t.Fatalf("resetCommandTreeFlags() = %v", err)
	}

	if got := sub.Flags().Lookup("title").Value.String(); got != "draft" {
		t.Errorf("title after reset = %q, want the default %q", got, "draft")
	}
	if got := sub.Flags().Lookup("force").Value.String(); got != "false" {
		t.Errorf("force after reset = %q, want the default false", got)
	}
	if got := sub.Flags().Lookup("ids").Value.String(); got != "[one,two]" {
		t.Errorf("ids after reset = %q, want the default [one,two]", got)
	}
	for _, name := range []string{"title", "force", "label", "ids"} {
		if sub.Flags().Lookup(name).Changed {
			t.Errorf("%s still marked Changed after reset", name)
		}
	}

	// A slice flag that was set before the reset must take a new value alone,
	// not append it to the old one.
	if err := sub.Flags().Set("label", "y"); err != nil {
		t.Fatalf("Set(label) = %v", err)
	}
	if got := sub.Flags().Lookup("label").Value.String(); got != "[y]" {
		t.Errorf("label after reset and a new Set = %q, want [y]", got)
	}
}

// initCLITestDatabase initializes dir as a bd workspace whose data lives in
// dbName on the test Dolt server.
//
// --database names the database explicitly. It is the supported way to pick a
// server database, and it is what init records in metadata.json, so the
// commands the test runs afterwards resolve to the same database. The flags
// mirror the server-init pattern other tests in this package use
// (init_realstore_integration_test.go).
func initCLITestDatabase(t *testing.T, dir, dbName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cliTestInitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, testBD, "init",
		"--server", "--external",
		"--server-host", "127.0.0.1",
		"--server-port", strconv.Itoa(testDoltServerPort),
		"--database", dbName,
		"--prefix", "test", "--quiet", "--skip-hooks", "--skip-agents")
	cmd.Dir = dir
	cmd.Env = withoutWorkspaceEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("bd init for database %s did not finish within %v\n%s", dbName, cliTestInitTimeout, out)
	}
	if err != nil {
		t.Fatalf("bd init for database %s failed: %v\n%s", dbName, err, out)
	}
}

// TestUniqueCLITestDatabaseName checks the name rules without a Dolt server:
// every name is valid for the server and in the test namespace, the same test
// name and suffix give the same name, and a different test name or a different
// suffix gives a different one.
func TestUniqueCLITestDatabaseName(t *testing.T) {
	const suffixA = "0123456789abcdef"
	const suffixB = "fedcba9876543210"
	long := strings.Repeat("TestVeryLongName", 10)
	tests := []struct {
		name             string
		a, b             string
		suffixA, suffixB string
		wantEqual        bool
	}{
		{"different test names differ", "TestCLI_List", "TestCLI_Create", suffixA, suffixA, false},
		{"same test name and suffix match", "TestCLI_List", "TestCLI_List", suffixA, suffixA, true},
		{"same test name with a new suffix differs", "TestCLI_List", "TestCLI_List", suffixA, suffixB, false},
		{"long names differ by suffix", long, long, suffixA, suffixB, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotA := cliTestDatabaseName(tc.a, tc.suffixA)
			gotB := cliTestDatabaseName(tc.b, tc.suffixB)
			if (gotA == gotB) != tc.wantEqual {
				t.Fatalf("cliTestDatabaseName(%q, %q) = %q, cliTestDatabaseName(%q, %q) = %q; equal = %v, want %v",
					tc.a, tc.suffixA, gotA, tc.b, tc.suffixB, gotB, gotA == gotB, tc.wantEqual)
			}
			for _, got := range []string{gotA, gotB} {
				if err := dolt.ValidateDatabaseName(got); err != nil {
					t.Fatalf("database name %q is invalid: %v", got, err)
				}
				// bd init only creates a database on a server that already
				// serves others when the name is reserved for tests; see
				// cliTestDatabaseNamePrefix.
				if !strings.HasPrefix(got, "testdb_") {
					t.Fatalf("database name %q is not in the test-database namespace, so bd init would refuse to create it on a populated server", got)
				}
			}
		})
	}
}

// TestSetupCLITestDBIsolatesDatabases proves on the live test server that
// setupCLITestDB does not hand two tests the same database: an issue created
// through one workspace is invisible through the other. This is the regression
// be-34sb fixes (TestCLI_List saw the issues three earlier TestCLI_ tests had
// created).
func TestSetupCLITestDBIsolatesDatabases(t *testing.T) {
	if testDoltServerPort == 0 {
		t.Skip("skipping: Dolt test container not available")
	}
	first := setupCLITestDBFor(t, t.Name()+"/first")
	second := setupCLITestDBFor(t, t.Name()+"/second")

	runBDInProcess(t, first, "create", "only in first", "-p", "1")

	out := runBDInProcess(t, second, "list", "--json")
	var issues []map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &issues); err != nil {
		t.Fatalf("Failed to parse list output: %v\nOutput: %s", err, out)
	}
	if len(issues) != 0 {
		t.Fatalf("second workspace sees %d issue(s) created in the first; each setupCLITestDB must own its database", len(issues))
	}

	// The first workspace must hold the issue, or the empty list above proves
	// nothing but a create that silently failed.
	out = runBDInProcess(t, first, "list", "--json")
	issues = nil
	if err := unmarshalEnvelope([]byte(out), &issues); err != nil {
		t.Fatalf("Failed to parse list output: %v\nOutput: %s", err, out)
	}
	if len(issues) != 1 {
		t.Fatalf("first workspace lists %d issue(s), want 1", len(issues))
	}
}

// createTempDirWithCleanup creates a temp directory with non-fatal cleanup
// This prevents test failures from SQLite file lock cleanup issues
func createTempDirWithCleanup(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "bd-cli-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	t.Cleanup(func() {
		// Don't fail the test if cleanup fails - just log it
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("Warning: Failed to clean up temp dir %s: %v", tmpDir, err)
		}
	})

	return tmpDir
}

// runBDInProcess runs bd commands in-process by calling rootCmd.Execute
// This is ~10-20x faster than exec.Command because it avoids process spawn overhead
func runBDInProcess(t *testing.T, dir string, args ...string) string {
	t.Helper()

	// Serialize all in-process test execution to avoid race conditions
	// rootCmd, cobra state, and viper are not thread-safe
	inProcessMutex.Lock()
	defer inProcessMutex.Unlock()

	// Save original state
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	oldDir, _ := os.Getwd()
	oldArgs := os.Args
	// rootCmd's pre-run sets BEADS_DIR for the workspace it resolves and never
	// restores it; unset the workspace variables for this run and put them back.
	for _, key := range workspaceEnvVars {
		if v, ok := os.LookupEnv(key); ok {
			defer os.Setenv(key, v)
		} else {
			defer os.Unsetenv(key)
		}
		os.Unsetenv(key)
	}

	// Change to test directory
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Failed to chdir to %s: %v", dir, err)
	}
	// The git context is cached per process; an earlier run in another
	// directory must not decide where this one's gate roots point.
	git.ResetCaches()

	// Capture stdout/stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	// Set args for rootCmd
	rootCmd.SetArgs(args)
	os.Args = append([]string{"bd"}, args...)

	// Execute command
	err := rootCmd.Execute()
	if resetErr := resetCommandTreeFlags(rootCmd); resetErr != nil {
		t.Errorf("resetting bd flags after the run: %v", resetErr)
	}

	// Close and clean up all global state to prevent contamination between tests
	if store != nil {
		store.Close()
		store = nil
	}
	// Reset all global flags and state
	dbPath = ""
	actor = ""
	jsonOutput = false
	sandboxMode = false
	// Reset context state
	rootCtx = nil
	rootCancel = nil

	// Close writers and restore
	wOut.Close()
	wErr.Close()
	os.Chdir(oldDir)
	os.Args = oldArgs
	rootCmd.SetArgs(nil)

	// Read output (keep stdout and stderr separate)
	var outBuf, errBuf bytes.Buffer
	outBuf.ReadFrom(rOut)
	errBuf.ReadFrom(rErr)

	stdout := outBuf.String()
	stderr := errBuf.String()

	if err != nil {
		t.Fatalf("bd %v failed: %v\nStdout: %s\nStderr: %s", args, err, stdout, stderr)
	}

	// Return only stdout (stderr contains warnings that break JSON parsing)
	return stdout
}

func TestCLI_Ready(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	runBDInProcess(t, tmpDir, "create", "Ready issue", "-p", "1")
	out := runBDInProcess(t, tmpDir, "ready")
	if !strings.Contains(out, "Ready issue") {
		t.Errorf("Expected 'Ready issue' in output, got: %s", out)
	}
}

func TestCLI_Create(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Test issue", "-p", "1", "--json")

	// Extract JSON from output (may contain warnings before JSON)
	jsonStart := strings.Index(out, "{")
	if jsonStart == -1 {
		t.Fatalf("No JSON found in output: %s", out)
	}
	jsonOut := out[jsonStart:]

	var result map[string]interface{}
	if err := unmarshalEnvelope([]byte(jsonOut), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, jsonOut)
	}
	if result["title"] != "Test issue" {
		t.Errorf("Expected title 'Test issue', got: %v", result["title"])
	}
}

// TestCLI_CreateActorPrecedence is a CLI-level regression test for GH#4645:
// with both BEADS_ACTOR and the deprecated BD_ACTOR set, `bd create` must
// stamp created_by with BEADS_ACTOR's value. Unlike TestResolveConfiguredActor
// (which calls resolveConfiguredActor() directly), this drives the real
// command through rootCmd.Execute() so it also covers the root
// PersistentPreRunE / refreshBoundCommandConfig wiring that pre-populates the
// global actor before create.go ever runs — reverting either call site would
// leave TestResolveConfiguredActor green but this test red.
func TestCLI_CreateActorPrecedence(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	t.Setenv("BD_ACTOR", "from-bd-actor")
	t.Setenv("BEADS_ACTOR", "from-beads-actor")

	out := runBDInProcess(t, tmpDir, "create", "Actor precedence test", "-p", "1", "--json")

	jsonStart := strings.Index(out, "{")
	if jsonStart == -1 {
		t.Fatalf("No JSON found in output: %s", out)
	}
	jsonOut := out[jsonStart:]

	var result map[string]interface{}
	if err := unmarshalEnvelope([]byte(jsonOut), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, jsonOut)
	}
	if result["created_by"] != "from-beads-actor" {
		t.Errorf("created_by = %v, want %q (BEADS_ACTOR must outrank deprecated BD_ACTOR)", result["created_by"], "from-beads-actor")
	}
}

func TestCLI_List(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	runBDInProcess(t, tmpDir, "create", "First", "-p", "1")
	runBDInProcess(t, tmpDir, "create", "Second", "-p", "2")

	out := runBDInProcess(t, tmpDir, "list", "--json")
	var issues []map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &issues); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if len(issues) != 2 {
		t.Errorf("Expected 2 issues, got %d", len(issues))
	}
}

func TestCLI_Update(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Issue to update", "-p", "1", "--json")

	var issue map[string]interface{}
	unmarshalEnvelope([]byte(out), &issue)
	id := issue["id"].(string)

	runBDInProcess(t, tmpDir, "update", id, "--status", "in_progress")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var updated []map[string]interface{}
	unmarshalEnvelope([]byte(out), &updated)
	if updated[0]["status"] != "in_progress" {
		t.Errorf("Expected status 'in_progress', got: %v", updated[0]["status"])
	}
}

// TestCLI_UpdateAcceptanceSyncsDescriptionSection is the regression for the
// spec-dispatcher shape: a bead whose criteria are a '## Acceptance' section
// of its description, with nothing in the acceptance_criteria column. The
// write has to reach the section, because that is what a reader of bd show
// sees as the criteria; writing only the column left the '- [ ]' lines in
// place and made the update look applied when nothing visible had moved.
func TestCLI_UpdateAcceptanceSyncsDescriptionSection(t *testing.T) {
	tmpDir := setupCLITestDB(t)
	description := "## Goal\n\nclose the gap\n\n## Acceptance\n\n- [ ] make check pass\n"
	out := runBDInProcess(t, tmpDir, "create", "Spec-shaped bead", "--description", description, "--json")

	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &issue); err != nil {
		t.Fatalf("Failed to parse create output: %v", err)
	}
	id := issue["id"].(string)

	runBDInProcess(t, tmpDir, "update", id, "--acceptance", "- [x] make check pass")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var updated []map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &updated); err != nil {
		t.Fatalf("Failed to parse show output: %v", err)
	}
	if got := updated[0]["acceptance_criteria"]; got != "- [x] make check pass" {
		t.Errorf("acceptance_criteria = %v, want the criteria that were written", got)
	}
	rewritten, _ := updated[0]["description"].(string)
	if !strings.Contains(rewritten, "- [x] make check pass") || strings.Contains(rewritten, "- [ ]") {
		t.Errorf("description's acceptance section was not updated:\n%s", rewritten)
	}
}

// TestCLI_UpdateAcceptanceRefusesAmbiguousDescription is the other half of the
// same regression: an acceptance write that cannot be applied (here, a
// description naming two acceptance sections and so no single place to write)
// must exit nonzero and change nothing, not print success.
func TestCLI_UpdateAcceptanceRefusesAmbiguousDescription(t *testing.T) {
	tmpDir := setupCLITestDB(t)
	description := "## Acceptance\n\n- [ ] one\n\n## Acceptance Criteria\n\n- [ ] two\n"
	out := runBDInProcess(t, tmpDir, "create", "Two acceptance sections", "--description", description, "--json")

	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &issue); err != nil {
		t.Fatalf("Failed to parse create output: %v", err)
	}
	id := issue["id"].(string)

	stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "update", id, "--acceptance", "- [x] one")
	if err == nil {
		t.Fatalf("expected the ambiguous acceptance write to fail, stdout=%q stderr=%q", stdout, stderr)
	}

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var updated []map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &updated); err != nil {
		t.Fatalf("Failed to parse show output: %v", err)
	}
	if got, ok := updated[0]["acceptance_criteria"]; ok {
		t.Errorf("refused write still stored acceptance_criteria = %v", got)
	}
	if updated[0]["description"] != description {
		t.Errorf("refused write changed the description:\n%v", updated[0]["description"])
	}
}

// resetCommandFlag restores one of a command's flags to its default, now and
// again when the test ends.
//
// runBDInProcess drives rootCmd.Execute inside the test process, and cobra
// keeps a flag's value and Changed bit across Execute calls. Without this, a
// test that passes --acceptance leaves the acceptance write armed for every
// later create in the package, which is how
// TestCLI_CreateWithoutAcceptanceLeavesDescriptionAlone first failed.
func resetCommandFlag(t *testing.T, cmd *cobra.Command, name string) {
	t.Helper()
	reset := func() {
		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			return
		}
		flag.Changed = false
		_ = flag.Value.Set(flag.DefValue)
	}
	reset()
	t.Cleanup(reset)
}

// resetCommandTreeFlags puts every flag in cmd's tree back to its default and
// clears its Changed bit, so a value one in-process run set cannot reach the
// next run. resetCommandFlag does the same for one flag.
//
// A slice flag is restored with Replace, since Set appends once a slice flag
// has been set. Replace does not clear that state, so the next Set appends to
// the default. Every slice flag in bd's commands defaults to empty, so there
// the next Set matches a fresh flag. A slice flag with a non-empty default
// would keep the default in front of later values.
func resetCommandTreeFlags(cmd *cobra.Command) error {
	var firstErr error
	reset := func(flag *pflag.Flag) {
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			def, err := parseSliceDefault(flag.DefValue)
			if err != nil && firstErr == nil {
				firstErr = err
			}
			_ = slice.Replace(def)
		} else {
			_ = flag.Value.Set(flag.DefValue)
		}
		flag.Changed = false
	}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(reset)
		c.PersistentFlags().VisitAll(reset)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cmd)
	return firstErr
}

// parseSliceDefault reads a slice flag's DefValue, which pflag writes as
// "[a,b]", back into the values it stands for.
func parseSliceDefault(defValue string) ([]string, error) {
	inner := strings.TrimSuffix(strings.TrimPrefix(defValue, "["), "]")
	if inner == "" {
		return nil, nil
	}
	return csv.NewReader(strings.NewReader(inner)).Read()
}

// TestCLI_CreateAcceptanceSyncsDescriptionSection covers the create half of
// the same two-spellings problem bd update --acceptance was fixed for: a
// description that already carries a '## Acceptance' section, created with
// --acceptance, must not be stored with the column and the section
// disagreeing. The section is what bd show prints as the criteria, so a create
// that wrote only the column would print the block it was handed rather than
// the criteria it was given.
func TestCLI_CreateAcceptanceSyncsDescriptionSection(t *testing.T) {
	resetCommandFlag(t, createCmd, "acceptance")
	resetCommandFlag(t, createCmd, "description")
	tmpDir := setupCLITestDB(t)
	description := "## Goal\n\nclose the gap\n\n## Acceptance\n\n- [ ] make check pass\n"
	out := runBDInProcess(t, tmpDir, "create", "Spec-shaped bead",
		"--description", description, "--acceptance", "- [x] make check pass", "--json")

	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &issue); err != nil {
		t.Fatalf("Failed to parse create output: %v", err)
	}
	if got := issue["acceptance_criteria"]; got != "- [x] make check pass" {
		t.Errorf("acceptance_criteria = %v, want the criteria that were written", got)
	}
	stored, _ := issue["description"].(string)
	if !strings.Contains(stored, "- [x] make check pass") || strings.Contains(stored, "- [ ]") {
		t.Errorf("description's acceptance section was not updated:\n%s", stored)
	}
	if !strings.Contains(stored, "## Goal\n\nclose the gap") {
		t.Errorf("create did not carry the rest of the description:\n%s", stored)
	}
}

// TestCLI_CreateWithoutAcceptanceLeavesDescriptionAlone is the other side of
// the create contract: a description carrying a '## Acceptance' section is the
// spec-dispatcher shape, and a create that writes no acceptance column has
// nothing to keep the section in step with, so the description is stored
// verbatim.
func TestCLI_CreateWithoutAcceptanceLeavesDescriptionAlone(t *testing.T) {
	resetCommandFlag(t, createCmd, "acceptance")
	resetCommandFlag(t, createCmd, "description")
	tmpDir := setupCLITestDB(t)
	description := "## Goal\n\nclose the gap\n\n## Acceptance\n\n- [ ] make check pass\n"
	out := runBDInProcess(t, tmpDir, "create", "Dispatcher-shaped bead", "--description", description, "--json")

	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &issue); err != nil {
		t.Fatalf("Failed to parse create output: %v", err)
	}
	if got, _ := issue["description"].(string); got != description {
		t.Errorf("description = %q, want it stored verbatim", got)
	}
}

// TestCLI_CreateAcceptanceRefusesAmbiguousDescription is the refusal half: a
// description naming two acceptance sections names no single place to write,
// so the create fails instead of guessing.
func TestCLI_CreateAcceptanceRefusesAmbiguousDescription(t *testing.T) {
	resetCommandFlag(t, createCmd, "acceptance")
	resetCommandFlag(t, createCmd, "description")
	tmpDir := setupCLITestDB(t)
	description := "## Acceptance\n\n- [ ] one\n\n## Acceptance Criteria\n\n- [ ] two\n"
	stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "create", "Two acceptance sections",
		"--description", description, "--acceptance", "- [x] one", "--json")
	if err == nil {
		t.Fatalf("expected the ambiguous acceptance create to fail, stdout=%q stderr=%q", stdout, stderr)
	}
}

// TestCLI_EditCommandIsUnknown pins the be-bxp removal: bd edit was the one
// command that opened $EDITOR, and with it gone the name resolves to nothing.
// The error text is not just cosmetic — envelope.go keys its invalid_args
// mapping on the "unknown command" prefix, so a machine caller gets a typed
// error rather than an untyped failure.
func TestCLI_EditCommandIsUnknown(t *testing.T) {
	_, _, err := rootCmd.Find([]string{"edit"})
	if err == nil {
		t.Fatal("bd edit is still a registered command")
	}
	if !strings.Contains(err.Error(), `unknown command "edit"`) {
		t.Errorf("bd edit: err = %v, want cobra's unknown-command error for a deleted command", err)
	}
}

func TestCLI_UpdateLabels(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Issue for label testing", "-p", "2", "--json")

	var issue map[string]interface{}
	unmarshalEnvelope([]byte(out), &issue)
	id := issue["id"].(string)

	// Test adding labels
	runBDInProcess(t, tmpDir, "update", id, "--add-label", "feature", "--add-label", "backend")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var updated []map[string]interface{}
	unmarshalEnvelope([]byte(out), &updated)
	labels := updated[0]["labels"].([]interface{})
	if len(labels) != 2 {
		t.Errorf("Expected 2 labels after add, got: %d", len(labels))
	}
	hasBackend, hasFeature := false, false
	for _, l := range labels {
		if l.(string) == "backend" {
			hasBackend = true
		}
		if l.(string) == "feature" {
			hasFeature = true
		}
	}
	if !hasBackend || !hasFeature {
		t.Errorf("Expected labels 'backend' and 'feature', got: %v", labels)
	}

	// Test removing a label
	runBDInProcess(t, tmpDir, "update", id, "--remove-label", "backend")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	unmarshalEnvelope([]byte(out), &updated)
	labels = updated[0]["labels"].([]interface{})
	if len(labels) != 1 {
		t.Errorf("Expected 1 label after remove, got: %d", len(labels))
	}
	if labels[0].(string) != "feature" {
		t.Errorf("Expected label 'feature', got: %v", labels[0])
	}

	// Test setting labels (replaces all)
	runBDInProcess(t, tmpDir, "update", id, "--set-labels", "api,database,critical")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	unmarshalEnvelope([]byte(out), &updated)
	labels = updated[0]["labels"].([]interface{})
	if len(labels) != 3 {
		t.Errorf("Expected 3 labels after set, got: %d", len(labels))
	}
	expectedLabels := map[string]bool{"api": true, "database": true, "critical": true}
	for _, l := range labels {
		if !expectedLabels[l.(string)] {
			t.Errorf("Unexpected label: %v", l)
		}
	}
}

func TestCLI_UpdateEphemeral(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Issue for ephemeral testing", "-p", "2", "--json")

	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &issue); err != nil {
		t.Fatalf("Failed to parse create output: %v", err)
	}
	id := issue["id"].(string)

	// Mark as ephemeral
	runBDInProcess(t, tmpDir, "update", id, "--ephemeral")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var updated []map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &updated); err != nil {
		t.Fatalf("Failed to parse show output: %v", err)
	}
	if updated[0]["ephemeral"] != true {
		t.Errorf("Expected ephemeral to be true after --ephemeral, got: %v", updated[0]["ephemeral"])
	}
}

func TestCLI_UpdatePersistent(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)

	// Create ephemeral issue directly
	out := runBDInProcess(t, tmpDir, "create", "Ephemeral issue", "-p", "2", "--ephemeral", "--json")

	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &issue); err != nil {
		t.Fatalf("Failed to parse create output: %v", err)
	}
	id := issue["id"].(string)

	// Verify it's ephemeral
	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var initial []map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &initial); err != nil {
		t.Fatalf("Failed to parse show output: %v", err)
	}
	if initial[0]["ephemeral"] != true {
		t.Fatalf("Expected issue to be ephemeral initially, got: %v", initial[0]["ephemeral"])
	}

	// Promote to persistent
	runBDInProcess(t, tmpDir, "update", id, "--persistent")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var updated []map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &updated); err != nil {
		t.Fatalf("Failed to parse show output after persistent: %v", err)
	}
	if updated[0]["ephemeral"] == true {
		t.Errorf("Expected ephemeral to be false after --persistent, got: %v", updated[0]["ephemeral"])
	}
}

func TestCLI_UpdateEphemeralMutualExclusion(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Issue for mutual exclusion test", "-p", "2", "--json")

	var issue map[string]interface{}
	unmarshalEnvelope([]byte(out), &issue)
	id := issue["id"].(string)

	// Both flags should error
	_, stderr, err := runBDInProcessAllowError(t, tmpDir, "update", id, "--ephemeral", "--persistent")
	if err == nil {
		t.Errorf("Expected error when both flags specified, got none")
	}
	if !strings.Contains(stderr, "cannot specify both") {
		t.Errorf("Expected mutual exclusion error message, got: %v", stderr)
	}
}

func TestCLI_UpdateAppendNotes(t *testing.T) {
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Issue for append-notes test", "-p", "2", "--notes", "Original notes", "--json")

	var issue map[string]interface{}
	unmarshalEnvelope([]byte(out), &issue)
	id := issue["id"].(string)

	// Test appending notes
	runBDInProcess(t, tmpDir, "update", id, "--append-notes", "Appended content")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var updated []map[string]interface{}
	unmarshalEnvelope([]byte(out), &updated)
	notes := updated[0]["notes"].(string)
	if notes != "Original notes\nAppended content" {
		t.Errorf("Expected 'Original notes\\nAppended content', got: %q", notes)
	}

	// Test appending to empty notes
	out = runBDInProcess(t, tmpDir, "create", "Issue with empty notes", "-p", "2", "--json")
	unmarshalEnvelope([]byte(out), &issue)
	id2 := issue["id"].(string)

	runBDInProcess(t, tmpDir, "update", id2, "--append-notes", "First note")

	out = runBDInProcess(t, tmpDir, "show", id2, "--json")
	unmarshalEnvelope([]byte(out), &updated)
	notes = updated[0]["notes"].(string)
	if notes != "First note" {
		t.Errorf("Expected 'First note', got: %q", notes)
	}
}

func TestCLI_UpdateAppendNotesMutualExclusion(t *testing.T) {
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Issue for notes mutual exclusion", "-p", "2", "--json")

	var issue map[string]interface{}
	unmarshalEnvelope([]byte(out), &issue)
	id := issue["id"].(string)

	// Both --notes and --append-notes should error
	_, stderr, err := runBDInProcessAllowError(t, tmpDir, "update", id, "--notes", "New notes", "--append-notes", "Appended")
	if err == nil {
		t.Errorf("Expected error when both --notes and --append-notes specified, got none")
	}
	if !strings.Contains(stderr, "cannot specify both --notes and --append-notes") {
		t.Errorf("Expected mutual exclusion error message, got: %v", stderr)
	}
}

func TestCLI_NoteCommand(t *testing.T) {
	tmpDir := setupCLITestDB(t)

	// Create an issue with initial notes
	out := runBDInProcess(t, tmpDir, "create", "Issue for note test", "-p", "2", "--notes", "Original notes", "--json")
	var issue map[string]interface{}
	unmarshalEnvelope([]byte(out), &issue)
	id := issue["id"].(string)

	// Test: bd note <id> <text> appends to existing notes
	runBDInProcess(t, tmpDir, "note", id, "Added via note command")
	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var updated []map[string]interface{}
	unmarshalEnvelope([]byte(out), &updated)
	notes := updated[0]["notes"].(string)
	if notes != "Original notes\nAdded via note command" {
		t.Errorf("Expected 'Original notes\\nAdded via note command', got: %q", notes)
	}

	// Test: bd note <id> with multiple words joins them
	runBDInProcess(t, tmpDir, "note", id, "second", "note", "here")
	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	unmarshalEnvelope([]byte(out), &updated)
	notes = updated[0]["notes"].(string)
	if notes != "Original notes\nAdded via note command\nsecond note here" {
		t.Errorf("Expected three lines, got: %q", notes)
	}

	// Test: bd note on issue with no existing notes
	out = runBDInProcess(t, tmpDir, "create", "Issue with no notes", "-p", "2", "--json")
	unmarshalEnvelope([]byte(out), &issue)
	id2 := issue["id"].(string)

	runBDInProcess(t, tmpDir, "note", id2, "First note ever")
	out = runBDInProcess(t, tmpDir, "show", id2, "--json")
	unmarshalEnvelope([]byte(out), &updated)
	notes = updated[0]["notes"].(string)
	if notes != "First note ever" {
		t.Errorf("Expected 'First note ever', got: %q", notes)
	}

	// Test: bd note with no text should fail
	_, stderr, err := runBDInProcessAllowError(t, tmpDir, "note", id)
	if err == nil {
		t.Errorf("Expected error when no note text provided, got none")
	}
	if !strings.Contains(stderr, "no note text provided") {
		t.Errorf("Expected 'no note text provided' error, got: %v", stderr)
	}
}

// TestCLI_NoteListMisplacedSyntax is a regression test for "note" silently
// mis-resolving a subcommand-like typo as an issue id (GH#5370). Mirrors
// TestCLI_CommentListMisplacedSyntax (#5369): "bd note list <text>" used to
// parse as id="list", text=[<text>...] with no validation on the id — since
// no issue is literally named "list", ResolvePartialID's substring/prefix
// fallback would silently match it against whatever existing bead or wisp id
// happened to contain "list" and append the note there instead of erroring.
// This asserts both halves of the fix: the command is rejected with a
// helpful hint, AND no note silently landed on an unrelated real issue.
func TestCLI_NoteListMisplacedSyntax(t *testing.T) {
	tmpDir := setupCLITestDB(t)

	out := runBDInProcess(t, tmpDir, "create", "Bystander issue", "-p", "1", "--json")
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("No JSON found in create output: %s", out)
	}
	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out[jsonStart:]), &issue); err != nil {
		t.Fatalf("Failed to parse create JSON: %v\nOutput: %s", err, out)
	}
	fullID := issue["id"].(string)

	// Snapshot notes BEFORE the rejected command. runBDInProcess reuses the
	// package-level rootCmd without resetting cobra flag state, so an earlier
	// test's --notes value can leak into this create. Asserting before==after
	// is immune to that leak and is a stronger regression check: the rejected
	// command wrote nothing (GH#5370 review fix-round).
	beforeShow := runBDInProcess(t, tmpDir, "show", fullID, "--json")
	var beforeShown []map[string]interface{}
	if err := unmarshalEnvelope([]byte(beforeShow), &beforeShown); err != nil {
		t.Fatalf("Failed to parse pre-reject show JSON: %v\nOutput: %s", err, beforeShow)
	}
	if len(beforeShown) == 0 {
		t.Fatalf("expected show to return the bystander issue before reject, got: %s", beforeShow)
	}
	notesBefore, _ := beforeShown[0]["notes"].(string)

	stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "note", "list", "accidental stray text")
	if err == nil {
		t.Fatalf("expected non-zero exit, got stdout=%q stderr=%q", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "bd note") || !strings.Contains(combined, "<issue-id>") {
		t.Fatalf("expected hint pointing at bd note <issue-id>, got stdout=%q stderr=%q", stdout, stderr)
	}

	// The regression check: the bystander issue's notes must be UNCHANGED.
	// Pre-fix, this could silently succeed and land a note on whatever
	// bead's id happened to contain "list" as a substring.
	showOut := runBDInProcess(t, tmpDir, "show", fullID, "--json")
	var shown []map[string]interface{}
	if err := unmarshalEnvelope([]byte(showOut), &shown); err != nil {
		t.Fatalf("Failed to parse show JSON: %v\nOutput: %s", err, showOut)
	}
	if len(shown) == 0 {
		t.Fatalf("expected show to return the bystander issue, got: %s", showOut)
	}
	notesAfter, _ := shown[0]["notes"].(string)
	if notesAfter != notesBefore {
		t.Fatalf("expected bystander issue %s notes unchanged after rejected 'note list' (before=%q after=%q)", fullID, notesBefore, notesAfter)
	}
}

// TestCLI_NoteAddMisplacedSyntax mirrors TestCLI_NoteListMisplacedSyntax
// for the symmetric "add" typo — a caller who types "bd note add <id> <text>"
// (confusing note with comments-style "add") would parse as id="add",
// text=[<id>, <text>...].
func TestCLI_NoteAddMisplacedSyntax(t *testing.T) {
	tmpDir := setupCLITestDB(t)

	out := runBDInProcess(t, tmpDir, "create", "Bystander issue for add-typo", "-p", "1", "--json")
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("No JSON found in create output: %s", out)
	}
	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out[jsonStart:]), &issue); err != nil {
		t.Fatalf("Failed to parse create JSON: %v\nOutput: %s", err, out)
	}
	fullID := issue["id"].(string)

	// Snapshot notes BEFORE the rejected command (same order-independence
	// rationale as TestCLI_NoteListMisplacedSyntax).
	beforeShow := runBDInProcess(t, tmpDir, "show", fullID, "--json")
	var beforeShown []map[string]interface{}
	if err := unmarshalEnvelope([]byte(beforeShow), &beforeShown); err != nil {
		t.Fatalf("Failed to parse pre-reject show JSON: %v\nOutput: %s", err, beforeShow)
	}
	if len(beforeShown) == 0 {
		t.Fatalf("expected show to return the bystander issue before reject, got: %s", beforeShow)
	}
	notesBefore, _ := beforeShown[0]["notes"].(string)

	stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "note", "add", fullID, "accidental stray text")
	if err == nil {
		t.Fatalf("expected non-zero exit, got stdout=%q stderr=%q", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "bd note") || !strings.Contains(combined, "<issue-id>") {
		t.Fatalf("expected hint pointing at bd note <issue-id>, got stdout=%q stderr=%q", stdout, stderr)
	}

	showOut := runBDInProcess(t, tmpDir, "show", fullID, "--json")
	var shown []map[string]interface{}
	if err := unmarshalEnvelope([]byte(showOut), &shown); err != nil {
		t.Fatalf("Failed to parse show JSON: %v\nOutput: %s", err, showOut)
	}
	if len(shown) == 0 {
		t.Fatalf("expected show to return the bystander issue, got: %s", showOut)
	}
	notesAfter, _ := shown[0]["notes"].(string)
	if notesAfter != notesBefore {
		t.Fatalf("expected bystander issue %s notes unchanged after rejected 'note add' (before=%q after=%q)", fullID, notesBefore, notesAfter)
	}
}

// TestCLI_NoteMisplacedSyntaxRejectedBeforeStoreOpen proves the guard
// fires from noteCmd's Args validator, before PersistentPreRunE ever
// opens a store, mirroring
// TestCLI_CommentMisplacedSyntaxRejectedBeforeStoreOpen for the comment
// sibling. Using a directory with no .beads/ at all proves the store was
// never touched: a RunE-only check would have failed first with "no beads
// database found".
func TestCLI_NoteMisplacedSyntaxRejectedBeforeStoreOpen(t *testing.T) {
	noBeadsCwd := t.TempDir()
	if _, err := os.Stat(filepath.Join(noBeadsCwd, ".beads")); err == nil {
		t.Fatalf("test setup: %s unexpectedly has a .beads dir", noBeadsCwd)
	}

	stdout, stderr, err := runBDInProcessAllowError(t, noBeadsCwd, "note", "list", "should not be stored")
	if err == nil {
		t.Fatalf("expected non-zero exit for misplaced 'note list', got success:\nstdout: %s", stdout)
	}
	combined := stdout + stderr
	if strings.Contains(combined, "no beads database found") {
		t.Fatalf("Args validation did not run before store open: got the pre-fix error.\nOutput:\n%s", combined)
	}
	if !strings.Contains(combined, "bd note") {
		t.Errorf("expected hint pointing to `bd note`, got:\n%s", combined)
	}
}

// TestCLI_NoteTextStartingWithReservedWordStillWorks confirms the fix is
// scoped to the id positional argument only: a real id followed by note
// TEXT that happens to start with "list" or "add" must still work exactly
// as before, since args[0] (the id slot) is what's checked, never the text.
func TestCLI_NoteTextStartingWithReservedWordStillWorks(t *testing.T) {
	tmpDir := setupCLITestDB(t)

	out := runBDInProcess(t, tmpDir, "create", "Issue for reserved-word-text test", "-p", "1", "--json")
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("No JSON found in create output: %s", out)
	}
	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out[jsonStart:]), &issue); err != nil {
		t.Fatalf("Failed to parse create JSON: %v\nOutput: %s", err, out)
	}
	fullID := issue["id"].(string)

	stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "note", fullID, "list", "of", "things", "to", "do")
	if err != nil {
		t.Fatalf("note with text starting in 'list' unexpectedly failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Note added") {
		t.Errorf("Expected 'Note added' in output, got: %s", stdout)
	}

	showOut := runBDInProcess(t, tmpDir, "show", fullID, "--json")
	if !strings.Contains(showOut, "list of things to do") {
		t.Fatalf("expected note text to be stored verbatim, got: %s", showOut)
	}
}

func TestCLI_Close(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Issue to close", "-p", "1", "--json")

	var issue map[string]interface{}
	unmarshalEnvelope([]byte(out), &issue)
	id := issue["id"].(string)

	runBDInProcess(t, tmpDir, "close", id, "--reason", "Done")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var closed []map[string]interface{}
	unmarshalEnvelope([]byte(out), &closed)
	if closed[0]["status"] != "closed" {
		t.Errorf("Expected status 'closed', got: %v", closed[0]["status"])
	}
	if closed[0]["close_reason"] != "Done" {
		t.Errorf("Expected close_reason 'Done', got: %v", closed[0]["close_reason"])
	}
}

func TestCLI_DepAdd(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)

	out1 := runBDInProcess(t, tmpDir, "create", "First", "-p", "1", "--json")
	out2 := runBDInProcess(t, tmpDir, "create", "Second", "-p", "1", "--json")

	var issue1, issue2 map[string]interface{}
	unmarshalEnvelope([]byte(out1), &issue1)
	unmarshalEnvelope([]byte(out2), &issue2)

	id1 := issue1["id"].(string)
	id2 := issue2["id"].(string)

	out := runBDInProcess(t, tmpDir, "dep", "add", id2, id1)
	if !strings.Contains(out, "Added dependency") {
		t.Errorf("Expected 'Added dependency', got: %s", out)
	}
}

// TestCLI_DepAdd_TypeBlockedByAliasGates is the regression test for GH#5069:
// `bd dep add A B --type blocked-by` used to store the literal custom type
// "blocked-by" instead of normalizing it to the canonical "blocks" type, so
// the dependency was displayed everywhere but silently never gated `bd
// ready`/`bd blocked`. Assert the alias now gates exactly like the default
// (canonical) type does.
func TestCLI_DepAdd_TypeBlockedByAliasGates(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)

	out1 := runBDInProcess(t, tmpDir, "create", "Blocker", "-p", "1", "--json")
	out2 := runBDInProcess(t, tmpDir, "create", "Blocked", "-p", "1", "--json")

	var issue1, issue2 map[string]interface{}
	unmarshalEnvelope([]byte(out1), &issue1)
	unmarshalEnvelope([]byte(out2), &issue2)

	blockerID := issue1["id"].(string)
	blockedID := issue2["id"].(string)

	// blockedID depends on (is blocked by) blockerID, using the --type
	// blocked-by alias rather than the default "blocks" type.
	out := runBDInProcess(t, tmpDir, "dep", "add", blockedID, blockerID, "--type", "blocked-by")
	if !strings.Contains(out, "Added dependency") {
		t.Fatalf("Expected 'Added dependency', got: %s", out)
	}

	blockedOut := runBDInProcess(t, tmpDir, "blocked")
	if !strings.Contains(blockedOut, "Blocked") {
		t.Errorf("Expected 'Blocked' issue in `bd blocked` output after --type blocked-by dep, got: %s", blockedOut)
	}

	readyOut := runBDInProcess(t, tmpDir, "ready")
	if strings.Contains(readyOut, "Blocked") {
		t.Errorf("Expected blocked issue to be excluded from `bd ready` after --type blocked-by dep, got: %s", readyOut)
	}
}

// TestCLI_Show_SupersedesIsNotABlocker is the regression test for bd show
// grouping every dependency type it did not name explicitly into the blocks
// bucket. A supersedes edge printed under "DEPENDS ON" on the replaced issue
// and "BLOCKS" on the replacement — a blocking claim that is false, since
// supersedes takes no part in the ready-work calculation.
func TestCLI_Show_SupersedesIsNotABlocker(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)

	outOld := runBDInProcess(t, tmpDir, "create", "V1 spec", "-p", "1", "--json")
	outNew := runBDInProcess(t, tmpDir, "create", "V2 spec", "-p", "1", "--json")

	var oldIssue, newIssue map[string]interface{}
	unmarshalEnvelope([]byte(outOld), &oldIssue)
	unmarshalEnvelope([]byte(outNew), &newIssue)

	oldID := oldIssue["id"].(string)
	newID := newIssue["id"].(string)

	runBDInProcess(t, tmpDir, "supersede", oldID, "--with", newID)

	// `bd supersede old --with new` stores (old, new), so the replaced issue is
	// the edge's source and names its replacement.
	replaced := runBDInProcess(t, tmpDir, "show", oldID)
	if !strings.Contains(replaced, "SUPERSEDED BY") {
		t.Errorf("expected SUPERSEDED BY section on the replaced issue, got: %s", replaced)
	}
	if strings.Contains(replaced, "DEPENDS ON") {
		t.Errorf("replaced issue must not claim it depends on its replacement, got: %s", replaced)
	}

	replacement := runBDInProcess(t, tmpDir, "show", newID)
	if !strings.Contains(replacement, "SUPERSEDES") {
		t.Errorf("expected SUPERSEDES section on the replacement, got: %s", replacement)
	}
	if strings.Contains(replacement, "BLOCKS") {
		t.Errorf("replacement must not claim it blocks the issue it replaced, got: %s", replacement)
	}
}

func TestCLI_DepRemove(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)

	out1 := runBDInProcess(t, tmpDir, "create", "First", "-p", "1", "--json")
	out2 := runBDInProcess(t, tmpDir, "create", "Second", "-p", "1", "--json")

	var issue1, issue2 map[string]interface{}
	unmarshalEnvelope([]byte(out1), &issue1)
	unmarshalEnvelope([]byte(out2), &issue2)

	id1 := issue1["id"].(string)
	id2 := issue2["id"].(string)

	runBDInProcess(t, tmpDir, "dep", "add", id2, id1)
	out := runBDInProcess(t, tmpDir, "dep", "remove", id2, id1)
	if !strings.Contains(out, "Removed dependency") {
		t.Errorf("Expected 'Removed dependency', got: %s", out)
	}
}

func TestCLI_DepTree(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)

	out1 := runBDInProcess(t, tmpDir, "create", "Parent", "-p", "1", "--json")
	out2 := runBDInProcess(t, tmpDir, "create", "Child", "-p", "1", "--json")

	var issue1, issue2 map[string]interface{}
	unmarshalEnvelope([]byte(out1), &issue1)
	unmarshalEnvelope([]byte(out2), &issue2)

	id1 := issue1["id"].(string)
	id2 := issue2["id"].(string)

	runBDInProcess(t, tmpDir, "dep", "add", id2, id1)
	out := runBDInProcess(t, tmpDir, "dep", "tree", id1)
	if !strings.Contains(out, "Parent") {
		t.Errorf("Expected 'Parent' in tree, got: %s", out)
	}
}

func TestCLI_Blocked(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)

	out1 := runBDInProcess(t, tmpDir, "create", "Blocker", "-p", "1", "--json")
	out2 := runBDInProcess(t, tmpDir, "create", "Blocked", "-p", "1", "--json")

	var issue1, issue2 map[string]interface{}
	unmarshalEnvelope([]byte(out1), &issue1)
	unmarshalEnvelope([]byte(out2), &issue2)

	id1 := issue1["id"].(string)
	id2 := issue2["id"].(string)

	runBDInProcess(t, tmpDir, "dep", "add", id2, id1)
	out := runBDInProcess(t, tmpDir, "blocked")
	if !strings.Contains(out, "Blocked") {
		t.Errorf("Expected 'Blocked' in output, got: %s", out)
	}
}

func TestCLI_Stats(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	runBDInProcess(t, tmpDir, "create", "Issue 1", "-p", "1")
	runBDInProcess(t, tmpDir, "create", "Issue 2", "-p", "1")

	out := runBDInProcess(t, tmpDir, "stats")
	if !strings.Contains(out, "Total") || !strings.Contains(out, "2") {
		t.Errorf("Expected stats to show 2 issues, got: %s", out)
	}
}

func TestCLI_Show(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Show test", "-p", "1", "--json")

	var issue map[string]interface{}
	unmarshalEnvelope([]byte(out), &issue)
	id := issue["id"].(string)

	out = runBDInProcess(t, tmpDir, "show", id)
	if !strings.Contains(out, "Show test") {
		t.Errorf("Expected 'Show test' in output, got: %s", out)
	}
}

func TestCLI_Export(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	runBDInProcess(t, tmpDir, "create", "Export test", "-p", "1")

	exportFile := filepath.Join(tmpDir, "export.jsonl")
	runBDInProcess(t, tmpDir, "export", "-o", exportFile)

	if _, err := os.Stat(exportFile); os.IsNotExist(err) {
		t.Errorf("Export file not created: %s", exportFile)
	}
}

func TestCLI_Import(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	runBDInProcess(t, tmpDir, "create", "Import test", "-p", "1")

	exportFile := filepath.Join(tmpDir, "export.jsonl")
	runBDInProcess(t, tmpDir, "export", "-o", exportFile)

	// Create new db and import
	tmpDir2 := createTempDirWithCleanup(t)
	runBDInProcess(t, tmpDir2, "init", "--prefix", "test", "--quiet")
	runBDInProcess(t, tmpDir2, "import", "-i", exportFile)

	out := runBDInProcess(t, tmpDir2, "list", "--json")
	var issues []map[string]interface{}
	unmarshalEnvelope([]byte(out), &issues)
	if len(issues) != 1 {
		t.Errorf("Expected 1 imported issue, got %d", len(issues))
	}
}

var testBD string

func init() {
	// Helper-process re-execs (worktree_remove_test.go's
	// runWorktreeRemoveProcess) run this init in a child whose cwd is a temp
	// git repo: the repo-root probe below misses and the `go build .`
	// fallback panics with "cannot find main module" — which is what six
	// Main integration shards reported at cd25e0919. The helper never uses
	// testBD, so skip the work entirely in that child.
	if os.Getenv(worktreeRemoveHelperEnv) != "" {
		return
	}
	// Prebuilt fast path (scripts/test.sh and CI export this), else build
	// once. No repo-root ./bd reuse: a stale checkout-root binary silently
	// substitutes itself for the source under test (wy-4mtr0).
	if prebuilt := os.Getenv("BEADS_TEST_BD_BINARY"); prebuilt != "" {
		if abs, err := filepath.Abs(prebuilt); err == nil {
			if _, statErr := os.Stat(abs); statErr == nil {
				testBD = abs
				return
			}
		}
	}
	bdBinary := "bd"
	if runtime.GOOS == "windows" {
		bdBinary = "bd.exe"
	}

	// Fall back to building once (for CI or fresh checkouts)
	tmpDir, err := os.MkdirTemp("", "bd-cli-test-*")
	if err != nil {
		panic(err)
	}
	testBD = filepath.Join(tmpDir, bdBinary)
	cmd := exec.Command("go", "build", "-tags", "gms_pure_go", "-o", testBD, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(string(out))
	}
}

// runBDExec runs bd via exec.Command for end-to-end testing
// This is kept for a few tests to ensure the actual binary works correctly
func runBDExec(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command(testBD, args...)
	cmd.Dir = dir
	cmd.Env = withoutWorkspaceEnv(os.Environ())

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bd %v failed: %v\nOutput: %s", args, err, out)
	}
	return string(out)
}

// runBDExecAllowErrorWithEnv runs bd via exec.Command with custom env vars,
// returning combined output and any error (does not fail the test on error).
func runBDExecAllowErrorWithEnv(t *testing.T, dir string, env []string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.Command(testBD, args...)
	cmd.Dir = dir
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestCLI_EndToEnd performs end-to-end testing using the actual binary
// This ensures the compiled binary works correctly when executed normally
func TestCLI_EndToEnd(t *testing.T) {
	if testDoltServerPort == 0 {
		t.Skip("skipping: Dolt test container not available")
	}
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway

	tmpDir := createTempDirWithCleanup(t)

	// Test full workflow with exec.Command to validate binary
	runBDExec(t, tmpDir, "init", "--prefix", "test", "--quiet")

	out := runBDExec(t, tmpDir, "create", "E2E test", "-p", "1", "--json")
	var issue map[string]interface{}
	jsonStart := strings.Index(out, "{")
	unmarshalEnvelope([]byte(out[jsonStart:]), &issue)
	id := issue["id"].(string)

	runBDExec(t, tmpDir, "update", id, "--status", "in_progress")
	runBDExec(t, tmpDir, "close", id, "--reason", "Done")

	out = runBDExec(t, tmpDir, "show", id, "--json")
	var closed []map[string]interface{}
	unmarshalEnvelope([]byte(out), &closed)

	if closed[0]["status"] != "closed" {
		t.Errorf("Expected status 'closed', got: %v", closed[0]["status"])
	}

	// Test export
	exportFile := filepath.Join(tmpDir, "export.jsonl")
	runBDExec(t, tmpDir, "export", "-o", exportFile)

	if _, err := os.Stat(exportFile); os.IsNotExist(err) {
		t.Errorf("Export file not created: %s", exportFile)
	}
}

func TestCLI_Labels(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Label test", "-p", "1", "--json")

	jsonStart := strings.Index(out, "{")
	jsonOut := out[jsonStart:]

	var issue map[string]interface{}
	unmarshalEnvelope([]byte(jsonOut), &issue)
	id := issue["id"].(string)

	// Add label
	runBDInProcess(t, tmpDir, "label", "add", id, "urgent")

	// List labels
	out = runBDInProcess(t, tmpDir, "label", "list", id)
	if !strings.Contains(out, "urgent") {
		t.Errorf("Expected 'urgent' label, got: %s", out)
	}

	// Remove label
	runBDInProcess(t, tmpDir, "label", "remove", id, "urgent")
	out = runBDInProcess(t, tmpDir, "label", "list", id)
	if strings.Contains(out, "urgent") {
		t.Errorf("Label should be removed, got: %s", out)
	}
}

func TestCLI_PriorityFormats(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)

	// Test numeric priority
	out := runBDInProcess(t, tmpDir, "create", "Test P0", "-p", "0", "--json")
	jsonStart := strings.Index(out, "{")
	jsonOut := out[jsonStart:]
	var issue map[string]interface{}
	unmarshalEnvelope([]byte(jsonOut), &issue)
	if issue["priority"].(float64) != 0 {
		t.Errorf("Expected priority 0, got: %v", issue["priority"])
	}

	// Test P-format priority
	out = runBDInProcess(t, tmpDir, "create", "Test P3", "-p", "P3", "--json")
	jsonStart = strings.Index(out, "{")
	jsonOut = out[jsonStart:]
	unmarshalEnvelope([]byte(jsonOut), &issue)
	if issue["priority"].(float64) != 3 {
		t.Errorf("Expected priority 3, got: %v", issue["priority"])
	}

	// Test update with P-format
	id := issue["id"].(string)
	runBDInProcess(t, tmpDir, "update", id, "-p", "P1")

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var updated []map[string]interface{}
	unmarshalEnvelope([]byte(out), &updated)
	if updated[0]["priority"].(float64) != 1 {
		t.Errorf("Expected priority 1 after update, got: %v", updated[0]["priority"])
	}
}

func TestCLI_Reopen(t *testing.T) {
	// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
	tmpDir := setupCLITestDB(t)
	out := runBDInProcess(t, tmpDir, "create", "Reopen test", "-p", "1", "--json")

	jsonStart := strings.Index(out, "{")
	jsonOut := out[jsonStart:]
	var issue map[string]interface{}
	unmarshalEnvelope([]byte(jsonOut), &issue)
	id := issue["id"].(string)

	// Close it
	runBDInProcess(t, tmpDir, "close", id)

	// Reopen it
	runBDInProcess(t, tmpDir, "reopen", id)

	out = runBDInProcess(t, tmpDir, "show", id, "--json")
	var reopened []map[string]interface{}
	unmarshalEnvelope([]byte(out), &reopened)
	if reopened[0]["status"] != "open" {
		t.Errorf("Expected status 'open', got: %v", reopened[0]["status"])
	}
}

// runBDInProcessAllowError is like runBDInProcess but doesn't fail on error
// Returns stdout, stderr, and any error from command execution
func runBDInProcessAllowError(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()

	inProcessMutex.Lock()
	defer inProcessMutex.Unlock()

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	oldDir, _ := os.Getwd()
	oldArgs := os.Args

	// Same workspace-variable handling as runBDInProcess: rootCmd's pre-run
	// sets BEADS_DIR and never restores it.
	for _, key := range workspaceEnvVars {
		if v, ok := os.LookupEnv(key); ok {
			defer os.Setenv(key, v)
		} else {
			defer os.Unsetenv(key)
		}
		os.Unsetenv(key)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Failed to chdir to %s: %v", dir, err)
	}
	git.ResetCaches()

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	rootCmd.SetArgs(args)
	os.Args = append([]string{"bd"}, args...)

	cmdErr := rootCmd.Execute()
	if resetErr := resetCommandTreeFlags(rootCmd); resetErr != nil {
		t.Errorf("resetting bd flags after the run: %v", resetErr)
	}

	if store != nil {
		store.Close()
		store = nil
	}
	dbPath = ""
	actor = ""
	jsonOutput = false
	sandboxMode = false
	rootCtx = nil
	rootCancel = nil

	wOut.Close()
	wErr.Close()
	os.Chdir(oldDir)
	os.Args = oldArgs
	rootCmd.SetArgs(nil)

	var outBuf, errBuf bytes.Buffer
	outBuf.ReadFrom(rOut)
	errBuf.ReadFrom(rErr)

	return outBuf.String(), errBuf.String(), cmdErr
}

// TestCLI_CreateDryRun tests the --dry-run flag for bd create command (bd-nib2)
func TestCLI_CreateDryRun(t *testing.T) {
	if testDoltServerPort == 0 {
		t.Skip("skipping: Dolt test container not available")
	}

	t.Run("BasicDryRunPreview", func(t *testing.T) {
		// Note: Not using t.Parallel() because inProcessMutex serializes execution anyway
		tmpDir := setupCLITestDB(t)

		// Run create with --dry-run
		out := runBDInProcess(t, tmpDir, "create", "Test dry run issue", "-p", "1", "--dry-run")

		// Verify output contains dry-run indicator
		if !strings.Contains(out, "[DRY RUN]") {
			t.Errorf("Expected '[DRY RUN]' in output, got: %s", out)
		}
		if !strings.Contains(out, "Would create issue") {
			t.Errorf("Expected 'Would create issue' in output, got: %s", out)
		}
		if !strings.Contains(out, "Test dry run issue") {
			t.Errorf("Expected title in output, got: %s", out)
		}
		if !strings.Contains(out, "(will be generated)") {
			t.Errorf("Expected '(will be generated)' for ID, got: %s", out)
		}

		// Verify no issue was actually created
		listOut := runBDInProcess(t, tmpDir, "list", "--json")
		var issues []map[string]interface{}
		unmarshalEnvelope([]byte(listOut), &issues)
		if len(issues) != 0 {
			t.Errorf("Expected 0 issues after dry-run, got %d", len(issues))
		}
	})

	t.Run("DryRunWithJSONOutput", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)

		// Run create with --dry-run --json
		out := runBDInProcess(t, tmpDir, "create", "JSON dry run test", "-p", "2", "-t", "bug", "--dry-run", "--json")

		// Find JSON in output (may have warnings before it)
		jsonStart := strings.Index(out, "{")
		if jsonStart < 0 {
			t.Fatalf("No JSON found in output: %s", out)
		}
		jsonOut := out[jsonStart:]

		var issue map[string]interface{}
		if err := unmarshalEnvelope([]byte(jsonOut), &issue); err != nil {
			t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, jsonOut)
		}

		// Verify JSON has empty ID (not a placeholder string)
		id, ok := issue["id"]
		if !ok {
			t.Error("Expected 'id' field in JSON output")
		}
		if id != "" {
			t.Errorf("Expected empty ID in dry-run JSON, got: %v", id)
		}

		// Verify other fields are populated
		if issue["title"] != "JSON dry run test" {
			t.Errorf("Expected title 'JSON dry run test', got: %v", issue["title"])
		}
		if issue["issue_type"] != "bug" {
			t.Errorf("Expected issue_type 'bug', got: %v", issue["issue_type"])
		}
		if issue["priority"].(float64) != 2 {
			t.Errorf("Expected priority 2, got: %v", issue["priority"])
		}

		// Verify no issue was actually created
		listOut := runBDInProcess(t, tmpDir, "list", "--json")
		var issues []map[string]interface{}
		unmarshalEnvelope([]byte(listOut), &issues)
		if len(issues) != 0 {
			t.Errorf("Expected 0 issues after dry-run, got %d", len(issues))
		}
	})

	t.Run("DryRunWithLabelsAndDeps", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)

		// Run create with --dry-run including labels and deps
		out := runBDInProcess(t, tmpDir, "create", "Issue with extras", "-p", "1",
			"--labels", "urgent,backend",
			"--deps", "blocks:test-123",
			"--dry-run")

		// Verify labels are shown in preview
		if !strings.Contains(out, "Labels:") {
			t.Errorf("Expected 'Labels:' in output, got: %s", out)
		}
		if !strings.Contains(out, "urgent") {
			t.Errorf("Expected 'urgent' label in output, got: %s", out)
		}
		if !strings.Contains(out, "backend") {
			t.Errorf("Expected 'backend' label in output, got: %s", out)
		}

		// Verify dependencies are shown
		if !strings.Contains(out, "Dependencies:") {
			t.Errorf("Expected 'Dependencies:' in output, got: %s", out)
		}
		if !strings.Contains(out, "blocks:test-123") {
			t.Errorf("Expected 'blocks:test-123' dependency in output, got: %s", out)
		}
	})

	t.Run("DryRunWithRigPrefix", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)

		// Run create with --dry-run and --prefix (simulates cross-rig creation)
		// Note: This won't actually route to another rig since we don't have one,
		// but it should show the target rig in the preview
		out := runBDInProcess(t, tmpDir, "create", "Cross-rig issue", "-p", "1",
			"--prefix", "other-rig",
			"--dry-run")

		// Verify target rig is shown in preview
		if !strings.Contains(out, "Target rig:") {
			t.Errorf("Expected 'Target rig:' in output, got: %s", out)
		}
		if !strings.Contains(out, "other-rig") {
			t.Errorf("Expected 'other-rig' in output, got: %s", out)
		}
	})

	t.Run("DryRunWithFileReturnsError", func(t *testing.T) {
		tmpDir := createTempDirWithCleanup(t)

		// Initialize the database first
		initCmd := exec.Command(testBD, "init", "--prefix", "test", "--quiet")
		initCmd.Dir = tmpDir
		initCmd.Env = withoutWorkspaceEnv(os.Environ())
		if out, err := initCmd.CombinedOutput(); err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}

		// Create a dummy markdown file
		mdFile := filepath.Join(tmpDir, "issues.md")
		os.WriteFile(mdFile, []byte("# Test Issue\n\nDescription here"), 0644)

		// Run create with --dry-run and --file (should error)
		cmd := exec.Command(testBD, "create", "--file", mdFile, "--dry-run")
		cmd.Dir = tmpDir
		cmd.Env = withoutWorkspaceEnv(os.Environ())
		out, err := cmd.CombinedOutput()

		if err == nil {
			t.Error("Expected error when using --dry-run with --file, but got none")
		}

		// Verify error message is informative
		if !strings.Contains(string(out), "--dry-run is not supported with --file") {
			t.Errorf("Expected error about --dry-run with --file, got: %s", out)
		}
	})

	t.Run("DryRunWithEventType", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)

		// Run create with --dry-run and event-specific fields
		out := runBDInProcess(t, tmpDir, "create", "Event issue", "-p", "1",
			"--type", "event",
			"--event-category", "agent.started",
			"--dry-run")

		// Verify event category is shown in preview
		if !strings.Contains(out, "Event category:") {
			t.Errorf("Expected 'Event category:' in output, got: %s", out)
		}
		if !strings.Contains(out, "agent.started") {
			t.Errorf("Expected 'agent.started' in output, got: %s", out)
		}
	})

	t.Run("DryRunWithExplicitID", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)

		// Run create with --dry-run and explicit ID
		out := runBDInProcess(t, tmpDir, "create", "Explicit ID issue", "-p", "1",
			"--id", "test-explicit123",
			"--dry-run")

		// Verify explicit ID is shown (not "(will be generated)")
		if strings.Contains(out, "(will be generated)") {
			t.Errorf("Expected explicit ID in output, but got '(will be generated)': %s", out)
		}
		if !strings.Contains(out, "test-explicit123") {
			t.Errorf("Expected 'test-explicit123' in output, got: %s", out)
		}
	})
}

// TestCLI_CommentsListMisplacedSyntax ensures "bd comments list" gets a helpful error (GH#3542).
func TestCLI_CommentsListMisplacedSyntax(t *testing.T) {
	t.Parallel()

	tmpDir := setupCLITestDB(t)
	stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "comments", "list")
	if err == nil {
		t.Fatalf("expected non-zero exit, got stdout=%q stderr=%q", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "bd comments") || !strings.Contains(combined, "<issue-id>") {
		t.Fatalf("expected hint with bd comments and issue-id placeholder, got stdout=%q stderr=%q", stdout, stderr)
	}
}

// TestCLI_CommentsSwappedAddRejectedBeforeStoreOpen is a regression test for
// GH#4642's should-fix: the swapped-order rejection (`bd comments <id> add
// <text>`) must fire from commentsCmd's Args validator, before
// PersistentPreRunE ever opens a store or runs a migration, and before
// RunE's usesProxiedServer() dispatch — not from a check inside RunE that
// only the local-store branch reaches. Using a directory with no .beads/ at
// all proves the store was never touched: the old RunE-only check needed an
// already-initialized store (PersistentPreRunE would have failed first with
// "no beads database found"), so this would fail with the wrong error before
// the fix.
func TestCLI_CommentsSwappedAddRejectedBeforeStoreOpen(t *testing.T) {
	noBeadsCwd := t.TempDir()
	if _, err := os.Stat(filepath.Join(noBeadsCwd, ".beads")); err == nil {
		t.Fatalf("test setup: %s unexpectedly has a .beads dir", noBeadsCwd)
	}

	stdout, stderr, err := runBDInProcessAllowError(t, noBeadsCwd, "comments", "bd-123", "add", "should not be stored")
	if err == nil {
		t.Fatalf("expected non-zero exit for swapped-order add, got success:\nstdout: %s", stdout)
	}
	combined := stdout + stderr
	if strings.Contains(combined, "no beads database found") {
		t.Fatalf("Args validation did not run before store open: got the pre-fix error.\nOutput:\n%s", combined)
	}
	if !strings.Contains(combined, "bd comments add") {
		t.Errorf("expected hint pointing to `bd comments add`, got:\n%s", combined)
	}
}

// TestCLI_CommentsAddShortID tests that 'comments add' accepts short IDs (issue #1070)
// Most bd commands accept short IDs (e.g., "5wbm") but comments add previously required
// full IDs (e.g., "mike.vibe-coding-5wbm"). This test ensures short IDs work.
//
// Note: Short IDs work because the code calls utils.ResolvePartialID().
func TestCLI_CommentsAddShortID(t *testing.T) {

	t.Run("ShortIDWithCommentsAdd", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)

		// Create an issue and get its full ID
		out := runBDInProcess(t, tmpDir, "create", "Issue for comment test", "-p", "1", "--json")

		jsonStart := strings.Index(out, "{")
		if jsonStart < 0 {
			t.Fatalf("No JSON found in output: %s", out)
		}
		jsonOut := out[jsonStart:]

		var issue map[string]interface{}
		if err := unmarshalEnvelope([]byte(jsonOut), &issue); err != nil {
			t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, jsonOut)
		}

		fullID := issue["id"].(string)
		t.Logf("Created issue with full ID: %s", fullID)

		// Extract short ID (the part after the last hyphen in prefix-hash format)
		// For IDs like "test-abc123", the short ID is "abc123"
		parts := strings.Split(fullID, "-")
		if len(parts) < 2 {
			t.Fatalf("Unexpected ID format: %s", fullID)
		}
		shortID := parts[len(parts)-1]
		t.Logf("Using short ID: %s", shortID)

		// Add a comment using the SHORT ID (not full ID)
		stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "comments", "add", shortID, "Test comment with short ID")
		if err != nil {
			t.Fatalf("comments add failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		if !strings.Contains(stdout, "Comment added") {
			t.Errorf("Expected 'Comment added' in output, got: %s", stdout)
		}

		// Verify the comment was actually added by listing comments (use full ID for list)
		stdout, stderr, err = runBDInProcessAllowError(t, tmpDir, "comments", fullID)
		if err != nil {
			t.Fatalf("comments list failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		if !strings.Contains(stdout, "Test comment with short ID") {
			t.Errorf("Expected comment text in list output, got: %s", stdout)
		}
	})

	t.Run("PartialIDWithCommentsAdd", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)

		// Create an issue
		out := runBDInProcess(t, tmpDir, "create", "Issue for partial ID test", "-p", "1", "--json")

		jsonStart := strings.Index(out, "{")
		jsonOut := out[jsonStart:]

		var issue map[string]interface{}
		unmarshalEnvelope([]byte(jsonOut), &issue)
		fullID := issue["id"].(string)

		// Extract short ID and use only first 4 characters (partial match)
		parts := strings.Split(fullID, "-")
		shortID := parts[len(parts)-1]
		if len(shortID) > 4 {
			shortID = shortID[:4] // Use only first 4 chars for partial match
		}
		t.Logf("Full ID: %s, Partial ID: %s", fullID, shortID)

		// Add comment using partial ID
		stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "comments", "add", shortID, "Comment via partial ID")
		if err != nil {
			t.Fatalf("comments add with partial ID failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		if !strings.Contains(stdout, "Comment added") {
			t.Errorf("Expected 'Comment added' in output, got: %s", stdout)
		}
	})

	t.Run("CommentAliasWithShortID", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)

		// Create an issue
		out := runBDInProcess(t, tmpDir, "create", "Issue for alias test", "-p", "1", "--json")

		jsonStart := strings.Index(out, "{")
		jsonOut := out[jsonStart:]

		var issue map[string]interface{}
		unmarshalEnvelope([]byte(jsonOut), &issue)
		fullID := issue["id"].(string)

		// Extract short ID
		parts := strings.Split(fullID, "-")
		shortID := parts[len(parts)-1]

		// Use the 'comment' alias (deprecated but should still work)
		stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "comment", shortID, "Comment via alias with short ID")
		if err != nil {
			t.Fatalf("comment alias failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		if !strings.Contains(stdout, "Comment added") {
			t.Errorf("Expected 'Comment added' in output, got: %s", stdout)
		}
	})
}

// TestCLI_CommentListMisplacedSyntax is a regression test for the singular
// "comment" shorthand silently mis-resolving a subcommand-like typo as an
// issue id. "bd comment list <text>" (meant as an attempt to list comments,
// using the singular noun by mistake) used to parse as id="list",
// text=[<text>...] with no validation on the id — since no issue is
// literally named "list", ResolvePartialID's substring/prefix fallback
// would silently match it against whatever existing bead or wisp id
// happened to contain "list" and post the comment there instead of
// erroring. This asserts both halves of the fix: the command is rejected
// with a helpful hint (mirroring TestCLI_CommentsListMisplacedSyntax for
// the plural sibling), AND — the part that actually matters for this
// specific bug — no comment silently landed on an unrelated real issue.
func TestCLI_CommentListMisplacedSyntax(t *testing.T) {
	t.Parallel()

	tmpDir := setupCLITestDB(t)

	out := runBDInProcess(t, tmpDir, "create", "Bystander issue", "-p", "1", "--json")
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("No JSON found in create output: %s", out)
	}
	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out[jsonStart:]), &issue); err != nil {
		t.Fatalf("Failed to parse create JSON: %v\nOutput: %s", err, out)
	}
	fullID := issue["id"].(string)

	stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "comment", "list", "accidental stray text")
	if err == nil {
		t.Fatalf("expected non-zero exit, got stdout=%q stderr=%q", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "bd comments") || !strings.Contains(combined, "<issue-id>") {
		t.Fatalf("expected hint pointing at bd comments <issue-id>, got stdout=%q stderr=%q", stdout, stderr)
	}

	// The regression check: the bystander issue must have received NO
	// comment. Pre-fix, this could silently succeed and land a comment on
	// whatever bead's id happened to contain "list" as a substring — this
	// DB has exactly one issue, so any stray write would show up here.
	commentsOut := runBDInProcess(t, tmpDir, "comments", fullID, "--json")
	trimmed := strings.TrimSpace(commentsOut)
	if trimmed != "[]" && trimmed != "null" {
		t.Fatalf("expected no comments on bystander issue %s after rejected 'comment list', got: %s", fullID, commentsOut)
	}
}

// TestCLI_CommentAddMisplacedSyntax mirrors TestCLI_CommentListMisplacedSyntax
// for the symmetric "add" typo — a caller who confuses the singular shorthand
// with the plural's "bd comments add <id> <text>" form might type
// "bd comment add <id> <text>", which the same unguarded id-resolution bug
// would parse as id="add", text=[<id>, <text>...].
func TestCLI_CommentAddMisplacedSyntax(t *testing.T) {
	t.Parallel()

	tmpDir := setupCLITestDB(t)

	out := runBDInProcess(t, tmpDir, "create", "Bystander issue for add-typo", "-p", "1", "--json")
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("No JSON found in create output: %s", out)
	}
	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out[jsonStart:]), &issue); err != nil {
		t.Fatalf("Failed to parse create JSON: %v\nOutput: %s", err, out)
	}
	fullID := issue["id"].(string)

	stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "comment", "add", fullID, "accidental stray text")
	if err == nil {
		t.Fatalf("expected non-zero exit, got stdout=%q stderr=%q", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "bd comments add") {
		t.Fatalf("expected hint pointing at bd comments add <issue-id>, got stdout=%q stderr=%q", stdout, stderr)
	}

	commentsOut := runBDInProcess(t, tmpDir, "comments", fullID, "--json")
	trimmed := strings.TrimSpace(commentsOut)
	if trimmed != "[]" && trimmed != "null" {
		t.Fatalf("expected no comments on bystander issue %s after rejected 'comment add', got: %s", fullID, commentsOut)
	}
}

// TestCLI_CommentMisplacedSyntaxRejectedBeforeStoreOpen proves the guard
// fires from commentCmd's Args validator, before PersistentPreRunE ever
// opens a store, mirroring
// TestCLI_CommentsSwappedAddRejectedBeforeStoreOpen for the plural sibling.
// Using a directory with no .beads/ at all proves the store was never
// touched: a RunE-only check would have failed first with "no beads
// database found".
func TestCLI_CommentMisplacedSyntaxRejectedBeforeStoreOpen(t *testing.T) {
	noBeadsCwd := t.TempDir()
	if _, err := os.Stat(filepath.Join(noBeadsCwd, ".beads")); err == nil {
		t.Fatalf("test setup: %s unexpectedly has a .beads dir", noBeadsCwd)
	}

	stdout, stderr, err := runBDInProcessAllowError(t, noBeadsCwd, "comment", "list", "should not be stored")
	if err == nil {
		t.Fatalf("expected non-zero exit for misplaced 'comment list', got success:\nstdout: %s", stdout)
	}
	combined := stdout + stderr
	if strings.Contains(combined, "no beads database found") {
		t.Fatalf("Args validation did not run before store open: got the pre-fix error.\nOutput:\n%s", combined)
	}
	if !strings.Contains(combined, "bd comments") {
		t.Errorf("expected hint pointing to `bd comments`, got:\n%s", combined)
	}
}

// TestCLI_CommentTextStartingWithReservedWordStillWorks confirms the fix is
// scoped to the id positional argument only: a real id followed by comment
// TEXT that happens to start with "list" or "add" must still work exactly
// as before, since args[0] (the id slot) is what's checked, never the text.
func TestCLI_CommentTextStartingWithReservedWordStillWorks(t *testing.T) {
	t.Parallel()

	tmpDir := setupCLITestDB(t)

	out := runBDInProcess(t, tmpDir, "create", "Issue for reserved-word-text test", "-p", "1", "--json")
	jsonStart := strings.Index(out, "{")
	if jsonStart < 0 {
		t.Fatalf("No JSON found in create output: %s", out)
	}
	var issue map[string]interface{}
	if err := unmarshalEnvelope([]byte(out[jsonStart:]), &issue); err != nil {
		t.Fatalf("Failed to parse create JSON: %v\nOutput: %s", err, out)
	}
	fullID := issue["id"].(string)

	stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, "comment", fullID, "list", "of", "things", "to", "do")
	if err != nil {
		t.Fatalf("comment with text starting in 'list' unexpectedly failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Comment added") {
		t.Errorf("Expected 'Comment added' in output, got: %s", stdout)
	}

	commentsOut := runBDInProcess(t, tmpDir, "comments", fullID, "--json")
	if !strings.Contains(commentsOut, "list of things to do") {
		t.Fatalf("expected comment text to be stored verbatim, got: %s", commentsOut)
	}
}

// TestCLI_CreateRejectsFlagLikeTitles verifies that positional arguments starting
// with - or -- are rejected as likely misinterpreted flags (bd-2c0).
func TestCLI_CreateRejectsFlagLikeTitles(t *testing.T) {
	if testDoltServerPort == 0 {
		t.Skip("skipping: Dolt test container not available")
	}

	tests := []struct {
		name  string
		title string
	}{
		{"DoubleDashHelp", "--help"},
		{"DoubleDashVersion", "--version"},
		{"SingleDashFlag", "-p"},
		{"DoubleDashArbitrary", "--foo-bar"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := createTempDirWithCleanup(t)

			// Initialize the database
			initCmd := exec.Command(testBD, "init", "--prefix", "test", "--quiet")
			initCmd.Dir = tmpDir
			initCmd.Env = withoutWorkspaceEnv(os.Environ())
			if out, err := initCmd.CombinedOutput(); err != nil {
				t.Fatalf("init failed: %v\n%s", err, out)
			}

			// Attempt to create with a flag-like positional title
			cmd := exec.Command(testBD, "create", tc.title)
			cmd.Dir = tmpDir
			cmd.Env = withoutWorkspaceEnv(os.Environ())
			out, err := cmd.CombinedOutput()

			if err == nil {
				t.Errorf("Expected error for flag-like title %q, but got none.\nOutput: %s", tc.title, out)
			}

			if !strings.Contains(string(out), "looks like a flag") {
				t.Errorf("Expected 'looks like a flag' error for %q, got: %s", tc.title, out)
			}
		})
	}

	// Verify that --title flag with dash-prefixed value is still allowed
	t.Run("TitleFlagAllowsDashes", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)

		out := runBDInProcess(t, tmpDir, "create", "--title", "--unusual-title", "-p", "1", "--json")
		if !strings.Contains(out, "--unusual-title") {
			t.Errorf("Expected title '--unusual-title' in output, got: %s", out)
		}
	})
}

// TestCLI_CreateRejectsEmptyTitle verifies that a whitespace-only title is
// refused rather than silently creating a title-less bead (GH#4771). Such a
// bead is functionally invisible in title-based views.
func TestCLI_CreateRejectsEmptyTitle(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"FlagSpaces", []string{"create", "--title", "   ", "-p", "2"}},
		{"FlagTab", []string{"create", "--title", "\t", "-p", "2"}},
		{"PositionalSpaces", []string{"create", "   ", "-p", "2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := setupCLITestDB(t)
			stdout, stderr, err := runBDInProcessAllowError(t, tmpDir, tc.args...)
			if err == nil {
				t.Fatalf("expected error for whitespace-only title, got success\nstdout: %s", stdout)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, "title cannot be empty") {
				t.Errorf("expected 'title cannot be empty' error, got: %s", combined)
			}
		})
	}
}

// TestCLI_CreateNoHistory tests that the --no-history CLI flag is wired through
// to the created issue (GH#2619). A storage-layer test already covers the DB
// semantics; this test verifies the CLI flag is actually parsed and passed.
func TestCLI_CreateNoHistory(t *testing.T) {

	t.Run("NoHistoryFlagSetOnCreatedIssue", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)
		out := runBDInProcess(t, tmpDir, "create", "No-history agent bead", "-p", "2", "--no-history", "--json")

		var result map[string]interface{}
		if err := unmarshalEnvelope([]byte(out), &result); err != nil {
			t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, out)
		}
		if result["no_history"] != true {
			t.Errorf("Expected no_history=true on created issue, got: %v", result["no_history"])
		}
		// Must NOT be ephemeral (mutually exclusive)
		if result["ephemeral"] == true {
			t.Errorf("no-history issue must not be ephemeral, but ephemeral=true")
		}
	})

	t.Run("NoHistoryPersistedAfterShow", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)
		out := runBDInProcess(t, tmpDir, "create", "No-history bead for show test", "-p", "2", "--no-history", "--json")

		var created map[string]interface{}
		if err := unmarshalEnvelope([]byte(out), &created); err != nil {
			t.Fatalf("Failed to parse create output: %v\nOutput: %s", err, out)
		}
		id := created["id"].(string)

		showOut := runBDInProcess(t, tmpDir, "show", id, "--json")
		var issues []map[string]interface{}
		if err := unmarshalEnvelope([]byte(showOut), &issues); err != nil {
			t.Fatalf("Failed to parse show output: %v\nOutput: %s", err, showOut)
		}
		if len(issues) == 0 {
			t.Fatalf("show returned no issues for id %s", id)
		}
		if issues[0]["no_history"] != true {
			t.Errorf("Expected no_history=true after show, got: %v", issues[0]["no_history"])
		}
	})

	t.Run("EphemeralAndNoHistoryMutuallyExclusive", func(t *testing.T) {
		tmpDir := setupCLITestDB(t)
		_, stderr, err := runBDInProcessAllowError(t, tmpDir, "create", "Should fail", "--ephemeral", "--no-history")
		if err == nil {
			t.Error("Expected error when combining --ephemeral and --no-history, got none")
		}
		if !strings.Contains(stderr, "mutually exclusive") {
			t.Errorf("Expected 'mutually exclusive' in stderr, got: %s", stderr)
		}
	})
}

// TestCLI_WispListTypeFilter tests that bd mol wisp list --type filters correctly.
func TestCLI_WispListTypeFilter(t *testing.T) {

	tmpDir := setupCLITestDB(t)

	// Create two ephemeral wisps of different built-in types
	runBDInProcess(t, tmpDir, "create", "Bug wisp", "--ephemeral", "--type", "bug", "-p", "2")
	runBDInProcess(t, tmpDir, "create", "Task wisp", "--ephemeral", "--type", "task", "-p", "2")

	// --type bug should return only the bug wisp
	out := runBDInProcess(t, tmpDir, "mol", "wisp", "list", "--type", "bug", "--json")
	var result map[string]interface{}
	if err := unmarshalEnvelope([]byte(out), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, out)
	}
	wisps, ok := result["wisps"].([]interface{})
	if !ok {
		t.Fatalf("Expected 'wisps' array in JSON output, got: %v", result)
	}
	if len(wisps) != 1 {
		t.Errorf("Expected 1 wisp with type=bug, got %d: %v", len(wisps), wisps)
	}
	if len(wisps) == 1 {
		w := wisps[0].(map[string]interface{})
		if w["type"] != "bug" {
			t.Errorf("Expected wisp type=bug, got: %v", w["type"])
		}
	}

	// --type task should return only the task wisp
	out = runBDInProcess(t, tmpDir, "mol", "wisp", "list", "--type", "task", "--json")
	if err := unmarshalEnvelope([]byte(out), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, out)
	}
	wisps, ok = result["wisps"].([]interface{})
	if !ok {
		t.Fatalf("Expected 'wisps' array, got: %v", result)
	}
	if len(wisps) != 1 {
		t.Errorf("Expected 1 wisp with type=task, got %d", len(wisps))
	}
	if len(wisps) == 1 {
		w := wisps[0].(map[string]interface{})
		if w["type"] != "task" {
			t.Errorf("Expected wisp type=task, got: %v", w["type"])
		}
	}

	// No --type filter returns both
	out = runBDInProcess(t, tmpDir, "mol", "wisp", "list", "--json")
	if err := unmarshalEnvelope([]byte(out), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, out)
	}
	wisps = result["wisps"].([]interface{})
	if len(wisps) != 2 {
		t.Errorf("Expected 2 wisps without type filter, got %d", len(wisps))
	}
}

// TestCLI_WispGCExcludeType tests that bd mol wisp gc --exclude-type skips
// wisps of the excluded type during garbage collection.
func TestCLI_WispGCExcludeType(t *testing.T) {

	tmpDir := setupCLITestDB(t)

	// Create two ephemeral wisps of different built-in types.
	// Use --age=0s so that ALL wisps (regardless of actual age) are treated
	// as abandoned GC candidates.
	bugOut := runBDInProcess(t, tmpDir, "create", "Bug wisp to keep", "--ephemeral", "--type", "bug", "-p", "2", "--json")
	taskOut := runBDInProcess(t, tmpDir, "create", "Task wisp to gc", "--ephemeral", "--type", "task", "-p", "2", "--json")

	var bugCreated, taskCreated map[string]interface{}
	if err := unmarshalEnvelope([]byte(bugOut), &bugCreated); err != nil {
		t.Fatalf("Failed to parse bug create output: %v\nOutput: %s", err, bugOut)
	}
	if err := unmarshalEnvelope([]byte(taskOut), &taskCreated); err != nil {
		t.Fatalf("Failed to parse task create output: %v\nOutput: %s", err, taskOut)
	}
	bugID := bugCreated["id"].(string)
	taskID := taskCreated["id"].(string)

	// Dry-run GC with --exclude-type bug, --age=0s (treats all wisps as abandoned).
	// The WispGCResult dry-run JSON uses cleaned_ids to list what would be deleted.
	gcOut := runBDInProcess(t, tmpDir, "mol", "wisp", "gc", "--exclude-type", "bug", "--age", "0s", "--dry-run", "--json")
	var gcResult map[string]interface{}
	if err := unmarshalEnvelope([]byte(gcOut), &gcResult); err != nil {
		t.Fatalf("Failed to parse gc JSON: %v\nOutput: %s", err, gcOut)
	}

	// cleaned_ids holds the IDs that would be deleted (dry_run=true).
	cleanedRaw, _ := gcResult["cleaned_ids"].([]interface{})
	cleanedIDs := make(map[string]bool, len(cleanedRaw))
	for _, v := range cleanedRaw {
		if id, ok := v.(string); ok {
			cleanedIDs[id] = true
		}
	}

	if !cleanedIDs[taskID] {
		t.Errorf("Expected task wisp %s in GC candidates (not excluded), got cleaned_ids: %v", taskID, cleanedRaw)
	}
	if cleanedIDs[bugID] {
		t.Errorf("Bug wisp %s should be excluded from GC via --exclude-type bug, but appeared in cleaned_ids", bugID)
	}
}
