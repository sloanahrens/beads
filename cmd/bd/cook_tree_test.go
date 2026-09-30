package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/formula"
)

var updateCookGolden = flag.Bool("update-cook-golden", false, "rewrite testdata/cook/tree.golden.json")

const (
	cookFormulaDir = "testdata/cook/formulas"
	cookOverlayDir = "testdata/cook/overlays"
	cookGoldenFile = "testdata/cook/tree.golden.json"
)

// cookAllTestdata cooks every formula under testdata/cook/formulas into a
// name -> tree map, with sources made relative so the golden is portable.
func cookAllTestdata(t *testing.T) []byte {
	t.Helper()
	dir, err := filepath.Abs(cookFormulaDir)
	if err != nil {
		t.Fatal(err)
	}
	overlays, err := filepath.Abs(cookOverlayDir)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*"+formula.FormulaExtTOML))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no formulas under %s: %v", dir, err)
	}
	sort.Strings(paths)
	trees := map[string]*cookTree{}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), formula.FormulaExtTOML)
		tree, err := cookTreeFor(name, []string{dir}, nil, false, false, overlays)
		if err != nil {
			t.Fatalf("cook %s: %v", name, err)
		}
		tree.Source = filepath.Base(tree.Source)
		if tree.Overlay != nil {
			tree.Overlay.Path = filepath.Base(tree.Overlay.Path)
		}
		trees[name] = tree
	}
	out, err := json.MarshalIndent(trees, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(out, '\n')
}

// Every testdata formula cooks, the result matches the golden file, and a
// second cook is byte-identical (deterministic ordering).
func TestCookTree_Golden(t *testing.T) {
	got := cookAllTestdata(t)
	if again := cookAllTestdata(t); !bytes.Equal(got, again) {
		t.Fatal("two cooks of the same formulas differ: output is not deterministic")
	}
	if *updateCookGolden {
		if err := os.WriteFile(cookGoldenFile, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(cookGoldenFile)
	if err != nil {
		t.Fatalf("read golden (run with -update-cook-golden): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("cook tree differs from %s; rerun with -update-cook-golden and review the diff", cookGoldenFile)
	}
}

// issuePaths flattens the tree the way pour names issues: a child's id is
// its parent's path, a dot, and its own id.
func issuePaths(steps []cookTreeStep, prefix string) []string {
	var out []string
	for _, s := range steps {
		p := s.ID
		if prefix != "" {
			p = prefix + "." + s.ID
		}
		out = append(out, p)
		out = append(out, issuePaths(s.Children, p)...)
	}
	return out
}

// The tree carries what pour materializes: the same steps, in the same
// order, as the subgraph cooked for pour and wisp.
func TestCookTree_MatchesPourSubgraph(t *testing.T) {
	dir, _ := filepath.Abs(cookFormulaDir)
	vars := map[string]string{"component": "api"}
	for _, name := range []string{"wf-basic", "wf-child", "wf-expanded", "wf-aspected", "wf-children", "wf-conditional", "exp-draft"} {
		tree, err := cookTreeFor(name, []string{dir}, vars, false, false, "")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		sg, err := resolveAndCookFormulaWithVars(name, []string{dir}, vars)
		if err != nil {
			t.Fatalf("%s pour cook: %v", name, err)
		}
		var pourIDs []string
		for _, iss := range sg.Issues {
			if iss == sg.Root {
				continue
			}
			id := strings.TrimPrefix(iss.ID, sg.Root.ID+".")
			if strings.HasPrefix(id, "gate-") {
				continue // a gate issue renders as its step's gate
			}
			pourIDs = append(pourIDs, id)
		}
		if got, want := strings.Join(issuePaths(tree.Steps, ""), ","), strings.Join(pourIDs, ","); got != want {
			t.Errorf("%s: tree steps %s, pour steps %s", name, got, want)
		}
	}
}

func TestCookTree_VarsAndModes(t *testing.T) {
	dir, _ := filepath.Abs(cookFormulaDir)
	tree, err := cookTreeFor("wf-basic", []string{dir}, map[string]string{"component": "api", "env": "prod"}, false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if tree.Description != "Ship api to prod" || tree.Steps[2].Gate.AwaitID != "ci.yml" || len(tree.UnresolvedVars) != 0 {
		t.Fatalf("runtime substitution: %q %q %v", tree.Description, tree.Steps[2].Gate.AwaitID, tree.UnresolvedVars)
	}
	if got := strings.Join(tree.Steps[1].Needs, ","); got != "design" {
		t.Fatalf("needs = depends_on ∪ needs, deduplicated: got %s", got)
	}

	compiled, err := cookTreeFor("wf-basic", []string{dir}, nil, true, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if compiled.Mode != "compile" || compiled.Steps[0].Title != "Design {{component}}" {
		t.Fatalf("compile mode must keep placeholders: %q", compiled.Steps[0].Title)
	}

	_, err = cookTreeFor("wf-basic", []string{dir}, nil, false, true, "")
	if !errors.Is(err, formula.ErrVarValidation) || !strings.Contains(err.Error(), "component") {
		t.Fatalf("explicit runtime with a missing var: want ErrVarValidation naming it, got %v", err)
	}

	_, err = cookTreeFor("wf-basic", []string{dir}, map[string]string{"component": "api", "env": "qa"}, false, false, "")
	if !errors.Is(err, formula.ErrVarValidation) {
		t.Fatalf("enum violation: want ErrVarValidation, got %v", err)
	}
}

func TestCookTree_ErrorsAreTyped(t *testing.T) {
	withMachineMode(t, true)
	dir := t.TempDir()
	writeFormulaFile(t, dir, "bad", "formula = \"bad\"\nversion = 1\n[[steps]]\nid = \"a\"\ntitle = \"A\"\ngate = { type = \"conditional\", condition = \"x\" }\n")

	for name, want := range map[string]errorKind{"bad": kindInvalidArgs, "missing": kindNotFound} {
		_, err := cookTreeFor(name, []string{dir}, nil, false, false, "")
		var ce *cliError
		if !errors.As(reportFormulaError(err), &ce) || ce.Kind != want {
			t.Errorf("%s: want %s, got %v", name, want, err)
		}
	}
}

// Overlay files are strict too: a typo in an overlay fails the cook.
func TestCookTree_InvalidOverlayFails(t *testing.T) {
	dir, _ := filepath.Abs(cookFormulaDir)
	ov := t.TempDir()
	if err := os.WriteFile(filepath.Join(ov, "wf-parent.toml"), []byte("[[step-overrides]]\nstep_id = \"plan\"\nmode = \"apend\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := cookTreeFor("wf-parent", []string{dir}, nil, false, false, ov); !errors.Is(err, formula.ErrInvalidFormula) {
		t.Fatalf("want ErrInvalidFormula, got %v", err)
	}
}

// Gastown discovers the D6 verbs through bd capabilities --json.
func TestCapabilities_ListsCookAndFormulaLint(t *testing.T) {
	report := buildCapabilities(rootCmd)
	found := map[string]bool{}
	for _, c := range report.Commands {
		found[c.Path] = true
	}
	for _, want := range []string{"cook", "formula lint"} {
		if !found[want] {
			t.Errorf("bd capabilities does not list %q", want)
		}
	}
}
