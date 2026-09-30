package formula

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Overlay modes, the same three gastown's overlay files use.
const (
	OverlayReplace = "replace" // swap the step description
	OverlayAppend  = "append"  // add text after the step description
	OverlaySkip    = "skip"    // remove the step; dependents inherit its predecessors
)

// StepOverride is one [[step-overrides]] entry of an overlay file.
type StepOverride struct {
	StepID      string `toml:"step_id" json:"step_id"`
	Mode        string `toml:"mode" json:"mode"`
	Description string `toml:"description" json:"description,omitempty"`
}

// Overlay is a per-formula override file, <overlay-dir>/<formula>.toml.
// It is the file format gastown's formula-overlays directories already use;
// bd reads it from ONE declared directory (config formula.overlay-dir), so
// the beads it pours and the checklist rendered from bd cook agree.
type Overlay struct {
	Path          string         `toml:"-" json:"path"`
	StepOverrides []StepOverride `toml:"step-overrides" json:"step_overrides"`
}

// LoadOverlay reads <dir>/<formulaName>.toml. An empty dir or a missing file
// is no overlay (nil, nil). Decoding is strict like formulas: an unknown key,
// a missing step_id or an unknown mode is a *FormulaError.
func LoadOverlay(dir, formulaName string) (*Overlay, error) {
	if dir == "" {
		return nil, nil
	}
	path := filepath.Join(dir, formulaName+".toml")
	// #nosec G304 -- path is inside the configured overlay directory
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading overlay %s: %w", path, err)
	}
	var ov Overlay
	md, err := toml.Decode(string(data), &ov)
	if err != nil {
		return nil, &FormulaError{File: path, Problems: []Problem{{Kind: ProblemSyntax, Message: err.Error()}}}
	}
	lines := newLineCursor(locateKeyLines(data))
	var problems []Problem
	for _, k := range md.Undecoded() {
		key := strings.Join(k, ".")
		problems = append(problems, Problem{Kind: ProblemUnknownKey, Key: key, Line: lines.next(key),
			Message: "unknown overlay key"})
	}
	for i, so := range ov.StepOverrides {
		line := lines.next("step-overrides")
		if so.StepID == "" {
			problems = append(problems, Problem{Kind: ProblemValidation, Key: "step-overrides.step_id", Line: line,
				Message: fmt.Sprintf("step-overrides[%d]: step_id is required", i)})
		}
		switch so.Mode {
		case OverlayReplace, OverlayAppend, OverlaySkip:
		default:
			problems = append(problems, Problem{Kind: ProblemValidation, Key: "step-overrides.mode", Line: line,
				Message: fmt.Sprintf("step-overrides[%d] (step_id=%q): invalid mode %q (must be replace, append, or skip)", i, so.StepID, so.Mode)})
		}
	}
	if len(problems) > 0 {
		return nil, &FormulaError{File: path, Problems: problems}
	}
	ov.Path = path
	return &ov, nil
}

// ApplyOverlay applies the overrides to the formula's top-level steps in
// file order, exactly as gastown did, and returns a warning for each
// override naming a step the formula does not have (a stale override).
// A skipped step's dependents inherit its predecessors (needs and
// depends_on together) in place of the edge to it, so no step is left blocked on a step that was removed.
func ApplyOverlay(f *Formula, ov *Overlay) []string {
	if ov == nil {
		return nil
	}
	var warnings []string
	for _, so := range ov.StepOverrides {
		idx := -1
		for i, st := range f.Steps {
			if st.ID == so.StepID {
				idx = i
				break
			}
		}
		if idx < 0 {
			warnings = append(warnings, fmt.Sprintf("overlay references unknown step %q (stale override)", so.StepID))
			continue
		}
		st := f.Steps[idx]
		switch so.Mode {
		case OverlayReplace:
			st.Description = so.Description
		case OverlayAppend:
			st.Description += "\n" + so.Description
		case OverlaySkip:
			f.Steps = append(f.Steps[:idx:idx], f.Steps[idx+1:]...)
			preds := append(append([]string{}, st.DependsOn...), st.Needs...)
			for _, other := range f.Steps {
				other.Needs = replaceEdge(other.Needs, st.ID, preds)
				other.DependsOn = replaceEdge(other.DependsOn, st.ID, preds)
			}
		}
	}
	return warnings
}

// replaceEdge swaps every occurrence of removed in edges for its
// predecessors, dropping duplicates while keeping order.
func replaceEdge(edges []string, removed string, predecessors []string) []string {
	found := false
	for _, e := range edges {
		if e == removed {
			found = true
			break
		}
	}
	if !found {
		return edges
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(edges)+len(predecessors))
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, e := range edges {
		if e == removed {
			for _, p := range predecessors {
				add(p)
			}
			continue
		}
		add(e)
	}
	return out
}
