//go:build cgo

package main

import (
	"testing"

	"github.com/steveyegge/beads/internal/storage/issueops"
)

// Generic update close policy belongs to shared lifecycle conformance. These
// command tests retain only command-specific wiring for direct assignee
// transfer and the batch, proxied, embedded, and cross-backend update paths.

// TestUpdateClosePolicyBatchGrammarForceToken pins the batch update grammar's
// spelling of the override, and — the part that matters — pins the allowlist
// that keeps the reserved update-map key from being client-reachable. A script
// asks for force by the grammar's own token; it can never name the transport
// key itself, which is what stops the key from becoming a policy bypass.
func TestUpdateClosePolicyBatchGrammarForceToken(t *testing.T) {
	updates, err := parseUpdateKVs([]string{"status=closed", "force=true"})
	if err != nil {
		t.Fatalf("parseUpdateKVs(force=true): %v", err)
	}
	if got := updates[issueops.OpForceClosePolicy]; got != true {
		t.Errorf("updates[%q] = %v, want true", issueops.OpForceClosePolicy, got)
	}
	if updates["status"] != "closed" {
		t.Errorf("updates[status] = %v, want closed", updates["status"])
	}

	unforced, err := parseUpdateKVs([]string{"status=closed", "force=false"})
	if err != nil {
		t.Fatalf("parseUpdateKVs(force=false): %v", err)
	}
	if got := unforced[issueops.OpForceClosePolicy]; got != false {
		t.Errorf("updates[%q] = %v, want false", issueops.OpForceClosePolicy, got)
	}

	if _, err := parseUpdateKVs([]string{"force=perhaps"}); err == nil {
		t.Error("parseUpdateKVs accepted a non-boolean force value")
	}
	if _, err := parseUpdateKVs([]string{"_force_close_policy=true"}); err == nil {
		t.Error("parseUpdateKVs accepted the reserved update-map key as a client token")
	}
	if _, err := parseUpdateKVs([]string{"description=foo"}); err == nil {
		t.Error("parseUpdateKVs stopped rejecting keys outside its allowlist")
	}
}

// The proxied path's own translation of `--force` is pinned in
// update_proxied_server_test.go (TestProxiedUpdateCarriesForce), against the
// UpdateRequest that path now hands issueops.Lifecycle — the spec it used to
// build by hand is gone.
