//go:build cgo && integration

package main

import (
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// TestUpdateClosePolicyDirectForceStillFencesAssigneeTransfer keeps the other
// half of `--force` intact. Conditioning it on an assignee edit must not turn
// it off when there IS one: a transfer away from a live foreign claim is still
// exactly what the flag authorizes.
func TestUpdateClosePolicyDirectForceStillFencesAssigneeTransfer(t *testing.T) {
	env := newParityEnv(t)
	env.seed("test-ucpa", "Held by another actor", func(i *types.Issue) {
		i.Assignee = "someone-else"
		i.Status = types.StatusInProgress
	})

	// Without --force the fence holds.
	env.setFlags(updateCmd, map[string]string{"assignee": "thief"})
	if res := env.run(updateCmd, "test-ucpa"); res.exitCode == 0 {
		t.Fatalf("unforced transfer succeeded; the fence is gone\nstderr:\n%s", res.stderr)
	}
	if got := env.get("test-ucpa").Assignee; got != "someone-else" {
		t.Fatalf("assignee = %q after a refused transfer, want someone-else", got)
	}

	// With it, the transfer is authorized.
	env.setFlags(updateCmd, map[string]string{"assignee": "thief", "force": "true"})
	if res := env.run(updateCmd, "test-ucpa"); res.exitCode != 0 {
		t.Fatalf("forced transfer exit = %d, want 0\nstderr:\n%s", res.exitCode, res.stderr)
	}
	if got := env.get("test-ucpa").Assignee; got != "thief" {
		t.Errorf("assignee = %q, want thief", got)
	}
}
