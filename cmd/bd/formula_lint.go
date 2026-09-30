package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/formula"
	"github.com/steveyegge/beads/internal/ui"
)

var formulaLintCmd = &cobra.Command{
	Use:   "lint <path|dir>",
	Short: "Report every key, gate type or var a formula would have silently dropped",
	Long: `Lint a .formula.toml file, or every .formula.toml directly in a directory,
against the strict decoder bd cooks with. It reports, per file:

  unknown_key        a key bd does not know (it used to be dropped silently)
  invalid_value      a var field of the wrong type (dropped the same way)
  invalid_gate_type  a step gate type no watcher or command resolves
  validation         structural errors, e.g. a var both required and defaulted
  syntax             the file is not valid TOML

A formula that extends another is resolved against its own directory, then
--search-path. Exits 1 when any file has a problem (machine mode: the
report is data and error.kind is invalid_args, exit 27).

Examples:
  bd formula lint ~/gt/.beads/formulas
  bd formula lint mol-shutdown-dance.formula.toml --json`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runFormulaLint,
}

func runFormulaLint(cmd *cobra.Command, args []string) error {
	searchPaths, _ := cmd.Flags().GetStringSlice("search-path")
	rep, err := formula.LintPath(args[0], searchPaths)
	if err != nil {
		if os.IsNotExist(err) {
			return failKind(kindNotFound, "%v", err)
		}
		return failKind(kindInvalidArgs, "%v", err)
	}

	if jsonOutput {
		if err := outputJSON(rep); err != nil {
			return err
		}
	} else {
		printFormulaLint(rep)
	}

	if rep.Failed == 0 {
		return nil
	}
	msg := fmt.Sprintf("%d of %d formulas would drop keys or fail to cook", rep.Failed, rep.Checked)
	if machineModeActive() {
		return &cliError{Kind: kindInvalidArgs, Message: msg}
	}
	return &exitError{Code: 1}
}

func printFormulaLint(rep *formula.LintReport) {
	for _, f := range rep.Files {
		for _, p := range f.Problems {
			loc := f.File
			if p.Line > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, p.Line)
			}
			key := ""
			if p.Key != "" {
				key = p.Key + ": "
			}
			fmt.Printf("%s: %s [%s] %s%s\n", loc, ui.RenderFail("✗"), p.Kind, key, p.Message)
		}
	}
	if rep.Failed == 0 {
		fmt.Printf("%s %d formula(s) clean\n", ui.RenderPass("✓"), rep.Checked)
		return
	}
	fmt.Printf("\n%d of %d formula(s) have problems\n", rep.Failed, rep.Checked)
}

func init() {
	formulaLintCmd.Flags().StringSlice("search-path", []string{}, "Additional paths to search when resolving extends")
	formulaCmd.AddCommand(formulaLintCmd)
}
