package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBackupStatusSizeErrorRespectsOutputMode(t *testing.T) {
	prepareBackupStatusTest(t)
	missingRoot := filepath.Join(t.TempDir(), "missing")
	sizeErr := errors.New("active database directory is missing: " + missingRoot)
	sizeDatabase := func(context.Context) (int64, bool, error) {
		return 0, false, sizeErr
	}

	t.Run("json", func(t *testing.T) {
		cmd := newBackupStatusTestRoot(sizeDatabase)
		cmd.SetArgs([]string{"backup", "status", "--json"})

		stdout, stderr, err := executeBackupStatusCommand(t, cmd)
		assertExitCode(t, err, 1)
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}

		var response struct {
			Data struct {
				Error string `json:"error"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &response); err != nil {
			t.Fatalf("stdout is not a single valid JSON response: %v\nstdout: %s", err, stdout)
		}
		if !strings.Contains(response.Data.Error, "measure database size") || !strings.Contains(response.Data.Error, missingRoot) {
			t.Errorf("data.error = %q, want database-size diagnostic containing %q", response.Data.Error, missingRoot)
		}
		if strings.Contains(stdout, "Usage:") {
			t.Errorf("stdout contains usage noise: %s", stdout)
		}
	})

	t.Run("human", func(t *testing.T) {
		cmd := newBackupStatusTestRoot(sizeDatabase)
		cmd.SetArgs([]string{"backup", "status"})

		stdout, stderr, err := executeBackupStatusCommand(t, cmd)
		assertExitCode(t, err, 1)
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "Error: measure database size:") || !strings.Contains(stderr, missingRoot) {
			t.Errorf("stderr = %q, want database-size diagnostic containing %q", stderr, missingRoot)
		}
		if strings.Contains(stderr, "Usage:") || strings.Contains(stderr, "exit code 1") {
			t.Errorf("stderr contains Cobra noise: %s", stderr)
		}
	})
}

func TestBackupStatusOmitsUnsupportedDatabaseSize(t *testing.T) {
	prepareBackupStatusTest(t)
	sizeDatabase := func(context.Context) (int64, bool, error) {
		return 0, false, nil
	}

	t.Run("json", func(t *testing.T) {
		cmd := newBackupStatusTestRoot(sizeDatabase)
		cmd.SetArgs([]string{"backup", "status", "--json"})

		stdout, stderr, err := executeBackupStatusCommand(t, cmd)
		if err != nil {
			t.Fatalf("backup status: %v", err)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
		report := backupStatusEnvelopeReport(t, backupStatusJSONDocument(t, stdout), stdout)
		if _, ok := report["database_size"]; ok {
			t.Errorf("database_size should be omitted: %s", stdout)
		}
		if _, ok := report["backup"]; !ok {
			t.Errorf("backup status missing backup field: %s", stdout)
		}
		if _, ok := report["dolt"]; !ok {
			t.Errorf("backup status missing dolt field: %s", stdout)
		}
	})

	t.Run("human", func(t *testing.T) {
		cmd := newBackupStatusTestRoot(sizeDatabase)
		cmd.SetArgs([]string{"backup", "status"})

		stdout, stderr, err := executeBackupStatusCommand(t, cmd)
		if err != nil {
			t.Fatalf("backup status: %v", err)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
		if strings.Contains(stdout, "Database size:") {
			t.Errorf("unsupported database size should be omitted: %s", stdout)
		}
		if !strings.Contains(stdout, "No backup has been performed yet.") {
			t.Errorf("remaining status output was not rendered: %s", stdout)
		}
	})
}

func TestBackupStatusIncludesAvailableDatabaseSize(t *testing.T) {
	prepareBackupStatusTest(t)
	sizeDatabase := func(context.Context) (int64, bool, error) {
		return 1536, true, nil
	}

	t.Run("json", func(t *testing.T) {
		cmd := newBackupStatusTestRoot(sizeDatabase)
		cmd.SetArgs([]string{"backup", "status", "--json"})
		stdout, stderr, err := executeBackupStatusCommand(t, cmd)
		if err != nil {
			t.Fatalf("backup status: %v", err)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
		// The report sits under data, beside the schema_version the wrapper
		// stamps on every --json document. A hand-rolled marshal would carry
		// neither, so the envelope check fails by name when it is bypassed.
		document := backupStatusJSONDocument(t, stdout)
		if _, ok := document["database_size"]; ok {
			t.Errorf("database_size sits at the top level beside the envelope; it belongs under data: %s", stdout)
		}
		report := backupStatusEnvelopeReport(t, document, stdout)
		assertDatabaseSizeBytes(t, report, 1536, stdout)
		if _, ok := report["backup"]; !ok {
			t.Errorf("envelope data lost the backup field: %s", stdout)
		}
		if _, ok := report["dolt"]; !ok {
			t.Errorf("envelope data lost the dolt field: %s", stdout)
		}
	})

	t.Run("human", func(t *testing.T) {
		cmd := newBackupStatusTestRoot(sizeDatabase)
		cmd.SetArgs([]string{"backup", "status"})
		stdout, stderr, err := executeBackupStatusCommand(t, cmd)
		if err != nil {
			t.Fatalf("backup status: %v", err)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
		if !strings.Contains(stdout, "Database size: 1.5 KB") {
			t.Errorf("available database size missing: %s", stdout)
		}
	})
}

// TestBackupStatusJSONKeepsMachineModeShape pins what this command does NOT
// change: in machine mode the bare report is what callers already get, and its
// shape belongs to the machine envelope's own bead, not to the schema-wrapper
// migration of the stdio path. Machine mode is a package-level flag that main()
// decides from the arguments, so the package's own withMachineMode seam forces
// it here.
func TestBackupStatusJSONKeepsMachineModeShape(t *testing.T) {
	prepareBackupStatusTest(t)
	withMachineMode(t, true)

	sizeDatabase := func(context.Context) (int64, bool, error) {
		return 1536, true, nil
	}

	cmd := newBackupStatusTestRoot(sizeDatabase)
	cmd.SetArgs([]string{"backup", "status", "--json"})
	stdout, stderr, err := executeBackupStatusCommand(t, cmd)
	if err != nil {
		t.Fatalf("backup status: %v", err)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}

	var response map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if _, ok := response["schema_version"]; ok {
		t.Errorf("machine mode gained a schema_version on the stdio path; BD_MACHINE output must not change here: %s", stdout)
	}
	if _, ok := response["backup"]; !ok {
		t.Errorf("machine mode report lost its backup field: %s", stdout)
	}
	if _, ok := response["database_size"]; !ok {
		t.Errorf("machine mode report lost its database_size field: %s", stdout)
	}
}

// backupStatusJSONDocument decodes one `backup status --json` document and
// asserts the wrapper contract every --json command shares: a top-level
// schema_version equal to JSONSchemaVersion. The hand-rolled marshal this
// branch replaced carried none, so a regression fails here by name. The
// report itself sits under data; the caller reads it through
// backupStatusEnvelopeReport.
func backupStatusJSONDocument(t *testing.T, stdout string) map[string]json.RawMessage {
	t.Helper()

	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	rawVersion, ok := document["schema_version"]
	if !ok {
		t.Fatalf("backup status --json carries no schema_version; it bypasses the schema wrapper: %s", stdout)
	}
	var version int
	if err := json.Unmarshal(rawVersion, &version); err != nil {
		t.Fatalf("schema_version is not a number: %v\nstdout: %s", err, stdout)
	}
	if version != JSONSchemaVersion {
		t.Errorf("schema_version = %d, want %d", version, JSONSchemaVersion)
	}
	return document
}

// backupStatusEnvelopeReport returns the "data" object of an envelope document
// and fails when the report is not nested under data.
func backupStatusEnvelopeReport(t *testing.T, document map[string]json.RawMessage, stdout string) map[string]json.RawMessage {
	t.Helper()

	rawData, ok := document["data"]
	if !ok {
		t.Fatalf("envelope output has no data object: %s", stdout)
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(rawData, &report); err != nil {
		t.Fatalf("envelope data is not an object: %v\nstdout: %s", err, stdout)
	}
	return report
}

// assertDatabaseSizeBytes asserts the report's database_size.bytes, failing
// when the field is missing so a dropped database_size is reported by name
// rather than as a zero.
func assertDatabaseSizeBytes(t *testing.T, report map[string]json.RawMessage, want int64, stdout string) {
	t.Helper()

	raw, ok := report["database_size"]
	if !ok {
		t.Fatalf("database_size missing from the JSON report: %s", stdout)
	}
	var size struct {
		Bytes int64 `json:"bytes"`
	}
	if err := json.Unmarshal(raw, &size); err != nil {
		t.Fatalf("database_size is not an object: %v\nstdout: %s", err, stdout)
	}
	if size.Bytes != want {
		t.Errorf("database_size.bytes = %d, want %d", size.Bytes, want)
	}
}

func prepareBackupStatusTest(t *testing.T) {
	t.Helper()

	beadsDir := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(filepath.Join(beadsDir, "embeddeddolt"), 0o700); err != nil {
		t.Fatalf("create workspace marker: %v", err)
	}
	t.Setenv("BEADS_DIR", beadsDir)
	initConfigForTest(t)

	oldJSONOutput := jsonOutput
	jsonOutput = false
	resetCommandContext()
	t.Cleanup(func() {
		jsonOutput = oldJSONOutput
		resetCommandContext()
	})
}

func newBackupStatusTestRoot(sizeDatabase backupSizeFunc) *cobra.Command {
	root := &cobra.Command{Use: "bd"}
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output JSON")
	backup := &cobra.Command{Use: "backup"}
	backup.AddCommand(newBackupStatusCommand(sizeDatabase))
	root.AddCommand(backup)
	return root
}

func executeBackupStatusCommand(t *testing.T, cmd *cobra.Command) (string, string, error) {
	t.Helper()

	stdioMutex.Lock()
	defer stdioMutex.Unlock()

	captureDir := t.TempDir()
	stdoutPath := filepath.Join(captureDir, "stdout")
	stderrPath := filepath.Join(captureDir, "stderr")
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		t.Fatalf("create stdout capture: %v", err)
	}
	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		_ = stdoutFile.Close()
		t.Fatalf("create stderr capture: %v", err)
	}

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	var commandErr error
	func() {
		defer func() {
			os.Stdout = oldStdout
			os.Stderr = oldStderr
		}()
		os.Stdout = stdoutFile
		os.Stderr = stderrFile
		commandErr = cmd.Execute()
	}()

	if err := stdoutFile.Close(); err != nil {
		t.Fatalf("close stdout capture: %v", err)
	}
	if err := stderrFile.Close(); err != nil {
		t.Fatalf("close stderr capture: %v", err)
	}
	stdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatalf("read stdout capture: %v", err)
	}
	stderr, err := os.ReadFile(stderrPath)
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	return string(stdout), string(stderr), commandErr
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("command succeeded, want exit code %d", want)
	}
	got, ok := exitCodeFromError(err)
	if !ok || got != want {
		t.Fatalf("command error = %v, want exit code %d", err, want)
	}
}
