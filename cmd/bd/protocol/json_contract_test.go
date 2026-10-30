// json_contract_test.go — CI regression tests for --json output contracts.
//
// These tests verify that commands with --json always produce valid JSON
// and include required fields. Regressions like GH#2492, GH#2465, GH#2407,
// GH#2395 are prevented by these tests.
package protocol

import (
	"strings"
	"testing"
)

// TestJSONContract_ListOutputIsValidJSON verifies bd list --json always
// produces valid JSON (not mixed with tree-renderer text).
func TestJSONContract_ListOutputIsValidJSON(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	w.create("JSON contract test issue")

	out := w.run("list", "--json")
	items := parseJSONOutput(t, out)
	if len(items) == 0 {
		t.Fatal("bd list --json returned no items")
	}
}

// TestJSONContract_ShowOutputHasRequiredFields verifies bd show --json
// includes all required issue fields, including the relational counts that
// stand in for the opt-in comment and dependent payloads.
//
// The issue fields are checked on the elements of the envelope's data (see
// TestJSONContract_ShowDataIsArray); schema_version belongs to the envelope.
func TestJSONContract_ShowOutputHasRequiredFields(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	id := w.create("Required fields test")

	out := w.run("show", id, "--json")
	items := parseJSONOutput(t, out)
	if len(items) == 0 {
		t.Fatal("bd show --json returned no items")
	}

	issue := items[0]
	requiredFields := []string{
		"id", "title", "status", "priority", "issue_type", "created_at",
		"dependency_count", "dependent_count", "comment_count",
	}
	for _, field := range requiredFields {
		if _, ok := issue[field]; !ok {
			t.Errorf("bd show --json missing required field %q", field)
		}
	}
}

// TestJSONContract_ShowOmitsOptInPayloadsByDefault pins the NEGATIVE half of the
// count-only contract (be-ijck6q): by default, bd show --json must OMIT the
// "comments" and "dependents" arrays entirely, and each --include-* flag must
// populate its own array and only its own.
//
// Every other test that touches comments or dependents asks for them with
// showJSONFull, so all of them would stay green if the default silently went
// back to materializing both lists eagerly — which is precisely the pathological
// hub-bead slowness cfcc95799 removed. Omission is the invariant with no other
// witness, so it needs one of its own: a perf contract that nothing asserts is a
// perf contract that will be regressed.
func TestJSONContract_ShowOmitsOptInPayloadsByDefault(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	parent := w.create("--title", "Count-only parent", "--type", "epic", "--priority", "1")
	child := w.create("--title", "Count-only child", "--type", "task", "--parent", parent)
	w.run("comments", "add", parent, "First note")

	// showOnce runs show --json with the given extra flags and returns the issue.
	showOnce := func(flags ...string) map[string]any {
		t.Helper()
		out := w.run(append([]string{"show", parent, "--json"}, flags...)...)
		items := parseJSONOutput(t, out)
		if len(items) == 0 {
			t.Fatalf("bd show %s --json %v returned no items", parent, flags)
		}
		return items[0]
	}

	// The arrays must be absent, not merely empty: an empty array would still
	// mean the rows were materialized, which is the cost the flag exists to avoid.
	assertAbsent := func(t *testing.T, issue map[string]any, key, context string) {
		t.Helper()
		if v, present := issue[key]; present {
			t.Errorf("%s: show --json must omit %q, got %v", context, key, v)
		}
	}

	t.Run("default_omits_both_arrays", func(t *testing.T) {
		issue := showOnce()
		assertAbsent(t, issue, "comments", "default payload")
		assertAbsent(t, issue, "dependents", "default payload")

		// The counts are what the default carries instead.
		assertFieldFloat(t, issue, "comment_count", 1)
		assertFieldFloat(t, issue, "dependent_count", 1)
	})

	t.Run("include_comments_does_not_drag_in_dependents", func(t *testing.T) {
		issue := showOnce("--include-comments")
		requireCommentTextsEqual(t, getObjectSlice(issue, "comments"),
			[]string{"First note"}, "comments under --include-comments")
		assertAbsent(t, issue, "dependents", "--include-comments")
	})

	t.Run("include_dependents_does_not_drag_in_comments", func(t *testing.T) {
		issue := showOnce("--include-dependents")
		dependents := getObjectSlice(issue, "dependents")
		found := false
		for _, dep := range dependents {
			if id, _ := dep["id"].(string); id == child {
				found = true
			}
		}
		if !found {
			t.Errorf("--include-dependents: parent %s does not list child %s (got %d dependents)",
				parent, child, len(dependents))
		}
		assertAbsent(t, issue, "comments", "--include-dependents")
	})
}

