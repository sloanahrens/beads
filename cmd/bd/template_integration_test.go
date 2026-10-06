//go:build cgo && integration

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// TestCloneSubgraph_SubstitutesGateRepoMetadata is the end-to-end regression
// test for the cook --persist -> pour path: a gate issue persisted with
// metadata.repo="{{gate_repo}}" (as createGateIssue/persistCookFormula would
// write it for `repo = "{{gate_repo}}"` in the formula) must have that
// placeholder substituted when the proto is cloned/poured with
// --var gate_repo=..., exactly like it would be if the repo selector were an
// AwaitID or Title placeholder instead.
func TestCloneSubgraph_SubstitutesGateRepoMetadata(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)
	ctx := context.Background()
	h := &templateTestHelper{s: s, ctx: ctx, t: t}

	epic := h.createIssue("Release {{version}}", "", types.TypeEpic, 1)
	h.addLabel(epic.ID, BeadsTemplateLabel)

	gate := &types.Issue{
		Title:     "Gate: gh:run",
		IssueType: "gate",
		Status:    types.StatusOpen,
		AwaitType: "gh:run",
		AwaitID:   "release.yml",
		Metadata:  json.RawMessage(`{"repo":"{{gate_repo}}"}`),
	}
	if err := s.CreateIssue(ctx, gate, "test-user"); err != nil {
		t.Fatalf("Failed to create gate issue: %v", err)
	}
	h.addParentChild(gate.ID, epic.ID)

	subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
	if err != nil {
		t.Fatalf("loadTemplateSubgraph failed: %v", err)
	}

	vars := map[string]string{"version": "2.0.0", "gate_repo": "srobroek/agentic-packages"}
	opts := CloneOptions{Vars: vars, Actor: "test-user"}
	result, err := cloneSubgraph(ctx, s, subgraph, opts)
	if err != nil {
		t.Fatalf("cloneSubgraph failed: %v", err)
	}

	newGateID, ok := result.IDMapping[gate.ID]
	if !ok {
		t.Fatalf("IDMapping missing entry for gate %s: %+v", gate.ID, result.IDMapping)
	}
	newGate, err := s.GetIssue(ctx, newGateID)
	if err != nil {
		t.Fatalf("Failed to get cloned gate issue: %v", err)
	}

	var metadata struct {
		Repo string `json:"repo"`
	}
	if err := json.Unmarshal(newGate.Metadata, &metadata); err != nil {
		t.Fatalf("newGate.Metadata = %s, not valid JSON: %v", newGate.Metadata, err)
	}
	if metadata.Repo != "srobroek/agentic-packages" {
		t.Errorf("cloned gate metadata.repo = %q, want %q (repo = \"{{gate_repo}}\" must be substituted at pour time, same as AwaitID/Title)", metadata.Repo, "srobroek/agentic-packages")
	}
}

