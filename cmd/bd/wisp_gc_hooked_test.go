package main

import (
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// TestIsProtectedWisp_HookedByLiveAgent is the regression test for be-yqp:
// a wisp currently referenced as hook_bead by a live agent must never be
// reclaimed by age-based wisp GC, even though hooking a wisp does not change
// the wisp's own status (it can be plain "open" and look idle by
// updated_at).
func TestIsProtectedWisp_HookedByLiveAgent(t *testing.T) {
	protectedStatuses := map[types.Status]bool{} // no status-based protection in play

	hooked := &types.Issue{ID: "wisp-hooked", Status: types.StatusOpen}
	idle := &types.Issue{ID: "wisp-idle", Status: types.StatusOpen}

	hookedSet := map[string]bool{"wisp-hooked": true}
	blockedSet := map[string]bool{}

	if !isProtectedWisp(hooked, blockedSet, hookedSet, protectedStatuses) {
		t.Errorf("wisp %s is referenced as an active hook_bead and must be protected", hooked.ID)
	}
	if isProtectedWisp(idle, blockedSet, hookedSet, protectedStatuses) {
		t.Errorf("wisp %s is not hooked, blocked, or in a protected status category and must stay a GC candidate (guard must not over-protect)", idle.ID)
	}
}
