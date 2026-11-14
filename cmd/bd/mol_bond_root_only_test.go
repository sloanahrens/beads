package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// makeMolBondTestCmd builds a cobra.Command with the same flag schema as the
// real `bd mol bond` command, with the given flag values set as defaults (so
// the bond path reads them back without needing arg parsing). Parsing the real
// command would run cobra's Execute and os.Exit paths; this mirrors the
// makeWispTestCmd seam in wisp_test.go.
func makeMolBondTestCmd(rootOnly, dryRun bool) *cobra.Command {
	c := &cobra.Command{Use: "bond"}
	c.Flags().String("type", types.BondTypeSequential, "")
	c.Flags().String("as", "", "")
	c.Flags().Bool("dry-run", dryRun, "")
	c.Flags().StringArray("var", []string{}, "")
	c.Flags().Bool("ephemeral", false, "")
	c.Flags().Bool("pour", false, "")
	c.Flags().Bool("root-only", rootOnly, "")
	c.Flags().String("ref", "", "")
	return c
}

// bondRootOnlyStore is a read-only fake for the lookups a bond makes: resolving
// both operands, then loading the proto subgraph. It embeds
// storage.DoltStorage so any method this test does not expect panics instead of
// answering zero.
type bondRootOnlyStore struct {
	storage.DoltStorage

	issues     map[string]*types.Issue
	dependents map[string][]*types.IssueWithDependencyMetadata
	deps       map[string][]*types.Dependency

	recorder *recordingMolWriter
}

func (s *bondRootOnlyStore) SearchIssues(_ context.Context, _ string, filter types.IssueFilter) ([]*types.Issue, error) {
	if len(filter.IDs) == 0 {
		return nil, nil // hierarchical-child probe: this fixture has none
	}
	var found []*types.Issue
	for _, id := range filter.IDs {
		if issue, ok := s.issues[id]; ok {
			found = append(found, issue)
		}
	}
	return found, nil
}

func (s *bondRootOnlyStore) SearchIssueIDs(context.Context, string, types.IssueFilter) ([]string, error) {
	return nil, nil
}

func (s *bondRootOnlyStore) GetConfig(context.Context, string) (string, error) {
	return "", nil
}

func (s *bondRootOnlyStore) GetIssue(_ context.Context, id string) (*types.Issue, error) {
	issue, ok := s.issues[id]
	if !ok {
		return nil, fmt.Errorf("issue %s not found", id)
	}
	return issue, nil
}

func (s *bondRootOnlyStore) GetDependentsWithMetadata(_ context.Context, id string) ([]*types.IssueWithDependencyMetadata, error) {
	return s.dependents[id], nil
}

func (s *bondRootOnlyStore) GetDependencyRecords(_ context.Context, id string) ([]*types.Dependency, error) {
	return s.deps[id], nil
}

func (s *bondRootOnlyStore) RunInTransaction(_ context.Context, _ string, fn func(tx storage.Transaction) error) error {
	return fn(&bondRootOnlyTx{recorder: s.recorder})
}

// bondRootOnlyTx stands in for the transaction cloneSubgraphInto writes
// through. Only the two calls the clone makes are implemented; the rest of
// storage.Transaction is the embedded nil interface, so an unexpected write
// panics rather than silently vanishing.
type bondRootOnlyTx struct {
	storage.Transaction
	recorder *recordingMolWriter
}

func (t *bondRootOnlyTx) CreateIssues(ctx context.Context, issues []*types.Issue, actor string) error {
	return t.recorder.CreateIssues(ctx, issues, actor)
}

func (t *bondRootOnlyTx) GetConfig(context.Context, string) (string, error) {
	return "", nil
}

// testMolBondEnv is the ambient state the bond path reads, with fakes injected
// instead of package-level globals.
func testMolBondEnv(s storage.DoltStorage) molBondEnv {
	return molBondEnv{ctx: context.Background(), store: s, actor: "tester"}
}

