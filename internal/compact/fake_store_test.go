package compact

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/steveyegge/beads/internal/types"
)

// errFakeNotFound is returned by fakeStore for an issue ID it does not hold.
var errFakeNotFound = errors.New("issue not found")

// fakeStore is an in-memory compactableStore. It skips the real store's age
// and dependency checks: a closed issue is eligible, and any other status is
// refused with the real store's status reason. Every write is recorded in
// writes, so a test can assert that nothing changed.
type fakeStore struct {
	mu     sync.Mutex
	issues map[string]*types.Issue
	writes []string
}

func newFakeStore(issues ...*types.Issue) *fakeStore {
	s := &fakeStore{issues: make(map[string]*types.Issue)}
	for _, issue := range issues {
		s.issues[issue.ID] = issue
	}
	return s
}

func (s *fakeStore) CheckEligibility(_ context.Context, issueID string, _ int) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	issue, ok := s.issues[issueID]
	if !ok {
		return false, "", fmt.Errorf("%s: %w", issueID, errFakeNotFound)
	}
	if issue.Status != types.StatusClosed {
		return false, fmt.Sprintf("issue is not closed (status: %s)", issue.Status), nil
	}
	return true, "", nil
}

func (s *fakeStore) GetIssue(_ context.Context, issueID string) (*types.Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	issue, ok := s.issues[issueID]
	if !ok {
		return nil, fmt.Errorf("%s: %w", issueID, errFakeNotFound)
	}
	cp := *issue
	return &cp, nil
}

func (s *fakeStore) SnapshotIssue(_ context.Context, issueID string, tier int) error {
	s.recordWrite(fmt.Sprintf("snapshot %s tier %d", issueID, tier))
	return nil
}

func (s *fakeStore) UpdateIssue(_ context.Context, issueID string, _ map[string]interface{}, _ string) error {
	s.recordWrite("update " + issueID)
	return nil
}

func (s *fakeStore) ApplyCompaction(_ context.Context, issueID string, tier int, _, _ int, _ string) error {
	s.recordWrite(fmt.Sprintf("apply %s tier %d", issueID, tier))
	return nil
}

func (s *fakeStore) AddComment(_ context.Context, issueID, _, _ string) error {
	s.recordWrite("comment " + issueID)
	return nil
}

func (s *fakeStore) recordWrite(desc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = append(s.writes, desc)
}
