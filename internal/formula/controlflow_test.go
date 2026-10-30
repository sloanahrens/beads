package formula

import (
	"strings"
	"testing"
)

func TestApplyBranches(t *testing.T) {
	steps := []*Step{
		{ID: "setup", Title: "Setup"},
		{ID: "test", Title: "Run tests"},
		{ID: "lint", Title: "Run linter"},
		{ID: "build", Title: "Build"},
		{ID: "deploy", Title: "Deploy"},
	}

	compose := &ComposeRules{
		Branch: []*BranchRule{
			{
				From:  "setup",
				Steps: []string{"test", "lint", "build"},
				Join:  "deploy",
			},
		},
	}

	result, err := ApplyBranches(steps, compose)
	if err != nil {
		t.Fatalf("ApplyBranches failed: %v", err)
	}

	// Build step map for checking
	stepMap := make(map[string]*Step)
	for _, s := range result {
		stepMap[s.ID] = s
	}

	// Verify branch steps depend on 'from'
	for _, branchStep := range []string{"test", "lint", "build"} {
		s := stepMap[branchStep]
		found := false
		for _, need := range s.Needs {
			if need == "setup" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Step %s should need 'setup', got %v", branchStep, s.Needs)
		}
	}

	// Verify 'join' depends on all branch steps
	deploy := stepMap["deploy"]
	for _, branchStep := range []string{"test", "lint", "build"} {
		found := false
		for _, need := range deploy.Needs {
			if need == branchStep {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("deploy should need %s, got %v", branchStep, deploy.Needs)
		}
	}
}

func TestApplyBranches_Validation(t *testing.T) {
	steps := []*Step{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
	}

	tests := []struct {
		name    string
		branch  *BranchRule
		wantErr string
	}{
		{
			name:    "missing from",
			branch:  &BranchRule{Steps: []string{"a"}, Join: "b"},
			wantErr: "from is required",
		},
		{
			name:    "missing steps",
			branch:  &BranchRule{From: "a", Join: "b"},
			wantErr: "steps is required",
		},
		{
			name:    "missing join",
			branch:  &BranchRule{From: "a", Steps: []string{"b"}},
			wantErr: "join is required",
		},
		{
			name:    "from not found",
			branch:  &BranchRule{From: "notfound", Steps: []string{"a"}, Join: "b"},
			wantErr: "from step \"notfound\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compose := &ComposeRules{Branch: []*BranchRule{tt.branch}}
			_, err := ApplyBranches(steps, compose)
			if err == nil {
				t.Error("Expected error, got nil")
			}
		})
	}
}

func TestApplyGates(t *testing.T) {
	steps := []*Step{
		{ID: "tests", Title: "Run tests"},
		{ID: "deploy", Title: "Deploy to production"},
	}

	compose := &ComposeRules{
		Gate: []*GateRule{
			{
				Before:    "deploy",
				Condition: "tests.status == 'complete'",
			},
		},
	}

	result, err := ApplyGates(steps, compose)
	if err != nil {
		t.Fatalf("ApplyGates failed: %v", err)
	}

	// Find deploy step
	var deploy *Step
	for _, s := range result {
		if s.ID == "deploy" {
			deploy = s
			break
		}
	}

	if deploy == nil {
		t.Fatal("deploy step not found")
	}

	// Check for gate label with JSON format
	found := false
	for _, label := range deploy.Labels {
		// Label format: gate:{"condition":"tests.status == 'complete'"}
		if len(label) > 5 && label[:5] == "gate:" {
			found = true
			if !strings.Contains(label, `"condition"`) || !strings.Contains(label, "tests.status") {
				t.Errorf("Gate label missing expected content: %s", label)
			}
			break
		}
	}

	if !found {
		t.Errorf("deploy should have gate label, got %v", deploy.Labels)
	}
}

func TestApplyGates_InvalidCondition(t *testing.T) {
	steps := []*Step{
		{ID: "deploy", Title: "Deploy"},
	}

	compose := &ComposeRules{
		Gate: []*GateRule{
			{
				Before:    "deploy",
				Condition: "invalid condition syntax ???",
			},
		},
	}

	_, err := ApplyGates(steps, compose)
	if err == nil {
		t.Error("Expected error for invalid condition, got nil")
	}
}

