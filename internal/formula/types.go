// Package formula provides parsing and validation for .formula.json files.
//
// Formulas are high-level workflow templates that compile down to proto beads.
// They support:
//   - Variable definitions with defaults and validation
//   - Step definitions that become issue hierarchies
//   - Composition rules for bonding formulas together
//   - Inheritance via extends
//
// Example .formula.json:
//
//	{
//	  "formula": "mol-feature",
//	  "description": "Standard feature workflow",
//	  "version": 1,
//	  "type": "workflow",
//	  "vars": {
//	    "component": {
//	      "description": "Component name",
//	      "required": true
//	    }
//	  },
//	  "steps": [
//	    {"id": "design", "title": "Design {{component}}", "type": "task"},
//	    {"id": "implement", "title": "Implement {{component}}", "depends_on": ["design"]}
//	  ]
//	}
package formula

import (
	"fmt"
	"strings"
)

// FormulaType categorizes formulas by their purpose.
type FormulaType string

const (
	// TypeWorkflow is a standard workflow template (sequence of steps).
	TypeWorkflow FormulaType = "workflow"

	// TypeExpansion is a macro that expands into multiple steps.
	// Used for common patterns like "test + lint + build".
	TypeExpansion FormulaType = "expansion"

	// TypeAspect is a cross-cutting concern that can be applied to other formulas.
	// Examples: add logging steps, add approval gates.
	TypeAspect FormulaType = "aspect"

	// TypeConvoy is a multi-agent workflow that coordinates parallel workers.
	// Examples: code review with multiple reviewers, design review sessions.
	TypeConvoy FormulaType = "convoy"
)

// IsValid checks if the formula type is recognized.
func (t FormulaType) IsValid() bool {
	switch t {
	case TypeWorkflow, TypeExpansion, TypeAspect, TypeConvoy:
		return true
	}
	return false
}

// Formula is the root structure for .formula.json files.
type Formula struct {
	// Formula is the unique identifier/name for this formula.
	// Convention: mol-<name> for molecules, exp-<name> for expansions.
	Formula string `json:"formula"`

	// Description explains what this formula does.
	Description string `json:"description,omitempty"`

	// Version is the schema version (currently 1).
	Version int `json:"version"`

	// Type categorizes the formula: workflow, expansion, or aspect.
	Type FormulaType `json:"type"`

	// Extends is a list of parent formulas to inherit from.
	// The child formula inherits all vars, steps, and compose rules.
	// Child definitions override parent definitions with the same ID.
	Extends []string `json:"extends,omitempty"`

	// Vars defines template variables with defaults and validation.
	Vars map[string]*VarDef `json:"vars,omitempty"`

	// Steps defines the work items to create.
	Steps []*Step `json:"steps,omitempty"`

	// Template defines expansion template steps (for TypeExpansion formulas).
	// Template steps use {target} and {target.description} placeholders
	// that get substituted when the expansion is applied to a target step.
	Template []*Step `json:"template,omitempty"`

	// Compose defines composition/bonding rules.
	Compose *ComposeRules `json:"compose,omitempty"`

	// Advice defines step transformations (before/after/around).
	// Applied during cooking to insert steps around matching targets.
	Advice []*AdviceRule `json:"advice,omitempty"`

	// Pointcuts defines target patterns for aspect formulas.
	// Used with TypeAspect to specify which steps the aspect applies to.
	Pointcuts []*Pointcut `json:"pointcuts,omitempty"`

	// Phase indicates the recommended instantiation phase: "liquid" (pour) or "vapor" (wisp).
	// If "vapor", bd pour will warn and suggest using bd mol wisp instead.
	// Patrol and release workflows should typically use "vapor" since they're operational.
	Phase string `json:"phase,omitempty"`

	// Pour controls whether steps are materialized as individual child issues.
	// If true, each step becomes a DB row with dependency tracking (checkpoint recovery).
	// If false (default), only the root issue is created; steps are read inline at prime time.
	// Reserve pour=true for critical, infrequent work (e.g. releases) where step-level
	// tracking is worth the DB overhead. Patrol formulas should NOT set this.
	Pour bool `json:"pour,omitempty"`

	// Source tracks where this formula was loaded from (set by parser).
	Source string `json:"source,omitempty"`

	// Intent is an optional caller-defined hint about the formula's runtime
	// intent (e.g. "mail_only"). Opaque to bd; consumed by downstream tools.
	Intent string `json:"intent,omitempty" toml:"intent,omitempty"`
}

