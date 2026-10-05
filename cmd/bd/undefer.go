package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/metrics"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
)

var undeferCmd = &cobra.Command{
	Use:   "undefer [id...]",
	Short: "Undefer one or more issues (restore to open)",
	Long: `Undefer issues to restore them to open status.

This brings issues back from the icebox so they can be worked on again.
Issues will appear in 'bd ready' if they have no blockers.

Examples:
  bd undefer bd-abc        # Undefer a single issue
  bd undefer bd-abc bd-def # Undefer multiple issues`,
	Args:          cobra.MinimumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		evt := metrics.NewCommandEvent("undefer")
		defer func() {
			if c := metrics.Global(); c != nil {
				c.CloseEventAndAdd(evt)
			}
		}()

		CheckReadonly("undefer")

		ctx := rootCtx

		undeferredIssues := []*types.Issue{}

		if store == nil {
			return HandleErrorWithHint("database not initialized", diagHint())
		}

		// Per-id failures make the command exit non-zero; it used to return 0
		// even when every id failed (B1-02).
		var failures []idOutcome
		fail := func(id string, kind errorKind, msg string) {
			fmt.Fprintln(os.Stderr, msg)
			failures = append(failures, idOutcome{ID: id, Kind: kind, Message: msg})
		}
		undeferred := 0

		for _, id := range args {
			// Routed resolution, for the same reasons as bd defer (B1-05,
			// be-sut): the store that owns a cross-rig id serves the update,
			// and an unreachable route stays route_unreachable.
			result, err := resolveAndGetIssueForMutation(ctx, store, id)
			if err != nil {
				if result != nil {
					result.Close()
				}
				fail(id, errorKindOf(err), fmt.Sprintf("Error resolving %s: %v", id, err))
				continue
			}
			if result == nil || result.Issue == nil {
				if result != nil {
					result.Close()
				}
				fail(id, kindNotFound, fmt.Sprintf("Issue %s not found", id))
				continue
			}
			issue := result.Issue
			fullID := result.ResolvedID
			issueStore := result.Store

			if issue.Status != types.StatusDeferred {
				result.Close()
				fail(fullID, kindRefused, fmt.Sprintf("%s is not deferred (status: %s)", fullID, string(issue.Status)))
				continue
			}

			updates := map[string]interface{}{
				"status":      string(types.StatusOpen),
				"defer_until": nil,
			}

			if err := issueStore.UpdateIssue(ctx, fullID, updates, actor); err != nil {
				result.Close()
				fail(fullID, errorKindOf(err), fmt.Sprintf("Error undeferring %s: %v", fullID, err))
				continue
			}
			undeferred++

			if jsonOutput {
				if issue, _ := issueStore.GetIssue(ctx, fullID); issue != nil {
					undeferredIssues = append(undeferredIssues, issue)
				}
			} else {
				fmt.Printf("%s Undeferred %s (now open)\n", ui.RenderPass("*"), fullID)
			}
			result.Close()
		}

		if len(args) > 0 {
			commandDidWrite.Store(true)
		}

		if jsonOutput && len(undeferredIssues) > 0 {
			if err := outputJSON(undeferredIssues); err != nil {
				return err
			}
		}

		return batchError(undeferred, failures)
	},
}

func init() {
	undeferCmd.ValidArgsFunction = issueIDCompletion
	rootCmd.AddCommand(undeferCmd)
}
