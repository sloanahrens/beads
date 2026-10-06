//go:build cgo

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/workapi"
)

type watchListDependencyStoreStub struct {
	allDeps map[string][]*types.Dependency
	err     error
}

func (s watchListDependencyStoreStub) GetAllDependencyRecords(_ context.Context) (map[string][]*types.Dependency, error) {
	return s.allDeps, s.err
}

func TestListParseTimeFlag(t *testing.T) {
	cases := []string{
		"2025-12-26",
		"2025-12-26T12:34:56",
		"2025-12-26 12:34:56",
		time.DateOnly,
		time.RFC3339,
	}

	for _, c := range cases {
		// Just make sure we accept the expected formats.
		var s string
		switch c {
		case time.DateOnly:
			s = "2025-12-26"
		case time.RFC3339:
			s = "2025-12-26T12:34:56Z"
		default:
			s = c
		}
		got, err := parseTimeFlag(s)
		if err != nil {
			t.Fatalf("parseTimeFlag(%q) error: %v", s, err)
		}
		if got.Year() != 2025 {
			t.Fatalf("parseTimeFlag(%q) year=%d, want 2025", s, got.Year())
		}
	}

	if _, err := parseTimeFlag("not-a-date"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestListPinIndicator(t *testing.T) {
	if pinIndicator(&types.Issue{Pinned: true}) == "" {
		t.Fatalf("expected pin indicator")
	}
	if pinIndicator(&types.Issue{Pinned: false}) != "" {
		t.Fatalf("expected empty pin indicator")
	}
}

func TestListFormatPrettyIssue_BadgesAndDefaults(t *testing.T) {
	iss := &types.Issue{ID: "bd-1", Title: "Hello", Status: "wat", Priority: 99, IssueType: "bug"}
	out := formatPrettyIssue(iss)
	if !strings.Contains(out, "bd-1") || !strings.Contains(out, "Hello") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "[bug]") {
		t.Fatalf("expected bug badge: %q", out)
	}
}

func TestListBuildIssueTree_ParentChildByDotID(t *testing.T) {
	parent := &types.Issue{ID: "bd-1", Title: "Parent", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}
	child := &types.Issue{ID: "bd-1.1", Title: "Child", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}
	orphan := &types.Issue{ID: "bd-2.1", Title: "Orphan", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}

	roots, children := buildIssueTree([]*types.Issue{child, parent, orphan})
	if len(children["bd-1"]) != 1 || children["bd-1"][0].ID != "bd-1.1" {
		t.Fatalf("expected bd-1 to have bd-1.1 child: %+v", children)
	}
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots (parent + orphan), got %d", len(roots))
	}
}

// Regression test for gastownhall/beads#3936:
// `relates-to` is a loose graph link, not a hierarchy edge. It must not nest
// issues under each other in `bd list` — and a bidirectional relates-to between
// two epics must not collapse both subtrees out of the root set.
func TestListBuildIssueTree_RelatesToDoesNotNestEpics(t *testing.T) {
	epicA := &types.Issue{ID: "bd-a", Title: "Epic A", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeEpic}
	epicB := &types.Issue{ID: "bd-b", Title: "Epic B", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeEpic}

	t.Run("OneDirection", func(t *testing.T) {
		allDeps := map[string][]*types.Dependency{
			"bd-a": {
				{IssueID: "bd-a", DependsOnID: "bd-b", Type: types.DepRelatesTo},
			},
		}
		roots, children := buildIssueTreeWithDeps([]*types.Issue{epicA, epicB}, allDeps)
		if len(roots) != 2 {
			t.Fatalf("expected both epics as roots, got %d: %+v", len(roots), roots)
		}
		if len(children["bd-b"]) != 0 {
			t.Fatalf("relates-to must not nest under target epic, got children: %+v", children["bd-b"])
		}
	})

	t.Run("Bidirectional", func(t *testing.T) {
		allDeps := map[string][]*types.Dependency{
			"bd-a": {
				{IssueID: "bd-a", DependsOnID: "bd-b", Type: types.DepRelatesTo},
			},
			"bd-b": {
				{IssueID: "bd-b", DependsOnID: "bd-a", Type: types.DepRelatesTo},
			},
		}
		roots, _ := buildIssueTreeWithDeps([]*types.Issue{epicA, epicB}, allDeps)
		if len(roots) != 2 {
			t.Fatalf("bidirectional relates-to must not drop epics from roots, got %d: %+v", len(roots), roots)
		}
	})
}