// newBondRootOnlyFixture builds a proto root with the given number of step
// issues under it, plus the beads the formula can be bonded to: one molecule,
// a second molecule, and a second proto with no steps.
func newBondRootOnlyFixture(steps int) *bondRootOnlyStore {
	root := &types.Issue{
		ID:         "proto-1",
		Title:      "Proto Root",
		Status:     types.StatusOpen,
		Priority:   2,
		IssueType:  types.TypeEpic,
		Labels:     []string{MoleculeLabel},
		IsTemplate: true,
	}
	s := &bondRootOnlyStore{
		issues:     map[string]*types.Issue{"proto-1": root},
		dependents: map[string][]*types.IssueWithDependencyMetadata{},
		deps:       map[string][]*types.Dependency{},
		recorder:   &recordingMolWriter{},
	}
	s.issues["mol-1"] = &types.Issue{
		ID:        "mol-1",
		Title:     "Bond target",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	s.issues["mol-2"] = &types.Issue{
		ID:        "mol-2",
		Title:     "Second bond target",
		Status:    types.StatusOpen,
		Priority:  2,
		IssueType: types.TypeTask,
	}
	s.issues["proto-2"] = &types.Issue{
		ID:         "proto-2",
		Title:      "Proto Without Steps",
		Status:     types.StatusOpen,
		Priority:   2,
		IssueType:  types.TypeEpic,
		Labels:     []string{MoleculeLabel},
		IsTemplate: true,
	}
	for i := 1; i <= steps; i++ {
		childID := fmt.Sprintf("proto-1.%d", i)
		child := &types.Issue{
			ID:        childID,
			Title:     fmt.Sprintf("Step %d", i),
			Status:    types.StatusOpen,
			Priority:  2,
			IssueType: types.TypeTask,
		}
		s.issues[childID] = child
		s.dependents[root.ID] = append(s.dependents[root.ID], &types.IssueWithDependencyMetadata{
			Issue:          *child,
			DependencyType: types.DepParentChild,
		})
		s.deps[childID] = []*types.Dependency{{
			IssueID:     childID,
			DependsOnID: root.ID,
			Type:        types.DepParentChild,
		}}
	}
	return s
}

func TestMolBondRegistersRootOnlyFlag(t *testing.T) {
	flag := molBondCmd.Flags().Lookup("root-only")
	if flag == nil {
		t.Fatal("bd mol bond does not register --root-only")
	}
	if flag.DefValue != "false" {
		t.Errorf("--root-only default = %q, want %q", flag.DefValue, "false")
	}
}

func TestMolBondRootOnlySpawnsOnlyRoot(t *testing.T) {
	const steps = 3
	for _, tc := range []struct {
		name        string
		rootOnly    bool
		wantTitles  []string
		wantSkipped string
	}{
		{
			name:        "root-only skips every step issue",
			rootOnly:    true,
			wantTitles:  []string{"Proto Root"},
			wantSkipped: "Steps skipped: 3 (--root-only)",
		},
		{
			name:        "without the flag every step is spawned",
			rootOnly:    false,
			wantTitles:  []string{"Proto Root", "Step 1", "Step 2", "Step 3"},
			wantSkipped: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newBondRootOnlyFixture(steps)

			out := captureStdout(t, func() error {
				return runMolBondWith(testMolBondEnv(s), makeMolBondTestCmd(tc.rootOnly, false), []string{"proto-1", "mol-1"})
			})

			written := s.recorder.created
			if len(written) != len(tc.wantTitles) {
				t.Fatalf("wrote %d issues, want %d: %v", len(written), len(tc.wantTitles), titlesOf(written))
			}
			if got := titlesOf(written); strings.Join(got, "|") != strings.Join(tc.wantTitles, "|") {
				t.Errorf("titles = %v, want %v", got, tc.wantTitles)
			}

			// The bond edge to the bead must survive the clone either way.
			root := written[0]
			attachEdges := 0
			for _, dep := range root.Dependencies {
				if dep.DependsOnID != "mol-1" {
					continue
				}
				attachEdges++
				if dep.IssueID != root.ID {
					t.Errorf("attach edge leaves %s, want the spawned root %s", dep.IssueID, root.ID)
				}
				if dep.Type != types.DepBlocks {
					t.Errorf("attach edge type = %s, want %s", dep.Type, types.DepBlocks)
				}
			}
			if attachEdges != 1 {
				t.Errorf("wrote %d attach edges to mol-1, want 1", attachEdges)
			}

			if tc.wantSkipped == "" {
				if strings.Contains(out, "Steps skipped") {
					t.Errorf("output reports skipped steps without --root-only:\n%s", out)
				}
			} else if !strings.Contains(out, tc.wantSkipped) {
				t.Errorf("output does not report %q:\n%s", tc.wantSkipped, out)
			}
		})
	}
}

