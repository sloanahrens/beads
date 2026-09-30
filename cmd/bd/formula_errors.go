package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/steveyegge/beads/internal/formula"
)

// isFormulaUserError reports whether a cook failure means "this IS a
// formula, and it (or the --var values given to it) is wrong". Such an
// error must be reported as is; falling through to a proto-ID lookup would
// turn it into a misleading "not found".
func isFormulaUserError(err error) bool {
	return errors.Is(err, formula.ErrVarValidation) || errors.Is(err, formula.ErrInvalidFormula)
}

// formulaErrorDetail is the envelope detail for an invalid formula: the
// file, the first problem's key and line, and every problem.
func formulaErrorDetail(err error) map[string]any {
	var fe *formula.FormulaError
	if !errors.As(err, &fe) {
		return nil
	}
	detail := map[string]any{"file": fe.File, "problems": fe.Problems}
	if len(fe.Problems) > 0 {
		detail["key"] = fe.Problems[0].Key
		detail["line"] = fe.Problems[0].Line
	}
	return detail
}

// reportFormulaError reports a formula load/cook failure. Invalid formulas
// and invalid --var values are invalid_args (27), an unknown formula is
// not_found (20); anything else is internal. Outside machine mode it prints
// one "Error:" line on stderr and exits 1, as HandleError does.
func reportFormulaError(err error) error {
	kind := kindInternal
	switch {
	case isFormulaUserError(err):
		kind = kindInvalidArgs
	case errors.Is(err, formula.ErrFormulaNotFound):
		kind = kindNotFound
	}
	if !machineModeActive() {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return &exitError{Code: 1}
	}
	return &cliError{Kind: kind, Message: err.Error(), Detail: formulaErrorDetail(err)}
}