// VarDef defines a template variable with optional validation.
type VarDef struct {
	// Description explains what this variable is for.
	Description string `json:"description,omitempty"`

	// Default is the value to use if not provided.
	// nil means no default (variable must be provided if referenced).
	// Non-nil (including &"") means the variable has an explicit default.
	Default *string `json:"default,omitempty"`

	// Required indicates the variable must be provided (no default).
	Required bool `json:"required,omitempty"`

	// Enum lists the allowed values (if non-empty).
	Enum []string `json:"enum,omitempty"`

	// Pattern is a regex pattern the value must match.
	Pattern string `json:"pattern,omitempty"`

	// Type is the expected value type: string (default), int, bool.
	Type string `json:"type,omitempty"`
}

// UnmarshalTOML implements toml.Unmarshaler for VarDef.
// This allows vars to be defined as either simple strings or tables:
//
//	[vars]
//	wisp_type = "patrol"           # simple string -> Default = "patrol"
//
//	[vars.component]               # table with full definition
//	description = "Component name"
//	required = true
func (v *VarDef) UnmarshalTOML(data interface{}) error {
	switch val := data.(type) {
	case string:
		// Simple string value becomes the default
		v.Default = &val
		return nil
	case map[string]interface{}:
		// Table format - parse each field
		if desc, ok := val["description"].(string); ok {
			v.Description = desc
		}
		if def, ok := val["default"].(string); ok {
			v.Default = &def
		}
		if req, ok := val["required"].(bool); ok {
			v.Required = req
		}
		if enum, ok := val["enum"].([]interface{}); ok {
			for _, e := range enum {
				if s, ok := e.(string); ok {
					v.Enum = append(v.Enum, s)
				}
			}
		}
		if pattern, ok := val["pattern"].(string); ok {
			v.Pattern = pattern
		}
		if typ, ok := val["type"].(string); ok {
			v.Type = typ
		}
		return nil
	default:
		return fmt.Errorf("type mismatch for formula.VarDef: expected string or table but found %T", data)
	}
}

