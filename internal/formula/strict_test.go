package formula

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func problemKeys(ps []Problem) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Key)
	}
	return out
}

func TestDecodeTOMLStrict_Clean(t *testing.T) {
	src := `formula = "mol-ok"
version = 1

[vars.component]
description = "Component"
required = true

[[steps]]
id = "design"
title = "Design {{component}}"
needs = []

[[steps]]
id = "ship"
title = "Ship"
depends_on = ["design"]
gate = { type = "gh:run", await_id = "ci", timeout = "1h" }
`
	f, problems, err := DecodeTOMLStrict([]byte(src))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(problems) != 0 {
		t.Fatalf("clean formula reported problems: %+v", problems)
	}
	if f.Formula != "mol-ok" || len(f.Steps) != 2 {
		t.Fatalf("decoded wrong formula: %+v", f)
	}
}

func TestDecodeTOMLStrict_UnknownKeysWithLines(t *testing.T) {
	src := `formula = "mol-bad"
version = 1

[squash]
trigger = "on_complete"

[[steps]]
id = "a"
title = "A"

[[steps]]
id = "b"
title = "B"
need = ["a"]
description = """
this line looks like = a key but is inside a string
"""

[[steps]]
id = "c"
title = "C"
needs = ["b"]
gate = { type = "conditional", condition = "no_response_1" }
`
	_, problems, err := DecodeTOMLStrict([]byte(src))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]int{
		"squash":               4,
		"steps.need":           14,
		"steps.gate.condition": 23,
		"steps.gate.type":      23,
	}
	got := map[string]int{}
	for _, p := range problems {
		got[p.Key] = p.Line
	}
	for k, line := range want {
		if got[k] != line {
			t.Errorf("key %s: line %d, want %d (all: %+v)", k, got[k], line, problems)
		}
	}
	if _, ok := got["squash.trigger"]; ok {
		t.Errorf("keys under an unknown table must fold into it: %+v", problems)
	}
	for _, p := range problems {
		if p.Key == "steps.gate.type" && p.Kind != ProblemInvalidGateType {
			t.Errorf("gate type problem kind = %q", p.Kind)
		}
		if p.Key == "steps.need" && p.Kind != ProblemUnknownKey {
			t.Errorf("need problem kind = %q", p.Kind)
		}
	}
}

func TestDecodeTOMLStrict_RepeatedKeyEachOccurrence(t *testing.T) {
	src := `formula = "mol-rep"
version = 1

[[steps]]
id = "a"
title = "A"
gate = { type = "human", condition = "x" }

[[steps]]
id = "b"
title = "B"
gate = { type = "human", condition = "y" }
`
	_, problems, err := DecodeTOMLStrict([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var lines []int
	for _, p := range problems {
		if p.Key == "steps.gate.condition" {
			lines = append(lines, p.Line)
		}
	}
	if len(lines) != 2 || lines[0] != 7 || lines[1] != 12 {
		t.Fatalf("want two occurrences at lines 7 and 12, got %v (%+v)", lines, problems)
	}
}

func TestDecodeTOMLStrict_GateTypes(t *testing.T) {
	for _, typ := range []string{"gh:run", "gh:pr", "timer", "bead", "human", "mail", "{{gate_kind}}"} {
		src := "formula = \"g\"\nversion = 1\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n[steps.gate]\ntype = \"" + typ + "\"\n"
		_, problems, err := DecodeTOMLStrict([]byte(src))
		if err != nil || len(problems) != 0 {
			t.Errorf("gate type %q rejected: %v %+v", typ, err, problems)
		}
	}
	for _, typ := range []string{"conditional", "", "approval"} {
		src := "formula = \"g\"\nversion = 1\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n[steps.gate]\ntype = \"" + typ + "\"\n"
		_, problems, _ := DecodeTOMLStrict([]byte(src))
		if len(problems) != 1 || problems[0].Kind != ProblemInvalidGateType || problems[0].Line != 7 {
			t.Errorf("gate type %q: want one invalid_gate_type at line 7, got %+v", typ, problems)
		}
	}
}

func TestDecodeTOMLStrict_ChildAndTemplateGates(t *testing.T) {
	src := `formula = "g"
version = 1
type = "expansion"

[[template]]
id = "{target}.x"
title = "X"
acceptance = "done"
gate = { type = "nope" }

[[steps]]
id = "p"
title = "P"

[[steps.children]]
id = "c"
title = "C"
gate = { type = "nope" }
`
	_, problems, err := DecodeTOMLStrict([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	keys := strings.Join(problemKeys(problems), ",")
	for _, k := range []string{"template.acceptance", "template.gate.type", "steps.children.gate.type"} {
		if !strings.Contains(keys, k) {
			t.Errorf("missing %s in %s", k, keys)
		}
	}
}

func TestDecodeTOMLStrict_VarTableKeys(t *testing.T) {
	src := `formula = "v"
version = 1

[vars]
simple = "x"

[vars.period]
description = "P"
requird = true
default = "day"
`
	_, problems, err := DecodeTOMLStrict([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || problems[0].Key != "vars.period.requird" || problems[0].Line != 9 {
		t.Fatalf("want vars.period.requird at line 9, got %+v", problems)
	}
}

func TestParseFile_StrictErrorIsOneLineAndTyped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mol-bad.formula.toml")
	src := "formula = \"mol-bad\"\nversion = 1\n[[steps]]\nid = \"a\"\ntitle = \"A\"\nneed = [\"x\"]\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewParser(dir).ParseFile(path)
	if err == nil {
		t.Fatal("expected strict decode error")
	}
	if !errors.Is(err, ErrInvalidFormula) {
		t.Fatalf("error is not ErrInvalidFormula: %v", err)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Fatalf("error is not one line: %q", err.Error())
	}
	for _, want := range []string{path, "line 6", "steps.need", "unknown key"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err.Error(), want)
		}
	}
	var fe *FormulaError
	if !errors.As(err, &fe) || fe.File != path || len(fe.Problems) != 1 {
		t.Fatalf("want *FormulaError with file and one problem, got %#v", err)
	}
}

func TestResolve_RequiredWithDefaultIsOneLineTyped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mol-rd.formula.toml")
	src := "formula = \"mol-rd\"\nversion = 1\n[vars.requester]\nrequired = true\ndefault = \"deacon\"\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewParser(dir)
	f, err := p.LoadByName("mol-rd")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	_, err = p.Resolve(f)
	if err == nil || !errors.Is(err, ErrInvalidFormula) {
		t.Fatalf("want ErrInvalidFormula, got %v", err)
	}
	if strings.Contains(err.Error(), "\n") {
		t.Fatalf("not one line: %q", err.Error())
	}
	for _, want := range []string{path, "vars.requester", "required:true and default"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err.Error(), want)
		}
	}
}

func TestLoadByName_NotFoundIsTyped(t *testing.T) {
	_, err := NewParser(t.TempDir()).LoadByName("nope")
	if !errors.Is(err, ErrFormulaNotFound) {
		t.Fatalf("want ErrFormulaNotFound, got %v", err)
	}
}

func TestDecodeTOMLStrict_FoldsUnderAnyUnknownAncestor(t *testing.T) {
	src := "formula = \"c\"\nversion = 1\n[inputs.branch]\ndescription = \"b\"\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n"
	_, problems, err := DecodeTOMLStrict([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || problems[0].Key != "inputs.branch" {
		t.Fatalf("want one problem for inputs.branch, got %+v", problems)
	}
}
