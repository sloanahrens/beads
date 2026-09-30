package scripts_test

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestUnitTierPolicy pins the wiring the be-b23 unit tier depends on. The
// tripwire itself (internal/testtier, called from schema.MigrateUp and
// doltserver.Start) is what fails a new unit-tier test that opens a fresh
// migrated store; this test keeps the wiring that arms it from drifting:
//
//   - make test runs scripts/test.sh and does not opt into Dolt, so the
//     hermetic env exports BD_TEST_TIER=unit (TestHermeticEnvExportsTestTier);
//   - scripts/test.sh links the tier into the bd it prebuilds, because some
//     cmd/bd helpers strip every BEADS_* and BD_* variable before spawning it
//     (a -X flag naming a symbol that does not exist is silently ignored, so
//     the symbol's existence is checked too);
//   - make test-integration opts into Dolt and the integration build tag.
func TestUnitTierPolicy(t *testing.T) {
	makefile := readRepoFile(t, "../Makefile")
	testRecipe := makeRecipe(t, makefile, "test")
	if !strings.Contains(testRecipe, "./scripts/test.sh") {
		t.Errorf("make test recipe does not run scripts/test.sh:\n%s", testRecipe)
	}
	for _, forbidden := range []string{"BEADS_TEST_ENV_RUN_DOLT", "BD_TEST_TIER", "TEST_TAGS"} {
		if strings.Contains(testRecipe, forbidden) {
			t.Errorf("make test recipe sets %s; the unit tier must not opt into Dolt:\n%s", forbidden, testRecipe)
		}
	}

	integRecipe := makeRecipe(t, makefile, "test-integration")
	for _, want := range []string{"BEADS_TEST_ENV_RUN_DOLT=1", "TEST_TAGS=integration", "./scripts/test.sh"} {
		if !strings.Contains(integRecipe, want) {
			t.Errorf("make test-integration recipe lacks %q:\n%s", want, integRecipe)
		}
	}

	runner := readRepoFile(t, "test.sh")
	for _, want := range []string{
		"beads_test_env_enter",
		"-X github.com/steveyegge/beads/internal/testtier.buildTier=${BD_TEST_TIER}",
	} {
		if !strings.Contains(runner, want) {
			t.Errorf("scripts/test.sh lacks %q", want)
		}
	}

	tier := readRepoFile(t, "../internal/testtier/testtier.go")
	if !regexp.MustCompile(`(?m)^var buildTier string$`).MatchString(tier) {
		t.Error("internal/testtier no longer declares `var buildTier string`; scripts/test.sh's -X link would silently do nothing")
	}
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// makeRecipe returns the recipe lines of a Makefile target.
func makeRecipe(t *testing.T, makefile, target string) string {
	t.Helper()
	lines := strings.Split(makefile, "\n")
	for i, line := range lines {
		if line != target+":" {
			continue
		}
		var recipe []string
		for _, l := range lines[i+1:] {
			if !strings.HasPrefix(l, "\t") {
				break
			}
			recipe = append(recipe, l)
		}
		return strings.Join(recipe, "\n")
	}
	t.Fatalf("Makefile has no %q target", target)
	return ""
}
