package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/metrics"
	"github.com/steveyegge/beads/internal/timeparsing"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
)

var deferCmd = &cobra.Command{
	Use:   "defer [id...]",
	Short: "Defer one or more issues for later",
	Long: `Defer issues to put them on ice for later.

Deferred issues are deliberately set aside - not blocked by anything specific,
just postponed for future consideration. Unlike blocked issues, there's no
dependency keeping them from being worked. Unlike closed issues, they will
be revisited.

Deferred issues don't show in 'bd ready' but remain visible in 'bd list'.

A defer WITH a date is a snooze: once --until passes, the next ready-front
read returns the issue to open automatically (same shape as 'bd undefer').
A defer WITHOUT a date is the indefinite icebox: it stays deferred until
someone runs 'bd undefer'.

Examples:
  bd defer bd-abc                  # Icebox indefinitely (until bd undefer)
  bd defer bd-abc --until=tomorrow # Snooze: auto-wakes once the date passes
  bd defer bd-abc --reason="waiting on API access"
  bd defer bd-abc bd-def           # Defer multiple issues`,
	Args:          cobra.MinimumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		evt := metrics.NewCommandEvent("defer")
		defer func() {
			if c := metrics.Global(); c != nil {
				c.CloseEventAndAdd(evt)
			}
		}()

		var deferUntil *time.Time
		untilStr, _ := cmd.Flags().GetString("until")
		if untilStr != "" {
			t, err := timeparsing.ParseRelativeTime(untilStr, time.Now())
			if err != nil {
				return HandleError("invalid --until format %q. Examples: +1h, tomorrow, next monday, 2025-01-15", untilStr)
			}
			if t.Before(time.Now()) && !jsonOutput {
				fmt.Fprintf(os.Stderr, "%s Defer date %q is in the past. Issue will appear in bd ready immediately.\n",
					ui.RenderWarn("!"), t.Local().Format("2006-01-02 15:04"))
				fmt.Fprintf(os.Stderr, "  Did you mean a future date? Use --until=+1h or --until=tomorrow\n")
			}
			deferUntil = &t
		}
		reason, _ := cmd.Flags().GetString("reason")
		reason = strings.TrimSpace(reason)
		if cmd.Flags().Changed("reason") && reason == "" {
			return HandleError("reason cannot be empty")
		}

		CheckReadonly("defer")

		ctx := rootCtx

		deferredIssues := []*types.Issue{}

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
		deferred := 0

		for _, id := range args {
			// Routed resolution: a cross-rig id is served by the store that
			// owns it, and a matched-but-unreachable prefix route keeps its
			// typed route_unreachableError instead of collapsing into a
			// definite "not found" (B1-05, be-sut). Resolution stays per-id so
			// one unresolvable argument does not discard the rest.
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
			fullID := result.ResolvedID
			issueStore := result.Store

			updates := map[string]interface{}{
				"status": string(types.StatusDeferred),
			}
			if deferUntil != nil {
				updates["defer_until"] = *deferUntil
			}
			if reason != "" {
				notes := result.Issue.Notes
				if notes != "" {
					notes += "\n"
				}
				updates["notes"] = notes + reason
			}

			if err := issueStore.UpdateIssue(ctx, fullID, updates, actor); err != nil {
				result.Close()
				fail(fullID, errorKindOf(err), fmt.Sprintf("Error deferring %s: %v", fullID, err))
				continue
			}
			deferred++

			if jsonOutput {
				if issue, _ := issueStore.GetIssue(ctx, fullID); issue != nil {
					deferredIssues = append(deferredIssues, issue)
				}
			} else {
				fmt.Printf("%s Deferred %s\n", ui.RenderAccent("*"), fullID)
			}
			result.Close()
		}

		if jsonOutput && len(deferredIssues) > 0 {
			if err := outputJSON(deferredIssues); err != nil {
				return err
			}
		}

		if len(args) > 0 {
			commandDidWrite.Store(true)
		}
		return batchError(deferred, failures)
	},
}

func init() {
	// Time-based scheduling flag (GH#820)
	deferCmd.Flags().String("until", "", "Defer until specific time (e.g., +1h, tomorrow, next monday)")
	deferCmd.Flags().String("reason", "", "Record why this issue is being deferred (appended to notes)")
	deferCmd.ValidArgsFunction = issueIDCompletion
	rootCmd.AddCommand(deferCmd)
}
