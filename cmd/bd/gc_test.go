package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// gcStubStore is a DoltStorage stand-in for bd gc: it counts the calls gc is
// allowed to make (DoltGC, a size probe) and the ones it must never make
// (SearchIssues for closed issues, DeleteIssue). SearchIssues hands back closed
// issues, so a regression that reintroduces the decay loop has something to
// delete and this test fails instead of passing on an empty result.
type gcStubStore struct {
	storage.DoltStorage
	gcCalls     int
	searchCalls int
	deleteCalls int
	deleted     []string
}

func (s *gcStubStore) DoltGC(context.Context) error { s.gcCalls++; return nil }

func (s *gcStubStore) ActiveDatabaseSize(context.Context) (int64, error) { return 4096, nil }

func (s *gcStubStore) SearchIssues(context.Context, string, types.IssueFilter) ([]*types.Issue, error) {
	s.searchCalls++
	return []*types.Issue{
		{ID: "be-ancient", Status: types.StatusClosed},
		{ID: "be-old", Status: types.StatusClosed},
	}, nil
}

func (s *gcStubStore) DeleteIssue(_ context.Context, id string) error {
	s.deleteCalls++
	s.deleted = append(s.deleted, id)
	return nil
}

// withGCStore points the package-level store at a stub and restores the gc
// globals afterwards, so these tests cannot leak state into the rest of the
// package's command tests.
func withGCStore(t *testing.T, stub *gcStubStore, dryRun, skipDolt, json bool) {
	t.Helper()
	oldStore := store
	oldDryRun, oldSkipDolt, oldJSON := gcDryRun, gcSkipDolt, jsonOutput
	t.Cleanup(func() {
		store = oldStore
		gcDryRun, gcSkipDolt, jsonOutput = oldDryRun, oldSkipDolt, oldJSON
	})
	store = stub
	gcDryRun, gcSkipDolt, jsonOutput = dryRun, skipDolt, json
}

// captureGCOutput runs fn with os.Stdout redirected and returns what it wrote.
func captureGCOutput(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()

	os.Stdout = w
	func() {
		defer func() {
			os.Stdout = origStdout
			_ = w.Close()
		}()
		fn()
	}()
	out := <-done
	_ = r.Close()
	return out
}

// TestGCRemovedFlagsAreUnknown is the flag half of the acceptance: the flags
// that configured the deleted phases must be gone, so pflag reports them the
// way it reports any typo instead of silently accepting a no-op.
func TestGCRemovedFlagsAreUnknown(t *testing.T) {
	for _, name := range []string{"older-than", "skip-decay"} {
		if f := gcCmd.Flags().Lookup(name); f != nil {
			t.Errorf("bd gc still defines --%s (%q): the phase it belonged to is deleted", name, f.Usage)
		}
	}
	for _, args := range [][]string{{"--older-than=30"}, {"--older-than", "30"}, {"--skip-decay"}} {
		if err := gcCmd.Flags().Parse(args); err == nil {
			t.Errorf("bd gc accepted removed flags %v; want an unknown-flag error", args)
		}
	}
}

// TestGCHelpOmitsDeletedPhasesAndFlags pins the help text: bd gc documents one
// phase, Dolt GC, and names neither the deleted phases nor the flags that
// configured them.
func TestGCHelpOmitsDeletedPhasesAndFlags(t *testing.T) {
	help := gcCmd.Short + "\n" + gcCmd.Long + "\n"
	if !strings.Contains(help, "Dolt garbage collection") {
		t.Errorf("bd gc help no longer describes Dolt GC:\n%s", help)
	}
	for _, banned := range []string{"Decay", "decay", "Compact", "compact", "older-than", "skip-decay", "Phase 1", "Phase 2", "Phase 3"} {
		if strings.Contains(help, banned) {
			t.Errorf("bd gc help still mentions %q:\n%s", banned, help)
		}
	}
	if !strings.Contains(help, "deletes no issues") {
		t.Errorf("bd gc help does not say GC deletes no issues:\n%s", help)
	}
}

// TestGCRunsDoltGCAndDeletesNoIssues is the behavior half: the one remaining
// phase still runs, and the closed issues the store holds are never looked up
// or deleted — the decay commit loop is gone, so a closed issue survives bd gc
// no matter how long it has been closed.
func TestGCRunsDoltGCAndDeletesNoIssues(t *testing.T) {
	stub := &gcStubStore{}
	withGCStore(t, stub, false, false, true)

	out := captureGCOutput(t, func() {
		if err := gcCmd.RunE(gcCmd, nil); err != nil {
			t.Fatalf("bd gc: %v", err)
		}
	})

	if stub.gcCalls != 1 {
		t.Errorf("DoltGC called %d time(s), want exactly 1", stub.gcCalls)
	}
	if stub.searchCalls != 0 {
		t.Errorf("gc searched for closed issues %d time(s); the decay phase is deleted", stub.searchCalls)
	}
	if stub.deleteCalls != 0 || len(stub.deleted) != 0 {
		t.Errorf("gc deleted %v; closed issues must survive bd gc", stub.deleted)
	}
	if !strings.Contains(out, "Dolt GC") {
		t.Errorf("json summary does not name the Dolt GC phase:\n%s", out)
	}
	if strings.Contains(out, "Decay") || strings.Contains(out, "Compact") {
		t.Errorf("json summary still reports deleted phases:\n%s", out)
	}
}

// TestGCDryRunRunsNoGC pins --dry-run: it previews the Dolt GC and leaves the
// store untouched.
func TestGCDryRunRunsNoGC(t *testing.T) {
	stub := &gcStubStore{}
	withGCStore(t, stub, true, false, false)

	out := captureGCOutput(t, func() {
		if err := gcCmd.RunE(gcCmd, nil); err != nil {
			t.Fatalf("bd gc --dry-run: %v", err)
		}
	})

	if stub.gcCalls != 0 {
		t.Errorf("dry run called DoltGC %d time(s), want 0", stub.gcCalls)
	}
	if !strings.Contains(out, "Would run DOLT_GC()") {
		t.Errorf("dry run did not preview the Dolt GC:\n%s", out)
	}
	if !strings.Contains(out, "DRY RUN complete") {
		t.Errorf("dry run summary missing:\n%s", out)
	}
}

// TestGCSkipDoltReportsSkipped keeps --skip-dolt honest: it is the one flag
// that still shapes the run, and it must not touch the store.
func TestGCSkipDoltReportsSkipped(t *testing.T) {
	stub := &gcStubStore{}
	withGCStore(t, stub, false, true, false)

	out := captureGCOutput(t, func() {
		if err := gcCmd.RunE(gcCmd, nil); err != nil {
			t.Fatalf("bd gc --skip-dolt: %v", err)
		}
	})

	if stub.gcCalls != 0 {
		t.Errorf("--skip-dolt called DoltGC %d time(s), want 0", stub.gcCalls)
	}
	if !strings.Contains(out, "Dolt GC: skipped") {
		t.Errorf("--skip-dolt summary missing:\n%s", out)
	}
}