// Regression test for https://github.com/steveyegge/beads/issues/1446
// A task with multiple dependencies on the same epic should only appear once.
func TestListBuildIssueTree_NoDuplicateChildrenFromMultipleDeps(t *testing.T) {
	epic := &types.Issue{ID: "bd-epic", Title: "Epic", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeEpic}
	task := &types.Issue{ID: "bd-task", Title: "Task", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}

	// The task has two different dependency types pointing at the same epic
	allDeps := map[string][]*types.Dependency{
		"bd-task": {
			{IssueID: "bd-task", DependsOnID: "bd-epic", Type: types.DepParentChild},
			{IssueID: "bd-task", DependsOnID: "bd-epic", Type: types.DepBlocks},
		},
	}

	roots, children := buildIssueTreeWithDeps([]*types.Issue{epic, task}, allDeps)

	if len(roots) != 1 || roots[0].ID != "bd-epic" {
		t.Fatalf("expected 1 root (epic), got %d: %+v", len(roots), roots)
	}
	if len(children["bd-epic"]) != 1 {
		t.Fatalf("expected 1 child under epic, got %d", len(children["bd-epic"]))
	}
	if children["bd-epic"][0].ID != "bd-task" {
		t.Fatalf("expected bd-task as child, got %s", children["bd-epic"][0].ID)
	}
}

// A non-parent-child dependency on an epic (blocks / waits-for / discovered-from)
// is a workflow edge, not membership. It must not nest the source under the epic
// in `bd list --tree`. The storage layer already scopes an epic's children to
// parent-child edges only (epic_closure.go), so the tree view must match: nesting
// a mere blocker as a child made 2-layer parent trees render as 6+ level tangles
// and triggered false "the hierarchy is broken" conclusions during grooming.
func TestListBuildIssueTree_NonParentChildDepOnEpicDoesNotNest(t *testing.T) {
	epic := &types.Issue{ID: "bd-epic", Title: "Epic", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeEpic}

	// Each non-parent-child edge type that AffectsReadyWork or is otherwise a
	// legitimate cross-cutting link to an epic — none of these imply parenthood.
	for _, depType := range []types.DependencyType{
		types.DepBlocks,
		types.DepWaitsFor,
		types.DepConditionalBlocks,
		types.DepDiscoveredFrom,
		types.DepRelated,
	} {
		t.Run(string(depType), func(t *testing.T) {
			task := &types.Issue{ID: "bd-task", Title: "Task", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask}
			allDeps := map[string][]*types.Dependency{
				"bd-task": {
					{IssueID: "bd-task", DependsOnID: "bd-epic", Type: depType},
				},
			}

			roots, children := buildIssueTreeWithDeps([]*types.Issue{epic, task}, allDeps)

			if len(roots) != 2 {
				t.Fatalf("expected both epic and task as roots (no nesting), got %d: %+v", len(roots), roots)
			}
			if len(children["bd-epic"]) != 0 {
				t.Fatalf("%s edge on an epic must not nest the source as a child, got: %+v", depType, children["bd-epic"])
			}
		})
	}
}

func TestFormatPrettyIssueWithContext(t *testing.T) {
	t.Parallel()

	issue := &types.Issue{
		ID:        "bd-42",
		Title:     "Implement feature",
		Status:    types.StatusOpen,
		Priority:  1,
		IssueType: types.TypeTask,
	}

	t.Run("WithoutParentEpic", func(t *testing.T) {
		out := formatPrettyIssueWithContext(issue, "")
		base := formatPrettyIssue(issue)
		if out != base {
			t.Errorf("Without parent epic, output should match formatPrettyIssue.\nGot:  %q\nWant: %q", out, base)
		}
	})

	t.Run("WithParentEpic", func(t *testing.T) {
		out := formatPrettyIssueWithContext(issue, "Auth Overhaul")
		if !strings.Contains(out, "bd-42") {
			t.Errorf("Expected issue ID in output: %q", out)
		}
		if !strings.Contains(out, "Implement feature") {
			t.Errorf("Expected title in output: %q", out)
		}
		if !strings.Contains(out, "Auth Overhaul") {
			t.Errorf("Expected parent epic title in output: %q", out)
		}
	})
}

func TestDisplayReadyList(t *testing.T) {
	t.Parallel()

	issues := []*types.Issue{
		{ID: "bd-1", Title: "Task A", Status: types.StatusOpen, Priority: 0, IssueType: types.TypeTask},
		{ID: "bd-2", Title: "Task B", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeBug},
	}

	t.Run("WithParentEpics", func(t *testing.T) {
		epicMap := map[string]string{"bd-1": "My Epic"}
		out := captureStdout(t, func() error {
			displayReadyList(issues, epicMap)
			return nil
		})
		if !strings.Contains(out, "bd-1") || !strings.Contains(out, "bd-2") {
			t.Errorf("Expected both issue IDs in output: %q", out)
		}
		if !strings.Contains(out, "My Epic") {
			t.Errorf("Expected parent epic annotation in output: %q", out)
		}
		if !strings.Contains(out, "Ready: 2 issues") {
			t.Errorf("Expected summary footer in output: %q", out)
		}
	})

	t.Run("WithNilEpicMap", func(t *testing.T) {
		out := captureStdout(t, func() error {
			displayReadyList(issues, nil)
			return nil
		})
		if !strings.Contains(out, "bd-1") || !strings.Contains(out, "bd-2") {
			t.Errorf("Expected both issue IDs in output: %q", out)
		}
		if !strings.Contains(out, "Ready: 2 issues") {
			t.Errorf("Expected summary footer in output: %q", out)
		}
	})
}

