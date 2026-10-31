package main

import (
	"testing"

	"github.com/steveyegge/beads/internal/storage"
)

// showBatchTestStore is a store handle with nothing behind it: the grouping
// below compares handles, it never reads through them.
type showBatchTestStore struct{ storage.DoltStorage }

// TestShowGetManyRequestCarriesEveryFlag is the tripwire on the one hop that
// turns the show flags into the reader's batch request. A dropped flag here
// changes the answer for every id in the list at once and is invisible
// everywhere else.
func TestShowGetManyRequestCarriesEveryFlag(t *testing.T) {
	ids := []string{"be-1", "be-2"}
	req := showGetManyRequest(ids, true, true, true)
	if len(req.IDs) != 2 || req.IDs[0] != "be-1" || req.IDs[1] != "be-2" {
		t.Errorf("IDs = %v, want the ids in request order", req.IDs)
	}
	if !req.IncludeDependents {
		t.Error("IncludeDependents was dropped")
	}
	if !req.IncludeComments {
		t.Error("IncludeComments was dropped")
	}
	if !req.BriefDeps {
		t.Error("BriefDeps was dropped")
	}

	off := showGetManyRequest(ids, false, false, false)
	if off.IncludeDependents || off.IncludeComments || off.BriefDeps {
		t.Errorf("flags were set on a request built without them: %+v", off)
	}
}

// TestAppendToShowBatchGroupGroupsByStoreInRequestOrder pins the projection
// between "these ids resolved, in this order" and "one read per store": the
// output order is the caller's, and every id that resolved against one store
// travels in one request.
func TestAppendToShowBatchGroupGroupsByStoreInRequestOrder(t *testing.T) {
	local := &showBatchTestStore{}
	routed := &showBatchTestStore{}

	var groups []showBatchGroup
	for _, r := range []struct {
		store storage.DoltStorage
		id    string
	}{
		{local, "be-1"},
		{routed, "hr-1"},
		{local, "be-2"},
		{local, "be-1"}, // a repeated id is answered twice, so it is listed twice
	} {
		groups = appendToShowBatchGroup(groups, r.store, r.id)
	}

	if len(groups) != 2 {
		t.Fatalf("groups = %d, want one per store", len(groups))
	}
	if !sameStoreHandle(groups[0].store, local) {
		t.Errorf("group 0 is not the store its first id came from")
	}
	if got := groups[0].ids; len(got) != 3 || got[0] != "be-1" || got[1] != "be-2" || got[2] != "be-1" {
		t.Errorf("local group ids = %v, want the local ids in request order, repeats included", got)
	}
	if !sameStoreHandle(groups[1].store, routed) {
		t.Errorf("group 1 is not the routed store")
	}
	if got := groups[1].ids; len(got) != 1 || got[0] != "hr-1" {
		t.Errorf("routed group ids = %v, want just hr-1", got)
	}

	// A store that repeats after another store's id still joins its own group
	// rather than opening a second one — the whole point of grouping.
	groups = appendToShowBatchGroup(groups, local, "be-3")
	if len(groups) != 2 {
		t.Fatalf("groups = %d after a returning store, want 2", len(groups))
	}
	if got := groups[0].ids; len(got) != 4 || got[3] != "be-3" {
		t.Errorf("local group ids = %v, want be-3 appended", got)
	}
}

func TestSameStoreHandle(t *testing.T) {
	a := &showBatchTestStore{}
	b := &showBatchTestStore{}
	if !sameStoreHandle(a, a) {
		t.Error("a store is not itself")
	}
	if sameStoreHandle(a, b) {
		t.Error("two distinct handles compared equal")
	}
	if sameStoreHandle(a, nil) || sameStoreHandle(nil, a) {
		t.Error("a store compared equal to nil")
	}
	if !sameStoreHandle(nil, nil) {
		t.Error("nil did not compare equal to nil")
	}
}
