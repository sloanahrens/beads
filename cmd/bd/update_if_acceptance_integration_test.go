//go:build cgo && integration

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// TestUpdateIfAcceptanceGuard drives `bd update --if-acceptance` through the
// command against a real store. A matching guard writes the new criteria; a
// mismatch writes nothing, exits ExitGuardMismatch (13) and names the
// machine-greppable sentinel.
func TestUpdateIfAcceptanceGuard(t *testing.T) {
	const stored = "- [ ] builds\n- [ ] tests pass"
	const written = "- [x] builds\n- [ ] tests pass"
	const stale = "- [x] builds\n- [x] tests pass"
	storedSum := sha256.Sum256([]byte(stored))
	storedDigest := hex.EncodeToString(storedSum[:])
	staleSum := sha256.Sum256([]byte(stale))
	staleDigest := hex.EncodeToString(staleSum[:])

	tests := []struct {
		name       string
		seeded     string
		guard      string
		wantExit   int
		wantStored string
	}{
		{name: "text guard matching the stored criteria writes", seeded: stored, guard: stored, wantExit: 0, wantStored: written},
		{name: "sha256 guard matching the stored criteria writes", seeded: stored, guard: storedDigest, wantExit: 0, wantStored: written},
		{name: "text guard against an amended criteria refuses", seeded: stale, guard: stored, wantExit: ExitGuardMismatch, wantStored: stale},
		{name: "sha256 guard against an amended criteria refuses", seeded: stale, guard: storedDigest, wantExit: ExitGuardMismatch, wantStored: stale},
		{name: "sha256 of the amendment is not the stored criteria", seeded: stored, guard: staleDigest, wantExit: ExitGuardMismatch, wantStored: stored},
		{name: "empty guard matches an empty field and writes", seeded: "", guard: "", wantExit: 0, wantStored: written},
		{name: "empty guard refuses a filled field", seeded: stored, guard: "", wantExit: ExitGuardMismatch, wantStored: stored},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newParityEnv(t)
			env.seed("test-acc1", "Acceptance guard", func(i *types.Issue) {
				i.AcceptanceCriteria = tt.seeded
			})
			env.setFlags(updateCmd, map[string]string{
				"acceptance":    written,
				"if-acceptance": tt.guard,
			})
			res := env.run(updateCmd, "test-acc1")
			if res.exitCode != tt.wantExit {
				t.Fatalf("exit = %d, want %d\nstderr:\n%s", res.exitCode, tt.wantExit, res.stderr)
			}
			if got := env.get("test-acc1").AcceptanceCriteria; got != tt.wantStored {
				t.Errorf("acceptance criteria = %q, want %q", got, tt.wantStored)
			}
			if tt.wantExit == ExitGuardMismatch {
				if !strings.Contains(res.stderr, "acceptance mismatch") {
					t.Errorf("stderr lacks the machine-greppable %q sentinel:\n%s", "acceptance mismatch", res.stderr)
				}
				if got := env.store.mutations(); len(got) != 0 {
					t.Errorf("store mutations = %v, want none (refused before any write)", got)
				}
			}
		})
	}
}
