package issueops

import (
	"errors"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	publicops "github.com/steveyegge/beads/issueops"
)

// TestIfAcceptanceRefusesClaim pins that the acceptance guard cannot ride on a
// claim, which is its own compare-and-set. The refusal is a validation error
// raised before any row is touched.
func TestIfAcceptanceRefusesClaim(t *testing.T) {
	expected := ""
	request := publicops.UpdateRequest{Actor: "a", IssueID: "bd-1", Claim: true, ExpectedAcceptance: &expected}
	err := ValidateUpdateRequest(request)
	if !errors.Is(err, storage.ErrValidation) {
		t.Fatalf("ValidateUpdateRequest(claim + acceptance guard) = %v, want ErrValidation", err)
	}
	request.Claim = false
	if err := ValidateUpdateRequest(request); err != nil {
		t.Errorf("ValidateUpdateRequest(acceptance guard without claim) = %v, want nil", err)
	}
}

func TestIfAcceptanceMismatch(t *testing.T) {
	tests := []struct {
		name     string
		stored   string
		expected *string
		wantErr  bool
	}{
		{name: "nil guard is not checked", stored: "- [ ] a", expected: nil, wantErr: false},
		{name: "matching text writes", stored: "- [ ] a", expected: strPtr("- [ ] a"), wantErr: false},
		{name: "stale text refuses", stored: "- [x] a", expected: strPtr("- [ ] a"), wantErr: true},
		{name: "empty guard matches empty field", stored: "", expected: strPtr(""), wantErr: false},
		{name: "empty guard refuses a non-empty field", stored: "- [ ] a", expected: strPtr(""), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := expectedAcceptanceMismatch("bd-1", tt.stored, tt.expected)
			if (err != nil) != tt.wantErr {
				t.Fatalf("expectedAcceptanceMismatch err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, storage.ErrAcceptanceMismatch) {
				t.Errorf("err = %v, want it to wrap ErrAcceptanceMismatch", err)
			}
		})
	}
}
