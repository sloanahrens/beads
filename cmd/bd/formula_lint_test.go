package main

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

func newLintTestCmd() *cobra.Command {
	c := &cobra.Command{}
	c.Flags().StringSlice("search-path", []string{}, "")
	return c
}

func TestFormulaLint_MachineModeReportsInvalidArgs(t *testing.T) {
	withMachineMode(t, true)
	dir := t.TempDir()
	writeFormulaFile(t, dir, "ok", "formula = \"ok\"\nversion = 1\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n")
	writeFormulaFile(t, dir, "bad", "formula = \"bad\"\nversion = 1\n[squash]\ntrigger = \"x\"\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n")

	err := runFormulaLint(newLintTestCmd(), []string{dir})
	var ce *cliError
	if !errors.As(err, &ce) || ce.Kind != kindInvalidArgs || ce.ExitCode() != 27 {
		t.Fatalf("want invalid_args/27, got %v", err)
	}
}

func TestFormulaLint_CleanDirSucceeds(t *testing.T) {
	withMachineMode(t, true)
	dir := t.TempDir()
	writeFormulaFile(t, dir, "ok", "formula = \"ok\"\nversion = 1\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n")
	if err := runFormulaLint(newLintTestCmd(), []string{dir}); err != nil {
		t.Fatalf("clean dir: %v", err)
	}
}

func TestFormulaLint_MissingPathIsNotFound(t *testing.T) {
	withMachineMode(t, true)
	err := runFormulaLint(newLintTestCmd(), []string{t.TempDir() + "/nope"})
	var ce *cliError
	if !errors.As(err, &ce) || ce.Kind != kindNotFound {
		t.Fatalf("want not_found, got %v", err)
	}
}