// Step defines a work item to create when the formula is instantiated.
type Step struct {
	// ID is the unique identifier within this formula.
	// Used for dependency references and bond points.
	ID string `json:"id"`

	// Title is the issue title (supports {{variable}} substitution).
	Title string `json:"title"`

	// Description is the issue description (supports substitution).
	Description string `json:"description,omitempty"`

	// Notes are additional notes for the issue (supports substitution).
	Notes string `json:"notes,omitempty"`

	// Type is the issue type: any built-in type (task by default; bug,
	// feature, epic, chore, decision, spike, story, milestone, ...) or a
	// custom type already registered in types.custom. Unregistered types
	// are flattened to task, with a warning, when the formula is cooked
	// or poured.
	Type string `json:"type,omitempty"`

	// Priority is the issue priority (0-4).
	Priority *int `json:"priority,omitempty"`

	// Labels are applied to the created issue.
	Labels []string `json:"labels,omitempty"`

	// Metadata is carried through to the created issue's Metadata field as
	// JSON. Lets formulas pre-declare keys that downstream tooling can project
	// without a post-pour compose step.
	Metadata map[string]interface{} `json:"metadata,omitempty" toml:"metadata,omitempty"`

	// DependsOn lists step IDs this step blocks on (within the formula).
	DependsOn []string `json:"depends_on,omitempty" toml:"depends_on,omitempty"`

	// Needs is a simpler alias for DependsOn - lists sibling step IDs that must complete first.
	// Either Needs or DependsOn can be used; they are merged during cooking.
	Needs []string `json:"needs,omitempty" toml:"needs,omitempty"`

	// Assignee is the default assignee (supports substitution).
	Assignee string `json:"assignee,omitempty"`

	// Expand references an expansion formula to inline here.
	// When set, this step is replaced by the expansion's template steps.
	// See ApplyInlineExpansions in expand.go for implementation.
	Expand string `json:"expand,omitempty"`

	// ExpandVars are variable overrides for the expansion.
	// Merged with the expansion formula's default vars during inline expansion.
	ExpandVars map[string]string `json:"expand_vars,omitempty" toml:"expand_vars,omitempty"`

	// Condition makes this step optional based on a variable.
	// Format: "{{var}}" (truthy), "!{{var}}" (negated), "{{var}} == value", "{{var}} != value".
	// Evaluated at cook/pour time via FilterStepsByCondition.
	Condition string `json:"condition,omitempty"`

	// Children are nested steps (for creating epic hierarchies).
	Children []*Step `json:"children,omitempty"`

	// Gate defines an async wait condition for this step.
	// When set, bd cook creates a gate issue that blocks this step.
	// Close the gate issue (bd close bd-xxx.gate-stepid) to unblock.
	Gate *Gate `json:"gate,omitempty"`

	// Source tracing fields: track where this step came from.
	// These are set during parsing/transformation and copied to Issues during cooking.

	// SourceFormula is the formula name where this step was defined.
	// For inherited steps, this is the parent formula, not the final composed formula.
	SourceFormula string `json:"-"` // Internal only, not serialized to JSON

	// SourceLocation is the path within the source formula.
	// Format: "steps[0]", "steps[2].children[1]", "advice[0].after"
	SourceLocation string `json:"-"` // Internal only, not serialized to JSON
}

// Gate defines an async wait condition for formula steps.
// When a step has a Gate, bd cook creates a gate issue that blocks the step.
// The gate must be closed (manually or via watchers) to unblock the step.
type Gate struct {
	// Type is the condition type: gh:run, gh:pr, timer, human, mail.
	Type string `json:"type"`

	// ID is the condition identifier (e.g., workflow name for gh:run).
	ID string `json:"id,omitempty"`

	// AwaitID is the runtime condition identifier. This is preferred by
	// formula authors because it maps directly to Issue.AwaitID.
	AwaitID string `json:"await_id,omitempty" toml:"await_id,omitempty"`

	// Timeout is how long to wait before escalation (e.g., "1h", "24h").
	Timeout string `json:"timeout,omitempty"`

	// Repo optionally selects the GitHub repository (OWNER/REPO or
	// HOST/OWNER/REPO) a gh:run or gh:pr gate's condition is checked
	// against. Empty means the current Git repository - the same default
	// as an ad-hoc `bd gate create` gate. Ignored for non-GitHub gate
	// types (human, timer, bead).
	Repo string `json:"repo,omitempty" toml:"repo,omitempty"`
}

// ComposeRules define how formulas can be bonded together.
type ComposeRules struct {
	// BondPoints are named locations where other formulas can attach.
	BondPoints []*BondPoint `json:"bond_points,omitempty" toml:"bond_points,omitempty"`

	// Hooks are automatic attachments triggered by labels or conditions.
	Hooks []*Hook `json:"hooks,omitempty"`

	// Expand applies an expansion template to a single target step.
	// The target step is replaced by the expanded template steps.
	Expand []*ExpandRule `json:"expand,omitempty"`

	// Map applies an expansion template to all steps matching a pattern.
	// Each matching step is replaced by the expanded template steps.
	Map []*MapRule `json:"map,omitempty"`

	// Aspects lists aspect formula names to apply to this formula.
	// Aspects are applied after expansions, adding before/after/around
	// steps to matching targets based on the aspect's advice rules.
	// Example: ["security-audit", "logging"]
	Aspects []string `json:"aspects,omitempty"`
}

