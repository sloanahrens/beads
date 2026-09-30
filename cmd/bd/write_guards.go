package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/issueops"
)

// writeGuards are the --if-status/--if-assignee write-time guards `bd close`
// and `bd delete` share with `bd update` (be-pgd): checked inside the write's
// own transaction, and a mismatch writes nothing for that id and is reported
// as guard_not_held (exit 13 when every failure is one).
type writeGuards struct {
	status   *string
	assignee *string
}

func (g writeGuards) set() bool { return g.status != nil || g.assignee != nil }

// registerWriteGuardFlags adds --if-status and --if-assignee to cmd.
func registerWriteGuardFlags(cmd *cobra.Command, verb string) {
	cmd.Flags().String("if-status", "", "Only "+verb+" while the current status equals this value; a mismatch writes nothing for that id and exits 13 (guard_not_held)")
	cmd.Flags().String("if-assignee", "", "Only "+verb+" while the current assignee equals this value (--if-assignee '' requires unassigned); a mismatch writes nothing for that id and exits 13 (guard_not_held)")
}

// writeGuardsFromFlags reads the guards with presence detected via Changed(),
// so `--if-assignee ""` is a real guard meaning "expected unassigned". An
// --if-status value is validated against the built-in and custom status set,
// so a typo fails fast (invalid_args) instead of mismatching forever.
func writeGuardsFromFlags(cmd *cobra.Command) (writeGuards, error) {
	var g writeGuards
	if cmd.Flags().Changed("if-assignee") {
		v, _ := cmd.Flags().GetString("if-assignee")
		g.assignee = &v
	}
	if cmd.Flags().Changed("if-status") {
		v, _ := cmd.Flags().GetString("if-status")
		// Custom statuses live in the store, so open it before validating: a
		// valid custom --if-status must not be refused as a typo.
		if store == nil {
			if err := ensureStoreActive(); err != nil {
				return writeGuards{}, handleClassifiedRespectJSON(err)
			}
		}
		var customStatuses []string
		if !types.Status(v).IsValidWithCustom(nil) {
			cs, err := store.GetCustomStatuses(rootCtx)
			if err != nil {
				return writeGuards{}, handleClassifiedRespectJSON(fmt.Errorf("reading custom statuses for --if-status: %w", err))
			}
			customStatuses = cs
		}
		if !types.Status(v).IsValidWithCustom(customStatuses) {
			return writeGuards{}, failKind(kindInvalidArgs, "invalid --if-status %q (built-in: open, in_progress, blocked, deferred, closed, pinned, hooked; or configure custom statuses via 'bd config set status.custom')", v)
		}
		g.status = &v
	}
	return g, nil
}

// deleteGuardFailure turns a *issueops.DeleteGuardError into the typed
// guard_not_held failure, one idOutcome per mismatched id. It returns nil for
// any other error. Outside machine mode the message goes to stderr and the exit is
// 13, the code bd update's guards use.
func deleteGuardFailure(err error) error {
	var ge *issueops.DeleteGuardError
	if !errors.As(err, &ge) {
		return nil
	}
	ids := make([]idOutcome, len(ge.IDs))
	for i, id := range ge.IDs {
		ids[i] = idOutcome{ID: id, Kind: kindGuardNotHeld, Message: ge.Errs[i].Error()}
	}
	e := &cliError{Kind: kindGuardNotHeld, Message: ge.Error(), IDs: ids}
	if !machineModeActive() {
		fmt.Fprintf(os.Stderr, "Error: %s\n", e.Message)
	}
	return e
}