// TestTemplateSuite consolidates template loading, cloning, and variable extraction
// tests that share one DB to reduce Dolt store initialization overhead.
func TestTemplateSuite(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)
	ctx := context.Background()
	h := &templateTestHelper{s: s, ctx: ctx, t: t}

	// --- LoadTemplateSubgraph tests ---

	t.Run("LoadTemplate_EpicWithNoChildren", func(t *testing.T) {
		epic := h.createIssue("Template Epic", "Description", types.TypeEpic, 1)
		h.addLabel(epic.ID, BeadsTemplateLabel)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}
		if subgraph.Root.ID != epic.ID {
			t.Errorf("Root ID = %s, want %s", subgraph.Root.ID, epic.ID)
		}
		if len(subgraph.Issues) != 1 {
			t.Errorf("Issues count = %d, want 1", len(subgraph.Issues))
		}
	})

	t.Run("LoadTemplate_EpicWithChildren", func(t *testing.T) {
		epic := h.createIssue("Template {{name}}", "Epic for {{name}}", types.TypeEpic, 1)
		h.addLabel(epic.ID, BeadsTemplateLabel)

		child1 := h.createIssue("Task 1 for {{name}}", "", types.TypeTask, 2)
		child2 := h.createIssue("Task 2 for {{name}}", "", types.TypeTask, 2)
		h.addParentChild(child1.ID, epic.ID)
		h.addParentChild(child2.ID, epic.ID)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}
		if len(subgraph.Issues) != 3 {
			t.Errorf("Issues count = %d, want 3", len(subgraph.Issues))
		}
		vars := extractAllVariables(subgraph)
		if len(vars) != 1 || vars[0] != "name" {
			t.Errorf("Variables = %v, want [name]", vars)
		}
	})

	t.Run("LoadTemplate_NestedChildren", func(t *testing.T) {
		epic := h.createIssue("Nested Template", "", types.TypeEpic, 1)
		child := h.createIssue("Child Task", "", types.TypeTask, 2)
		grandchild := h.createIssue("Grandchild Task", "", types.TypeTask, 3)

		h.addParentChild(child.ID, epic.ID)
		h.addParentChild(grandchild.ID, child.ID)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}
		if len(subgraph.Issues) != 3 {
			t.Errorf("Issues count = %d, want 3 (epic + child + grandchild)", len(subgraph.Issues))
		}
	})

	// --- CloneSubgraph tests ---

	t.Run("Clone_SimpleTemplate", func(t *testing.T) {
		epic := h.createIssue("Release {{version}}", "Release notes for {{version}}", types.TypeEpic, 1)
		h.addLabel(epic.ID, BeadsTemplateLabel)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		vars := map[string]string{"version": "2.0.0"}
		opts := CloneOptions{Vars: vars, Actor: "test-user"}
		result, err := cloneSubgraph(ctx, s, subgraph, opts)
		if err != nil {
			t.Fatalf("cloneSubgraph failed: %v", err)
		}

		if result.Created != 1 {
			t.Errorf("Created = %d, want 1", result.Created)
		}
		if result.NewEpicID == epic.ID {
			t.Error("NewEpicID should be different from template ID")
		}

		newEpic, err := s.GetIssue(ctx, result.NewEpicID)
		if err != nil {
			t.Fatalf("Failed to get cloned issue: %v", err)
		}
		if newEpic.Title != "Release 2.0.0" {
			t.Errorf("Title = %q, want %q", newEpic.Title, "Release 2.0.0")
		}
		if newEpic.Description != "Release notes for 2.0.0" {
			t.Errorf("Description = %q, want %q", newEpic.Description, "Release notes for 2.0.0")
		}
	})

	t.Run("Clone_TemplateWithChildren", func(t *testing.T) {
		epic := h.createIssue("Deploy {{service}}", "", types.TypeEpic, 1)
		child1 := h.createIssue("Build {{service}}", "", types.TypeTask, 2)
		child2 := h.createIssue("Test {{service}}", "", types.TypeTask, 2)

		h.addParentChild(child1.ID, epic.ID)
		h.addParentChild(child2.ID, epic.ID)
		h.addLabel(epic.ID, BeadsTemplateLabel)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		vars := map[string]string{"service": "api-gateway"}
		opts := CloneOptions{Vars: vars, Actor: "test-user"}
		result, err := cloneSubgraph(ctx, s, subgraph, opts)
		if err != nil {
			t.Fatalf("cloneSubgraph failed: %v", err)
		}
		if result.Created != 3 {
			t.Errorf("Created = %d, want 3", result.Created)
		}

		if _, ok := result.IDMapping[epic.ID]; !ok {
			t.Error("ID mapping missing epic")
		}
		if _, ok := result.IDMapping[child1.ID]; !ok {
			t.Error("ID mapping missing child1")
		}
		if _, ok := result.IDMapping[child2.ID]; !ok {
			t.Error("ID mapping missing child2")
		}

		newEpic, err := s.GetIssue(ctx, result.NewEpicID)
		if err != nil {
			t.Fatalf("Failed to get cloned epic: %v", err)
		}
		if newEpic.Title != "Deploy api-gateway" {
			t.Errorf("Epic title = %q, want %q", newEpic.Title, "Deploy api-gateway")
		}

		deps, err := s.GetDependencyRecords(ctx, result.IDMapping[child1.ID])
		if err != nil {
			t.Fatalf("Failed to get dependencies: %v", err)
		}
		hasParentChild := false
		for _, dep := range deps {
			if dep.DependsOnID == result.NewEpicID && dep.Type == types.DepParentChild {
				hasParentChild = true
				break
			}
		}
		if !hasParentChild {
			t.Error("Cloned child should have parent-child dependency on cloned epic")
		}
	})

	t.Run("Clone_StartsWithOpenStatus", func(t *testing.T) {
		epic := h.createIssue("Template", "", types.TypeEpic, 1)
		err := s.UpdateIssue(ctx, epic.ID, map[string]interface{}{"status": "in_progress"}, "test-user")
		if err != nil {
			t.Fatalf("Failed to update status: %v", err)
		}

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		opts := CloneOptions{Actor: "test-user"}
		result, err := cloneSubgraph(ctx, s, subgraph, opts)
		if err != nil {
			t.Fatalf("cloneSubgraph failed: %v", err)
		}

		newEpic, err := s.GetIssue(ctx, result.NewEpicID)
		if err != nil {
			t.Fatalf("Failed to get cloned issue: %v", err)
		}
		if newEpic.Status != types.StatusOpen {
			t.Errorf("Status = %s, want %s", newEpic.Status, types.StatusOpen)
		}
	})

	t.Run("Clone_AssigneeOverrideRootOnly", func(t *testing.T) {
		epic := h.createIssue("Root Epic", "", types.TypeEpic, 1)
		child := h.createIssue("Child Task", "", types.TypeTask, 2)
		h.addParentChild(child.ID, epic.ID)

		err := s.UpdateIssue(ctx, epic.ID, map[string]interface{}{"assignee": "template-owner"}, "test-user")
		if err != nil {
			t.Fatalf("Failed to set epic assignee: %v", err)
		}
		err = s.UpdateIssue(ctx, child.ID, map[string]interface{}{"assignee": "child-owner"}, "test-user")
		if err != nil {
			t.Fatalf("Failed to set child assignee: %v", err)
		}

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		opts := CloneOptions{Assignee: "new-assignee", Actor: "test-user"}
		result, err := cloneSubgraph(ctx, s, subgraph, opts)
		if err != nil {
			t.Fatalf("cloneSubgraph failed: %v", err)
		}

		newEpic, err := s.GetIssue(ctx, result.NewEpicID)
		if err != nil {
			t.Fatalf("Failed to get cloned epic: %v", err)
		}
		if newEpic.Assignee != "new-assignee" {
			t.Errorf("Epic assignee = %q, want %q", newEpic.Assignee, "new-assignee")
		}

		newChildID := result.IDMapping[child.ID]
		newChild, err := s.GetIssue(ctx, newChildID)
		if err != nil {
			t.Fatalf("Failed to get cloned child: %v", err)
		}
		if newChild.Assignee != "child-owner" {
			t.Errorf("Child assignee = %q, want %q", newChild.Assignee, "child-owner")
		}
	})

	t.Run("Clone_RootOnly_CreatedCountMatchesActual", func(t *testing.T) {
		epic := h.createIssue("Root Epic", "", types.TypeEpic, 1)
		child1 := h.createIssue("Child 1", "", types.TypeTask, 2)
		child2 := h.createIssue("Child 2", "", types.TypeTask, 2)
		h.addParentChild(child1.ID, epic.ID)
		h.addParentChild(child2.ID, epic.ID)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		// With RootOnly=true, only the root should be created
		opts := CloneOptions{RootOnly: true, Actor: "test-user"}
		result, err := cloneSubgraph(ctx, s, subgraph, opts)
		if err != nil {
			t.Fatalf("cloneSubgraph failed: %v", err)
		}

		if result.Created != 1 {
			t.Errorf("Created = %d, want 1 (only root should be counted when RootOnly=true)", result.Created)
		}
		if len(result.IDMapping) != 1 {
			t.Errorf("IDMapping has %d entries, want 1", len(result.IDMapping))
		}
	})

	// --- ExtractAllVariables tests ---

	t.Run("ExtractAllVariables", func(t *testing.T) {
		epic := h.createIssue("Release {{version}}", "For {{product}}", types.TypeEpic, 1)
		child := h.createIssue("Deploy to {{environment}}", "", types.TypeTask, 2)
		h.addParentChild(child.ID, epic.ID)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		vars := extractAllVariables(subgraph)
		varMap := make(map[string]bool)
		for _, v := range vars {
			varMap[v] = true
		}
		if !varMap["version"] {
			t.Error("Missing variable: version")
		}
		if !varMap["product"] {
			t.Error("Missing variable: product")
		}
		if !varMap["environment"] {
			t.Error("Missing variable: environment")
		}
	})

	// --- LoadTemplateSubgraphWithManyChildren tests (bd-c8d5) ---

	t.Run("ManyChildren_4Children", func(t *testing.T) {
		epic := h.createIssue("Proto Workflow", "Workflow with 4 steps", types.TypeEpic, 1)
		h.addLabel(epic.ID, BeadsTemplateLabel)

		child1 := h.createIssue("load-context", "", types.TypeTask, 2)
		child2 := h.createIssue("implement", "", types.TypeTask, 2)
		child3 := h.createIssue("self-review", "", types.TypeTask, 2)
		child4 := h.createIssue("request-shutdown", "", types.TypeTask, 2)

		h.addParentChild(child1.ID, epic.ID)
		h.addParentChild(child2.ID, epic.ID)
		h.addParentChild(child3.ID, epic.ID)
		h.addParentChild(child4.ID, epic.ID)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		if len(subgraph.Issues) != 5 {
			t.Errorf("Issues count = %d, want 5 (epic + 4 children)", len(subgraph.Issues))
			for _, iss := range subgraph.Issues {
				t.Logf("  - %s: %s", iss.ID, iss.Title)
			}
		}

		childIDs := []string{child1.ID, child2.ID, child3.ID, child4.ID}
		for _, childID := range childIDs {
			if _, ok := subgraph.IssueMap[childID]; !ok {
				t.Errorf("Child %s not found in subgraph", childID)
			}
		}
	})

	t.Run("ManyChildren_CloneCreatesAll4", func(t *testing.T) {
		epic := h.createIssue("Polecat Work", "", types.TypeEpic, 1)
		h.addLabel(epic.ID, BeadsTemplateLabel)

		child1 := h.createIssue("load-context", "", types.TypeTask, 2)
		child2 := h.createIssue("implement", "", types.TypeTask, 2)
		child3 := h.createIssue("self-review", "", types.TypeTask, 2)
		child4 := h.createIssue("request-shutdown", "", types.TypeTask, 2)

		h.addParentChild(child1.ID, epic.ID)
		h.addParentChild(child2.ID, epic.ID)
		h.addParentChild(child3.ID, epic.ID)
		h.addParentChild(child4.ID, epic.ID)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		opts := CloneOptions{Actor: "test-user"}
		result, err := cloneSubgraph(ctx, s, subgraph, opts)
		if err != nil {
			t.Fatalf("cloneSubgraph failed: %v", err)
		}

		if result.Created != 5 {
			t.Errorf("Created = %d, want 5", result.Created)
		}
		for _, childID := range []string{child1.ID, child2.ID, child3.ID, child4.ID} {
			if _, ok := result.IDMapping[childID]; !ok {
				t.Errorf("Child %s not in ID mapping", childID)
			}
		}
	})

	t.Run("ManyChildren_HierarchicalIDs", func(t *testing.T) {
		epic := h.createIssueWithID("test-lwuu", "mol-polecat-work", "", types.TypeEpic, 1)
		h.addLabel(epic.ID, BeadsTemplateLabel)

		child1 := h.createIssueWithID("test-lwuu.1", "load-context", "", types.TypeTask, 2)
		child2 := h.createIssueWithID("test-lwuu.2", "implement", "", types.TypeTask, 2)
		child3 := h.createIssueWithID("test-lwuu.3", "self-review", "", types.TypeTask, 2)
		child8 := h.createIssueWithID("test-lwuu.8", "request-shutdown", "", types.TypeTask, 2)

		h.addParentChild(child1.ID, epic.ID)
		h.addParentChild(child2.ID, epic.ID)
		h.addParentChild(child3.ID, epic.ID)
		h.addParentChild(child8.ID, epic.ID)

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		if len(subgraph.Issues) != 5 {
			t.Errorf("Issues count = %d, want 5", len(subgraph.Issues))
			for _, iss := range subgraph.Issues {
				t.Logf("  - %s: %s", iss.ID, iss.Title)
			}
		}

		for _, childID := range []string{"test-lwuu.1", "test-lwuu.2", "test-lwuu.3", "test-lwuu.8"} {
			if _, ok := subgraph.IssueMap[childID]; !ok {
				t.Errorf("Child %s not found in subgraph", childID)
			}
		}
	})

	t.Run("ManyChildren_WrongDepTypeNotLoaded", func(t *testing.T) {
		epic := h.createIssue("Proto with mixed deps", "", types.TypeEpic, 1)
		h.addLabel(epic.ID, BeadsTemplateLabel)

		child1 := h.createIssue("load-context", "", types.TypeTask, 2)
		child2 := h.createIssue("implement", "", types.TypeTask, 2)
		child3 := h.createIssue("self-review", "", types.TypeTask, 2)
		child4 := h.createIssue("request-shutdown", "", types.TypeTask, 2)

		h.addParentChild(child1.ID, epic.ID)
		h.addParentChild(child2.ID, epic.ID)

		// child3 and child4 have "related" dependency (wrong type — not parent-child)
		for _, childID := range []string{child3.ID, child4.ID} {
			blocksDep := &types.Dependency{IssueID: childID, DependsOnID: epic.ID, Type: types.DepRelated}
			if err := s.AddDependency(ctx, blocksDep, "test-user"); err != nil {
				t.Fatalf("Failed to add blocks dependency: %v", err)
			}
		}

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		if len(subgraph.Issues) != 3 {
			t.Errorf("Expected 3 issues (without hierarchical ID fallback), got %d", len(subgraph.Issues))
		}
	})

	t.Run("ManyChildren_HierarchicalWrongDepTypeLoaded", func(t *testing.T) {
		epic := h.createIssueWithID("test-pcat", "Proto with mixed deps", "", types.TypeEpic, 1)
		h.addLabel(epic.ID, BeadsTemplateLabel)

		child1 := h.createIssueWithID("test-pcat.1", "load-context", "", types.TypeTask, 2)
		child2 := h.createIssueWithID("test-pcat.2", "implement", "", types.TypeTask, 2)
		h.addParentChild(child1.ID, epic.ID)
		h.addParentChild(child2.ID, epic.ID)

		// child3 has NO dependency at all (broken data)
		_ = h.createIssueWithID("test-pcat.3", "self-review", "", types.TypeTask, 2)

		// child8 has wrong dependency type (related, not parent-child)
		child8 := h.createIssueWithID("test-pcat.8", "request-shutdown", "", types.TypeTask, 2)
		relatedDep := &types.Dependency{IssueID: child8.ID, DependsOnID: epic.ID, Type: types.DepRelated}
		if err := s.AddDependency(ctx, relatedDep, "test-user"); err != nil {
			t.Fatalf("Failed to add related dependency: %v", err)
		}

		subgraph, err := loadTemplateSubgraph(ctx, s, epic.ID)
		if err != nil {
			t.Fatalf("loadTemplateSubgraph failed: %v", err)
		}

		if len(subgraph.Issues) != 5 {
			t.Errorf("Expected 5 issues (root + 4 hierarchical children), got %d", len(subgraph.Issues))
		}
		for _, childID := range []string{"test-pcat.1", "test-pcat.2", "test-pcat.3", "test-pcat.8"} {
			if _, ok := subgraph.IssueMap[childID]; !ok {
				t.Errorf("Child %s not found in subgraph", childID)
			}
		}
	})
}

