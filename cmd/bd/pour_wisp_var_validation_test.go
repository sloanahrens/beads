//go:build cgo

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// Regression coverage for mybd-u2r6: bd mol pour/wisp previously validated
// only variable *presence* (extractRequiredVariables), never the enum/pattern
// constraints cook enforces. Once resolveAndCookFormulaWithVars started
// calling formula.ValidateProvidedVars (mybd-u2r6 fix), pour/wisp needed to
// distinguish "the arg isn't a formula at all" (fall through to legacy
// proto-ID resolution) from "it IS a formula, but the given --var values
// violate a constraint" (report directly) without disturbing the missing-var
// hint path, which has its own better UX and must keep working unchanged.

const varValidationFormulaTOML = `formula = "pour-wisp-var-validation-test"
version = 1
type = "workflow"

[vars.policy]
required = true
enum = ["merge-completes", "tracking-only"]

[vars.slug]
pattern = "^[a-z]+$"

[[steps]]
id = "publish"
title = "Publish with {{policy}} / {{slug}}"
`

// writeVarValidationFormula writes the shared fixture under a temp town root so
// pour/wisp's DefaultSearchPaths() resolution (which always passes a nil
// search-path list) discovers it by name without perturbing the real
// project's formula registry. Both the GT_TOWN_ROOT name and its deprecated
// GT_ROOT alias point at the temp root so the fixture is found either way.
func writeVarValidationFormula(t *testing.T) {
	t.Helper()
	gtRoot := t.TempDir()
	formulaDir := filepath.Join(gtRoot, ".beads", "formulas")
	if err := os.MkdirAll(formulaDir, 0o755); err != nil {
		t.Fatalf("mkdir formula dir: %v", err)
	}
	formulaPath := filepath.Join(formulaDir, "pour-wisp-var-validation-test.formula.toml")
	if err := os.WriteFile(formulaPath, []byte(varValidationFormulaTOML), 0o600); err != nil {
		t.Fatalf("write formula fixture: %v", err)
	}
	t.Setenv("GT_TOWN_ROOT", gtRoot)
	t.Setenv("GT_ROOT", gtRoot)
}

func makePourVarTestCmd(varFlags []string) *cobra.Command {
	c := &cobra.Command{Use: "pour"}
	c.Flags().Bool("dry-run", true, "")
	c.Flags().StringArray("var", varFlags, "")
	c.Flags().String("assignee", "", "")
	c.Flags().StringSlice("attach", []string{}, "")
	c.Flags().String("attach-type", "", "")
	return c
}

func makeWispVarTestCmd(varFlags []string) *cobra.Command {
	c := &cobra.Command{Use: "wisp"}
	c.Flags().StringArray("var", varFlags, "")
	c.Flags().Bool("dry-run", true, "")
	c.Flags().Bool("root-only", false, "")
	return c
}