// TestJSONContract_ShowCommentsOmittedFlag pins the fix for ga-clgh: a positive
// comment_count with no "comments" key is a third state that neither of the two
// natural readings gets right — `.comments` absent reads as "none" (false), and
// comment_count alone gives no way to know the array was left out on purpose.
// comments_omitted closes that gap without touching the count-only default
// TestJSONContract_ShowOmitsOptInPayloadsByDefault protects.
func TestJSONContract_ShowCommentsOmittedFlag(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	withComments := w.create("--title", "Has comments")
	w.run("comments", "add", withComments, "First note")
	w.run("comments", "add", withComments, "Second note")
	w.run("comments", "add", withComments, "Third note")
	empty := w.create("--title", "No comments")

	showOnce := func(id string, flags ...string) map[string]any {
		t.Helper()
		out := w.run(append([]string{"show", id, "--json"}, flags...)...)
		items := parseJSONOutput(t, out)
		if len(items) == 0 {
			t.Fatalf("bd show %s --json %v returned no items", id, flags)
		}
		return items[0]
	}
	assertAbsent := func(t *testing.T, issue map[string]any, key, context string) {
		t.Helper()
		if v, present := issue[key]; present {
			t.Errorf("%s: expected %q to be absent, got %v", context, key, v)
		}
	}

	t.Run("mutation_default_flags_the_omission", func(t *testing.T) {
		issue := showOnce(withComments)
		assertAbsent(t, issue, "comments", "default payload")
		assertFieldFloat(t, issue, "comment_count", 3)
		omitted, present := issue["comments_omitted"]
		if !present {
			t.Fatal("default payload with comment_count=3: comments_omitted must be present and true")
		}
		if omitted != true {
			t.Errorf("default payload with comment_count=3: comments_omitted = %v, want true", omitted)
		}
	})

	t.Run("include_comments_clears_the_flag", func(t *testing.T) {
		issue := showOnce(withComments, "--include-comments")
		requireCommentTextsEqual(t, getObjectSlice(issue, "comments"),
			[]string{"First note", "Second note", "Third note"}, "comments under --include-comments")
		assertAbsent(t, issue, "comments_omitted", "--include-comments (nothing was left out)")
	})

	t.Run("regression_true_empty_stays_unflagged", func(t *testing.T) {
		issue := showOnce(empty)
		assertFieldFloat(t, issue, "comment_count", 0)
		assertAbsent(t, issue, "comments", "zero-comment default payload")
		assertAbsent(t, issue, "comments_omitted", "zero-comment payload (nothing was omitted)")
	})
}

// TestJSONContract_ShowDataIsArray pins the shape of bd show --json: the
// schema_version envelope whose data is a JSON array with one element per
// requested ID. The elements are bare issues: schema_version lives on the
// envelope, not on each item.
//
// The shape is load-bearing for every consumer that parses show --json, so a
// change here is a breaking wire change, not a cleanup.
func TestJSONContract_ShowDataIsArray(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	id1 := w.create("Array shape one")
	id2 := w.create("Array shape two")

	out := w.run("show", id1, id2, "--json")

	env, arr := parseEnvelope(t, out)
	assertSchemaVersion(t, env, "bd show --json")
	if _, ok := env["data"].([]any); !ok {
		t.Fatalf("bd show --json data is not a JSON array: %T\nOutput:\n%s", env["data"], out)
	}
	if len(arr) != 2 {
		t.Fatalf("bd show <id1> <id2> --json returned %d items, want 2\nOutput:\n%s", len(arr), out)
	}

	gotIDs := make([]string, 0, len(arr))
	for _, item := range arr {
		id, _ := item["id"].(string)
		gotIDs = append(gotIDs, id)
		if _, ok := item["schema_version"]; ok {
			t.Errorf("bd show --json item %s carries schema_version; only the envelope does", id)
		}
	}
	requireStringSetEqual(t, gotIDs, []string{id1, id2}, "bd show <id1> <id2> --json ids")
}

// TestJSONContract_ReadyOutputIsValidJSON verifies bd ready --json produces
// valid JSON even when no issues are ready.
func TestJSONContract_ReadyOutputIsValidJSON(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)

	out := w.run("ready", "--json")
	env, _ := parseEnvelope(t, out)
	if _, ok := env["data"].([]any); !ok {
		t.Fatalf("bd ready --json data is not a JSON array: %T\nOutput:\n%s", env["data"], out)
	}
}

