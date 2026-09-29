package protocol

import (
	"strings"
	"testing"
)

// TestProtocol_DeferPastDateWarns asserts that bd defer --until with a past
// date warns the user. The bd update --defer path already warns; bd defer
// should be consistent.
func TestProtocol_DeferPastDateWarns(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	id := w.create("--title", "Defer target", "--type", "task")

	out, _ := w.tryRun("defer", id, "--until=2020-01-01")
	if !strings.Contains(out, "past") {
		t.Errorf("bd defer --until=<past date> should warn about past date:\n%s", out)
	}
}

// TestProtocol_DeferReasonAppendsNotes asserts that `bd defer --reason`
// records the audit context at the same time as the deferral.
func TestProtocol_DeferReasonAppendsNotes(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	id := w.create("--title", "Reasoned defer", "--type", "task", "--notes", "existing context")

	w.run("defer", id, "--until=2099-12-31", "--reason", "blocked on external API availability")

	issue := w.showJSON(id)
	assertField(t, issue, "status", "deferred")
	assertField(t, issue, "notes", "existing context\nblocked on external API availability")
}

// TestProtocol_DeferRejectsEmptyReason keeps whitespace-only reasons from
// creating ambiguous blank audit entries.
func TestProtocol_DeferRejectsEmptyReason(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	id := w.create("--title", "Blank reason", "--type", "task")

	if _, err := w.tryRun("defer", id, "--reason", "   "); err == nil {
		t.Error("bd defer --reason with whitespace-only text should fail but exited 0")
	}
}

// TestProtocol_UndeferPartialFailureExitsNonZero pins the batch rule for
// defer/undefer: any id that fails makes the command exit non-zero, while
// the ids that could be undeferred still are. It used to exit 0 even when
// every id failed (B1-02).
func TestProtocol_UndeferPartialFailureExitsNonZero(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	deferred := w.create("--title", "Deferred", "--type", "task")
	open := w.create("--title", "Still open", "--type", "task")
	w.run("defer", deferred)

	_, code := w.runExpectError("undefer", deferred, open)
	if code != 1 {
		t.Errorf("undefer with a not-deferred id: exit %d, want 1", code)
	}
	assertField(t, w.showJSON(deferred), "status", "open")

	// Every id failing is a failure too.
	if _, code := w.runExpectError("undefer", open); code != 1 {
		t.Errorf("undefer of only not-deferred ids: exit %d, want 1", code)
	}
}