// TestResolveProtoIDOrTitle tests proto lookup by ID or title (bd-drcx).
// Kept separate from TestTemplateSuite because title-based search is affected by shared data.
func TestResolveProtoIDOrTitle(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)
	ctx := context.Background()
	h := &templateTestHelper{s: s, ctx: ctx, t: t}

	proto1 := h.createIssue("mol-polecat-work", "Polecat workflow", types.TypeEpic, 1)
	h.addLabel(proto1.ID, BeadsTemplateLabel)

	proto2 := h.createIssue("mol-version-bump", "Version bump workflow", types.TypeEpic, 1)
	h.addLabel(proto2.ID, BeadsTemplateLabel)

	proto3 := h.createIssue("mol-release", "Release workflow", types.TypeEpic, 1)
	h.addLabel(proto3.ID, BeadsTemplateLabel)

	nonProto := h.createIssue("mol-test", "Not a proto", types.TypeTask, 2)

	t.Run("resolve by exact ID", func(t *testing.T) {
		resolved, err := resolveProtoIDOrTitle(ctx, s, proto1.ID)
		if err != nil {
			t.Fatalf("Failed to resolve by ID: %v", err)
		}
		if resolved != proto1.ID {
			t.Errorf("Expected %s, got %s", proto1.ID, resolved)
		}
	})

	t.Run("resolve by exact title", func(t *testing.T) {
		resolved, err := resolveProtoIDOrTitle(ctx, s, "mol-polecat-work")
		if err != nil {
			t.Fatalf("Failed to resolve by title: %v", err)
		}
		if resolved != proto1.ID {
			t.Errorf("Expected %s, got %s", proto1.ID, resolved)
		}
	})

	t.Run("resolve by title case-insensitive", func(t *testing.T) {
		resolved, err := resolveProtoIDOrTitle(ctx, s, "MOL-POLECAT-WORK")
		if err != nil {
			t.Fatalf("Failed to resolve by title (case-insensitive): %v", err)
		}
		if resolved != proto1.ID {
			t.Errorf("Expected %s, got %s", proto1.ID, resolved)
		}
	})

	t.Run("resolve by unique partial title", func(t *testing.T) {
		resolved, err := resolveProtoIDOrTitle(ctx, s, "polecat")
		if err != nil {
			t.Fatalf("Failed to resolve by partial title: %v", err)
		}
		if resolved != proto1.ID {
			t.Errorf("Expected %s, got %s", proto1.ID, resolved)
		}
	})

	t.Run("ambiguous partial title returns error", func(t *testing.T) {
		_, err := resolveProtoIDOrTitle(ctx, s, "mol-")
		if err == nil {
			t.Fatal("Expected error for ambiguous title, got nil")
		}
		if !strings.Contains(err.Error(), "ambiguous") {
			t.Errorf("Expected 'ambiguous' in error, got: %v", err)
		}
	})

	t.Run("non-existent returns error", func(t *testing.T) {
		_, err := resolveProtoIDOrTitle(ctx, s, "nonexistent-proto")
		if err == nil {
			t.Fatal("Expected error for non-existent proto, got nil")
		}
		if !strings.Contains(err.Error(), "no proto found") {
			t.Errorf("Expected 'no proto found' in error, got: %v", err)
		}
	})

	t.Run("non-proto ID returns error", func(t *testing.T) {
		_, err := resolveProtoIDOrTitle(ctx, s, nonProto.ID)
		if err == nil {
			t.Fatal("Expected error for non-proto ID, got nil")
		}
	})
}

func TestFindHierarchicalChildren_DirectChildrenOnly(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	testDB := filepath.Join(tmpDir, ".beads", "beads.db")
	s := newTestStore(t, testDB)
	ctx := context.Background()
	h := &templateTestHelper{s: s, ctx: ctx, t: t}

	h.createHierarchy(
		"test-tree",
		"test-tree.1",
		"test-tree.2",
		"test-tree.1.1",
		"test-tree.2.1",
		"test-tree.2.1.1",
	)
	h.createIssueWithID("test-other.1", "test-other.1", "", types.TypeTask, 2)

	children, err := findHierarchicalChildren(ctx, s, "test-tree")
	if err != nil {
		t.Fatalf("findHierarchicalChildren(root) failed: %v", err)
	}
	requireIssueIDs(t, children, "test-tree.1", "test-tree.2")

	nestedChildren, err := findHierarchicalChildren(ctx, s, "test-tree.2")
	if err != nil {
		t.Fatalf("findHierarchicalChildren(child) failed: %v", err)
	}
	requireIssueIDs(t, nestedChildren, "test-tree.2.1")
}
