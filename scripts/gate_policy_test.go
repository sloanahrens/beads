package scripts_test

import (
	"regexp"
	"strings"
	"testing"
)

// stageInvocation matches a sub-make call (`$(MAKE) target`) inside a recipe.
// The gate's stages are read through this instead of by grepping the target
// name, so the test tracks the command a stage runs rather than the target's
// name appearing anywhere in its recipe (the gate recipe names gate-lint and
// gate-test in its ignore-errors guard text too).
var stageInvocation = regexp.MustCompile(`\$(?:\(|\{)MAKE(?:\)|\})\s+([A-Za-z0-9_.-]+)`)

// TestLandingGatePolicy pins the wiring of `make gate`, the gate the landing
// worker runs before a polecat branch lands. Nothing else does, so an edit to
// the recipes would otherwise silently weaken (or lengthen) what every landing
// runs. Two properties are pinned:
//
//   - the gate is exactly its two stages, gate-lint then gate-test, in that
//     order, so a caller can bound each one separately;
//   - those stages are ci-pr-lint and the unit tier (test) and nothing else.
//     The landing gate is deliberately smaller than CI - the integration tier
//     needs Docker and takes far longer - so widening gate-test to
//     test-integration, or gate-lint to ci-pr-core/ci-pr-policy, must fail
//     here.
//
// Recipes are matched by the commands they invoke, not by exact whitespace, so
// the gate may change shape without failing this test (be-7tk added the
// ignore-errors guard to the gate recipe).
func TestLandingGatePolicy(t *testing.T) {
	makefile := readRepoFile(t, "../Makefile")

	recipes := map[string]string{}
	for _, target := range []string{"gate", "gate-lint", "gate-test"} {
		recipes[target] = makeRecipe(t, makefile, target)
	}

	wantStages := []string{"gate-lint", "gate-test"}
	stages := stageInvocation.FindAllStringSubmatch(recipes["gate"], -1)
	if len(stages) != len(wantStages) {
		t.Errorf("make gate invokes %d stages, want exactly %v:\n%s",
			len(stages), wantStages, recipes["gate"])
	}
	for i, want := range wantStages {
		if i >= len(stages) {
			break
		}
		if stages[i][1] != want {
			t.Errorf("make gate stage %d invokes %s, want %s (order matters):\n%s",
				i+1, stages[i][1], want, recipes["gate"])
		}
	}

	for _, tc := range []struct{ target, delegates string }{
		{"gate-lint", "ci-pr-lint"},
		{"gate-test", "test"},
	} {
		recipe := recipes[tc.target]
		invoked := stageInvocation.FindAllStringSubmatch(recipe, -1)
		if len(invoked) != 1 || invoked[0][1] != tc.delegates {
			t.Errorf("make %s should invoke exactly `$(MAKE) %s`:\n%s",
				tc.target, tc.delegates, recipe)
		}
	}

	// The gate must not reach for the integration tier or the other CI
	// wrappers, directly or through one of its stages.
	for _, target := range []string{"gate", "gate-lint", "gate-test"} {
		for _, forbidden := range []string{"test-integration", "ci-pr-core", "ci-pr-policy"} {
			if strings.Contains(recipes[target], forbidden) {
				t.Errorf("make %s mentions %s; the landing gate is lint then the unit tier, never the integration tier or the other CI wrappers:\n%s",
					target, forbidden, recipes[target])
			}
		}
	}

	phony := phonyTargets(t, makefile)
	for _, target := range []string{"gate", "gate-lint", "gate-test"} {
		if !phony[target] {
			t.Errorf(".PHONY does not list %s; a file named %s in the repo root would shadow the target",
				target, target)
		}
	}
}

// phonyTargets returns every name listed on a .PHONY line of the Makefile.
func phonyTargets(t *testing.T, makefile string) map[string]bool {
	t.Helper()
	targets := map[string]bool{}
	for _, line := range strings.Split(makefile, "\n") {
		if !strings.HasPrefix(line, ".PHONY:") {
			continue
		}
		for _, word := range strings.Fields(strings.TrimPrefix(line, ".PHONY:")) {
			targets[word] = true
		}
	}
	if len(targets) == 0 {
		t.Fatal("Makefile declares no .PHONY targets")
	}
	return targets
}
