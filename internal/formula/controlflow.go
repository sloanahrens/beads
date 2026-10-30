// Package formula provides control flow operators for step transformation.
//
// Control flow operators enable:
//   - branch: Fork-join parallel execution patterns
//   - gate: Conditional waits before steps proceed
//
// These operators are applied during formula cooking to transform
// the step graph before creating the proto bead.
package formula

import (
	"encoding/json"
	"fmt"
)

// ApplyBranches wires fork-join dependency patterns.
// For each branch rule:
//   - All branch steps depend on the 'from' step
//   - The 'join' step depends on all branch steps
//
// Returns a new steps slice with dependencies added.
// The original steps slice is not modified.
func ApplyBranches(steps []*Step, compose *ComposeRules) ([]*Step, error) {
	if compose == nil || len(compose.Branch) == 0 {
		return steps, nil
	}

	// Clone steps to avoid mutating input
	cloned := cloneStepsRecursive(steps)
	stepMap := buildStepMap(cloned)

	if err := applyBranchesWithMap(stepMap, compose); err != nil {
		return nil, err
	}

	return cloned, nil
}

// applyBranchesWithMap applies branch rules using a pre-built stepMap.
// This is the internal implementation used by both ApplyBranches and ApplyControlFlow.
// The stepMap entries are modified in place.
func applyBranchesWithMap(stepMap map[string]*Step, compose *ComposeRules) error {
	if compose == nil || len(compose.Branch) == 0 {
		return nil
	}

	for _, branch := range compose.Branch {
		// Validate the branch rule
		if branch.From == "" {
			return fmt.Errorf("branch: from is required")
		}
		if len(branch.Steps) == 0 {
			return fmt.Errorf("branch: steps is required")
		}
		if branch.Join == "" {
			return fmt.Errorf("branch: join is required")
		}

		// Verify all steps exist
		if _, ok := stepMap[branch.From]; !ok {
			return fmt.Errorf("branch: from step %q not found", branch.From)
		}
		if _, ok := stepMap[branch.Join]; !ok {
			return fmt.Errorf("branch: join step %q not found", branch.Join)
		}
		for _, stepID := range branch.Steps {
			if _, ok := stepMap[stepID]; !ok {
				return fmt.Errorf("branch: parallel step %q not found", stepID)
			}
		}

		// Add dependencies: branch steps depend on 'from'
		for _, stepID := range branch.Steps {
			step := stepMap[stepID]
			step.Needs = appendUnique(step.Needs, branch.From)
		}

		// Add dependencies: 'join' depends on all branch steps
		joinStep := stepMap[branch.Join]
		for _, stepID := range branch.Steps {
			joinStep.Needs = appendUnique(joinStep.Needs, stepID)
		}
	}

	return nil
}

// ApplyGates adds gate conditions to steps.
// For each gate rule:
//   - The target step gets a "gate:condition" label
//   - At runtime, the patrol executor evaluates the condition
//
// Returns a new steps slice with gate labels added.
// The original steps slice is not modified.
func ApplyGates(steps []*Step, compose *ComposeRules) ([]*Step, error) {
	if compose == nil || len(compose.Gate) == 0 {
		return steps, nil
	}

	// Clone steps to avoid mutating input
	cloned := cloneStepsRecursive(steps)
	stepMap := buildStepMap(cloned)

	if err := applyGatesWithMap(stepMap, compose); err != nil {
		return nil, err
	}

	return cloned, nil
}

// applyGatesWithMap applies gate rules using a pre-built stepMap.
// This is the internal implementation used by both ApplyGates and ApplyControlFlow.
// The stepMap entries are modified in place.
func applyGatesWithMap(stepMap map[string]*Step, compose *ComposeRules) error {
	if compose == nil || len(compose.Gate) == 0 {
		return nil
	}

	for _, gate := range compose.Gate {
		// Validate the gate rule
		if gate.Before == "" {
			return fmt.Errorf("gate: before is required")
		}
		if gate.Condition == "" {
			return fmt.Errorf("gate: condition is required")
		}

		// Validate the condition syntax
		_, err := ParseCondition(gate.Condition)
		if err != nil {
			return fmt.Errorf("gate: invalid condition %q: %w", gate.Condition, err)
		}

		// Find the target step
		step, ok := stepMap[gate.Before]
		if !ok {
			return fmt.Errorf("gate: target step %q not found", gate.Before)
		}

		// Add gate label for runtime evaluation using JSON for unambiguous parsing
		gateMeta := map[string]string{"condition": gate.Condition}
		gateJSON, _ := json.Marshal(gateMeta)
		gateLabel := fmt.Sprintf("gate:%s", string(gateJSON))
		step.Labels = appendUnique(step.Labels, gateLabel)
	}

	return nil
}

// ApplyControlFlow applies the control flow operators in the correct order:
// 1. Branches (wire fork-join dependencies)
// 2. Gates (add condition labels)
//
// Returns a new steps slice. The original steps slice is not modified.
func ApplyControlFlow(steps []*Step, compose *ComposeRules) ([]*Step, error) {
	// Clone once: branches and gates below mutate stepMap entries in place.
	cloned := cloneStepsRecursive(steps)
	stepMap := buildStepMap(cloned)

	// Apply branches (wires dependencies)
	if err := applyBranchesWithMap(stepMap, compose); err != nil {
		return nil, fmt.Errorf("applying branches: %w", err)
	}

	// Apply gates (adds labels)
	if err := applyGatesWithMap(stepMap, compose); err != nil {
		return nil, fmt.Errorf("applying gates: %w", err)
	}

	return cloned, nil
}

// cloneStepDeep creates a deep copy of a step including children.
func cloneStepDeep(s *Step) *Step {
	clone := cloneStep(s)

	if len(s.Children) > 0 {
		clone.Children = make([]*Step, len(s.Children))
		for i, child := range s.Children {
			clone.Children[i] = cloneStepDeep(child)
		}
	}

	return clone
}

// cloneStepsRecursive creates a deep copy of a slice of steps.
func cloneStepsRecursive(steps []*Step) []*Step {
	result := make([]*Step, len(steps))
	for i, step := range steps {
		result[i] = cloneStepDeep(step)
	}
	return result
}
