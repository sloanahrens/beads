//go:build cgo

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// bdChildren runs "bd children" with the given args and returns stdout.
func bdChildren(t *testing.T, bd, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"children"}, args...)
	cmd := exec.Command(bd, fullArgs...)
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	stdout, stderr, err := runCommandBuffers(t, cmd)
	if err != nil {
		t.Fatalf("bd children %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func TestEmbeddedChildren(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "ch")

	t.Run("children_basic", func(t *testing.T) {
		parent := bdCreate(t, bd, dir, "Parent epic", "--type", "epic")
		child1 := bdCreate(t, bd, dir, "Child task 1", "--type", "task")
		child2 := bdCreate(t, bd, dir, "Child task 2", "--type", "task")
		bdDepAdd(t, bd, dir, child1.ID, parent.ID, "--type", "parent-child")
		bdDepAdd(t, bd, dir, child2.ID, parent.ID, "--type", "parent-child")

		out := bdChildren(t, bd, dir, parent.ID)
		if !strings.Contains(out, child1.ID) {
			t.Errorf("expected child1 %s in output: %s", child1.ID, out)
		}
		if !strings.Contains(out, child2.ID) {
			t.Errorf("expected child2 %s in output: %s", child2.ID, out)
		}
	})

	t.Run("children_json", func(t *testing.T) {
		parent := bdCreate(t, bd, dir, "JSON parent", "--type", "epic")
		child := bdCreate(t, bd, dir, "JSON child", "--type", "task")
		bdDepAdd(t, bd, dir, child.ID, parent.ID, "--type", "parent-child")

		cmd := exec.Command(bd, "children", parent.ID, "--json")
		cmd.Dir = dir
		cmd.Env = bdEnv(dir)
		stdout, stderr, err := runCommandBuffers(t, cmd)
		if err != nil {
			t.Fatalf("bd children --json failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
		}
		s := strings.TrimSpace(stdout.String())
		start := strings.Index(s, "[")
		if start >= 0 {
			var issues []map[string]interface{}
			if err := json.Unmarshal([]byte(s[start:]), &issues); err != nil {
				t.Fatalf("parse children JSON: %v\n%s", err, s)
			}
			if len(issues) != 1 {
				t.Errorf("expected 1 child, got %d", len(issues))
			}
		}
	})

	t.Run("children_empty", func(t *testing.T) {
		parent := bdCreate(t, bd, dir, "No children parent", "--type", "task")
		out := bdChildren(t, bd, dir, parent.ID)
		// Should not error, may show empty message
		_ = out
	})

	t.Run("children_includes_all_statuses", func(t *testing.T) {
		parent := bdCreate(t, bd, dir, "All status parent", "--type", "epic")
		openChild := bdCreate(t, bd, dir, "Open child", "--type", "task")
		closedChild := bdCreate(t, bd, dir, "Closed child", "--type", "task")
		bdDepAdd(t, bd, dir, openChild.ID, parent.ID, "--type", "parent-child")
		bdDepAdd(t, bd, dir, closedChild.ID, parent.ID, "--type", "parent-child")
		bdClose(t, bd, dir, closedChild.ID)

		out := bdChildren(t, bd, dir, parent.ID)
		if !strings.Contains(out, openChild.ID) {
			t.Errorf("expected open child in output: %s", out)
		}
		if !strings.Contains(out, closedChild.ID) {
			t.Errorf("expected closed child in output (--all implied): %s", out)
		}
	})

	// be-8ws: the durable-only plane suppression the default listing carries
	// used to reach a --parent scope too. An ephemeral parent's children are
	// wisps, whose parent-child edges are written to wisp_dependencies, so
	// `bd children <wisp>` and `bd list --parent <wisp>` answered "has no
	// children" for a parent that had them — silently, with exit 0.
	t.Run("children_of_an_ephemeral_parent", func(t *testing.T) {
		parent := bdCreate(t, bd, dir, "Wisp root", "--ephemeral")
		child1 := bdCreate(t, bd, dir, "Wisp step 1", "--ephemeral")
		child2 := bdCreate(t, bd, dir, "Wisp step 2", "--ephemeral")
		bdDepAdd(t, bd, dir, child1.ID, parent.ID, "--type", "parent-child")
		bdDepAdd(t, bd, dir, child2.ID, parent.ID, "--type", "parent-child")

		out := bdChildren(t, bd, dir, parent.ID)
		for _, id := range []string{child1.ID, child2.ID} {
			if !strings.Contains(out, id) {
				t.Errorf("expected ephemeral child %s in output: %s", id, out)
			}
		}
	})

	// The scope admits the wisp PLANE, not wisp-specific rows: a parent-child
	// edge into the wisp plane from a durable parent is a real child, and
	// `bd show <id> --children` — which has always unioned both dependency
	// tables — already reports it. `bd children` has to agree with it, so this
	// pins the pair rather than the durable-only half.
	t.Run("durable_parent_with_an_ephemeral_child", func(t *testing.T) {
		parent := bdCreate(t, bd, dir, "Durable parent", "--type", "epic")
		durableChild := bdCreate(t, bd, dir, "Durable child", "--type", "task")
		wispChild := bdCreate(t, bd, dir, "Ephemeral child", "--ephemeral")
		bdDepAdd(t, bd, dir, durableChild.ID, parent.ID, "--type", "parent-child")
		bdDepAdd(t, bd, dir, wispChild.ID, parent.ID, "--type", "parent-child")

		out := bdChildren(t, bd, dir, parent.ID)
		for _, id := range []string{durableChild.ID, wispChild.ID} {
			if !strings.Contains(out, id) {
				t.Errorf("expected child %s in output: %s", id, out)
			}
		}
	})

	t.Run("children_nonexistent_parent", func(t *testing.T) {
		cmd := exec.Command(bd, "children", "ch-nonexistent999")
		cmd.Dir = dir
		cmd.Env = bdEnv(dir)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected children of nonexistent to fail, got: %s", out)
		}
	})
}

// TestEmbeddedChildrenConcurrent exercises children listing concurrently.
func TestEmbeddedChildrenConcurrent(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "cx")

	// Create parents with children
	var parentIDs []string
	for i := 0; i < 4; i++ {
		parent := bdCreate(t, bd, dir, fmt.Sprintf("Concurrent parent %d", i), "--type", "epic")
		for j := 0; j < 2; j++ {
			child := bdCreate(t, bd, dir, fmt.Sprintf("Child %d-%d", i, j), "--type", "task")
			bdDepAdd(t, bd, dir, child.ID, parent.ID, "--type", "parent-child")
		}
		parentIDs = append(parentIDs, parent.ID)
	}

	const numWorkers = 8
	type workerResult struct {
		worker int
		err    error
	}

	results := make([]workerResult, numWorkers)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func(worker int) {
			defer wg.Done()
			r := workerResult{worker: worker}

			parentID := parentIDs[worker%len(parentIDs)]
			cmd := exec.Command(bd, "children", parentID, "--json")
			cmd.Dir = dir
			cmd.Env = bdEnv(dir)
			out, err := cmd.CombinedOutput()
			if err != nil {
				r.err = fmt.Errorf("children %s: %v\n%s", parentID, err, out)
			}

			results[worker] = r
		}(w)
	}
	wg.Wait()

	for _, r := range results {
		if r.err != nil && !strings.Contains(r.err.Error(), "one writer at a time") {
			t.Errorf("worker %d failed: %v", r.worker, r.err)
		}
	}
}
