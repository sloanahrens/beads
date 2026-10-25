package main

import (
	"bytes"
	"context"
	"os"
	"sync"
	"testing"

	"github.com/spf13/pflag"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// The text `bd ready` listing reads the ready front PROJECTED by default
// (ready.go's text branch sets types.WorkFilter.Lite) because none of its
// renderings prints a free-form field. This file drives the real RunE against a
// store that emulates both projections, so it pins the decision AND the reason
// the decision is free: the lite rows render to the same bytes as whole ones.

// readyLiteSpyStore is a store whose only real reads are the ready front and
// the two decorations the text listing fetches. It embeds the interface, so any
// other method the command reaches for panics rather than quietly answering
// zero — that is the assertion that the text route opens nothing else.
type readyLiteSpyStore struct {
	storage.DoltStorage

	respectLite bool
	full        []*types.Issue
	lite        []*types.Issue

	mu      sync.Mutex
	filters []types.WorkFilter
}

func (s *readyLiteSpyStore) GetAllConfig(context.Context) (map[string]string, error) {
	return nil, nil
}

func (s *readyLiteSpyStore) GetDependencyRecordsForIssues(context.Context, []string) (map[string][]*types.Dependency, error) {
	return nil, nil
}

func (s *readyLiteSpyStore) GetStatistics(context.Context) (*types.Statistics, error) {
	return &types.Statistics{}, nil
}

// GetReadyWork emulates a backend that honors the projection. respectLite
// FALSE emulates one that ignores it while still answering the same rows, which
// is what makes the byte comparison below a renderer test rather than a
// tautology.
func (s *readyLiteSpyStore) GetReadyWork(_ context.Context, filter types.WorkFilter) ([]*types.Issue, error) {
	s.record(filter)
	if s.respectLite && filter.Lite {
		return s.lite, nil
	}
	return s.full, nil
}

func (s *readyLiteSpyStore) GetReadyWorkWithCounts(_ context.Context, filter types.WorkFilter) ([]*types.IssueWithCounts, error) {
	s.record(filter)
	rows := s.full
	if s.respectLite && filter.Lite {
		rows = s.lite
	}
	out := make([]*types.IssueWithCounts, len(rows))
	for i, issue := range rows {
		out[i] = &types.IssueWithCounts{Issue: issue}
	}
	return out, nil
}

func (s *readyLiteSpyStore) record(filter types.WorkFilter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filters = append(s.filters, filter)
}

func (s *readyLiteSpyStore) captured() []types.WorkFilter {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]types.WorkFilter(nil), s.filters...)
}

// readyHeavyRow is one ready issue carrying every field the lite SELECT drops.
func readyHeavyRow() *types.Issue {
	return &types.Issue{
		ID:                 "be-lite-text",
		Title:              "a title the listing prints",
		Status:             types.StatusOpen,
		Priority:           2,
		IssueType:          types.TypeTask,
		Description:        "a body the listing never prints",
		Design:             "a design the listing never prints",
		AcceptanceCriteria: "criteria the listing never prints",
		Notes:              "notes the listing never prints",
		Payload:            "payload the listing never prints",
		Waiters:            []string{"a-waiter-the-listing-never-prints"},
	}
}

// readyLiteRow is the same issue as a backend that honored the projection
// would return it: the six heavy fields zero-valued, IsLitePartial set.
func readyLiteRow() *types.Issue {
	row := readyHeavyRow()
	row.Description = ""
	row.Design = ""
	row.AcceptanceCriteria = ""
	row.Notes = ""
	row.Payload = ""
	row.Waiters = nil
	row.IsLitePartial = true
	return row
}

// runReadyListing drives readyCmd.RunE with the given store and captures both
// streams. Every LOCAL flag is reset to its default first (pflag's Changed bit
// outlives a test, and other tests in this package set readyCmd's flags) and
// the JSON output mode is set explicitly afterwards, because --json is a root
// persistent flag that joins readyCmd's flag set once any test parses the root
// command — resetting it here would silently flip the mode this test asked for.
func runReadyListing(t *testing.T, json bool, spy *readyLiteSpyStore) (string, []types.WorkFilter) {
	t.Helper()

	stdioMutex.Lock()
	defer stdioMutex.Unlock()

	resetReadyCmdFlags(t)

	oldStore, oldCtx, oldQuiet, oldJSON := store, rootCtx, quietFlag, jsonOutput
	store, rootCtx, quietFlag, jsonOutput = spy, context.Background(), true, json
	t.Cleanup(func() { store, rootCtx, quietFlag, jsonOutput = oldStore, oldCtx, oldQuiet, oldJSON })

	oldStdout, oldStderr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout, os.Stderr = wOut, wErr

	drain := func(r *os.File) <-chan string {
		done := make(chan string, 1)
		go func() {
			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			done <- buf.String()
		}()
		return done
	}
	outDone, errDone := drain(rOut), drain(rErr)

	runErr := readyCmd.RunE(readyCmd, nil)

	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = oldStdout, oldStderr
	shown := <-outDone + <-errDone
	_ = rOut.Close()
	_ = rErr.Close()

	if runErr != nil {
		t.Fatalf("readyCmd.RunE: %v (%s)", runErr, shown)
	}
	return shown, spy.captured()
}

