package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/metrics"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/ui"
)

var depPruneOrphansCmd = &cobra.Command{
	Use:   "prune-orphans",
	Short: "Remove dependency rows whose issue or target no longer exists",
	Long: `Remove dependency rows whose issue or target no longer exists.

Scans both dependency tables (issues and wisps). A row is an orphan when its
source id, or its issue/wisp target id, names a row in neither plane. External
targets are never orphans. Sources of pruned edges have their blocked state
recomputed in the same transaction.

  bd dep prune-orphans --dry-run   # count only
  bd dep prune-orphans             # delete and report counts`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		evt := metrics.NewCommandEvent("dep-prune-orphans")
		defer func() {
			if c := metrics.Global(); c != nil {
				c.CloseEventAndAdd(evt)
			}
		}()

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		if !dryRun {
			CheckReadonly("dep prune-orphans")
		}
		if usesProxiedServer() {
			return failKind(kindInvalidArgs, "dep prune-orphans is not supported in proxied-server mode")
		}
		if store == nil {
			if err := ensureStoreActive(); err != nil {
				return handleClassifiedRespectJSON(err)
			}
		}
		pruner, ok := storage.UnwrapStore(store).(storage.OrphanDependencyPruner)
		if !ok {
			return failKind(kindRefused, "dep prune-orphans is not supported by this storage backend")
		}
		result, err := pruner.PruneOrphanDependencies(rootCtx, dryRun)
		if err != nil {
			return handleClassifiedRespectJSON(fmt.Errorf("prune orphan dependencies: %w", err))
		}
		if !dryRun && result.Total > 0 {
			commandDidWrite.Store(true)
		}

		if jsonOutput {
			return outputJSON(result)
		}
		verb := "Removed"
		if dryRun {
			verb = "Would remove"
		}
		fmt.Printf("%s %s %d orphan dependency row(s): %d issue, %d wisp\n",
			ui.RenderPass("✓"), verb, result.Total, result.Dependencies, result.WispDependencies)
		return nil
	},
}

func init() {
	depPruneOrphansCmd.Flags().Bool("dry-run", false, "Count orphan rows without deleting them")
	depCmd.AddCommand(depPruneOrphansCmd)
}
