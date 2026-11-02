// create_routing_notice_test.go - Tests for the stderr notice printed when
// `bd create` auto-routes a write to a different workspace.
//
// Deliberately untagged (no `cgo` build tag): printCreateRoutingNotice and
// routingNoticeText need no store, so these tests must stay compilable under
// CGO_ENABLED=0 with the gms_pure_go build tag alongside the rest of the
// pure-Go test suite.

package main

import (
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/routing"
)

// TestCreateRoutingNotice verifies each auto-routing rule produces exactly one
// stderr line naming the target repo, the .beads directory actually opened,
// the matched rule and its fix.
func TestCreateRoutingNotice(t *testing.T) {
	origQuiet := quietFlag
	quietFlag = false
	t.Cleanup(func() {
		quietFlag = origQuiet
		resetCreateRoutingNoticeOnce()
	})

	// Deliberately not target+".beads": the notice must print the resolved
	// .beads directory (which can differ from filepath.Join(target, ".beads")
	// when a redirect is followed), so the two strings must be distinguishable.
	const (
		target   = "/work/planning"
		beadsDir = "/srv/beads-store/planning/.beads"
	)

	tests := []struct {
		name     string
		rule     routing.RoutingRule
		wantRule string
		wantFix  string
	}{
		{"RuleContributor", routing.RuleContributor, "beads.role=contributor", "git config beads.role maintainer"},
		{"RuleMaintainer", routing.RuleMaintainer, "routing.maintainer", "bd config unset routing.maintainer"},
		{"RuleDefault", routing.RuleDefault, "routing.default", "bd config unset routing.default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCreateRoutingNoticeOnce()
			out := captureStderr(t, func() {
				printCreateRoutingNotice(target, beadsDir, tt.rule)
			})

			if got := strings.Count(out, "\n"); got != 1 {
				t.Fatalf("printCreateRoutingNotice(rule=%v) wrote %d stderr lines, want 1: %q", tt.rule, got, out)
			}
			for _, want := range []string{target, beadsDir, tt.wantRule, tt.wantFix} {
				if !strings.Contains(out, want) {
					t.Errorf("printCreateRoutingNotice(rule=%v) stderr = %q, want substring %q", tt.rule, out, want)
				}
			}
		})
	}
}

// TestCreateRoutingNotice_Suppressed verifies a create that was not routed,
// and --quiet, both print nothing — and that a suppressed call does not consume
// the once guard, so a later routed create is still announced.
func TestCreateRoutingNotice_Suppressed(t *testing.T) {
	origQuiet := quietFlag
	quietFlag = false
	t.Cleanup(func() {
		quietFlag = origQuiet
		resetCreateRoutingNoticeOnce()
	})

	t.Run("unrouted target", func(t *testing.T) {
		resetCreateRoutingNoticeOnce()
		out := captureStderr(t, func() {
			printCreateRoutingNotice(".", ".", routing.RuleNone)
		})
		if out != "" {
			t.Errorf("printCreateRoutingNotice(\".\", rule=RuleNone) wrote stderr = %q, want empty", out)
		}
		out = captureStderr(t, func() {
			printCreateRoutingNotice("/work/planning", "/work/planning/.beads", routing.RuleContributor)
		})
		if out == "" {
			t.Error("printCreateRoutingNotice() after a suppressed call wrote no stderr, want non-empty (the once guard was consumed early)")
		}
	})

	t.Run("empty target", func(t *testing.T) {
		resetCreateRoutingNoticeOnce()
		out := captureStderr(t, func() {
			printCreateRoutingNotice("", "/work/planning/.beads", routing.RuleContributor)
		})
		if out != "" {
			t.Errorf("printCreateRoutingNotice(\"\", ...) wrote stderr = %q, want empty", out)
		}
	})

	t.Run("quiet flag", func(t *testing.T) {
		resetCreateRoutingNoticeOnce()
		quietFlag = true
		out := captureStderr(t, func() {
			printCreateRoutingNotice("/work/planning", "/work/planning/.beads", routing.RuleContributor)
		})
		if out != "" {
			t.Errorf("printCreateRoutingNotice() with quietFlag=true wrote stderr = %q, want empty", out)
		}

		quietFlag = false
		out = captureStderr(t, func() {
			printCreateRoutingNotice("/work/planning", "/work/planning/.beads", routing.RuleContributor)
		})
		if out == "" {
			t.Error("printCreateRoutingNotice() after a quiet call wrote no stderr, want non-empty (the once guard was consumed early)")
		}
	})
}

// TestCreateRoutingNotice_OncePerProcess verifies two routed creates in one
// process print the notice only once.
func TestCreateRoutingNotice_OncePerProcess(t *testing.T) {
	origQuiet := quietFlag
	quietFlag = false
	t.Cleanup(func() {
		quietFlag = origQuiet
		resetCreateRoutingNoticeOnce()
	})

	resetCreateRoutingNoticeOnce()
	out := captureStderr(t, func() {
		printCreateRoutingNotice("/work/planning", "/work/planning/.beads", routing.RuleMaintainer)
		printCreateRoutingNotice("/work/planning", "/work/planning/.beads", routing.RuleMaintainer)
	})
	if got := strings.Count(out, "note: auto-routed create"); got != 1 {
		t.Errorf("two routed creates wrote the notice %d times, want 1: %q", got, out)
	}
}