// ExpandRule applies an expansion template to a single target step.
type ExpandRule struct {
	// Target is the step ID to expand.
	Target string `json:"target"`

	// With is the name of the expansion formula to apply.
	With string `json:"with"`

	// Vars are variable overrides for the expansion.
	Vars map[string]string `json:"vars,omitempty"`
}

// MapRule applies an expansion template to all matching steps.
type MapRule struct {
	// Select is a glob pattern matching step IDs to expand.
	// Examples: "*.implement", "shiny.*"
	Select string `json:"select"`

	// With is the name of the expansion formula to apply.
	With string `json:"with"`

	// Vars are variable overrides for the expansion.
	Vars map[string]string `json:"vars,omitempty"`
}

// BondPoint is a named attachment site for composition.
type BondPoint struct {
	// ID is the unique identifier for this bond point.
	ID string `json:"id"`

	// Description explains what should be attached here.
	Description string `json:"description,omitempty"`

	// AfterStep is the step ID after which to attach.
	// Mutually exclusive with BeforeStep.
	AfterStep string `json:"after_step,omitempty" toml:"after_step,omitempty"`

	// BeforeStep is the step ID before which to attach.
	// Mutually exclusive with AfterStep.
	BeforeStep string `json:"before_step,omitempty" toml:"before_step,omitempty"`

	// Parallel makes attached steps run in parallel with the anchor step.
	Parallel bool `json:"parallel,omitempty"`
}

// Hook defines automatic formula attachment based on conditions.
type Hook struct {
	// Trigger is what activates this hook.
	// Formats: "label:security", "type:bug", "priority:0-1".
	Trigger string `json:"trigger"`

	// Attach is the formula to attach when triggered.
	Attach string `json:"attach"`

	// At is the bond point to attach at (default: end).
	At string `json:"at,omitempty"`

	// Vars are variable overrides for the attached formula.
	Vars map[string]string `json:"vars,omitempty"`
}

// Pointcut defines a target pattern for advice application.
// Used in aspect formulas to specify which steps the advice applies to.
type Pointcut struct {
	// Glob is a glob pattern to match step IDs.
	// Examples: "*.implement", "shiny.*", "review"
	Glob string `json:"glob,omitempty"`

	// Type matches steps by their type field.
	// Examples: "task", "bug", "epic"
	Type string `json:"type,omitempty"`

	// Label matches steps that have a specific label.
	Label string `json:"label,omitempty"`
}

// AdviceRule defines a step transformation rule.
// Advice operators insert steps before, after, or around matching targets.
type AdviceRule struct {
	// Target is a glob pattern matching step IDs to apply advice to.
	// Examples: "*.implement", "design", "shiny.*"
	Target string `json:"target"`

	// Before inserts a step before the target.
	Before *AdviceStep `json:"before,omitempty"`

	// After inserts a step after the target.
	After *AdviceStep `json:"after,omitempty"`

	// Around wraps the target with before and after steps.
	Around *AroundAdvice `json:"around,omitempty"`
}

// AdviceStep defines a step to insert via advice.
type AdviceStep struct {
	// ID is the step identifier. Supports {step.id} substitution.
	ID string `json:"id"`

	// Title is the step title. Supports {step.id} substitution.
	Title string `json:"title,omitempty"`

	// Description is the step description.
	Description string `json:"description,omitempty"`

	// Type is the issue type (task, bug, etc).
	Type string `json:"type,omitempty"`

	// Args are additional context passed to the step.
	Args map[string]string `json:"args,omitempty"`

	// Output defines expected outputs from this step.
	Output map[string]string `json:"output,omitempty"`
}

