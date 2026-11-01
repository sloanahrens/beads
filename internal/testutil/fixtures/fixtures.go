// Package fixtures provides realistic test data generation for benchmarks and tests.
package fixtures

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/beads/internal/storage/dolt"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/issueops"
)

// labels used across all fixtures
var commonLabels = []string{
	"backend",
	"frontend",
	"urgent",
	"tech-debt",
	"documentation",
	"performance",
	"security",
	"ux",
	"api",
	"database",
}

// assignees used across all fixtures
var commonAssignees = []string{
	"alice",
	"bob",
	"charlie",
	"diana",
	"eve",
	"frank",
}

// epic titles for realistic data
var epicTitles = []string{
	"User Authentication System",
	"Payment Processing Integration",
	"Mobile App Redesign",
	"Performance Optimization",
	"API v2 Migration",
	"Search Functionality Enhancement",
	"Analytics Dashboard",
	"Multi-tenant Support",
	"Notification System",
	"Data Export Feature",
}

// feature titles (under epics)
var featureTitles = []string{
	"OAuth2 Integration",
	"Password Reset Flow",
	"Two-Factor Authentication",
	"Session Management",
	"API Endpoints",
	"Database Schema",
	"UI Components",
	"Background Jobs",
	"Error Handling",
	"Testing Infrastructure",
}

// task titles (under features)
var taskTitles = []string{
	"Implement login endpoint",
	"Add validation logic",
	"Write unit tests",
	"Update documentation",
	"Fix memory leak",
	"Optimize query performance",
	"Add error logging",
	"Refactor helper functions",
	"Update database migrations",
	"Configure deployment",
}

// Fixture size rationale:
// We only provide Large (10K) and XLarge (20K) fixtures because:
// - Performance characteristics only emerge at scale (10K+ issues)
// - Smaller fixtures don't provide meaningful optimization insights
// - Code weight matters; we avoid unused complexity
// - Target use case: repositories with thousands of issues

// DataConfig controls the distribution and characteristics of generated test data
type DataConfig struct {
	TotalIssues       int     // total number of issues to generate
	EpicRatio         float64 // percentage of issues that are epics (e.g., 0.1 for 10%)
	FeatureRatio      float64 // percentage of issues that are features (e.g., 0.3 for 30%)
	OpenRatio         float64 // percentage of issues that are open (e.g., 0.5 for 50%)
	CrossLinkRatio    float64 // percentage of tasks with cross-epic blocking dependencies (e.g., 0.2 for 20%)
	MaxEpicAgeDays    int     // maximum age in days for epics (e.g., 180)
	MaxFeatureAgeDays int     // maximum age in days for features (e.g., 150)
	MaxTaskAgeDays    int     // maximum age in days for tasks (e.g., 120)
	MaxClosedAgeDays  int     // maximum days since closure (e.g., 30)
	RandSeed          int64   // random seed for reproducibility
}

// DefaultLargeConfig returns configuration for 10K issue dataset
func DefaultLargeConfig() DataConfig {
	return DataConfig{
		TotalIssues:       10000,
		EpicRatio:         0.1,
		FeatureRatio:      0.3,
		OpenRatio:         0.5,
		CrossLinkRatio:    0.2,
		MaxEpicAgeDays:    180,
		MaxFeatureAgeDays: 150,
		MaxTaskAgeDays:    120,
		MaxClosedAgeDays:  30,
		RandSeed:          42,
	}
}

// DefaultXLargeConfig returns configuration for 20K issue dataset
func DefaultXLargeConfig() DataConfig {
	return DataConfig{
		TotalIssues:       20000,
		EpicRatio:         0.1,
		FeatureRatio:      0.3,
		OpenRatio:         0.5,
		CrossLinkRatio:    0.2,
		MaxEpicAgeDays:    180,
		MaxFeatureAgeDays: 150,
		MaxTaskAgeDays:    120,
		MaxClosedAgeDays:  30,
		RandSeed:          43,
	}
}

// LargeDolt creates a 10K issue database with realistic patterns
func LargeDolt(ctx context.Context, store *dolt.DoltStore) error {
	cfg := DefaultLargeConfig()
	return generateIssuesWithConfig(ctx, store, cfg)
}

// XLargeDolt creates a 20K issue database with realistic patterns
func XLargeDolt(ctx context.Context, store *dolt.DoltStore) error {
	cfg := DefaultXLargeConfig()
	return generateIssuesWithConfig(ctx, store, cfg)
}

