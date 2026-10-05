package main

import (
	"strings"
	"testing"

	"github.com/steveyegge/beads/issueops"
)

// specDispatchedDescription is the shape the spec dispatcher writes: the
// bead's criteria are a '## Acceptance' section of the description and the
// acceptance_criteria column is empty.
const specDispatchedDescription = "## Goal\n\nclose the gap\n\n## Constraints\n\nnone\n\n## Acceptance\n\n- [ ] make check pass\n- [ ] docs updated\n"

func TestRewriteAcceptanceSectionRewritesTheSectionBody(t *testing.T) {
	got, err := rewriteAcceptanceSection(specDispatchedDescription, "- [x] make check pass\n- [ ] docs updated")
	if err != nil {
		t.Fatalf("rewriteAcceptanceSection: %v", err)
	}
	want := "## Goal\n\nclose the gap\n\n## Constraints\n\nnone\n\n## Acceptance\n\n- [x] make check pass\n- [ ] docs updated\n"
	if got != want {
		t.Errorf("rewritten description:\n got %q\nwant %q", got, want)
	}
}

func TestRewriteAcceptanceSectionLeavesOtherSectionsAlone(t *testing.T) {
	description := "## Goal\n\ng\n\n## Acceptance\n\n- [ ] old\n\n## Notes\n\nkeep me\n"
	got, err := rewriteAcceptanceSection(description, "- [x] new")
	if err != nil {
		t.Fatalf("rewriteAcceptanceSection: %v", err)
	}
	want := "## Goal\n\ng\n\n## Acceptance\n\n- [x] new\n\n## Notes\n\nkeep me\n"
	if got != want {
		t.Errorf("rewritten description:\n got %q\nwant %q", got, want)
	}
}

func TestRewriteAcceptanceSectionWithoutASectionIsANoOp(t *testing.T) {
	description := "## Goal\n\ng\n"
	got, err := rewriteAcceptanceSection(description, "- [x] new")
	if err != nil {
		t.Fatalf("rewriteAcceptanceSection: %v", err)
	}
	if got != description {
		t.Errorf("description changed without an acceptance section: %q", got)
	}
}

func TestRewriteAcceptanceSectionRecognizesHeadingSpellings(t *testing.T) {
	for _, heading := range []string{
		"## Acceptance",
		"## Acceptance Criteria",
		"## acceptance criteria",
		"##  Acceptance  ",
		"## Acceptance ##",
		"## Acceptance:",
	} {
		t.Run(heading, func(t *testing.T) {
			description := heading + "\n\n- [ ] old\n"
			got, err := rewriteAcceptanceSection(description, "- [x] new")
			if err != nil {
				t.Fatalf("rewriteAcceptanceSection: %v", err)
			}
			if !strings.HasSuffix(got, "- [x] new\n") || strings.Contains(got, "- [ ] old") {
				t.Errorf("%q was not treated as an acceptance section: %q", heading, got)
			}
		})
	}
}

func TestRewriteAcceptanceSectionKeepsDeeperHeadingsInTheBody(t *testing.T) {
	description := "## Acceptance\n\n- [ ] old\n\n### Details\n\nstill the criteria\n"
	got, err := rewriteAcceptanceSection(description, "- [x] new")
	if err != nil {
		t.Fatalf("rewriteAcceptanceSection: %v", err)
	}
	want := "## Acceptance\n\n- [x] new\n"
	if got != want {
		t.Errorf("a '###' heading did not stay inside the section:\n got %q\nwant %q", got, want)
	}
}

func TestRewriteAcceptanceSectionRefusesAmbiguousDescriptions(t *testing.T) {
	description := "## Acceptance\n\n- [ ] one\n\n## Acceptance Criteria\n\n- [ ] two\n"
	got, err := rewriteAcceptanceSection(description, "- [x] new")
	if err == nil {
		t.Fatalf("expected two acceptance sections to be refused, got %q", got)
	}
	if !strings.Contains(err.Error(), "2 '## Acceptance' sections") {
		t.Errorf("refusal does not name the ambiguity: %v", err)
	}
}

func TestRewriteAcceptanceSectionRefusesCriteriaThatWouldEndTheSection(t *testing.T) {
	if _, err := rewriteAcceptanceSection(specDispatchedDescription, "- [x] one\n## Notes\n- [ ] two"); err == nil {
		t.Fatal("expected criteria carrying a level 2 heading to be refused")
	}
}

func TestRewriteAcceptanceSectionIsIdempotent(t *testing.T) {
	criteria := "- [x] make check pass\n- [ ] docs updated"
	once, err := rewriteAcceptanceSection(specDispatchedDescription, criteria)
	if err != nil {
		t.Fatalf("rewriteAcceptanceSection: %v", err)
	}
	twice, err := rewriteAcceptanceSection(once, criteria)
	if err != nil {
		t.Fatalf("rewriteAcceptanceSection: %v", err)
	}
	if twice != once {
		t.Errorf("rewriting the same criteria twice moved the description:\n once %q\ntwice %q", once, twice)
	}
}

func TestRewriteAcceptanceSectionHandlesCRLF(t *testing.T) {
	description := "## Goal\r\n\r\ng\r\n\r\n## Acceptance\r\n\r\n- [ ] old\r\n"
	got, err := rewriteAcceptanceSection(description, "- [x] new")
	if err != nil {
		t.Fatalf("rewriteAcceptanceSection: %v", err)
	}
	if strings.Contains(got, "- [ ] old") || !strings.Contains(got, "- [x] new") {
		t.Errorf("CRLF description's acceptance section was not rewritten: %q", got)
	}
}

func TestSyncAcceptanceSectionCarriesTheDescription(t *testing.T) {
	patch := issueops.IssuePatch{AcceptanceCriteria: setField("- [x] make check pass\n- [x] docs updated")}
	got, err := syncAcceptanceSection(patch, specDispatchedDescription)
	if err != nil {
		t.Fatalf("syncAcceptanceSection: %v", err)
	}
	if !got.Description.Set {
		t.Fatal("acceptance write did not carry the description's acceptance section")
	}
	if strings.Contains(got.Description.Value, "- [ ]") {
		t.Errorf("description still carries unchecked criteria: %q", got.Description.Value)
	}
	if !strings.Contains(got.Description.Value, "- [x] make check pass") {
		t.Errorf("description does not carry the written criteria: %q", got.Description.Value)
	}
}

func TestSyncAcceptanceSectionLeavesTheDescriptionAlone(t *testing.T) {
	tests := []struct {
		name        string
		patch       issueops.IssuePatch
		description string
	}{
		{
			name:        "no acceptance write",
			patch:       issueops.IssuePatch{},
			description: specDispatchedDescription,
		},
		{
			name: "explicit description wins",
			patch: issueops.IssuePatch{
				AcceptanceCriteria: setField("- [x] new"),
				Description:        setField("## Goal\n\nrewritten by the caller\n"),
			},
			description: specDispatchedDescription,
		},
		{
			name:        "no acceptance section to keep in step",
			patch:       issueops.IssuePatch{AcceptanceCriteria: setField("- [x] new")},
			description: "## Goal\n\nno criteria section here\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := syncAcceptanceSection(tt.patch, tt.description)
			if err != nil {
				t.Fatalf("syncAcceptanceSection: %v", err)
			}
			if got.Description != tt.patch.Description {
				t.Errorf("description write = %+v, want the patch's own %+v", got.Description, tt.patch.Description)
			}
		})
	}
}