// TestMolBondRefusesRootOnlyWithoutSpawn pins that the flag is never silently
// dropped: a bond that spawns no proto issues has no steps to skip, so asking
// for root-only is an error rather than a no-op.
func TestMolBondRefusesRootOnlyWithoutSpawn(t *testing.T) {
	for _, tc := range []struct {
		name      string
		argA      string
		argB      string
		wantKinds string
	}{
		{name: "proto plus proto", argA: "proto-1", argB: "proto-2", wantKinds: "proto + proto"},
		{name: "molecule plus molecule", argA: "mol-1", argB: "mol-2", wantKinds: "molecule + molecule"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newBondRootOnlyFixture(3)
			var err error
			stderr := captureStderr(t, func() {
				err = runMolBondWith(testMolBondEnv(s), makeMolBondTestCmd(true, false), []string{tc.argA, tc.argB})
			})

			if err == nil {
				t.Fatal("--root-only on a bond that spawns no issues returned nil")
			}
			if !strings.Contains(stderr, "--root-only") || !strings.Contains(stderr, tc.wantKinds) {
				t.Errorf("stderr = %q, want it to name --root-only and %q", stderr, tc.wantKinds)
			}
			if len(s.recorder.created) != 0 {
				t.Errorf("refused bond wrote %d issues: %v", len(s.recorder.created), titlesOf(s.recorder.created))
			}
		})
	}
}

// TestMolBondDryRunRefusesRootOnlyWithoutSpawn keeps the preview honest: the
// dry run resolves the same operand kinds and refuses the same combinations
// instead of promising a skip the real bond would not make.
func TestMolBondDryRunRefusesRootOnlyWithoutSpawn(t *testing.T) {
	s := newBondRootOnlyFixture(3)
	var err error
	stderr := captureStderr(t, func() {
		err = runMolBondWith(testMolBondEnv(s), makeMolBondTestCmd(true, true), []string{"mol-1", "mol-2"})
	})

	if err == nil {
		t.Fatal("dry run with --root-only on a molecule+molecule bond returned nil")
	}
	if !strings.Contains(stderr, "--root-only") {
		t.Errorf("stderr = %q, want it to name --root-only", stderr)
	}
}

func TestRenderMolBondDryRunReportsRootOnly(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rootOnly bool
		want     string
	}{
		{name: "root-only preview", rootOnly: true, want: "Steps: root issue only (--root-only)"},
		{name: "full preview", rootOnly: false, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := captureStdout(t, func() error {
				renderMolBondDryRun(molBondInput{
					argA:     "proto-1",
					argB:     "mol-1",
					bondType: types.BondTypeSequential,
					rootOnly: tc.rootOnly,
				}, nil, "", nil, "")
				return nil
			})
			if tc.want == "" {
				if strings.Contains(out, "--root-only") {
					t.Errorf("dry run mentions --root-only without the flag:\n%s", out)
				}
				return
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("dry run does not report %q:\n%s", tc.want, out)
			}
		})
	}
}

func titlesOf(issues []*types.Issue) []string {
	titles := make([]string, len(issues))
	for i, issue := range issues {
		titles[i] = issue.Title
	}
	return titles
}
