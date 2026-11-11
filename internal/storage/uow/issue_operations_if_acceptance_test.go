package uow

import (
	"testing"

	"github.com/steveyegge/beads/internal/storage/domain"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/issueops"
)

// TestIfAcceptanceReachesUpdateSpec pins that the --if-acceptance guard
// travels from the public request into the domain spec, so ApplyUpdate can
// check it inside the unit of work.
func TestIfAcceptanceReachesUpdateSpec(t *testing.T) {
	expected := "- [ ] builds"
	spec, err := updateSpec(issueops.UpdateRequest{
		Patch:              issueops.IssuePatch{AcceptanceCriteria: issueops.Field[string]{Set: true, Value: "- [x] builds"}},
		ExpectedAcceptance: &expected,
	})
	if err != nil {
		t.Fatalf("updateSpec() error = %v", err)
	}
	if spec.ExpectedAcceptance == nil || *spec.ExpectedAcceptance != expected {
		t.Fatalf("spec.ExpectedAcceptance = %v, want %q", spec.ExpectedAcceptance, expected)
	}
}

// TestIfAcceptanceGuardOnlySpecIsNotAWrite pins that a spec whose only content
// is the acceptance guard does not count as a write, so a hook does not fire
// for a guard that merely checked the row. A spec that also writes still does.
func TestIfAcceptanceGuardOnlySpecIsNotAWrite(t *testing.T) {
	expected := "- [ ] builds"
	if specWrites(domain.UpdateSpec{ExpectedAcceptance: &expected}) {
		t.Error("specWrites(guard only) = true, want false")
	}
	if !specWrites(domain.UpdateSpec{
		Fields:             map[string]any{"acceptance_criteria": "- [x] builds"},
		ExpectedAcceptance: &expected,
	}) {
		t.Error("specWrites(guard with a field write) = false, want true")
	}
}

// TestIfAcceptanceMismatchStandsDownTheAssigneeFence pins that a stale
// --if-acceptance is a precondition that does not hold, so the assignee fence
// stands down and ApplyUpdate reports the mismatch instead.
func TestIfAcceptanceMismatchStandsDownTheAssigneeFence(t *testing.T) {
	before := &types.Issue{AcceptanceCriteria: "- [x] builds"}
	stale := "- [ ] builds"
	current := "- [x] builds"
	tests := []struct {
		name  string
		guard string
		want  bool
	}{
		{name: "matching guard holds the preconditions", guard: current, want: true},
		{name: "stale guard does not hold the preconditions", guard: stale, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := tt.guard
			request := issueops.UpdateRequest{ExpectedAcceptance: &guard}
			if got := updatePreconditionsHold(request, before); got != tt.want {
				t.Errorf("updatePreconditionsHold() = %v, want %v", got, tt.want)
			}
		})
	}
}
