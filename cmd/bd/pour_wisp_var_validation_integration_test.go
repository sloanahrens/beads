//go:build cgo && integration

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestPourRejectsEnumViolatingVar(t *testing.T) {
	writeVarValidationFormula(t)
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")
	withWispTestGlobals(t, s, context.Background())

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runPour(makePourVarTestCmd([]string{"policy=bogus", "slug=abc"}), []string{"pour-wisp-var-validation-test"})
	})

	if runErr == nil {
		t.Fatal("runPour accepted a value outside the declared enum")
	}
	if !strings.Contains(stderr, `not in allowed values`) {
		t.Fatalf("runPour stderr = %q, want an enum-violation message", stderr)
	}
}

func TestPourRejectsPatternViolatingVar(t *testing.T) {
	writeVarValidationFormula(t)
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")
	withWispTestGlobals(t, s, context.Background())

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runPour(makePourVarTestCmd([]string{"policy=merge-completes", "slug=NOT-LOWERCASE"}), []string{"pour-wisp-var-validation-test"})
	})

	if runErr == nil {
		t.Fatal("runPour accepted a value violating the declared pattern")
	}
	if !strings.Contains(stderr, `does not match pattern`) {
		t.Fatalf("runPour stderr = %q, want a pattern-violation message", stderr)
	}
}

// TestPourMissingVarHintUnchanged asserts that a genuinely absent required
// var still surfaces through checkPourVars/missingVarHint's existing
// HandleErrorWithHint path, not the new enum/pattern validation error path.
func TestPourMissingVarHintUnchanged(t *testing.T) {
	writeVarValidationFormula(t)
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")
	withWispTestGlobals(t, s, context.Background())

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runPour(makePourVarTestCmd([]string{}), []string{"pour-wisp-var-validation-test"})
	})

	if runErr == nil {
		t.Fatal("runPour accepted a formula with a missing required variable")
	}
	if !strings.Contains(stderr, "missing required variables") {
		t.Fatalf("runPour stderr = %q, want the missing-required-variables message", stderr)
	}
	if !strings.Contains(stderr, "Hint: Provide them with: --var policy=") {
		t.Fatalf("runPour stderr = %q, want the pour-specific --var hint text", stderr)
	}
}

// TestPourRejectsExplicitlyEmptyRequiredVar covers the bead's headline repro:
// an unset shell variable interpolated into --var (e.g. --var
// policy=$UNSET_SHELL_VAR) arrives as an explicitly-*provided*, empty-string
// value rather than an absent flag, so it must hit ValidateProvidedVars'
// required-and-empty check rather than the missing-var hint path.
func TestPourRejectsExplicitlyEmptyRequiredVar(t *testing.T) {
	writeVarValidationFormula(t)
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")
	withWispTestGlobals(t, s, context.Background())

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runPour(makePourVarTestCmd([]string{"policy=", "slug=abc"}), []string{"pour-wisp-var-validation-test"})
	})

	if runErr == nil {
		t.Fatal("runPour accepted an explicitly-empty value for a required variable")
	}
	if !strings.Contains(stderr, "is required and cannot be empty") {
		t.Fatalf("runPour stderr = %q, want the required-and-empty message", stderr)
	}
}

func TestWispRejectsEnumViolatingVar(t *testing.T) {
	writeVarValidationFormula(t)
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")
	withWispTestGlobals(t, s, context.Background())

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runWispCreate(makeWispVarTestCmd([]string{"policy=bogus", "slug=abc"}), []string{"pour-wisp-var-validation-test"})
	})

	if runErr == nil {
		t.Fatal("runWispCreate accepted a value outside the declared enum")
	}
	if !strings.Contains(stderr, `not in allowed values`) {
		t.Fatalf("runWispCreate stderr = %q, want an enum-violation message", stderr)
	}
}

func TestWispRejectsPatternViolatingVar(t *testing.T) {
	writeVarValidationFormula(t)
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")
	withWispTestGlobals(t, s, context.Background())

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runWispCreate(makeWispVarTestCmd([]string{"policy=merge-completes", "slug=NOT-LOWERCASE"}), []string{"pour-wisp-var-validation-test"})
	})

	if runErr == nil {
		t.Fatal("runWispCreate accepted a value violating the declared pattern")
	}
	if !strings.Contains(stderr, `does not match pattern`) {
		t.Fatalf("runWispCreate stderr = %q, want a pattern-violation message", stderr)
	}
}

// TestWispMissingVarHintUnchanged mirrors TestPourMissingVarHintUnchanged for
// wisp's own checkRequiredVars/firstMissingVar hint path.
func TestWispMissingVarHintUnchanged(t *testing.T) {
	writeVarValidationFormula(t)
	s := newTestStoreWithPrefix(t, filepath.Join(t.TempDir(), "test.db"), "test")
	withWispTestGlobals(t, s, context.Background())

	var runErr error
	stderr := captureStderr(t, func() {
		runErr = runWispCreate(makeWispVarTestCmd([]string{}), []string{"pour-wisp-var-validation-test"})
	})

	if runErr == nil {
		t.Fatal("runWispCreate accepted a formula with a missing required variable")
	}
	if !strings.Contains(stderr, "missing required variables") {
		t.Fatalf("runWispCreate stderr = %q, want the missing-required-variables message", stderr)
	}
	if !strings.Contains(stderr, "Hint: Provide them with: --var policy=") {
		t.Fatalf("runWispCreate stderr = %q, want the wisp-specific --var hint text", stderr)
	}
}
