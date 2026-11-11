//go:build cgo

package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/steveyegge/beads/internal/storage"
)

// The store-backed round trips for --if-acceptance live in
// update_if_acceptance_integration_test.go. The tests here stay in the unit
// tier: they exercise the flag, the refusal wording and the exit-13 mapping
// without opening a Dolt store.

// TestUpdateIfAcceptanceFlagIsRegistered pins that the guard is a real flag on
// bd update, with usage naming the hash form that keeps argv short.
func TestUpdateIfAcceptanceFlagIsRegistered(t *testing.T) {
	flag := updateCmd.Flags().Lookup("if-acceptance")
	if flag == nil {
		t.Fatal("bd update has no --if-acceptance flag")
	}
	if !strings.Contains(flag.Usage, "sha256") {
		t.Errorf("--if-acceptance usage = %q, want it to name the sha256 form", flag.Usage)
	}
}

// TestUpdateIfAcceptanceMismatchIsGuardNotHeld pins that a stale
// --if-acceptance refusal takes the same guard_not_held path as --if-assignee
// and --if-status: classed as a guard mismatch, exit 13, machine kind
// guard_not_held. It drives the classification the command uses.
func TestUpdateIfAcceptanceMismatchIsGuardNotHeld(t *testing.T) {
	cause := fmt.Errorf("%w: test-acc1 acceptance criteria do not match the expected value", storage.ErrAcceptanceMismatch)
	if !isGuardMismatch(cause) {
		t.Fatal("an acceptance mismatch is not classed as a guard mismatch")
	}

	env, _, _, code := runMachine(t, func() error {
		return reportUpdateFailures([]updateIDFailure{{
			ID:            "test-acc1",
			Error:         "updating issue: " + cause.Error(),
			GuardMismatch: isGuardMismatch(cause),
			Kind:          errorKindOf(cause),
		}}, 0, 1)
	})
	if code != ExitGuardMismatch {
		t.Fatalf("exit = %d, want %d", code, ExitGuardMismatch)
	}
	if env.Error == nil || env.Error.Kind != "guard_not_held" {
		t.Fatalf("error = %+v, want kind guard_not_held", env.Error)
	}
}

// TestUpdateGuardCombinationErrorsNameIfAcceptance pins that the two refusals a
// guard can hit before any write name --if-acceptance next to the other guards,
// so a caller passing only the acceptance guard is told which flags it
// combined. Both refusals are checked on stderr, where the command prints them.
func TestUpdateGuardCombinationErrorsNameIfAcceptance(t *testing.T) {
	newGuardCmd := func(t *testing.T) *cobra.Command {
		t.Helper()
		c := &cobra.Command{}
		c.Flags().String("if-assignee", "", "")
		c.Flags().String("if-status", "", "")
		c.Flags().String("if-acceptance", "", "")
		if err := c.Flags().Set("if-acceptance", "expected"); err != nil {
			t.Fatal(err)
		}
		return c
	}

	tests := []struct {
		name    string
		claim   bool
		updates map[string]interface{}
		want    string
	}{
		{
			name:    "with --claim",
			claim:   true,
			updates: map[string]interface{}{"priority": 1},
			want:    "cannot combine --if-assignee/--if-status/--if-acceptance with --claim",
		},
		{
			name:    "with label edits only",
			updates: map[string]interface{}{"add_labels": []string{"x"}},
			want:    "--if-assignee/--if-status/--if-acceptance require at least one field update",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			stderr := captureStderr(t, func() {
				_, _, _, err = updateGuardsFromFlags(newGuardCmd(t), tt.claim, tt.updates)
			})
			if err == nil {
				t.Fatal("updateGuardsFromFlags accepted the combination, want a refusal")
			}
			if !strings.Contains(stderr, tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tt.want)
			}
		})
	}
}