// LargeFromJSONL creates a 10K issue database by exporting to JSONL and reimporting
func LargeFromJSONL(ctx context.Context, store *dolt.DoltStore, tempDir string) error {
	cfg := DefaultLargeConfig()
	cfg.RandSeed = 44 // different seed for JSONL path
	return generateFromJSONL(ctx, store, tempDir, cfg)
}

// fixtureChunkSize bounds how many issues one CreateIssues call writes.
const fixtureChunkSize = 500

// generateIssuesWithConfig creates issues with realistic epic hierarchies and cross-links using provided configuration.
// The dataset is planned in memory first (planIssues), then written in chunks of
// fixtureChunkSize, one CreateIssues call per chunk. CreateIssues persists the
// Labels and Dependencies set on each issue. Cross-links into a later chunk are
// added in one batch once every chunk exists.
func generateIssuesWithConfig(ctx context.Context, store *dolt.DoltStore, cfg DataConfig) error {
	issues := planIssues(cfg, time.Now())

	created := 0
	deferred, err := writePlanInChunks(issues, fixtureChunkSize, func(chunk []*types.Issue) error {
		if err := store.CreateIssues(ctx, chunk, "fixture"); err != nil {
			return fmt.Errorf("failed to create issues: %w", err)
		}
		created += len(chunk)
		fmt.Printf("  Progress: %d%% (%d/%d issues created)\n", created*100/len(issues), created, len(issues))
		return nil
	})
	if err != nil {
		return err
	}

	edges, err := dependencyEdges(issues, deferred)
	if err != nil {
		return fmt.Errorf("failed to resolve cross-links: %w", err)
	}
	if len(edges) == 0 {
		return nil
	}
	editor, err := store.DependencyEditor()
	if err != nil {
		return fmt.Errorf("failed to open dependency editor: %w", err)
	}
	if _, err := editor.AddDependencies(ctx, issueops.AddDependenciesRequest{Actor: "fixture", Edges: edges}); err != nil {
		return fmt.Errorf("failed to add cross-links: %w", err)
	}
	return nil
}

// dependencyEdges turns deps, still in plan refs, into edges between the IDs
// the store has assigned to their issues.
func dependencyEdges(issues []*types.Issue, deps []*types.Dependency) ([]issueops.DependencyEdge, error) {
	edges := make([]issueops.DependencyEdge, 0, len(deps))
	for _, dep := range deps {
		from, err := planEndID(issues, dep.IssueID)
		if err != nil {
			return nil, err
		}
		to, err := planEndID(issues, dep.DependsOnID)
		if err != nil {
			return nil, err
		}
		edges = append(edges, issueops.DependencyEdge{IssueID: from, DependsOnID: to, Type: dep.Type})
	}
	return edges, nil
}

// planEndID returns the ID of the plan issue that end names.
func planEndID(issues []*types.Issue, end string) (string, error) {
	index, err := planRefIndex(end)
	if err != nil {
		return "", err
	}
	if index >= len(issues) {
		return "", fmt.Errorf("dependency end %q names issue %d of %d", end, index, len(issues))
	}
	if issues[index].ID == "" {
		return "", fmt.Errorf("issue %d has no ID after its chunk was written", index)
	}
	return issues[index].ID, nil
}

// generateFromJSONL creates issues, exports to JSONL, clears DB, and reimports
func generateFromJSONL(ctx context.Context, store *dolt.DoltStore, tempDir string, cfg DataConfig) error {
	// First generate issues normally
	if err := generateIssuesWithConfig(ctx, store, cfg); err != nil {
		return fmt.Errorf("failed to generate issues: %w", err)
	}

	// Export to JSONL
	jsonlPath := filepath.Join(tempDir, "issues.jsonl")
	if err := exportToJSONL(ctx, store, jsonlPath); err != nil {
		return fmt.Errorf("failed to export to JSONL: %w", err)
	}

	// Clear all issues (we'll reimport them)
	allIssues, err := store.SearchIssues(ctx, "", types.IssueFilter{})
	if err != nil {
		return fmt.Errorf("failed to get all issues: %w", err)
	}

	for _, issue := range allIssues {
		if err := store.DeleteIssue(ctx, issue.ID); err != nil {
			return fmt.Errorf("failed to delete issue %s: %w", issue.ID, err)
		}
	}

	// Import from JSONL
	if err := importFromJSONL(ctx, store, jsonlPath); err != nil {
		return fmt.Errorf("failed to import from JSONL: %w", err)
	}

	return nil
}

