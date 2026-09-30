package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/formula"
)

func writeFormulaFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name+formula.FormulaExtTOML)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A formula with a key bd would drop must fail the pour/wisp cook path with
// the strict error, so callers report it instead of "not found as formula
// or proto ID".
func TestResolveAndCook_InvalidFormulaIsTyped(t *testing.T) {
	dir := t.TempDir()
	path := writeFormulaFile(t, dir, "mol-dropped",
		"formula = \"mol-dropped\"\nversion = 1\n[squash]\ntrigger = \"on_complete\"\n[[steps]]\nid = \"a\"\ntitle = \"A\"\n")
	_, err := resolveAndCookFormulaWithVars("mol-dropped", []string{dir}, map[string]string{})
	if err == nil || !isFormulaUserError(err) {
		t.Fatalf("want an invalid-formula error, got %v", err)
	}
	if strings.Contains(err.Error(), "\n") || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "line 3: squash") {
		t.Fatalf("want one line naming file, line and key, got %q", err.Error())
	}
}

func TestReportFormulaError_MachineKinds(t *testing.T) {
	withMachineMode(t, true)
	fe := &formula.FormulaError{File: "/x/mol.formula.toml", Problems: []formula.Problem{
		{Kind: formula.ProblemUnknownKey, Key: "steps.gate.condition", Line: 235, Message: "unknown key"},
	}}
	cases := []struct {
		err  error
		kind errorKind
		code int
	}{
		{fe, kindInvalidArgs, 27},
		{formula.ErrVarValidation, kindInvalidArgs, 27},
		{errors.Join(formula.ErrFormulaNotFound), kindNotFound, 20},
		{errors.New("boom"), kindInternal, 1},
	}
	for _, c := range cases {
		var ce *cliError
		if !errors.As(reportFormulaError(c.err), &ce) || ce.Kind != c.kind || ce.ExitCode() != c.code {
			t.Errorf("%v: got %+v, want kind %s exit %d", c.err, ce, c.kind, c.code)
		}
		if errorKindOf(c.err) != c.kind {
			t.Errorf("errorKindOf(%v) = %s, want %s", c.err, errorKindOf(c.err), c.kind)
		}
	}
	var ce *cliError
	errors.As(reportFormulaError(fe), &ce)
	if ce.Detail["file"] != "/x/mol.formula.toml" || ce.Detail["key"] != "steps.gate.condition" || ce.Detail["line"] != 235 {
		t.Fatalf("detail = %+v", ce.Detail)
	}
}

// Outside machine mode the formula error path is HandleError: exit 1 and no
// typed cliError, exactly what pour and wisp returned before.
func TestReportFormulaError_LegacyModeUnchanged(t *testing.T) {
	withMachineMode(t, false)
	err := reportFormulaError(&formula.FormulaError{File: "f", Problems: []formula.Problem{{Message: "m"}}})
	var ee *exitError
	if !errors.As(err, &ee) || ee.Code != 1 {
		t.Fatalf("want exitError{1}, got %#v", err)
	}
	var ce *cliError
	if errors.As(err, &ce) {
		t.Fatalf("legacy mode must not return a typed error: %#v", ce)
	}
}
