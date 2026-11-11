package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// acceptanceGuardRepo serves the one issue the guard reads. Every other
// repository method is the embedded nil interface, so reaching one panics: the
// guard must refuse before ApplyUpdate writes anything.
type acceptanceGuardRepo struct {
	IssueSQLRepository
	issue *types.Issue
}

func (r acceptanceGuardRepo) Exists(context.Context, string, IssueTableOpts) (bool, error) {
	return false, nil
}

func (r acceptanceGuardRepo) Get(context.Context, string, IssueTableOpts) (*types.Issue, error) {
	return r.issue, nil
}

// TestIfAcceptanceApplyUpdateRefusesStaleGuard pins that ApplyUpdate refuses a
// --if-acceptance guard that no longer names the stored criteria, with
// ErrAcceptanceMismatch, before any write.
func TestIfAcceptanceApplyUpdateRefusesStaleGuard(t *testing.T) {
	const amended = "- [x] builds"
	empty := ""
	stale := "- [ ] builds"
	tests := []struct {
		name  string
		guard *string
	}{
		{name: "stale text refuses", guard: &stale},
		{name: "empty guard refuses a filled field", guard: &empty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &issueUseCaseImpl{issueRepo: acceptanceGuardRepo{issue: &types.Issue{ID: "bd-1", AcceptanceCriteria: amended}}}
			spec := UpdateSpec{
				Fields:             map[string]any{"acceptance_criteria": "- [x] tests pass"},
				ExpectedAcceptance: tt.guard,
			}
			_, err := uc.ApplyUpdate(context.Background(), "bd-1", spec, "actor")
			if !errors.Is(err, storage.ErrAcceptanceMismatch) {
				t.Fatalf("ApplyUpdate err = %v, want ErrAcceptanceMismatch", err)
			}
		})
	}
}
