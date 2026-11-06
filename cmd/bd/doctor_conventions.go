package main

import (
	"context"
	"fmt"

	"github.com/steveyegge/beads/cmd/bd/doctor"
	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/ui"
	"github.com/steveyegge/beads/internal/validation"
)

// runConventionsCheck runs a composite conventions check: lint, stale, and orphans.
// All findings are advisory (warning, never error) - conventions are a choice.
func runConventionsCheck(path string) error {
	// doctor opts out of PersistentPreRun DB init via skipStoreAnnotation, so
	// PersistentPreRun doesn't open the store.
	// The lint/stale/orphans primitives all require the global store, so
	// initialize it lazily here; ensureDirectMode routes to embedded or
	// server based on metadata.json (GH#3597).
	if err := ensureDirectMode("conventions check requires direct mode"); err != nil {
		return HandleError("%v", err)
	}

	var checks []doctorCheck

	lintChecks, err := runConventionsLint()
	if err != nil {
		return err
	}
	checks = append(checks, lintChecks...)
	checks = append(checks, runConventionsStale()...)
	checks = append(checks, runConventionsOrphans(path)...)

	if jsonOutput {
		overallOK := true
		for _, c := range checks {
			if c.Status != statusOK {
				overallOK = false
				break
			}
		}
		return outputJSON(struct {
			Path      string        `json:"path"`
			Checks    []doctorCheck `json:"checks"`
			OverallOK bool          `json:"overall_ok"`
		}{
			Path:      path,
			Checks:    checks,
			OverallOK: overallOK,
		})
	}

	// Human-readable output
	fmt.Println()
	fmt.Println(ui.RenderCategory("Conventions"))

	var passCount, warnCount int
	for _, c := range checks {
		var statusIcon string
		switch c.Status {
		case statusOK:
			statusIcon = ui.RenderPassIcon()
			passCount++
		case statusWarning:
			statusIcon = ui.RenderWarnIcon()
			warnCount++
		}

		fmt.Printf("  %s  %s", statusIcon, c.Name)
		if c.Message != "" {
			fmt.Printf("%s", ui.RenderMuted(" "+c.Message))
		}
		fmt.Println()
		if c.Detail != "" {
			fmt.Printf("     %s%s\n", ui.MutedStyle.Render(ui.TreeLast), ui.RenderMuted(c.Detail))
		}
		if c.Fix != "" {
			fmt.Printf("     %s\n", ui.RenderMuted("Fix: "+c.Fix))
		}
	}

	fmt.Println()
	fmt.Println(ui.RenderSeparator())
	fmt.Printf("%s %d passed  %s %d warnings\n",
		ui.RenderPassIcon(), passCount,
		ui.RenderWarnIcon(), warnCount,
	)

	if warnCount == 0 {
		fmt.Println()
		fmt.Printf("%s\n", ui.RenderPass("✓ All convention checks passed"))
	}
	return nil
}

// runConventionsLint checks open issues for missing template sections.
// runConventionsLint's doctorCheck results are advisory (per the composite
// comment above), but a MaxRows cap violation is a different kind of
// failure — an infrastructure circuit breaker, not a data-quality finding —
// so it returns a non-nil error instead of a doctorCheck in that one case.
// The caller must propagate it rather than folding it into the checks list.
func runConventionsLint() ([]doctorCheck, error) {
	if store == nil {
		return []doctorCheck{{
			Name:     "conventions.lint",
			Status:   statusWarning,
			Message:  "database not available",
			Category: "Conventions",
		}}, nil
	}

	ctx := rootCtx
	openStatus := types.StatusOpen
	// Env-only cap (designer §4): operator opt-in via BEADS_MAX_ROWS.
	maxRows, maxRowsSource := resolveMaxRowsEnvOnly()
	issues, err := store.SearchIssues(ctx, "", types.IssueFilter{
		Status:        &openStatus,
		MaxRows:       maxRows,
		MaxRowsSource: maxRowsSource,
	})
	if err != nil {
		if capErr := handleMaxRowsError(err); capErr != nil {
			return nil, capErr
		}
		return []doctorCheck{{
			Name:     "conventions.lint",
			Status:   statusWarning,
			Message:  fmt.Sprintf("error reading issues: %v", err),
			Category: "Conventions",
		}}, nil
	}

	warningCount := 0
	for _, issue := range issues {
		if err := validation.LintIssue(issue); err != nil {
			warningCount++
		}
	}

	if warningCount == 0 {
		return []doctorCheck{{
			Name:     "conventions.lint",
			Status:   statusOK,
			Message:  fmt.Sprintf("all %d open issues pass template checks", len(issues)),
			Category: "Conventions",
		}}, nil
	}

	return []doctorCheck{{
		Name:     "conventions.lint",
		Status:   statusWarning,
		Message:  fmt.Sprintf("%d of %d open issues missing recommended sections", warningCount, len(issues)),
		Fix:      "bd lint",
		Category: "Conventions",
	}}, nil
}

// runConventionsStale checks for issues with no recent activity.
func runConventionsStale() []doctorCheck {
	if store == nil {
		return []doctorCheck{{
			Name:     "conventions.stale",
			Status:   statusWarning,
			Message:  "database not available",
			Category: "Conventions",
		}}
	}

	ctx := rootCtx
	filter := types.StaleFilter{Days: 14, Limit: 100}
	staleIssues, err := store.GetStaleIssues(ctx, filter)
	if err != nil {
		return []doctorCheck{{
			Name:     "conventions.stale",
			Status:   statusWarning,
			Message:  fmt.Sprintf("error checking stale issues: %v", err),
			Category: "Conventions",
		}}
	}

	if len(staleIssues) == 0 {
		return []doctorCheck{{
			Name:     "conventions.stale",
			Status:   statusOK,
			Message:  "no issues inactive for 14+ days",
			Category: "Conventions",
		}}
	}

	return []doctorCheck{{
		Name:     "conventions.stale",
		Status:   statusWarning,
		Message:  fmt.Sprintf("%d issues inactive for 14+ days", len(staleIssues)),
		Fix:      "bd stale",
		Category: "Conventions",
	}}
}

