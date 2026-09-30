package formula

import (
	"errors"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
)

// ErrInvalidFormula marks a formula that exists but cannot be cooked: a key
// bd does not know (it would otherwise be silently dropped), an invalid gate
// type, or a structural validation failure. Callers match it with errors.Is
// and report it instead of falling through to another resolution path.
var ErrInvalidFormula = errors.New("invalid formula")

// ErrFormulaNotFound marks a formula name no search path holds.
var ErrFormulaNotFound = errors.New("formula not found")

// Problem kinds reported by strict decode, validation and bd formula lint.
const (
	ProblemUnknownKey      = "unknown_key"
	ProblemInvalidGateType = "invalid_gate_type"
	ProblemInvalidValue    = "invalid_value"
	ProblemValidation      = "validation"
)

// Problem is one reason a formula file cannot be cooked.
type Problem struct {
	Kind    string `json:"kind"`
	Key     string `json:"key,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

func (p Problem) String() string {
	var b strings.Builder
	if p.Line > 0 {
		fmt.Fprintf(&b, "line %d: ", p.Line)
	}
	if p.Key != "" {
		fmt.Fprintf(&b, "%s: ", p.Key)
	}
	b.WriteString(p.Message)
	return b.String()
}

// FormulaError is a formula that cannot be cooked. Its message is one line.
type FormulaError struct {
	File     string
	Problems []Problem

	// decoded is the loosely decoded formula (what bd cooked before strict
	// decode), kept so a non-strict parser can warn and still cook it.
	decoded *Formula
}

// maxProblemsInMessage bounds the one-line message; bd formula lint lists all.
const maxProblemsInMessage = 5

func (e *FormulaError) Error() string {
	var b strings.Builder
	b.WriteString("invalid formula")
	if e.File != "" {
		b.WriteString(" ")
		b.WriteString(e.File)
	}
	b.WriteString(": ")
	for i, p := range e.Problems {
		if i == maxProblemsInMessage {
			fmt.Fprintf(&b, "; and %d more (run bd formula lint)", len(e.Problems)-i)
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(strings.ReplaceAll(p.String(), "\n", " "))
	}
	return b.String()
}

func (e *FormulaError) Unwrap() error { return ErrInvalidFormula }

// varDefFields are the keys a [vars.<name>] table may carry, with the TOML
// type VarDef.UnmarshalTOML accepts for each. A value of another type is
// dropped by that unmarshaler, so strict decode reports it.
var varDefFields = map[string]string{
	"description": "String",
	"default":     "String",
	"required":    "Bool",
	"enum":        "Array",
	"pattern":     "String",
	"type":        "String",
}

// DecodeTOMLStrict decodes a formula and reports every key the Formula
// struct would silently drop, every var-table key VarDef ignores, and every
// gate whose type no watcher or command resolves. err is set only when the
// document is not valid TOML at all.
func DecodeTOMLStrict(data []byte) (*Formula, []Problem, error) {
	var f Formula
	md, err := toml.Decode(string(data), &f)
	if err != nil {
		return nil, nil, fmt.Errorf("toml: %w", err)
	}
	lines := newLineCursor(locateKeyLines(data))
	var problems []Problem

	// Unknown keys. A key whose parent table is itself unknown is folded
	// into the parent: "[squash]" is one problem, not one per entry.
	undecoded := map[string]bool{}
	for _, k := range md.Undecoded() {
		path := strings.Join(k, ".")
		if hasUndecodedAncestor(k, undecoded) {
			lines.next(path) // consume the occurrence
			continue
		}
		undecoded[path] = true
		problems = append(problems, Problem{
			Kind:    ProblemUnknownKey,
			Key:     path,
			Line:    lines.next(path),
			Message: "unknown key (bd would silently drop it)",
		})
	}

	// Var tables: VarDef has its own unmarshaler, which toml counts as
	// decoding every key under it.
	for _, k := range md.Keys() {
		if len(k) != 3 || k[0] != "vars" {
			continue
		}
		path := strings.Join(k, ".")
		want, known := varDefFields[k[2]]
		switch {
		case !known:
			problems = append(problems, Problem{Kind: ProblemUnknownKey, Key: path, Line: lines.next(path),
				Message: "unknown var field (bd would silently drop it)"})
		case md.Type(k...) != want:
			problems = append(problems, Problem{Kind: ProblemInvalidValue, Key: path, Line: lines.next(path),
				Message: fmt.Sprintf("must be a %s, got %s (bd would silently drop it)", strings.ToLower(want), strings.ToLower(md.Type(k...)))})
		}
	}

	checkGates(f.Steps, "steps", lines, &problems)
	checkGates(f.Template, "template", lines, &problems)

	if f.Version == 0 {
		f.Version = 1
	}
	if f.Type == "" {
		f.Type = TypeWorkflow
	}
	return &f, problems, nil
}

func hasUndecodedAncestor(k toml.Key, undecoded map[string]bool) bool {
	for n := 1; n < len(k); n++ {
		if undecoded[strings.Join(k[:n], ".")] {
			return true
		}
	}
	return false
}

// validGateType reports whether a gate type is one bd can resolve: the
// GitHub watchers (gh:run, gh:pr and their qualified forms), timer and bead
// watchers, and the manually closed human and mail gates. A {{var}} type is
// substituted at pour time and checked there.
func validGateType(t string) bool {
	switch {
	case strings.Contains(t, "{{"):
		return true
	case t == "gh:run", t == "gh:pr", strings.HasPrefix(t, "gh:run:"), strings.HasPrefix(t, "gh:pr:"):
		return true
	case t == "timer", t == "bead", t == "human", t == "mail":
		return true
	}
	return false
}

func checkGates(steps []*Step, path string, lines *lineCursor, problems *[]Problem) {
	for _, st := range steps {
		if st.Gate != nil {
			gateLine := lines.next(path + ".gate")
			typeLine := lines.nextWithin(path+".gate.type", gateLine, lines.peek(path+".gate"))
			if !validGateType(st.Gate.Type) {
				line := typeLine
				if line == 0 {
					line = gateLine
				}
				*problems = append(*problems, Problem{
					Kind: ProblemInvalidGateType,
					Key:  path + ".gate.type",
					Line: line,
					Message: fmt.Sprintf("step %q: gate type %q is not one bd resolves (gh:run, gh:pr, timer, bead, human, mail)",
						st.ID, st.Gate.Type),
				})
			}
		}
		checkGates(st.Children, path+".children", lines, problems)
	}
}

// lineCursor hands out the located lines of each key path in document order.
type lineCursor struct {
	lines map[string][]int
	used  map[string]int
}

func newLineCursor(lines map[string][]int) *lineCursor {
	return &lineCursor{lines: lines, used: map[string]int{}}
}

// next returns the next unused line for path, or 0 when none is left.
func (c *lineCursor) next(path string) int {
	ls := c.lines[path]
	n := c.used[path]
	if n >= len(ls) {
		return 0
	}
	c.used[path] = n + 1
	return ls[n]
}

// peek returns the next unused line for path without consuming it, or 0.
func (c *lineCursor) peek(path string) int {
	ls := c.lines[path]
	if n := c.used[path]; n < len(ls) {
		return ls[n]
	}
	return 0
}

// nextWithin consumes and returns the next line for path that lies at or
// after lo and, when hi > 0, before hi; 0 when there is none. A gate's type
// is optional, so its line is looked up only between this gate and the next.
func (c *lineCursor) nextWithin(path string, lo, hi int) int {
	ls := c.lines[path]
	for n := c.used[path]; n < len(ls); n++ {
		if ls[n] < lo {
			continue
		}
		if hi > 0 && ls[n] >= hi {
			return 0
		}
		c.used[path] = n + 1
		return ls[n]
	}
	return 0
}