// AroundAdvice wraps a target with before and after steps.
type AroundAdvice struct {
	// Before is a list of steps to insert before the target.
	Before []*AdviceStep `json:"before,omitempty"`

	// After is a list of steps to insert after the target.
	After []*AdviceStep `json:"after,omitempty"`
}

// Validate checks the formula for structural errors.
func (f *Formula) Validate() error {
	if errs := f.validationProblems(); len(errs) > 0 {
		return fmt.Errorf("formula validation failed:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// validationProblems lists every structural error, one message each.
func (f *Formula) validationProblems() []string {
	var errs []string

	if f.Formula == "" {
		errs = append(errs, "formula: name is required")
	}

	if f.Version < 1 {
		errs = append(errs, "version: must be >= 1")
	}

	if f.Type != "" && !f.Type.IsValid() {
		errs = append(errs, fmt.Sprintf("type: invalid value %q (must be workflow, expansion, or aspect)", f.Type))
	}

	// Validate variables
	for name, v := range f.Vars {
		if name == "" {
			errs = append(errs, "vars: variable name cannot be empty")
			continue
		}
		if v.Required && v.Default != nil {
			errs = append(errs, fmt.Sprintf("vars.%s: cannot have both required:true and default", name))
		}
	}

	// Validate steps - track where each ID was first defined for better error messages
	stepIDLocations := make(map[string]string) // ID -> location where first defined
	for i, step := range f.Steps {
		prefix := fmt.Sprintf("steps[%d]", i)
		if step.ID == "" {
			errs = append(errs, fmt.Sprintf("%s: id is required", prefix))
			continue
		}
		if firstLoc, exists := stepIDLocations[step.ID]; exists {
			errs = append(errs, fmt.Sprintf("%s: duplicate id %q (first defined at %s)", prefix, step.ID, firstLoc))
		} else {
			stepIDLocations[step.ID] = prefix
		}

		if step.Title == "" && step.Expand == "" {
			errs = append(errs, fmt.Sprintf("%s (%s): title is required (unless using expand)", prefix, step.ID))
		}

		// Validate priority range
		if step.Priority != nil && (*step.Priority < 0 || *step.Priority > 4) {
			errs = append(errs, fmt.Sprintf("%s (%s): priority must be 0-4", prefix, step.ID))
		}

		// Collect child IDs (for dependency validation)
		collectChildIDs(step.Children, stepIDLocations, &errs, prefix)
	}

	// Validate step dependencies reference valid IDs (including children)
	for i, step := range f.Steps {
		for _, dep := range step.DependsOn {
			if _, exists := stepIDLocations[dep]; !exists {
				errs = append(errs, fmt.Sprintf("steps[%d] (%s): depends_on references unknown step %q", i, step.ID, dep))
			}
		}
		// Validate needs field - same validation as depends_on
		for _, need := range step.Needs {
			if _, exists := stepIDLocations[need]; !exists {
				errs = append(errs, fmt.Sprintf("steps[%d] (%s): needs references unknown step %q", i, step.ID, need))
			}
		}
		// Validate children's depends_on and needs recursively
		validateChildDependsOn(step.Children, stepIDLocations, &errs, fmt.Sprintf("steps[%d]", i))
	}

	// Validate compose rules
	if f.Compose != nil {
		for i, bp := range f.Compose.BondPoints {
			if bp.ID == "" {
				errs = append(errs, fmt.Sprintf("compose.bond_points[%d]: id is required", i))
			}
			if bp.AfterStep != "" && bp.BeforeStep != "" {
				errs = append(errs, fmt.Sprintf("compose.bond_points[%d] (%s): cannot have both after_step and before_step", i, bp.ID))
			}
			if bp.AfterStep != "" {
				if _, exists := stepIDLocations[bp.AfterStep]; !exists {
					errs = append(errs, fmt.Sprintf("compose.bond_points[%d] (%s): after_step references unknown step %q", i, bp.ID, bp.AfterStep))
				}
			}
			if bp.BeforeStep != "" {
				if _, exists := stepIDLocations[bp.BeforeStep]; !exists {
					errs = append(errs, fmt.Sprintf("compose.bond_points[%d] (%s): before_step references unknown step %q", i, bp.ID, bp.BeforeStep))
				}
			}
		}

		for i, hook := range f.Compose.Hooks {
			if hook.Trigger == "" {
				errs = append(errs, fmt.Sprintf("compose.hooks[%d]: trigger is required", i))
			}
			if hook.Attach == "" {
				errs = append(errs, fmt.Sprintf("compose.hooks[%d]: attach is required", i))
			}
		}
	}

	return errs
}

// collectChildIDs recursively collects step IDs from children.
// idLocations maps ID -> location where first defined (for better duplicate error messages).
func collectChildIDs(children []*Step, idLocations map[string]string, errs *[]string, prefix string) {
	for i, child := range children {
		childPrefix := fmt.Sprintf("%s.children[%d]", prefix, i)
		if child.ID == "" {
			*errs = append(*errs, fmt.Sprintf("%s: id is required", childPrefix))
			continue
		}
		if firstLoc, exists := idLocations[child.ID]; exists {
			*errs = append(*errs, fmt.Sprintf("%s: duplicate id %q (first defined at %s)", childPrefix, child.ID, firstLoc))
		} else {
			idLocations[child.ID] = childPrefix
		}

		if child.Title == "" && child.Expand == "" {
			*errs = append(*errs, fmt.Sprintf("%s (%s): title is required", childPrefix, child.ID))
		}

		// Validate priority range for children
		if child.Priority != nil && (*child.Priority < 0 || *child.Priority > 4) {
			*errs = append(*errs, fmt.Sprintf("%s (%s): priority must be 0-4", childPrefix, child.ID))
		}

		collectChildIDs(child.Children, idLocations, errs, childPrefix)
	}
}

// validateChildDependsOn recursively validates depends_on and needs references for children.
func validateChildDependsOn(children []*Step, idLocations map[string]string, errs *[]string, prefix string) {
	for i, child := range children {
		childPrefix := fmt.Sprintf("%s.children[%d]", prefix, i)
		for _, dep := range child.DependsOn {
			if _, exists := idLocations[dep]; !exists {
				*errs = append(*errs, fmt.Sprintf("%s (%s): depends_on references unknown step %q", childPrefix, child.ID, dep))
			}
		}
		// Validate needs field
		for _, need := range child.Needs {
			if _, exists := idLocations[need]; !exists {
				*errs = append(*errs, fmt.Sprintf("%s (%s): needs references unknown step %q", childPrefix, child.ID, need))
			}
		}
		validateChildDependsOn(child.Children, idLocations, errs, childPrefix)
	}
}

// GetRequiredVars returns the names of all required variables.
func (f *Formula) GetRequiredVars() []string {
	var required []string
	for name, v := range f.Vars {
		if v.Required {
			required = append(required, name)
		}
	}
	return required
}

// GetStepByID finds a step by its ID (searches recursively).
func (f *Formula) GetStepByID(id string) *Step {
	for _, step := range f.Steps {
		if found := findStepByID(step, id); found != nil {
			return found
		}
	}
	return nil
}

// findStepByID recursively searches for a step by ID.
func findStepByID(step *Step, id string) *Step {
	if step.ID == id {
		return step
	}
	for _, child := range step.Children {
		if found := findStepByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

// StringPtr returns a pointer to s. Useful for constructing VarDef literals.
func StringPtr(s string) *string { return &s }

// GetBondPoint finds a bond point by ID.
func (f *Formula) GetBondPoint(id string) *BondPoint {
	if f.Compose == nil {
		return nil
	}
	for _, bp := range f.Compose.BondPoints {
		if bp.ID == id {
			return bp
		}
	}
	return nil
}
