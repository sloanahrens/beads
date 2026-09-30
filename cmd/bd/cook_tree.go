package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/steveyegge/beads/internal/formula"
)

// cookTree is what `bd cook <formula>` returns under machine mode: the step
// tree bd would pour, after extends, expansions, aspects/advice, overlays
// and step conditions, with vars resolved. Gastown renders its checklist
// from it, so the checklist and the beads come from one engine (D6).
//
// Ordering is deterministic: steps in cook order (the order pour creates
// them), children nested, vars sorted by name.
type cookTree struct {
	Formula     string `json:"formula"`
	Source      string `json:"source"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Phase       string `json:"phase"`
	// Mode is "runtime" (defaults and --var substituted) or "compile"
	// (--mode=compile: {{placeholders}} kept).
	Mode           string          `json:"mode"`
	Vars           []cookTreeVar   `json:"vars"`
	UnresolvedVars []string        `json:"unresolved_vars"`
	Overlay        *cookTreeSource `json:"overlay"`
	Warnings       []string        `json:"warnings"`
	Steps          []cookTreeStep  `json:"steps"`
}

type cookTreeSource struct {
	Path string `json:"path"`
}

type cookTreeVar struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     *string  `json:"default"`
	Enum        []string `json:"enum"`
	Pattern     string   `json:"pattern"`
	Type        string   `json:"type"`
	// Value is what was substituted: the --var value, else the default;
	// null when the var has neither (runtime) or in compile mode.
	Value    *string `json:"value"`
	Provided bool    `json:"provided"`
}

type cookTreeStep struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Notes       string `json:"notes"`
	// Type is the issue type pour gives the step (task by default, epic for
	// an undeclared parent), not the raw formula field.
	Type     string   `json:"type"`
	Priority int      `json:"priority"`
	Assignee string   `json:"assignee"`
	Labels   []string `json:"labels"`
	// Needs is depends_on followed by needs, deduplicated: every step this
	// one is blocked on.
	Needs          []string       `json:"needs"`
	WaitsFor       string         `json:"waits_for"`
	Gate           *formula.Gate  `json:"gate"`
	Metadata       map[string]any `json:"metadata"`
	SourceFormula  string         `json:"source_formula"`
	SourceLocation string         `json:"source_location"`
	Children       []cookTreeStep `json:"children"`
}

// cookTreeFor loads and cooks a formula into its tree. vars are the --var
// values; compile keeps placeholders; strictRuntime (explicit
// --mode=runtime) makes a var left without a value an error.
func cookTreeFor(nameOrPath string, searchPaths []string, vars map[string]string, compile, strictRuntime bool, overlayDir string) (*cookTree, error) {
	if vars == nil {
		vars = map[string]string{}
	}
	parser := formula.NewParser(searchPaths...)
	parser.Strict = true // the gastown renderer path is strict now (D6)
	f, err := loadFormulaByNameOrPath(parser, nameOrPath)
	if err != nil {
		return nil, err
	}
	resolved, cooked, err := cookPipeline(parser, f, vars, overlayDir)
	if err != nil {
		return nil, err
	}

	tree := &cookTree{
		Formula:        resolved.Formula,
		Source:         resolved.Source,
		Type:           string(resolved.Type),
		Phase:          resolved.Phase,
		Mode:           "runtime",
		Vars:           []cookTreeVar{},
		UnresolvedVars: []string{},
		Warnings:       cooked.warnings,
	}
	if tree.Warnings == nil {
		tree.Warnings = []string{}
	}
	if cooked.overlay != nil {
		tree.Overlay = &cookTreeSource{Path: cooked.overlay.Path}
	}

	values := formula.ApplyDefaults(resolved, vars)
	if compile {
		tree.Mode = "compile"
	} else {
		substituteFormulaVars(resolved, values)
		for _, name := range formula.ExtractVariables(resolved) {
			if _, ok := values[name]; !ok {
				tree.UnresolvedVars = append(tree.UnresolvedVars, name)
			}
		}
		sort.Strings(tree.UnresolvedVars)
		if strictRuntime && len(tree.UnresolvedVars) > 0 {
			return nil, fmt.Errorf("%w: runtime mode requires every variable to have a value; missing: %s",
				formula.ErrVarValidation, strings.Join(tree.UnresolvedVars, ", "))
		}
	}
	tree.Description = resolved.Description

	names := make([]string, 0, len(resolved.Vars))
	for name := range resolved.Vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := resolved.Vars[name]
		v := cookTreeVar{
			Name: name, Description: def.Description, Required: def.Required, Default: def.Default,
			Enum: def.Enum, Pattern: def.Pattern, Type: def.Type,
		}
		if v.Enum == nil {
			v.Enum = []string{}
		}
		_, v.Provided = vars[name]
		if val, ok := values[name]; ok && !compile {
			v.Value = &val
		}
		tree.Vars = append(tree.Vars, v)
	}

	tree.Steps = cookTreeSteps(resolved.Steps)
	return tree, nil
}

func cookTreeSteps(steps []*formula.Step) []cookTreeStep {
	out := make([]cookTreeStep, 0, len(steps))
	for _, st := range steps {
		issueType := stepTypeToIssueType(st.Type)
		if len(st.Children) > 0 && strings.TrimSpace(st.Type) == "" {
			issueType = "epic"
		}
		priority := 2
		if st.Priority != nil {
			priority = *st.Priority
		}
		node := cookTreeStep{
			ID:             st.ID,
			Title:          st.Title,
			Description:    st.Description,
			Notes:          st.Notes,
			Type:           string(issueType),
			Priority:       priority,
			Assignee:       st.Assignee,
			Labels:         append([]string{}, st.Labels...),
			Needs:          []string{},
			WaitsFor:       st.WaitsFor,
			Gate:           st.Gate,
			Metadata:       st.Metadata,
			SourceFormula:  st.SourceFormula,
			SourceLocation: st.SourceLocation,
			Children:       cookTreeSteps(st.Children),
		}
		seen := map[string]bool{}
		for _, id := range append(append([]string{}, st.DependsOn...), st.Needs...) {
			if !seen[id] {
				seen[id] = true
				node.Needs = append(node.Needs, id)
			}
		}
		if node.Metadata == nil {
			node.Metadata = map[string]any{}
		}
		out = append(out, node)
	}
	return out
}

// runCookTree is bd cook under machine mode (without --persist/--dry-run).
func runCookTree(flags *cookFlags) error {
	mode := "runtime"
	if flags.explicitMode != "" {
		mode = flags.explicitMode
	}
	tree, err := cookTreeFor(flags.formulaPath, flags.searchPaths, flags.inputVars,
		mode == "compile", flags.explicitMode == "runtime", formulaOverlayDir())
	if err != nil {
		return reportFormulaError(err)
	}
	return outputJSON(tree)
}