// resetReadyCmdFlags restores every LOCAL flag on readyCmd to its declared
// default, clearing pflag's Changed bit so gatherReadyInput reads only what
// this test set. Inherited flags are skipped: once a test parses the root
// command, cobra merges the root's persistent flags (--json among them) into
// readyCmd's set, and writing their defaults here would clobber the caller's
// chosen output mode.
func resetReadyCmdFlags(t *testing.T) {
	t.Helper()
	inherited := map[string]bool{}
	readyCmd.InheritedFlags().VisitAll(func(f *pflag.Flag) { inherited[f.Name] = true })
	readyCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if inherited[f.Name] {
			return
		}
		switch f.Value.Type() {
		case "stringSlice", "stringArray":
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				if err := sv.Replace(nil); err != nil {
					t.Fatalf("reset --%s: %v", f.Name, err)
				}
			}
		default:
			if err := f.Value.Set(f.DefValue); err != nil {
				t.Fatalf("reset --%s to %q: %v", f.Name, f.DefValue, err)
			}
		}
		f.Changed = false
	})
}

// TestReadyTextListingDefaultsToLite is the per-command pin: the text listing
// asks the ready front for the lite projection, and the --json listing keeps
// whole rows (the projection stays opt-in there via --brief, which
// TestReadyBriefReachesTheFilter covers at the builder).
func TestReadyTextListingDefaultsToLite(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		spy := &readyLiteSpyStore{respectLite: true, full: []*types.Issue{readyHeavyRow()}, lite: []*types.Issue{readyLiteRow()}}
		_, filters := runReadyListing(t, false, spy)
		if len(filters) != 1 {
			t.Fatalf("text listing ran %d ready queries, want 1: %v", len(filters), filters)
		}
		if !filters[0].Lite {
			t.Error("text listing asked for the whole row; the renderer prints none of the six fields the lite SELECT drops")
		}
	})

	t.Run("json", func(t *testing.T) {
		spy := &readyLiteSpyStore{respectLite: true, full: []*types.Issue{readyHeavyRow()}, lite: []*types.Issue{readyLiteRow()}}
		_, filters := runReadyListing(t, true, spy)
		if len(filters) != 1 {
			t.Fatalf("json listing ran %d ready queries, want 1: %v", len(filters), filters)
		}
		if filters[0].Lite {
			t.Errorf("--json defaulted to the projection; a script reading a whole issue must keep getting one (filters: %v)", filters)
		}
	})
}

// TestReadyTextListingRendersProjectedRowsIdentically is the output half:
// a listing whose rows came back projected is byte-identical to the same
// listing over whole rows. If a future text rendering starts printing one of
// the six fields, this fails before the default can change user-visible output.
func TestReadyTextListingRendersProjectedRowsIdentically(t *testing.T) {
	proxying := &readyLiteSpyStore{respectLite: true, full: []*types.Issue{readyHeavyRow()}, lite: []*types.Issue{readyLiteRow()}}
	whole := &readyLiteSpyStore{respectLite: false, full: []*types.Issue{readyHeavyRow()}}

	projectedOut, projectedFilters := runReadyListing(t, false, proxying)
	wholeOut, wholeFilters := runReadyListing(t, false, whole)

	if len(projectedFilters) == 0 || !projectedFilters[0].Lite {
		t.Fatalf("precondition: the text listing no longer asks for the projection (filters: %v)", projectedFilters)
	}
	if len(wholeFilters) == 0 || !wholeFilters[0].Lite {
		t.Fatalf("precondition: the control listing asked for the same request (filters: %v)", wholeFilters)
	}
	if projectedOut != wholeOut {
		t.Errorf("projected and whole rows rendered differently.\nprojected:\n%s\nwhole:\n%s", projectedOut, wholeOut)
	}
}
