package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// TestFormulaPrimitiveExamples is the smoke harness for
// examples/formulas/primitives/. Each entry parses a fixture, cooks it
// through the full transformation pipeline, and asserts the primitive's
// observable effect on the resulting subgraph.
//
// A new fixture added to the directory without a registered case here is
// a deliberate failure (subTestForUnregisteredFixtures): the harness
// exists to prove primitives are wired, not that fixtures parse.
func TestFormulaPrimitiveExamples(t *testing.T) {
	primDir := primitivesDir(t)

	cases := []struct {
		name   string
		assert func(t *testing.T, sg *TemplateSubgraph)
	}{
		{
			name: "children-epic",
			assert: func(t *testing.T, sg *TemplateSubgraph) {
				// Children get the parent step's ID prefixed to their own.
				for _, want := range []string{
					"children-epic.ship-feature",
					"children-epic.ship-feature.draft",
					"children-epic.ship-feature.polish",
				} {
					if !hasIssueID(sg, want) {
						t.Errorf("missing issue %q; have %v", want, allIssueIDs(sg))
					}
				}
				if !hasBlockingDep(sg, "children-epic.ship-feature.polish", "children-epic.ship-feature.draft") {
					t.Errorf("polish must block on draft; deps: %v", allDeps(sg))
				}
			},
		},
		{
			name: "gate-timer",
			assert: func(t *testing.T, sg *TemplateSubgraph) {
				gate := findGate(sg)
				if gate == nil {
					t.Fatalf("no gate-typed sibling issue in subgraph; issues: %v", allIssueIDs(sg))
				}
				if gate.AwaitType != "timer" {
					t.Errorf("gate.AwaitType = %q, want %q", gate.AwaitType, "timer")
				}
				if !hasBlockingDep(sg, "gate-timer.verify", gate.ID) {
					t.Errorf("verify must block on gate %s; deps: %v", gate.ID, allDeps(sg))
				}
			},
		},
	}

	registered := map[string]bool{}
	for _, c := range cases {
		registered[c.name] = true

		t.Run(c.name, func(t *testing.T) {
			sg, err := resolveAndCookFormulaWithVars(c.name, []string{primDir}, nil)
			if err != nil {
				t.Fatalf("cook %q: %v", c.name, err)
			}
			c.assert(t, sg)
		})
	}

	t.Run("every_fixture_has_a_registered_case", func(t *testing.T) {
		entries, err := os.ReadDir(primDir)
		if err != nil {
			t.Fatalf("readdir %s: %v", primDir, err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".formula.toml") && !strings.HasSuffix(e.Name(), ".formula.json") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".formula.toml")
			name = strings.TrimSuffix(name, ".formula.json")
			if !registered[name] {
				t.Errorf("fixture %q has no smoke assertion in cases[]; add one or delete the fixture", e.Name())
			}
		}
	})
}

func primitivesDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "examples", "formulas", "primitives"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("primitives dir missing: %v", err)
	}
	return dir
}

func hasIssueID(sg *TemplateSubgraph, id string) bool {
	for _, i := range sg.Issues {
		if i.ID == id {
			return true
		}
	}
	return false
}

func hasBlockingDep(sg *TemplateSubgraph, from, to string) bool {
	for _, d := range sg.Dependencies {
		if d.IssueID == from && d.DependsOnID == to && d.Type == "blocks" {
			return true
		}
	}
	return false
}

func findGate(sg *TemplateSubgraph) *types.Issue {
	for _, i := range sg.Issues {
		if i.IssueType == "gate" {
			return i
		}
	}
	return nil
}

func childIssueIDs(sg *TemplateSubgraph, rootID string) []string {
	var out []string
	for _, i := range sg.Issues {
		if i.ID != rootID {
			out = append(out, i.ID)
		}
	}
	return out
}

func allIssueIDs(sg *TemplateSubgraph) []string {
	out := make([]string, 0, len(sg.Issues))
	for _, i := range sg.Issues {
		out = append(out, i.ID)
	}
	return out
}

func allDeps(sg *TemplateSubgraph) []string {
	out := make([]string, 0, len(sg.Dependencies))
	for _, d := range sg.Dependencies {
		out = append(out, d.IssueID+" -> "+d.DependsOnID+" ("+string(d.Type)+")")
	}
	return out
}