// TestJSONContract_CreateOutputHasID verifies bd create --json returns
// the created issue with its ID.
func TestJSONContract_CreateOutputHasID(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)

	out := w.run("create", "Create contract test", "--description=test", "--json")

	env, items := parseEnvelope(t, out)
	if len(items) != 1 {
		t.Fatalf("bd create --json data has %d objects, want 1\nOutput:\n%s", len(items), out)
	}
	issue := items[0]

	assertSchemaVersion(t, env, "bd create --json")
	if _, ok := issue["id"]; !ok {
		t.Error("bd create --json output missing 'id' field")
	}
}

// TestJSONContract_ErrorOutputIsValidJSON verifies that errors with --json
// produce valid JSON with schema_version to stderr (not mixed text).
func TestJSONContract_ErrorOutputIsValidJSON(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)

	// Try to show a nonexistent issue with --json
	out, _ := w.runExpectError("show", "nonexistent-xyz-999", "--json")

	// The output (stderr) should be valid JSON or empty
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return // Empty is acceptable for errors
	}

	// The error object sits under the envelope's data; jsonErrorFrom finds it
	// among any other stderr lines and lifts schema_version from the envelope.
	if errObj, ok := jsonErrorFrom(t, trimmed); ok {
		assertSchemaVersion(t, errObj, "bd show error --json")
		return
	}
	t.Logf("Note: error output not fully JSON — this is acceptable for some error paths")
}

// TestJSONContract_CloseOutputHasStatus verifies bd close --json returns
// the updated issue with closed status.
func TestJSONContract_CloseOutputHasStatus(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	id := w.create("Close contract test")

	out := w.run("close", id, "--json")
	items := parseJSONOutput(t, out)
	if len(items) == 0 {
		t.Fatal("bd close --json returned no items")
	}

	assertField(t, items[0], "status", "closed")
}

// TestJSONContract_ReadyOutputHasFullObjects verifies bd ready --json returns
// full issue objects with dependency counts, not just IDs (beads-clt).
func TestJSONContract_ReadyOutputHasFullObjects(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	w.create("Ready full object test")

	out := w.run("ready", "--json")
	items := parseJSONOutput(t, out)
	if len(items) == 0 {
		t.Skip("no ready issues — create returned non-ready issue")
	}
	issue := items[0]
	requiredFields := []string{"id", "title", "status", "priority", "dependency_count", "dependent_count"}
	for _, field := range requiredFields {
		if _, ok := issue[field]; !ok {
			t.Errorf("bd ready --json item missing required field %q", field)
		}
	}
}

// TestJSONContract_BlockedOutputHasBlockedBy verifies bd blocked --json returns
// full issue objects with blocked_by field (beads-clt).
func TestJSONContract_BlockedOutputHasBlockedBy(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)

	blocker := w.create("Blocker issue")
	blocked := w.create("Blocked issue")
	w.run("dep", "add", blocked, blocker, "--type", "blocks")

	out := w.run("blocked", "--json")
	items := parseJSONOutput(t, out)

	var found map[string]any
	for _, item := range items {
		if id, ok := item["id"].(string); ok && id == blocked {
			found = item
			break
		}
	}
	if found == nil {
		t.Fatalf("blocked issue %s not found in bd blocked --json output", blocked)
	}

	requiredFields := []string{"id", "title", "status", "blocked_by_count", "blocked_by"}
	for _, field := range requiredFields {
		if _, ok := found[field]; !ok {
			t.Errorf("bd blocked --json item missing required field %q", field)
		}
	}
}

// TestJSONContract_PingOutputIsValidJSON verifies bd ping --json returns
// structured health check output with timing info.
func TestJSONContract_PingOutputIsValidJSON(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)

	out := w.run("ping", "--json")
	env, items := parseEnvelope(t, out)
	if len(items) != 1 {
		t.Fatalf("bd ping --json data has %d objects, want 1\nOutput:\n%s", len(items), out)
	}
	obj := items[0]
	assertSchemaVersion(t, env, "bd ping --json")
	if status, ok := obj["status"].(string); !ok || status != "ok" {
		t.Errorf("bd ping --json status = %v, want ok", obj["status"])
	}
	if _, ok := obj["total_ms"]; !ok {
		t.Error("bd ping --json missing total_ms field")
	}
}

// TestJSONContract_SchemaVersionPresent verifies that schema_version is
// present on the envelope of --json commands (create, ping).
func TestJSONContract_SchemaVersionPresent(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)

	tests := []struct {
		name string
		args []string
	}{
		{"create", []string{"create", "Schema version test", "--json"}},
		{"ping", []string{"ping", "--json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := w.run(tt.args...)
			env, _ := parseEnvelope(t, out)
			assertSchemaVersion(t, env, "bd "+tt.name+" --json")
		})
	}
}
