package issueops

import (
	"errors"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// SnapshotUnsetCreateIDs records which IDs in issues are empty at call time and
// returns a reset that empties exactly those fields again. A create fills the
// caller's structs with the IDs it mints; once its transaction rolls back those
// rows do not exist, so leaving the minted IDs in place makes a retry of the
// same struct upsert a rolled-back ID instead of minting afresh (GH#8457). The
// reset hands the structs back the way the caller passed them in.
//
// Only fields that were empty at snapshot time are recorded, so an ID the
// caller supplied itself is never touched. Take the snapshot once, before the
// retry loop, never inside it: a snapshot taken after a failed attempt would
// see the IDs that attempt minted as caller values and would no longer clear
// them. Nil issues, dependencies and comments are skipped.
func SnapshotUnsetCreateIDs(issues []*types.Issue) (reset func()) {
	var unset []*string
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		if issue.ID == "" {
			unset = append(unset, &issue.ID)
		}
		for _, dep := range issue.Dependencies {
			if dep != nil && dep.IssueID == "" {
				unset = append(unset, &dep.IssueID)
			}
		}
		for _, comment := range issue.Comments {
			if comment != nil && comment.ID == "" {
				unset = append(unset, &comment.ID)
			}
		}
	}
	return func() {
		for _, field := range unset {
			*field = ""
		}
	}
}

// ResetCreateIDsIfRolledBack returns err unchanged and calls reset when err
// reports a create that did not land, so the caller's structs are handed back
// as they were passed in and a retry of the same struct mints fresh IDs instead
// of upserting rolled-back ones (GH#8457).
//
// A nil error means the rows were written and their IDs are kept. An error that
// matches storage.ErrCommitIndeterminate, directly or wrapped in errors.Join,
// means the commit may have landed, so the IDs are kept as well. Take the
// snapshot that reset restores once, before the retry loop, never inside it.
func ResetCreateIDsIfRolledBack(err error, reset func()) error {
	if err == nil || reset == nil || errors.Is(err, storage.ErrCommitIndeterminate) {
		return err
	}
	reset()
	return err
}
