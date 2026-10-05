package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/metrics"
	"github.com/steveyegge/beads/internal/storage"
)

var (
	gcDryRun   bool
	gcForce    bool
	gcSkipDolt bool
)

var gcCmd = &cobra.Command{
	Use:     "gc",
	GroupID: "maint",
	Short:   "Garbage collect: run Dolt GC to reclaim disk space",
	Long: `Run Dolt garbage collection to reclaim disk space.

GC reclaims unreferenced storage only: it deletes no issues and rewrites no
Dolt history. Deleting closed issues is a separate decision with its own
commands (bd prune, bd purge); rewriting history is an offline procedure, not
part of this command.

Examples:
  bd gc              # Run Dolt GC
  bd gc --dry-run    # Preview what would happen
  bd gc --skip-dolt  # Report what would run, reclaim nothing`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		evt := metrics.NewCommandEvent("gc")
		defer func() {
			if c := metrics.Global(); c != nil {
				c.CloseEventAndAdd(evt)
			}
		}()

		if !gcDryRun {
			CheckReadonly("gc")
		}
		ctx := rootCtx
		start := time.Now()

		var detail string
		var gcSizeInfo map[string]interface{}
		if gcSkipDolt {
			// Skipped: no work, no header — the summary line reports it.
		} else {
			if !jsonOutput {
				fmt.Println("Dolt GC (reclaim disk space)")
			}
			detail, gcSizeInfo = runDoltGCPhase(ctx)
		}

		elapsed := time.Since(start)

		if jsonOutput {
			summaryMap := make(map[string]interface{})
			summaryMap["dry_run"] = gcDryRun
			summaryMap["elapsed_ms"] = elapsed.Milliseconds()
			phase := map[string]interface{}{
				"name":    "Dolt GC",
				"skipped": gcSkipDolt,
			}
			if detail != "" {
				phase["detail"] = detail
			}
			summaryMap["phases"] = []map[string]interface{}{phase}
			if gcSizeInfo != nil {
				summaryMap["dolt_gc"] = gcSizeInfo
			}
			return outputJSON(summaryMap)
		}

		mode := "✓ GC complete"
		if gcDryRun {
			mode = "DRY RUN complete"
		}
		fmt.Printf("%s (%v)\n", mode, elapsed.Round(time.Millisecond))
		if gcSkipDolt {
			fmt.Printf("  Dolt GC: skipped\n")
		} else {
			fmt.Printf("  Dolt GC: %s\n", detail)
		}
		return nil
	},
}

// runDoltGCPhase runs bd gc's one phase, Dolt garbage collection, and returns
// the summary detail plus the size/ref measurements for JSON output (nil until
// a run completes). Progress lines go to stdout unless jsonOutput is set, which
// is the same rule the rest of the command follows.
func runDoltGCPhase(ctx context.Context) (string, map[string]interface{}) {
	gc, ok := storage.UnwrapStore(store).(storage.GarbageCollector)
	if !ok {
		if !jsonOutput {
			fmt.Println("  Storage backend does not support GC, skipping")
		}
		return "not supported", nil
	}
	if gcDryRun {
		if !jsonOutput {
			fmt.Println("  Would run DOLT_GC()")
		}
		return "dry-run", nil
	}

	// bd gc runs without a preceding squash, so remote-tracking refs are left
	// alone here (they cache the remote tip for the migrate gate); flatten and
	// compact prune them before their GC (bd-agctw). Sizes are reported so a
	// no-op reclaim is visible.
	sizeBefore := storeSizeBytes(ctx)
	remoteRefs, tags := listRemoteRefsAndTags(ctx)
	if err := gc.DoltGC(ctx); err != nil {
		WarnError("dolt gc failed: %v", err)
		return "failed", nil
	}

	sizeAfter := storeSizeBytes(ctx)
	detail := "complete"
	if line := gcSizeLine(sizeBefore, sizeAfter); line != "" {
		detail = "complete: " + line
	}
	if !jsonOutput {
		fmt.Printf("  Done (%s)\n", detail)
		if len(remoteRefs)+len(tags) > 0 {
			fmt.Printf("  Note: %d remote-tracking ref(s) and %d tag(s) anchor history;\n", len(remoteRefs), len(tags))
			fmt.Printf("  after a history squash, use bd flatten / bd compact so they are pruned first.\n")
		}
	}
	sizeInfo := map[string]interface{}{
		"remote_refs": len(remoteRefs),
		"tags":        len(tags),
	}
	addGCSizeJSON(sizeInfo, sizeBefore, sizeAfter)
	return detail, sizeInfo
}

func init() {
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false, "Preview without making changes")
	gcCmd.Flags().BoolVarP(&gcForce, "force", "f", false, "Accepted for compatibility; gc never prompts")
	gcCmd.Flags().BoolVar(&gcSkipDolt, "skip-dolt", false, "Skip Dolt garbage collection phase")

	rootCmd.AddCommand(gcCmd)
}
