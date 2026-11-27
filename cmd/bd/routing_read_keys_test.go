package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/steveyegge/beads/internal/routing"
	"github.com/steveyegge/beads/internal/storage"
)

// routingKeysStore answers the routing config read from a canned table,
// records which keys it was asked for, and fails the test if the whole config
// table is read. Any other method call panics via the embedded nil interface.
type routingKeysStore struct {
	storage.DoltStorage
	t      *testing.T
	values map[string]string
	err    error
	calls  int
	asked  []string
}

func (s *routingKeysStore) GetConfigs(_ context.Context, keys ...string) (map[string]string, error) {
	s.calls++
	s.asked = append([]string(nil), keys...)
	if s.err != nil {
		return nil, s.err
	}
	return s.values, nil
}

func (s *routingKeysStore) GetAllConfig(context.Context) (map[string]string, error) {
	s.t.Errorf("determineAutoRoutedRepoPath read the whole config table with GetAllConfig; it must ask GetConfigs for the routing keys only")
	return nil, nil
}

// readyLiteSpyStore, missingIssueStore and showResolveTestStore answer only the
// old GetAllConfig read; be-xxfv lets this file answer the new method for them,
// keeping each fixture's "routing is not configured" behaviour.
func (*readyLiteSpyStore) GetConfigs(context.Context, ...string) (map[string]string, error) {
	return nil, nil
}
func (missingIssueStore) GetConfigs(context.Context, ...string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (*showResolveTestStore) GetConfigs(context.Context, ...string) (map[string]string, error) {
	return map[string]string{}, nil
}

func TestRoutingReadKeys(t *testing.T) {
	initConfigForTest(t)
	cases := []struct {
		name     string
		values   map[string]string
		err      error
		wantPath string
		wantRule routing.RoutingRule
	}{
		{"routes by the values returned", map[string]string{"routing.default": "/planning/repo"}, nil, "/planning/repo", routing.RuleDefault},
		{"read error routes nowhere", nil, errors.New("config read failed"), ".", routing.RuleNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &routingKeysStore{t: t, values: tc.values, err: tc.err}
			path, rule := determineAutoRoutedRepoPath(context.Background(), store)
			if path != tc.wantPath || rule != tc.wantRule {
				t.Errorf("determineAutoRoutedRepoPath() = %q, %v; want %q, %v", path, rule, tc.wantPath, tc.wantRule)
			}
			if want := slices.Sorted(slices.Values(routingConfigKeys)); store.calls != 1 || !slices.Equal(slices.Sorted(slices.Values(store.asked)), want) {
				t.Errorf("GetConfigs calls = %d, asked %v; want 1 call with exactly %v", store.calls, store.asked, want)
			}
		})
	}
}
