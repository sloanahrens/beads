package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

// recordingMolWriter records the writes bondProtoProtoInto makes without a
// database. It embeds the molReader INTERFACE rather than implementing it: the
// bond path under test performs no reads, so a nil reader is the honest
// declaration that a read here would be a new dependency, and it panics loudly
// rather than quietly answering zero if one ever appears.
type recordingMolWriter struct {
	molReader
	created []*types.Issue
	deps    []*types.Dependency
	seq     int
}

func (w *recordingMolWriter) CreateIssue(_ context.Context, issue *types.Issue, _ string) error {
	if issue.ID == "" {
		issue.ID = "cmp-1"
	}
	clone := *issue
	clone.Labels = append([]string(nil), issue.Labels...)
	w.created = append(w.created, &clone)
	return nil
}

// CreateIssues follows the store's batch contract: IDs are assigned in place
// before any issue is written, then each batch:<index> dependency end is
// resolved to the ID of issues[index], and each issue is recorded as
// CreateIssue records it.
func (w *recordingMolWriter) CreateIssues(ctx context.Context, issues []*types.Issue, actor string) error {
	for _, issue := range issues {
		if issue.ID == "" {
			w.seq++
			issue.ID = fmt.Sprintf("cmp-%d", w.seq)
		}
	}
	for _, issue := range issues {
		for _, dep := range issue.Dependencies {
			for _, end := range []*string{&dep.IssueID, &dep.DependsOnID} {
				digits, isRef := strings.CutPrefix(*end, "batch:")
				if !isRef {
					continue
				}
				index, err := strconv.Atoi(digits)
				if err != nil || index < 0 || index >= len(issues) {
					return fmt.Errorf("dependency ref %q is not a position in this %d-issue batch", *end, len(issues))
				}
				*end = issues[index].ID
			}
		}
	}
	for _, issue := range issues {
		if err := w.CreateIssue(ctx, issue, actor); err != nil {
			return err
		}
	}
	return nil
}

func (w *recordingMolWriter) AddDependency(_ context.Context, dep *types.Dependency, _ string) error {
	w.deps = append(w.deps, dep)
	return nil
}

func (w *recordingMolWriter) UpdateIssue(context.Context, string, map[string]interface{}, string) error {
	panic("UpdateIssue: not part of the proto+proto bond")
}
func (w *recordingMolWriter) CloseIssue(context.Context, string, string, string) error {
	panic("CloseIssue: not part of the proto+proto bond")
}
func (w *recordingMolWriter) DeleteIssue(context.Context, string, string) error {
	panic("DeleteIssue: not part of the proto+proto bond")
}
func (w *recordingMolWriter) SetConfig(context.Context, string, string) error {
	panic("SetConfig: not part of the proto+proto bond")
}
func (w *recordingMolWriter) ClaimStepIfOpen(context.Context, string, string) error {
	panic("ClaimStepIfOpen: not part of the proto+proto bond")
}

// TestBondProtoProtoLabelsTheCompoundAtCreation pins the property that makes
// the compound's molecule label unable to land in the wrong plane: it travels
// in the SAME create that decides which table the compound row itself goes to.
//
// The label used to be a second write — molWriter.AddLabel — and each
// implementation of that method picked the label's plane again, with its own
// predicate: storeMolWriter deferred to the storage layer's isActiveWisp, while
// uowMolWriter consulted a cache it filled from IssueUseCase().GetWisp. Two
// predicates for one fact is the shape a row and its labels come apart in. One
// create makes the disagreement unrepresentable rather than unlikely.
func TestBondProtoProtoLabelsTheCompoundAtCreation(t *testing.T) {
	w := &recordingMolWriter{}
	protoA := &types.Issue{ID: "bd-a", Title: "Proto A", Priority: 2}
	protoB := &types.Issue{ID: "bd-b", Title: "Proto B", Priority: 1}

	result, err := bondProtoProtoInto(context.Background(), w, protoA, protoB, types.BondTypeSequential, "", "tester")
	if err != nil {
		t.Fatalf("bondProtoProtoInto: %v", err)
	}

	if len(w.created) != 1 {
		t.Fatalf("created %d issues, want exactly the compound", len(w.created))
	}
	compound := w.created[0]
	if compound.ID != result.ResultID {
		t.Errorf("result names %s but the create made %s", result.ResultID, compound.ID)
	}
	if !containsLabel(compound.Labels, MoleculeLabel) {
		t.Errorf("compound created with labels %v, want the create itself to carry %q", compound.Labels, MoleculeLabel)
	}
}

func containsLabel(labels []string, want string) bool {
	for _, l := range labels {
		if l == want {
			return true
		}
	}
	return false
}