func TestApplyControlFlow_Integration(t *testing.T) {
	// Test the combined ApplyControlFlow function
	steps := []*Step{
		{ID: "setup", Title: "Setup"},
		{ID: "test", Title: "Run tests"},
		{ID: "lint", Title: "Run linter"},
		{ID: "cleanup", Title: "Cleanup"},
	}

	compose := &ComposeRules{
		Branch: []*BranchRule{
			{
				From:  "setup",
				Steps: []string{"test", "lint"},
				Join:  "cleanup",
			},
		},
		Gate: []*GateRule{
			{
				Before:    "cleanup",
				Condition: "steps.complete >= 2",
			},
		},
	}

	result, err := ApplyControlFlow(steps, compose)
	if err != nil {
		t.Fatalf("ApplyControlFlow failed: %v", err)
	}

	// Should have: setup, test, lint, cleanup
	if len(result) != 4 {
		t.Errorf("Expected 4 steps, got %d", len(result))
	}

	stepMap := make(map[string]*Step)
	for _, s := range result {
		stepMap[s.ID] = s
	}

	// Both applications must be present in one pass: branch needs plus gate label.
	for _, branchStep := range []string{"test", "lint"} {
		if !containsString(stepMap[branchStep].Needs, "setup") {
			t.Errorf("%s should need setup, got %v", branchStep, stepMap[branchStep].Needs)
		}
	}
	if !containsString(stepMap["cleanup"].Needs, "test") || !containsString(stepMap["cleanup"].Needs, "lint") {
		t.Errorf("cleanup should need test and lint, got %v", stepMap["cleanup"].Needs)
	}

	hasGate := false
	for _, label := range stepMap["cleanup"].Labels {
		// Label format: gate:{"condition":"steps.complete >= 2"}
		if len(label) > 5 && label[:5] == "gate:" && strings.Contains(label, "steps.complete") {
			hasGate = true
			break
		}
	}
	if !hasGate {
		t.Errorf("cleanup should have gate label, got %v", stepMap["cleanup"].Labels)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestApplyControlFlow_Immutability pins that the combined pass copies the
// input steps before wiring branches and gates onto them.
func TestApplyControlFlow_Immutability(t *testing.T) {
	steps := []*Step{
		{ID: "setup", Title: "Setup"},
		{ID: "test", Title: "Run tests"},
		{ID: "cleanup", Title: "Cleanup"},
	}

	compose := &ComposeRules{
		Branch: []*BranchRule{
			{From: "setup", Steps: []string{"test"}, Join: "cleanup"},
		},
		Gate: []*GateRule{
			{Before: "cleanup", Condition: "test.status == 'complete'"},
		},
	}

	result, err := ApplyControlFlow(steps, compose)
	if err != nil {
		t.Fatalf("ApplyControlFlow failed: %v", err)
	}

	for _, step := range steps {
		if len(step.Needs) != 0 {
			t.Errorf("Original step %q was mutated: Needs = %v (expected nil)", step.ID, step.Needs)
		}
		if len(step.Labels) != 0 {
			t.Errorf("Original step %q was mutated: Labels = %v (expected nil)", step.ID, step.Labels)
		}
	}

	// The returned slice must carry the wiring, so a pass that mutated the
	// input in place would still satisfy the assertions above only if it also
	// returned the input; check the result explicitly.
	var cleanup *Step
	for _, s := range result {
		if s.ID == "cleanup" {
			cleanup = s
			break
		}
	}
	if cleanup == nil {
		t.Fatal("cleanup step not found in result")
	}
	if !containsString(cleanup.Needs, "test") {
		t.Errorf("result cleanup should need test, got %v", cleanup.Needs)
	}
	if len(cleanup.Labels) == 0 {
		t.Error("result cleanup should have a gate label")
	}
}

// gt-v1pcg: Tests for immutability of ApplyBranches and ApplyGates

func TestApplyBranches_Immutability(t *testing.T) {
	// Create steps with no initial dependencies
	steps := []*Step{
		{ID: "setup", Title: "Setup", Needs: nil},
		{ID: "test", Title: "Run tests", Needs: nil},
		{ID: "deploy", Title: "Deploy", Needs: nil},
	}

	compose := &ComposeRules{
		Branch: []*BranchRule{
			{From: "setup", Steps: []string{"test"}, Join: "deploy"},
		},
	}

	// Call ApplyBranches
	result, err := ApplyBranches(steps, compose)
	if err != nil {
		t.Fatalf("ApplyBranches failed: %v", err)
	}

	// Verify original steps are NOT mutated
	for _, step := range steps {
		if len(step.Needs) != 0 {
			t.Errorf("Original step %q was mutated: Needs = %v (expected nil)", step.ID, step.Needs)
		}
	}

	// Verify result has the expected dependencies
	resultMap := make(map[string]*Step)
	for _, s := range result {
		resultMap[s.ID] = s
	}

	if len(resultMap["test"].Needs) != 1 || resultMap["test"].Needs[0] != "setup" {
		t.Errorf("Result test step should need setup, got %v", resultMap["test"].Needs)
	}
}

func TestApplyGates_Immutability(t *testing.T) {
	// Create steps with no initial labels
	steps := []*Step{
		{ID: "tests", Title: "Run tests", Labels: nil},
		{ID: "deploy", Title: "Deploy", Labels: nil},
	}

	compose := &ComposeRules{
		Gate: []*GateRule{
			{Before: "deploy", Condition: "tests.status == 'complete'"},
		},
	}

	// Call ApplyGates
	result, err := ApplyGates(steps, compose)
	if err != nil {
		t.Fatalf("ApplyGates failed: %v", err)
	}

	// Verify original steps are NOT mutated
	for _, step := range steps {
		if len(step.Labels) != 0 {
			t.Errorf("Original step %q was mutated: Labels = %v (expected nil)", step.ID, step.Labels)
		}
	}

	// Verify result has the expected labels
	var deployResult *Step
	for _, s := range result {
		if s.ID == "deploy" {
			deployResult = s
			break
		}
	}

	if deployResult == nil {
		t.Fatal("deploy step not found in result")
	}

	hasGate := false
	for _, label := range deployResult.Labels {
		if len(label) > 5 && label[:5] == "gate:" {
			hasGate = true
			break
		}
	}
	if !hasGate {
		t.Errorf("Result deploy step should have gate label, got %v", deployResult.Labels)
	}
}
