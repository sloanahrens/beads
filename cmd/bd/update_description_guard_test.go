package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/steveyegge/beads"
	"github.com/steveyegge/beads/internal/debug"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/issueops"
)

func newUpdateDescriptionGuardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {},
	}
	registerCommonIssueFlags(cmd)
	cmd.Flags().Bool("allow-empty-description", false, "Allow empty description replacement when reading from stdin or file")
	return cmd
}

func withTestStdin(t *testing.T, content string, fn func()) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	go func() {
		_, _ = w.WriteString(content)
		_ = w.Close()
	}()

	fn()
}

func TestValidateDescriptionUpdateRejectsEmptyStdinWithoutOptIn(t *testing.T) {
	withTestStdin(t, "", func() {
		cmd := newUpdateDescriptionGuardCmd()
		if err := cmd.ParseFlags([]string{"--stdin"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}

		description, changed, flagErr := getDescriptionFlag(cmd)
		if flagErr != nil {
			t.Fatalf("unexpected error: %v", flagErr)
		}
		err := validateDescriptionUpdate(cmd, description, changed)
		if err == nil {
			t.Fatal("expected empty stdin description to be rejected")
		}
		if !strings.Contains(err.Error(), "--allow-empty-description") {
			t.Fatalf("expected opt-in guidance in error, got: %v", err)
		}
	})
}

func TestValidateDescriptionUpdateRejectsEmptyDashShorthandWithoutOptIn(t *testing.T) {
	withTestStdin(t, "", func() {
		cmd := newUpdateDescriptionGuardCmd()
		if err := cmd.ParseFlags([]string{"--description", "-"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}

		description, changed, flagErr := getDescriptionFlag(cmd)
		if flagErr != nil {
			t.Fatalf("unexpected error: %v", flagErr)
		}
		err := validateDescriptionUpdate(cmd, description, changed)
		if err == nil {
			t.Fatal("expected empty dash shorthand description to be rejected")
		}
		if !strings.Contains(err.Error(), "--allow-empty-description") {
			t.Fatalf("expected opt-in guidance in error, got: %v", err)
		}
	})
}

func TestValidateDescriptionUpdateRejectsEmptyBodyFileWithoutOptIn(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "empty.md")
	if err := os.WriteFile(filePath, []byte{}, 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}

	cmd := newUpdateDescriptionGuardCmd()
	if err := cmd.ParseFlags([]string{"--body-file", filePath}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	description, changed, flagErr := getDescriptionFlag(cmd)
	if flagErr != nil {
		t.Fatalf("unexpected error: %v", flagErr)
	}
	err := validateDescriptionUpdate(cmd, description, changed)
	if err == nil {
		t.Fatal("expected empty body file description to be rejected")
	}
	if !strings.Contains(err.Error(), "--allow-empty-description") {
		t.Fatalf("expected opt-in guidance in error, got: %v", err)
	}
}

func TestValidateDescriptionUpdateAllowsEmptyStdinWithOptIn(t *testing.T) {
	withTestStdin(t, "", func() {
		cmd := newUpdateDescriptionGuardCmd()
		if err := cmd.ParseFlags([]string{"--stdin", "--allow-empty-description"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}

		description, changed, flagErr := getDescriptionFlag(cmd)
		if flagErr != nil {
			t.Fatalf("unexpected error: %v", flagErr)
		}
		if err := validateDescriptionUpdate(cmd, description, changed); err != nil {
			t.Fatalf("expected opt-in empty stdin to succeed, got: %v", err)
		}
		if description != "" {
			t.Fatalf("expected empty description, got %q", description)
		}
	})
}

func TestValidateDescriptionUpdateAllowsNonEmptyStdinWithoutOptIn(t *testing.T) {
	withTestStdin(t, "Updated from stdin\n", func() {
		cmd := newUpdateDescriptionGuardCmd()
		if err := cmd.ParseFlags([]string{"--stdin"}); err != nil {
			t.Fatalf("failed to parse flags: %v", err)
		}

		description, changed, flagErr := getDescriptionFlag(cmd)
		if flagErr != nil {
			t.Fatalf("unexpected error: %v", flagErr)
		}
		if err := validateDescriptionUpdate(cmd, description, changed); err != nil {
			t.Fatalf("expected non-empty stdin to succeed, got: %v", err)
		}
		if description != "Updated from stdin\n" {
			t.Fatalf("expected stdin content to round-trip, got %q", description)
		}
	})
}

// The two types below are the seams a description-only `bd update` reaches: the
// store reads its resolution step makes, and the guarded lifecycle write.
// Any other store method resolves through the embedded nil interface and
// panics, so a code path this test did not anticipate fails loudly instead of
// passing on a fake that answered everything.

type descriptionGuardStore struct {
	storage.DoltStorage
	issue *types.Issue
}

func (s *descriptionGuardStore) SearchIssues(_ context.Context, _ string, filter types.IssueFilter) ([]*types.Issue, error) {
	for _, id := range filter.IDs {
		if id == s.issue.ID {
			return []*types.Issue{s.issue}, nil
		}
	}
	return nil, nil
}

func (s *descriptionGuardStore) GetIssue(_ context.Context, id string) (*types.Issue, error) {
	if id != s.issue.ID {
		return nil, fmt.Errorf("issue %s not found", id)
	}
	return s.issue, nil
}

type recordingUpdateLifecycle struct {
	requests []issueops.UpdateRequest
	issue    *types.Issue
}

func (l *recordingUpdateLifecycle) Create(context.Context, issueops.CreateRequest) (issueops.CreateResult, error) {
	panic("bd update called Create")
}

func (l *recordingUpdateLifecycle) Update(_ context.Context, request issueops.UpdateRequest) (issueops.UpdateResult, error) {
	l.requests = append(l.requests, request)
	return issueops.UpdateResult{Issue: l.issue, Changed: true}, nil
}

func (l *recordingUpdateLifecycle) Close(context.Context, issueops.CloseRequest) (issueops.CloseResult, error) {
	panic("bd update called Close")
}

func (l *recordingUpdateLifecycle) Reopen(context.Context, issueops.ReopenRequest) (issueops.ReopenResult, error) {
	panic("bd update called Reopen")
}

// newDescriptionUpdateEnv wires the package globals updateCmd.RunE reads at a
// fake store and a recording lifecycle, hermetically: the working directory is
// a temporary directory (CheckMigrationFreeze walks it looking for a town
// marker) and the town-root variables are pinned empty so no run can read the
// live town tree.
func newDescriptionUpdateEnv(t *testing.T) *recordingUpdateLifecycle {
	t.Helper()

	saveAndRestoreGlobals(t)
	ensureCleanGlobalState(t)

	beadsDir := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(beadsDir, 0o700); err != nil {
		t.Fatalf("create beads dir: %v", err)
	}
	t.Setenv("BEADS_DIR", beadsDir)
	t.Setenv("GT_TOWN_ROOT", "")
	t.Setenv("GT_ROOT", "")
	t.Setenv("NO_COLOR", "1")
	t.Chdir(t.TempDir())
	initConfigForTest(t)

	savedCtx, savedJSON, savedActor := rootCtx, jsonOutput, actor
	savedQuiet, savedReadonly := quietFlag, readonlyMode
	savedOps := newIssueOperations
	savedDebugQuiet := debug.IsQuiet()
	t.Cleanup(func() {
		rootCtx, jsonOutput, actor = savedCtx, savedJSON, savedActor
		quietFlag, readonlyMode = savedQuiet, savedReadonly
		newIssueOperations = savedOps
		debug.SetQuiet(savedDebugQuiet)
	})

	issue := &types.Issue{
		ID:          "test-desc",
		Title:       "description guard",
		Status:      types.StatusOpen,
		Priority:    2,
		IssueType:   types.TypeTask,
		Description: "original description",
	}
	lifecycle := &recordingUpdateLifecycle{issue: issue}

	store = &descriptionGuardStore{issue: issue}
	rootCtx = context.Background()
	jsonOutput = false
	actor = "description-guard-test"
	quietFlag = true
	readonlyMode = false
	debug.SetQuiet(true)
	newIssueOperations = func(beads.Storage) (issueops.Lifecycle, error) {
		return lifecycle, nil
	}

	return lifecycle
}

// setUpdateFlags sets flags on the shared updateCmd and resets them afterwards.
func setUpdateFlags(t *testing.T, values map[string]string) {
	t.Helper()
	t.Cleanup(resetUpdateCmdFlagsToDefaults)
	for name, value := range values {
		if err := updateCmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%q: %v", name, value, err)
		}
	}
}

func resetUpdateCmdFlagsToDefaults() {
	updateCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if slice, ok := f.Value.(pflag.SliceValue); ok {
			_ = slice.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
}

// TestUpdateRefusesEmptyDescription is the be-neiv guard: an empty or
// whitespace-only --description, in either spelling, is refused before the
// per-issue write loop, with the flag and every named id in the error, and
// nothing written.
func TestUpdateRefusesEmptyDescription(t *testing.T) {
	tests := []struct {
		name  string
		value string
		ids   []string
	}{
		{name: "explicit empty value", value: "", ids: []string{"test-desc"}},
		{name: "whitespace only value", value: " \n\t ", ids: []string{"test-desc"}},
		{name: "every named id appears in the refusal", value: "", ids: []string{"test-desc", "test-other"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lifecycle := newDescriptionUpdateEnv(t)
			setUpdateFlags(t, map[string]string{"description": tc.value})

			var runErr error
			stderr := captureStderr(t, func() {
				runErr = updateCmd.RunE(updateCmd, tc.ids)
			})

			if runErr == nil {
				t.Fatalf("bd update --description %q succeeded, want a refusal\nstderr:\n%s", tc.value, stderr)
			}
			if code, ok := exitCodeFromError(runErr); !ok || code != 1 {
				t.Errorf("exit code = %d (recognized=%v), want 1\nstderr:\n%s", code, ok, stderr)
			}
			if !strings.Contains(stderr, "--description") {
				t.Errorf("refusal does not name --description\nstderr:\n%s", stderr)
			}
			for _, id := range tc.ids {
				if !strings.Contains(stderr, id) {
					t.Errorf("refusal does not name %s\nstderr:\n%s", id, stderr)
				}
			}
			if len(lifecycle.requests) != 0 {
				t.Fatalf("refused update still performed %d mutation(s), want none", len(lifecycle.requests))
			}
		})
	}
}

// TestUpdateRefusesEmptyDescriptionFromStdinDash keeps the pre-existing
// refusal of an empty stdin body: --description - reads stdin, and an empty
// read is an empty description.
func TestUpdateRefusesEmptyDescriptionFromStdinDash(t *testing.T) {
	lifecycle := newDescriptionUpdateEnv(t)
	setUpdateFlags(t, map[string]string{"description": "-"})

	var runErr error
	var stderr string
	withTestStdin(t, "", func() {
		stderr = captureStderr(t, func() {
			runErr = updateCmd.RunE(updateCmd, []string{"test-desc"})
		})
	})

	if runErr == nil {
		t.Fatalf("bd update --description - with empty stdin succeeded, want a refusal\nstderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--description") || !strings.Contains(stderr, "test-desc") {
		t.Errorf("refusal does not name the flag and id\nstderr:\n%s", stderr)
	}
	if len(lifecycle.requests) != 0 {
		t.Fatalf("refused update still performed %d mutation(s), want none", len(lifecycle.requests))
	}
}

// TestUpdateAllowsEmptyDescriptionWithOptIn is the escape hatch: the caller
// that means it says so, and the empty or whitespace-only value reaches the
// write unchanged.
func TestUpdateAllowsEmptyDescriptionWithOptIn(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "whitespace only", value: " \n\t "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lifecycle := newDescriptionUpdateEnv(t)
			setUpdateFlags(t, map[string]string{
				"description":             tc.value,
				"allow-empty-description": "true",
			})

			if err := updateCmd.RunE(updateCmd, []string{"test-desc"}); err != nil {
				t.Fatalf("bd update --description %q --allow-empty-description failed: %v", tc.value, err)
			}

			if len(lifecycle.requests) != 1 {
				t.Fatalf("mutation count = %d, want 1", len(lifecycle.requests))
			}
			description := lifecycle.requests[0].Patch.Description
			if !description.Set || description.Value != tc.value {
				t.Fatalf("patch description = %+v, want %q", description, tc.value)
			}
		})
	}
}

// TestUpdateAcceptsNonEmptyDescription pins the unchanged path: a real
// description still reaches the write, as does a non-empty stdin body.
func TestUpdateAcceptsNonEmptyDescription(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		stdin string
		want  string
	}{
		{
			name:  "inline value",
			flags: map[string]string{"description": "a real description"},
			want:  "a real description",
		},
		{
			name:  "stdin dash",
			flags: map[string]string{"description": "-"},
			stdin: "from stdin\n",
			want:  "from stdin\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lifecycle := newDescriptionUpdateEnv(t)
			setUpdateFlags(t, tc.flags)

			var runErr error
			read := func() {
				runErr = updateCmd.RunE(updateCmd, []string{"test-desc"})
			}
			if tc.stdin != "" {
				withTestStdin(t, tc.stdin, read)
			} else {
				read()
			}
			if runErr != nil {
				t.Fatalf("bd update failed: %v", runErr)
			}

			if len(lifecycle.requests) != 1 {
				t.Fatalf("mutation count = %d, want 1", len(lifecycle.requests))
			}
			description := lifecycle.requests[0].Patch.Description
			if !description.Set || description.Value != tc.want {
				t.Fatalf("patch description = %+v, want %q", description, tc.want)
			}
		})
	}
}

func TestValidateDescriptionUpdateAllowsExplicitInlineEmpty(t *testing.T) {
	cmd := newUpdateDescriptionGuardCmd()
	if err := cmd.ParseFlags([]string{"--description", ""}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	description, changed, flagErr := getDescriptionFlag(cmd)
	if flagErr != nil {
		t.Fatalf("unexpected error: %v", flagErr)
	}
	if err := validateDescriptionUpdate(cmd, description, changed); err != nil {
		t.Fatalf("expected explicit inline empty description to succeed, got: %v", err)
	}
	if description != "" {
		t.Fatalf("expected empty inline description, got %q", description)
	}
}
