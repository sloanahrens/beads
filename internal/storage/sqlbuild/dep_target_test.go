package sqlbuild

import (
	"reflect"
	"strings"
	"testing"
)

func TestDepTargetSelectSQL(t *testing.T) {
	t.Parallel()

	got := DepTargetSelectSQL("dependencies", "issue_id", "%s = ?")
	want := strings.Join([]string{
		"SELECT issue_id FROM dependencies WHERE depends_on_issue_id = ?",
		"SELECT issue_id FROM dependencies WHERE depends_on_wisp_id = ?",
		"SELECT issue_id FROM dependencies WHERE depends_on_external = ?",
	}, "\n\t\t\t\tUNION ALL\n\t\t\t\t")
	if got != want {
		t.Fatalf("DepTargetSelectSQL:\n got: %q\nwant: %q", got, want)
	}

	// The branch order IS the COALESCE precedence order, and every %s in the
	// project and predicate is substituted (the rc subquery uses both).
	projPred := DepTargetSelectSQL("wisp_dependencies", "%s AS dep_id",
		"type = 'blocks' AND %s IN (?,?)")
	for i, col := range DepTargetColumns {
		pred := "SELECT " + col + " AS dep_id FROM wisp_dependencies WHERE type = 'blocks' AND " +
			col + " IN (?,?)"
		if !strings.Contains(projPred, pred) {
			t.Errorf("branch %d missing exact per-column predicate %q:\n%s", i, pred, projPred)
		}
	}
	if strings.Contains(projPred, "%s") {
		t.Errorf("unsubstituted %%s left in rendered SQL:\n%s", projPred)
	}
}

func TestDepTargetCountSQL(t *testing.T) {
	t.Parallel()

	got := DepTargetCountSQL("dependencies", "%s = ?")
	for _, want := range []string{
		"SELECT count(*) FROM (",
		"SELECT 1 FROM dependencies WHERE depends_on_issue_id = ?",
		"SELECT 1 FROM dependencies WHERE depends_on_wisp_id = ?",
		"SELECT 1 FROM dependencies WHERE depends_on_external = ?",
		") dep_targets",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("DepTargetCountSQL missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "COALESCE") {
		t.Errorf("count must not predicate on the unindexable COALESCE target:\n%s", got)
	}
	// Three placeholders, one per branch: callers bind the target once per
	// typed column.
	if n := strings.Count(got, "?"); n != len(DepTargetColumns) {
		t.Errorf("placeholders = %d, want %d:\n%s", n, len(DepTargetColumns), got)
	}
}

func TestDepTargetBranchArgs(t *testing.T) {
	t.Parallel()

	got := DepTargetBranchArgs("target", "blocks")
	want := []any{"target", "blocks", "target", "blocks", "target", "blocks"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DepTargetBranchArgs = %v, want %v", got, want)
	}
	if n := len(DepTargetBranchArgs("x")); n != len(DepTargetColumns) {
		t.Fatalf("one-arg branch renders %d bindings, want %d", n, len(DepTargetColumns))
	}
}
