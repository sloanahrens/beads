//go:build cgo

package main

import (
	"testing"

	"github.com/steveyegge/beads/internal/types"
)

func TestValidateIssueUpdatable(t *testing.T) {
	if err := validateIssueUpdatable("x", nil); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if err := validateIssueUpdatable("x", &types.Issue{IsTemplate: false}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if err := validateIssueUpdatable("bd-1", &types.Issue{IsTemplate: true}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidateIssueClosable(t *testing.T) {
	if err := validateIssueClosable("x", nil, "alice", false); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if err := validateIssueClosable("bd-1", &types.Issue{IsTemplate: true}, "alice", false); err == nil {
		t.Fatalf("expected template close error")
	}
	if err := validateIssueClosable("bd-2", &types.Issue{Status: types.StatusPinned}, "alice", false); err == nil {
		t.Fatalf("expected pinned close error")
	}
	if err := validateIssueClosable("bd-2", &types.Issue{Status: types.StatusPinned}, "alice", true); err != nil {
		t.Fatalf("expected pinned close to succeed with force, got %v", err)
	}

	// ga-z3vht: pinned=true protects the bead independently of status, so
	// `bd close` refuses it without --force on both the direct and proxied path.
	booleanPinned := &types.Issue{Status: types.StatusOpen, Pinned: true}
	if err := validateIssueClosable("bd-6", booleanPinned, "alice", false); err == nil {
		t.Fatalf("expected boolean-pinned close error")
	}
	if err := validateIssueClosable("bd-6", booleanPinned, "alice", true); err != nil {
		t.Fatalf("expected boolean-pinned close to succeed with force, got %v", err)
	}

	// ga-ktn9pe.4.8: a closed row carrying pinned=true is the residue left by a
	// forced close, and this guard still refuses it — closed status earns no
	// exemption from the boolean trigger. Idempotent re-close was restored by
	// ORDERING instead: cmd/bd/close.go and close_proxied_server.go skip close
	// validation entirely for a row already closed at resolve time, so this guard
	// is never reached on the no-op retry. Do not "simplify" it by exempting
	// closed here — that would also disarm the guard on live status transitions.
	closedPinnedResidue := &types.Issue{Status: types.StatusClosed, Pinned: true}
	if err := validateIssueClosable("bd-7", closedPinnedResidue, "alice", false); err == nil {
		t.Fatalf("expected closed+pinned residue to refuse a plain close")
	}
	if err := validateIssueClosable("bd-7", closedPinnedResidue, "alice", true); err != nil {
		t.Fatalf("expected closed+pinned residue close to succeed with force, got %v", err)
	}

	// be-035: actor != assignee must be refused without --force.
	mismatched := &types.Issue{Assignee: "bob"}
	if err := validateIssueClosable("bd-3", mismatched, "alice", false); err == nil {
		t.Fatalf("expected actor/assignee mismatch error")
	}
	// --force overrides the authority check.
	if err := validateIssueClosable("bd-3", mismatched, "alice", true); err != nil {
		t.Fatalf("expected close to succeed with force despite mismatch, got %v", err)
	}
	// Same-actor close is allowed.
	if err := validateIssueClosable("bd-4", &types.Issue{Assignee: "alice"}, "alice", false); err != nil {
		t.Fatalf("expected matching-assignee close to succeed, got %v", err)
	}
	// Unassigned beads can be closed by anyone (lots of bd's flow involves
	// closing beads nobody claimed).
	if err := validateIssueClosable("bd-5", &types.Issue{Assignee: ""}, "alice", false); err != nil {
		t.Fatalf("expected unassigned close to succeed, got %v", err)
	}
}