func TestListSortIssues_ClosedNilLast(t *testing.T) {
	t1 := time.Now().Add(-2 * time.Hour)
	t2 := time.Now().Add(-1 * time.Hour)

	closedOld := &types.Issue{ID: "bd-1", ClosedAt: &t1}
	closedNew := &types.Issue{ID: "bd-2", ClosedAt: &t2}
	open := &types.Issue{ID: "bd-3", ClosedAt: nil}

	issues := []*types.Issue{open, closedOld, closedNew}
	workapi.SortIssues(issues, "closed", false)
	if issues[0].ID != "bd-2" || issues[1].ID != "bd-1" || issues[2].ID != "bd-3" {
		t.Fatalf("unexpected order: %s, %s, %s", issues[0].ID, issues[1].ID, issues[2].ID)
	}
}

func TestListDisplayPrettyList(t *testing.T) {
	out := captureStdout(t, func() error {
		displayPrettyList(nil, false)
		return nil
	})
	if !strings.Contains(out, "No issues found") {
		t.Fatalf("unexpected output: %q", out)
	}

	issues := []*types.Issue{
		{ID: "bd-1", Title: "A", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask},
		{ID: "bd-2", Title: "B", Status: types.StatusInProgress, Priority: 1, IssueType: types.TypeFeature},
		{ID: "bd-1.1", Title: "C", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask},
	}

	out = captureStdout(t, func() error {
		displayPrettyList(issues, false)
		return nil
	})
	if !strings.Contains(out, "bd-1") || !strings.Contains(out, "bd-1.1") || !strings.Contains(out, "Total:") {
		t.Fatalf("unexpected output: %q", out)
	}
	if strings.Contains(out, "Showing ") {
		t.Fatalf("untruncated list must keep Total: wording, got: %q", out)
	}
}

// GH#5362: a page cut by --limit must not label the page size as Total.
func TestListDisplayPrettyList_TruncatedSummary(t *testing.T) {
	issues := []*types.Issue{
		{ID: "bd-1", Title: "A", Status: types.StatusOpen, Priority: 2, IssueType: types.TypeTask},
		{ID: "bd-2", Title: "B", Status: types.StatusInProgress, Priority: 1, IssueType: types.TypeFeature},
	}

	out := captureStdout(t, func() error {
		displayPrettyListWithDepsMode(issues, false, nil, "", true, false, "")
		return nil
	})
	if !strings.Contains(out, "Showing 2 issues") {
		t.Fatalf("expected honest Showing N summary when truncated, got: %q", out)
	}
	if !strings.Contains(out, "truncated by --limit") {
		t.Fatalf("expected truncation notice in summary, got: %q", out)
	}
	if strings.Contains(out, "Total:") {
		t.Fatalf("truncated summary must not claim Total:, got: %q", out)
	}
}

func TestDisplayWatchedIssueList_UsesDependencyHierarchy(t *testing.T) {
	parent := &types.Issue{ID: "bd-zparent", Title: "Parent", Status: types.StatusOpen, Priority: 1, IssueType: types.TypeEpic}
	child := &types.Issue{ID: "bd-achild", Title: "Child", Status: types.StatusOpen, Priority: 1, IssueType: types.TypeTask}
	store := watchListDependencyStoreStub{
		allDeps: map[string][]*types.Dependency{
			child.ID: {
				{IssueID: child.ID, DependsOnID: parent.ID, Type: types.DepParentChild},
			},
		},
	}

	out := captureStdout(t, func() error {
		displayWatchedIssueList(context.Background(), store, []*types.Issue{child, parent}, false, false, "")
		return nil
	})

	parentLine := strings.Index(out, "bd-zparent")
	childLine := strings.Index(out, "└──")
	if parentLine == -1 || childLine == -1 {
		t.Fatalf("expected parent root and child connector in output, got:\n%s", out)
	}
	if childLine < parentLine {
		t.Fatalf("expected child to render under parent in watch output, got:\n%s", out)
	}
	if strings.Contains(out, "\nbd-achild ") || strings.HasPrefix(out, "bd-achild ") {
		t.Fatalf("expected child not to render as a root in watch output, got:\n%s", out)
	}
}