// runConventionsOrphans checks for issues referenced in commits but still open.
func runConventionsOrphans(path string) []doctorCheck {
	orphans, err := findOrphanedIssues(path, nil, nil)
	if err != nil {
		// Not an error - orphan detection may fail in non-git repos
		return []doctorCheck{{
			Name:     "conventions.orphans",
			Status:   statusOK,
			Message:  "orphan check skipped (no git history)",
			Category: "Conventions",
		}}
	}

	if len(orphans) == 0 {
		return []doctorCheck{{
			Name:     "conventions.orphans",
			Status:   statusOK,
			Message:  "no orphaned issues found",
			Category: "Conventions",
		}}
	}

	return []doctorCheck{{
		Name:     "conventions.orphans",
		Status:   statusWarning,
		Message:  fmt.Sprintf("%d issues referenced in commits but still open", len(orphans)),
		Fix:      "bd orphans",
		Category: "Conventions",
	}}
}

// The orphan conversion chain moved here with the be-xu2.6.6 verb deletions:
// runConventionsOrphans is the only remaining caller of the shared
// doctor.FindOrphanedIssues result, so it converts that result into the
// doctor check output itself.

// orphanIssueOutput is the result shape of findOrphanedIssues.
type orphanIssueOutput struct {
	IssueID             string `json:"issue_id"`
	Title               string `json:"title"`
	Status              string `json:"status"`
	LatestCommit        string `json:"latest_commit,omitempty"`
	LatestCommitMessage string `json:"latest_commit_message,omitempty"`
}

// doctorFindOrphanedIssues is the function used to find orphaned issues.
// It accepts a git path and an IssueProvider for flexibility (cross-repo, mock testing).
var doctorFindOrphanedIssues = doctor.FindOrphanedIssues

// doltStoreProvider wraps the global Dolt store to implement types.IssueProvider.
type doltStoreProvider struct {
	labels    []string // AND semantics: issue must have ALL these labels
	labelsAny []string // OR semantics: issue must have AT LEAST ONE of these labels
}

func (p *doltStoreProvider) GetOpenIssues(ctx context.Context) ([]*types.Issue, error) {
	openStatus := types.StatusOpen
	openIssues, err := store.SearchIssues(ctx, "", types.IssueFilter{
		Status:    &openStatus,
		Labels:    p.labels,
		LabelsAny: p.labelsAny,
	})
	if err != nil {
		return nil, err
	}
	inProgressStatus := types.StatusInProgress
	inProgressIssues, err := store.SearchIssues(ctx, "", types.IssueFilter{
		Status:    &inProgressStatus,
		Labels:    p.labels,
		LabelsAny: p.labelsAny,
	})
	if err != nil {
		return nil, err
	}
	return append(openIssues, inProgressIssues...), nil
}

func (p *doltStoreProvider) GetIssuePrefix() string {
	// YAML config takes precedence — in shared-server mode the DB
	// may belong to a different project (GH#2469).
	if yamlPrefix := config.GetString("issue-prefix"); yamlPrefix != "" {
		return yamlPrefix
	}
	ctx := context.Background()
	prefix, err := store.GetConfig(ctx, "issue_prefix")
	if err != nil || prefix == "" {
		return "bd"
	}
	return prefix
}

// getIssueProviderFn is the function used to create an IssueProvider.
// It is a variable so tests can substitute a mock without needing a real store.
var getIssueProviderFn = func(labels, labelsAny []string) (types.IssueProvider, func(), error) {
	if store != nil {
		return &doltStoreProvider{labels: labels, labelsAny: labelsAny}, func() {}, nil
	}
	return nil, nil, fmt.Errorf("no database available")
}

// getIssueProvider returns an IssueProvider backed by the global Dolt store.
// labels and labelsAny are passed through to SearchIssues for label filtering.
func getIssueProvider(labels, labelsAny []string) (types.IssueProvider, func(), error) {
	return getIssueProviderFn(labels, labelsAny)
}

// findOrphanedIssues wraps the shared doctor package function and converts to output format.
// labels and labelsAny are passed to the issue provider to restrict which issues are considered.
func findOrphanedIssues(path string, labels, labelsAny []string) ([]orphanIssueOutput, error) {
	provider, cleanup, err := getIssueProvider(labels, labelsAny)
	if err != nil {
		return nil, fmt.Errorf("unable to find orphaned issues: %w", err)
	}
	defer cleanup()

	return findOrphanedIssuesWithProvider(path, provider)
}

func findOrphanedIssuesWithProvider(path string, provider types.IssueProvider) ([]orphanIssueOutput, error) {
	orphans, err := doctorFindOrphanedIssues(path, provider)
	if err != nil {
		return nil, fmt.Errorf("unable to find orphaned issues: %w", err)
	}

	var output []orphanIssueOutput
	for _, orphan := range orphans {
		output = append(output, orphanIssueOutput{
			IssueID:             orphan.IssueID,
			Title:               orphan.Title,
			Status:              orphan.Status,
			LatestCommit:        orphan.LatestCommit,
			LatestCommitMessage: orphan.LatestCommitMessage,
		})
	}
	return output, nil
}