// exportToJSONL exports all issues to a JSONL file
func exportToJSONL(ctx context.Context, store *dolt.DoltStore, path string) error {
	// Get all issues
	allIssues, err := store.SearchIssues(ctx, "", types.IssueFilter{})
	if err != nil {
		return fmt.Errorf("failed to query issues: %w", err)
	}

	// Populate dependencies and labels for each issue
	allDeps, err := store.GetAllDependencyRecords(ctx)
	if err != nil {
		return fmt.Errorf("failed to get dependencies: %w", err)
	}

	for _, issue := range allIssues {
		issue.Dependencies = allDeps[issue.ID]

		labels, err := store.GetLabels(ctx, issue.ID)
		if err != nil {
			return fmt.Errorf("failed to get labels for %s: %w", issue.ID, err)
		}
		issue.Labels = labels
	}

	// Write to JSONL file
	// #nosec G304 -- fixture exports to deterministic file controlled by tests
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create JSONL file: %w", err)
	}
	defer f.Close()

	encoder := json.NewEncoder(f)
	for _, issue := range allIssues {
		if err := encoder.Encode(issue); err != nil {
			return fmt.Errorf("failed to encode issue: %w", err)
		}
	}

	return nil
}

// importFromJSONL imports issues from a JSONL file
func importFromJSONL(ctx context.Context, store *dolt.DoltStore, path string) error {
	// Read JSONL file
	// #nosec G304 -- fixture imports from deterministic file created earlier in test
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read JSONL file: %w", err)
	}

	// Parse issues
	var issues []*types.Issue
	lines := string(data)
	for i, line := range splitLines(lines) {
		if len(line) == 0 {
			continue
		}

		var issue types.Issue
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			return fmt.Errorf("failed to parse issue at line %d: %w", i+1, err)
		}

		issues = append(issues, &issue)
	}

	// Import issues directly using storage interface
	// Step 1: Create all issues first (without dependencies/labels)
	type savedMetadata struct {
		deps   []*types.Dependency
		labels []string
	}
	metadata := make(map[string]savedMetadata)

	for _, issue := range issues {
		// Save dependencies and labels for later
		metadata[issue.ID] = savedMetadata{
			deps:   issue.Dependencies,
			labels: issue.Labels,
		}
		issue.Dependencies = nil
		issue.Labels = nil

		if err := store.CreateIssue(ctx, issue, "fixture"); err != nil {
			return fmt.Errorf("failed to create issue %s: %w", issue.ID, err)
		}
	}

	// Step 2: Add all dependencies (now that all issues exist)
	for issueID, meta := range metadata {
		for _, dep := range meta.deps {
			if err := store.AddDependency(ctx, dep, "fixture"); err != nil {
				// Ignore duplicate and cycle errors
				if !strings.Contains(err.Error(), "already exists") &&
					!strings.Contains(err.Error(), "cycle") {
					return fmt.Errorf("failed to add dependency for %s: %w", issueID, err)
				}
			}
		}

		// Add labels
		for _, label := range meta.labels {
			_ = store.AddLabel(ctx, issueID, label, "fixture")
		}
	}

	return nil
}

// splitLines splits a string by newlines
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// randomStatus returns a random status with given open ratio
func randomStatus(rng *rand.Rand, openRatio float64) types.Status {
	r := rng.Float64()
	if r < openRatio {
		// Open statuses: open, in_progress, blocked
		statuses := []types.Status{types.StatusOpen, types.StatusInProgress, types.StatusBlocked}
		return statuses[rng.Intn(len(statuses))]
	}
	return types.StatusClosed
}

// randomPriority returns a random priority with realistic distribution
// P0: 5%, P1: 15%, P2: 50%, P3: 25%, P4: 5%
func randomPriority(rng *rand.Rand) int {
	r := rng.Intn(100)
	switch {
	case r < 5:
		return 0
	case r < 20:
		return 1
	case r < 70:
		return 2
	case r < 95:
		return 3
	default:
		return 4
	}
}

// randomTime returns a random time up to maxDaysAgo days before now
func randomTime(rng *rand.Rand, now time.Time, maxDaysAgo int) time.Time {
	daysAgo := rng.Intn(maxDaysAgo)
	return now.Add(-time.Duration(daysAgo) * 24 * time.Hour)
}
