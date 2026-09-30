package formula

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLintFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLintPath_Directory(t *testing.T) {
	dir := t.TempDir()
	writeLintFile(t, dir, "base.formula.toml", "formula = \"base\"\nversion = 1\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n")
	writeLintFile(t, dir, "child.formula.toml", "formula = \"child\"\nversion = 1\nextends = [\"base\"]\n[[steps]]\nid = \"b\"\ntitle = \"B\"\nneeds = [\"a\"]\n")
	writeLintFile(t, dir, "dropped.formula.toml", "formula = \"dropped\"\nversion = 1\npresets = 1\n[[steps]]\nid = \"a\"\ntitle = \"A\"\ngate = { type = \"conditional\", condition = \"x\" }\n")
	writeLintFile(t, dir, "conflict.formula.toml", "formula = \"conflict\"\nversion = 1\n[vars.period]\nrequired = true\ndefault = \"day\"\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n")
	writeLintFile(t, dir, "broken.formula.toml", "formula = \n")
	writeLintFile(t, dir, "notes.toml", "ignored = true\n")

	rep, err := LintPath(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked != 5 || rep.Failed != 3 {
		t.Fatalf("checked=%d failed=%d, want 5/3: %+v", rep.Checked, rep.Failed, rep)
	}
	byName := map[string][]Problem{}
	for _, f := range rep.Files {
		byName[filepath.Base(f.File)] = f.Problems
	}
	if len(byName["base.formula.toml"]) != 0 || len(byName["child.formula.toml"]) != 0 {
		t.Fatalf("clean files flagged: %+v", byName)
	}
	want := map[string]int{"presets": 3, "steps.gate.condition": 7, "steps.gate.type": 7}
	for _, p := range byName["dropped.formula.toml"] {
		if want[p.Key] != p.Line {
			t.Errorf("dropped: %+v", p)
		}
		delete(want, p.Key)
	}
	if len(want) != 0 {
		t.Errorf("dropped: missing %v", want)
	}
	c := byName["conflict.formula.toml"]
	if len(c) != 1 || c[0].Kind != ProblemValidation || c[0].Key != "vars.period" {
		t.Errorf("conflict: %+v", c)
	}
	b := byName["broken.formula.toml"]
	if len(b) != 1 || b[0].Kind != ProblemSyntax || b[0].Line != 1 {
		t.Errorf("broken: %+v", b)
	}
	// Files are reported in sorted order.
	for i := 1; i < len(rep.Files); i++ {
		if rep.Files[i-1].File > rep.Files[i].File {
			t.Fatalf("files not sorted: %v, %v", rep.Files[i-1].File, rep.Files[i].File)
		}
	}
}

func TestLintPath_SingleFile(t *testing.T) {
	dir := t.TempDir()
	p := writeLintFile(t, dir, "x.formula.toml", "formula = \"x\"\nversion = 1\n[[steps]]\nid = \"a\"\ntitle = \"A\"\nacceptance = \"ok\"\n")
	rep, err := LintPath(p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked != 1 || rep.Failed != 1 || rep.Files[0].Problems[0].Key != "steps.acceptance" {
		t.Fatalf("%+v", rep)
	}
}
