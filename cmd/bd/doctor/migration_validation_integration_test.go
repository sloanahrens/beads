//go:build cgo && integration

package doctor

import (
	"context"
	"testing"
)

func TestCategorizeDoltExtras_AllForeign(t *testing.T) {
	ctx := context.Background()
	store := newTestDoltStore(t, "bd")

	// Create local issues via store
	for _, id := range []string{"bd-001", "bd-002"} {
		if err := store.CreateIssue(ctx, newTestIssue(id), "test"); err != nil {
			t.Fatalf("failed to create issue %s: %v", id, err)
		}
	}
	// Insert foreign-prefix issues directly (bypassing prefix validation)
	for _, id := range []string{"gt-abc", "gt-def", "hq-xyz"} {
		insertIssueDirectly(t, store, id)
	}

	// JSONL contains only the bd-* issues
	jsonlIDs := map[string]bool{"bd-001": true, "bd-002": true}

	foreignCount, foreignPrefixes, ephemeralCount := categorizeDoltExtras(ctx, store, jsonlIDs)

	if foreignCount != 3 {
		t.Errorf("foreignCount = %d, want 3", foreignCount)
	}
	if foreignPrefixes["gt"] != 2 {
		t.Errorf("foreignPrefixes[gt] = %d, want 2", foreignPrefixes["gt"])
	}
	if foreignPrefixes["hq"] != 1 {
		t.Errorf("foreignPrefixes[hq] = %d, want 1", foreignPrefixes["hq"])
	}
	if ephemeralCount != 0 {
		t.Errorf("ephemeralCount = %d, want 0", ephemeralCount)
	}
}

func TestCategorizeDoltExtras_MixedEphemeralAndForeign(t *testing.T) {
	ctx := context.Background()
	store := newTestDoltStore(t, "bd")

	// Create local issues via store
	for _, id := range []string{"bd-001", "bd-003"} {
		if err := store.CreateIssue(ctx, newTestIssue(id), "test"); err != nil {
			t.Fatalf("failed to create issue %s: %v", id, err)
		}
	}
	// Insert foreign-prefix issue directly
	insertIssueDirectly(t, store, "gt-abc")

	jsonlIDs := map[string]bool{"bd-001": true}

	foreignCount, foreignPrefixes, ephemeralCount := categorizeDoltExtras(ctx, store, jsonlIDs)

	if foreignCount != 1 {
		t.Errorf("foreignCount = %d, want 1", foreignCount)
	}
	if foreignPrefixes["gt"] != 1 {
		t.Errorf("foreignPrefixes[gt] = %d, want 1", foreignPrefixes["gt"])
	}
	if ephemeralCount != 1 {
		t.Errorf("ephemeralCount = %d, want 1", ephemeralCount)
	}
}

func TestCategorizeDoltExtras_AllEphemeral(t *testing.T) {
	ctx := context.Background()
	store := newTestDoltStore(t, "bd")

	// All extras are same-prefix (ephemeral)
	for _, id := range []string{"bd-001", "bd-002", "bd-003"} {
		if err := store.CreateIssue(ctx, newTestIssue(id), "test"); err != nil {
			t.Fatalf("failed to create issue %s: %v", id, err)
		}
	}

	jsonlIDs := map[string]bool{"bd-001": true}

	foreignCount, _, ephemeralCount := categorizeDoltExtras(ctx, store, jsonlIDs)

	if foreignCount != 0 {
		t.Errorf("foreignCount = %d, want 0", foreignCount)
	}
	if ephemeralCount != 2 {
		t.Errorf("ephemeralCount = %d, want 2", ephemeralCount)
	}
}

func TestCategorizeDoltExtras_NoExtras(t *testing.T) {
	ctx := context.Background()
	store := newTestDoltStore(t, "bd")

	for _, id := range []string{"bd-001", "bd-002"} {
		if err := store.CreateIssue(ctx, newTestIssue(id), "test"); err != nil {
			t.Fatalf("failed to create issue %s: %v", id, err)
		}
	}

	// All Dolt issues are in JSONL
	jsonlIDs := map[string]bool{"bd-001": true, "bd-002": true}

	foreignCount, _, ephemeralCount := categorizeDoltExtras(ctx, store, jsonlIDs)

	if foreignCount != 0 {
		t.Errorf("foreignCount = %d, want 0", foreignCount)
	}
	if ephemeralCount != 0 {
		t.Errorf("ephemeralCount = %d, want 0", ephemeralCount)
	}
}
