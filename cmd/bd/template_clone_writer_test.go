package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// cloneCallCounter counts the write calls a clone makes and forwards them to
// the recording fake, so the test can see how the clone reached the store.
type cloneCallCounter struct {
	*recordingMolWriter
	createIssue   int
	createIssues  int
	addDependency int
}

func (c *cloneCallCounter) CreateIssue(ctx context.Context, issue *types.Issue, actor string) error {
	c.createIssue++
	return c.recordingMolWriter.CreateIssue(ctx, issue, actor)
}

func (c *cloneCallCounter) CreateIssues(ctx context.Context, issues []*types.Issue, actor string) error {
	c.createIssues++
	return c.recordingMolWriter.CreateIssues(ctx, issues, actor)
}

func (c *cloneCallCounter) AddDependency(ctx context.Context, dep *types.Dependency, actor string) error {
	c.addDependency++
	return c.recordingMolWriter.AddDependency(ctx, dep, actor)
}

// cloneWriterFixture is a molecule shaped like mol-polecat-work: a root with
// eight steps under it, and each step needing the one before it.
func cloneWriterFixture() *TemplateSubgraph {
	root := &types.Issue{ID: "tmpl-root", Title: "Root", IssueType: types.TypeTask}
	subgraph := &TemplateSubgraph{Root: root, Issues: []*types.Issue{root}}
	for i := 1; i <= 8; i++ {
		step := &types.Issue{
			ID:        fmt.Sprintf("tmpl-s%d", i),
			Title:     fmt.Sprintf("Step %d", i),
			IssueType: types.TypeTask,
		}
		subgraph.Issues = append(subgraph.Issues, step)
		subgraph.Dependencies = append(subgraph.Dependencies, &types.Dependency{
			IssueID:     step.ID,
			DependsOnID: root.ID,
			Type:        types.DepParentChild,
		})
		if i > 1 {
			subgraph.Dependencies = append(subgraph.Dependencies, &types.Dependency{
				IssueID:     step.ID,
				DependsOnID: fmt.Sprintf("tmpl-s%d", i-1),
				Type:        types.DepBlocks,
			})
		}
	}
	return subgraph
}

func TestCloneSubgraphWritesOneBatch(t *testing.T) {
	counter := &cloneCallCounter{recordingMolWriter: &recordingMolWriter{}}
	opts := CloneOptions{
		Actor:         "tester",
		Ephemeral:     true,
		AttachToID:    "bd-target",
		AttachDepType: types.DepParentChild,
	}

	result, err := cloneSubgraphInto(context.Background(), counter, cloneWriterFixture(), opts)
	if err != nil {
		t.Fatalf("cloneSubgraphInto: %v", err)
	}

	if counter.createIssues != 1 {
		t.Errorf("CreateIssues called %d times, want 1", counter.createIssues)
	}
	if counter.createIssue != 0 {
		t.Errorf("CreateIssue called %d times, want 0", counter.createIssue)
	}
	if counter.addDependency != 0 {
		t.Errorf("AddDependency called %d times, want 0", counter.addDependency)
	}

	written := counter.created
	if len(written) != 9 {
		t.Fatalf("wrote %d issues, want 9", len(written))
	}

	edges := 0
	attachEdges := 0
	for _, issue := range written {
		for _, dep := range issue.Dependencies {
			edges++
			if dep.IssueID == "" || dep.DependsOnID == "" {
				t.Errorf("edge %+v carries an empty end", dep)
			}
			if strings.HasPrefix(dep.IssueID, "batch:") || strings.HasPrefix(dep.DependsOnID, "batch:") {
				t.Errorf("edge %+v still names a batch position", dep)
			}
			if dep.DependsOnID == opts.AttachToID {
				attachEdges++
				if dep.IssueID != result.NewEpicID {
					t.Errorf("attach edge leaves %s, want the spawned root %s", dep.IssueID, result.NewEpicID)
				}
			}
		}
	}
	if edges != 16 {
		t.Errorf("wrote %d dependencies, want 16", edges)
	}
	if attachEdges != 1 {
		t.Errorf("wrote %d attach edges to %s, want 1", attachEdges, opts.AttachToID)
	}

	if result.Created != 9 {
		t.Errorf("Created = %d, want 9", result.Created)
	}
	if result.NewEpicID == "" || result.NewEpicID != written[0].ID {
		t.Errorf("NewEpicID = %q, want the root's assigned ID %q", result.NewEpicID, written[0].ID)
	}
	if len(result.IDMapping) != 9 {
		t.Fatalf("IDMapping has %d entries, want 9", len(result.IDMapping))
	}
	for oldID, newID := range result.IDMapping {
		if newID == "" {
			t.Errorf("IDMapping[%s] is empty", oldID)
		}
	}
}
