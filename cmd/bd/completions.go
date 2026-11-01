package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/types"
)

// issueIDCompletion provides shell completion for issue IDs by querying the storage
// and returning a list of IDs with their titles as descriptions
func issueIDCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Initialize storage if not already initialized
	ctx := context.Background()
	if rootCtx != nil {
		ctx = rootCtx
	}

	// An open store answers without a database path: a caller that has the
	// store but no .beads in its working directory still gets its completions.
	// A path is resolved only to open a read-only store when none is open.
	currentStore := store
	if currentStore == nil {
		// Get database path - use same logic as in PersistentPreRun
		currentDBPath := dbPath
		if currentDBPath == "" {
			currentDBPath = beads.FindDatabasePath()
			if currentDBPath == "" {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		}

		var err error
		currentStore, err = openReadOnlyStoreForDBPath(ctx, currentDBPath)
		if err != nil {
			// If we can't open database, return empty completion
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		defer func() { _ = currentStore.Close() }()
	}

	// Use SearchIssues with IDPrefix filter to efficiently query matching issues
	filter := types.IssueFilter{
		IDPrefix: toComplete, // Filter at database level for better performance
	}
	issues, err := currentStore.SearchIssues(ctx, "", filter)
	if err != nil {
		// If we can't list issues, return empty completion
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Build completion list
	completions := make([]string, 0, len(issues))
	for _, issue := range issues {
		// Format: ID\tTitle (shown during completion)
		completions = append(completions, fmt.Sprintf("%s\t%s", issue.ID, issue.Title))
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
