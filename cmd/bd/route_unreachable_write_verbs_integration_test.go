//go:build cgo && integration

// Command-level regression tests for be-sut (deep review B1-04..06, be-qm8.2
// residual b).
//
// A cross-rig bead id whose prefix matches a route in routes.jsonl, but whose
// target database cannot be asked, is UNKNOWN — not a definite negative. bd
// show and bd update already report the typed route_unreachableError
// (kindRouteUnreachable, exit 24 in machine mode). bd close, bd defer and bd
// undefer used to report plain "not found" for the same id (close.go dropped
// the prefix-route error; defer/undefer resolved against the local store
// only), which tells a program the issue does not exist when it may well.
//
// These drive the three commands end to end against a real Dolt store, so they
// live behind the integration tier (TESTING.md, "The Two Tiers"). The
// resolution seam they exercise has its own unit-tier cover in
// route_unreachable_resolution_test.go, which is what `make gate` runs.

package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// routeUnreachableEnv returns a parity env with one live local store and a
// matched-but-unopenable prefix route for "zz-" ids.
func routeUnreachableEnv(t *testing.T) *parityEnv {
	t.Helper()
	env := newParityEnv(t)

	// Prefix routing resolves the current beads dir from dbPath, so point it
	// at the env's own .beads directory (which carries the routes table).
	dbPath = filepath.Join(env.beadsDir, "beads.db")
	writeUnreachableRouteFixture(t, filepath.Dir(env.beadsDir), env.beadsDir)
	return env
}

// kindOfError reads the typed kind off a command's returned error, reporting
// the captured stderr when the error carries no kind (the legacy exitError
// path each of these commands used for an unreachable route before be-sut).
func kindOfError(t *testing.T, err error, stderr string) errorKind {
	t.Helper()
	var ce *cliError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v (%T), want *cliError\nstderr:\n%s", err, err, stderr)
	}
	return ce.Kind
}

// TestWriteVerbsReportRouteUnreachable is acceptance criterion 2: each of bd
// close, bd defer and bd undefer reports the typed route error already used by
// bd show and bd update, in plain mode naming the route and the prefix rather
// than "not found".
func TestWriteVerbsReportRouteUnreachable(t *testing.T) {
	cases := []struct {
		name string
		run  func(env *parityEnv) runResult
	}{
		{"close", func(env *parityEnv) runResult {
			env.setFlags(closeCmd, nil)
			return env.run(closeCmd, "zz-abc")
		}},
		{"defer", func(env *parityEnv) runResult {
			env.setFlags(deferCmd, nil)
			return env.run(deferCmd, "zz-abc")
		}},
		{"undefer", func(env *parityEnv) runResult {
			env.setFlags(undeferCmd, nil)
			return env.run(undeferCmd, "zz-abc")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := routeUnreachableEnv(t)
			res := tc.run(env)

			if res.exitCode == 0 {
				t.Fatalf("exit = 0, want non-zero for an unreachable route\nstderr:\n%s", res.stderr)
			}
			// Criterion 4: name the route and the prefix.
			if !strings.Contains(res.stderr, "zz-") {
				t.Errorf("stderr does not name the prefix %q:\n%s", "zz-", res.stderr)
			}
			if !strings.Contains(res.stderr, "rig") {
				t.Errorf("stderr does not name the route target %q:\n%s", "rig", res.stderr)
			}
			if strings.Contains(res.stderr, "not found") {
				t.Errorf("unreachable route reported as a definite negative:\n%s", res.stderr)
			}
			if got := kindOfError(t, res.err, res.stderr); got != kindRouteUnreachable {
				t.Errorf("kind = %q, want %q\nstderr:\n%s", got, kindRouteUnreachable, res.stderr)
			}
		})
	}
}

