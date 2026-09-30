//go:build cgo && integration

// Integration tier (be-b23): moved verbatim from the unit-tier sibling file.
// These tests run bd init against a fresh store, which migrates; the unit
// tier's BD_TEST_TIER tripwire refuses that.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitMetricsDisabledSuppresses(t *testing.T) {
	bd := buildEmbeddedBD(t)
	home, err := testTempDir("bd-metrics-disabled-home-*")
	if err != nil {
		t.Fatalf("temp home: %v", err)
	}
	repo, err := testTempDir("bd-metrics-disabled-repo-*")
	if err != nil {
		t.Fatalf("temp repo: %v", err)
	}
	initGitRepoAt(t, repo)

	cmd := exec.Command(bd, "init", "--non-interactive", "--quiet")
	cmd.Dir = repo
	env := append([]string{}, bdEnv(home)...)
	env = append(env, "BD_DISABLE_METRICS=1")
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("bd init failed: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}

	dir := filepath.Join(home, ".beads", "eventsData")
	if _, err := os.Stat(dir); err == nil {
		entries, _ := os.ReadDir(dir)
		var evtqs []string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".evtq") {
				evtqs = append(evtqs, e.Name())
			}
		}
		if len(evtqs) > 0 {
			t.Errorf("BD_DISABLE_METRICS=1 still produced .evtq files: %v", evtqs)
		}
	}
}

// TestMetricsTodoAliasEmitsSingleEvent is the double-emit regression for PR
// #4419: bare `bd todo` delegates to the todo-list behavior, but it must record
// exactly one cli_command event ("todo"), not also a phantom "todo-list".
func TestMetricsTodoAliasEmitsSingleEvent(t *testing.T) {
	bd := buildEmbeddedBD(t)
	home, err := testTempDir("bd-metrics-todo-home-*")
	if err != nil {
		t.Fatalf("temp home: %v", err)
	}
	repo, err := testTempDir("bd-metrics-todo-repo-*")
	if err != nil {
		t.Fatalf("temp repo: %v", err)
	}
	initGitRepoAt(t, repo)

	// A store is needed so `bd todo` (which lists task issues) reaches its RunE.
	if _, errOut := runBdForMetrics(t, bd, repo, home, "init", "--non-interactive", "--quiet"); errOut != "" {
		// init may print warnings; only fail later if todo produces no event.
		_ = errOut
	}

	// Isolate the next invocation by dropping init's queued event.
	if err := os.RemoveAll(filepath.Join(home, ".beads", "eventsData")); err != nil {
		t.Fatalf("clear eventsData: %v", err)
	}

	_, errOut := runBdForMetrics(t, bd, repo, home, "todo")

	got := allCommandEvents(t, home)
	if len(got) != 1 || got[0] != "todo" {
		t.Errorf("bd todo emitted %v, want exactly [todo] (double-emit regression)\nstderr:\n%s", got, errOut)
	}
}

// TestMetricsReadyGatedAliasEmitsSingleEvent is the double-emit regression for
// PR #4419: `bd ready --gated` delegates to the gate-ready molecule discovery,
// but it must record exactly one cli_command event ("ready"), not also a phantom
// "mol-ready-gated".
func TestMetricsReadyGatedAliasEmitsSingleEvent(t *testing.T) {
	bd := buildEmbeddedBD(t)
	home, err := testTempDir("bd-metrics-readygated-home-*")
	if err != nil {
		t.Fatalf("temp home: %v", err)
	}
	repo, err := testTempDir("bd-metrics-readygated-repo-*")
	if err != nil {
		t.Fatalf("temp repo: %v", err)
	}
	initGitRepoAt(t, repo)

	// A store is needed so `bd ready --gated` reaches its discovery body.
	if _, errOut := runBdForMetrics(t, bd, repo, home, "init", "--non-interactive", "--quiet"); errOut != "" {
		_ = errOut
	}

	// Isolate the next invocation by dropping init's queued event.
	if err := os.RemoveAll(filepath.Join(home, ".beads", "eventsData")); err != nil {
		t.Fatalf("clear eventsData: %v", err)
	}

	_, errOut := runBdForMetrics(t, bd, repo, home, "ready", "--gated")

	got := allCommandEvents(t, home)
	if len(got) != 1 || got[0] != "ready" {
		t.Errorf("bd ready --gated emitted %v, want exactly [ready] (double-emit regression)\nstderr:\n%s", got, errOut)
	}
}

// TestMetricsWispAliasEmitsSingleEvent is the double-emit regression for PR
// #4419: bare `bd mol wisp <proto>` delegates to the wisp-create behavior, but it
// must record exactly one cli_command event ("wisp"), not also a phantom
// "wisp-create". The proto does not exist, so the command fails after the event
// is recorded; the event count is what this guards.
func TestMetricsWispAliasEmitsSingleEvent(t *testing.T) {
	bd := buildEmbeddedBD(t)
	home, err := testTempDir("bd-metrics-wisp-home-*")
	if err != nil {
		t.Fatalf("temp home: %v", err)
	}
	repo, err := testTempDir("bd-metrics-wisp-repo-*")
	if err != nil {
		t.Fatalf("temp repo: %v", err)
	}
	initGitRepoAt(t, repo)

	// A store is needed so `bd mol wisp <proto>` reaches its RunE body.
	if _, errOut := runBdForMetrics(t, bd, repo, home, "init", "--non-interactive", "--quiet"); errOut != "" {
		_ = errOut
	}

	// Isolate the next invocation by dropping init's queued event.
	if err := os.RemoveAll(filepath.Join(home, ".beads", "eventsData")); err != nil {
		t.Fatalf("clear eventsData: %v", err)
	}

	// A non-existent proto makes the create fail, but the "wisp" event is still
	// recorded before delegation, which is exactly what this regression checks.
	_, errOut := runBdForMetrics(t, bd, repo, home, "mol", "wisp", "mol-nonexistent-proto")

	got := allCommandEvents(t, home)
	if len(got) != 1 || got[0] != "wisp" {
		t.Errorf("bd mol wisp <proto> emitted %v, want exactly [wisp] (double-emit regression)\nstderr:\n%s", got, errOut)
	}
}