// TestWriteVerbsRouteUnreachableMachineExitCode is acceptance criterion 2's
// exit-code half: route_unreachable is exit 24 in machine mode.
func TestWriteVerbsRouteUnreachableMachineExitCode(t *testing.T) {
	cases := []struct {
		name string
		run  func(env *parityEnv) runResult
	}{
		{"close", func(env *parityEnv) runResult { return env.run(closeCmd, "zz-abc") }},
		{"defer", func(env *parityEnv) runResult { return env.run(deferCmd, "zz-abc") }},
		{"undefer", func(env *parityEnv) runResult { return env.run(undeferCmd, "zz-abc") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := routeUnreachableEnv(t)
			withMachineMode(t, true)
			res := tc.run(env)
			if res.exitCode != 24 {
				t.Fatalf("exit = %d, want 24 (route_unreachable)\nstderr:\n%s", res.exitCode, res.stderr)
			}
		})
	}
}

// TestWriteVerbsRouteUnreachableMultiID is acceptance criterion 3: a multi-id
// call reports each id's own outcome — one unreachable, one fine — and exits
// non-zero, with the fine id's effect applied.
func TestWriteVerbsRouteUnreachableMultiID(t *testing.T) {
	const localID = "test-local"

	t.Run("close", func(t *testing.T) {
		env := routeUnreachableEnv(t)
		env.seed(localID, "Local closable", nil)

		env.setFlags(closeCmd, nil)
		res := env.run(closeCmd, localID, "zz-abc")

		if res.exitCode == 0 {
			t.Fatalf("exit = 0, want non-zero\nstderr:\n%s", res.stderr)
		}
		if got := env.get(localID).Status; got != types.StatusClosed {
			t.Errorf("%s status = %q, want closed (the resolvable id must still close)", localID, got)
		}
		assertPerIDRouteOutcome(t, res, localID)
	})

	t.Run("defer", func(t *testing.T) {
		env := routeUnreachableEnv(t)
		env.seed(localID, "Local deferrable", nil)

		env.setFlags(deferCmd, nil)
		res := env.run(deferCmd, localID, "zz-abc")

		if res.exitCode == 0 {
			t.Fatalf("exit = 0, want non-zero\nstderr:\n%s", res.stderr)
		}
		if got := env.get(localID).Status; got != types.StatusDeferred {
			t.Errorf("%s status = %q, want deferred (the resolvable id must still defer)", localID, got)
		}
		assertPerIDRouteOutcome(t, res, localID)
	})

	t.Run("undefer", func(t *testing.T) {
		env := routeUnreachableEnv(t)
		env.seed(localID, "Local deferrable", func(i *types.Issue) { i.Status = types.StatusDeferred })

		env.setFlags(undeferCmd, nil)
		res := env.run(undeferCmd, localID, "zz-abc")

		if res.exitCode == 0 {
			t.Fatalf("exit = 0, want non-zero\nstderr:\n%s", res.stderr)
		}
		if got := env.get(localID).Status; got != types.StatusOpen {
			t.Errorf("%s status = %q, want open (the resolvable id must still undefer)", localID, got)
		}
		assertPerIDRouteOutcome(t, res, localID)
	})
}

// assertPerIDRouteOutcome checks the batch error carries one entry for the
// unreachable id, classified route_unreachable, and one for the id that
// succeeded.
func assertPerIDRouteOutcome(t *testing.T, res runResult, succeededID string) {
	t.Helper()
	var ce *cliError
	if !errors.As(res.err, &ce) {
		t.Fatalf("err = %v (%T), want *cliError", res.err, res.err)
	}
	outcomeKind := map[string]errorKind{}
	for _, o := range ce.IDs {
		outcomeKind[o.ID] = o.Kind
	}
	if got := outcomeKind["zz-abc"]; got != kindRouteUnreachable {
		t.Errorf("reported kind for zz-abc = %q, want %q (ids=%+v)", got, kindRouteUnreachable, ce.IDs)
	}
	if _, ok := outcomeKind[succeededID]; !ok && ce.Kind != kindPartial {
		t.Errorf("no per-id outcome for %s in %+v", succeededID, ce.IDs)
	}
}
