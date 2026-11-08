package scripts_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The post-land workflow is prepared in ci/post-land.workflow.yml because the
// workflow-protect hook refuses an agent push into .forgejo/workflows/. The
// operator copies it to its live name after one green run shows on main. This
// test pins both copies: the text the operator is asked to commit, and, once
// it exists, the live file itself.
//
// The status context is `post-land / check (push)`, built from the workflow
// name and the single job key. The rig setting
// merge_queue.post_land_status_context names the same context but lives outside
// this repo (as in gastown), so this test cannot read it; it pins the two names
// the setting must match instead.
const (
	postLandWorkflowPath = "ci/post-land.workflow.yml"
	postLandLivePath     = ".forgejo/workflows/post-land.yml"
	postLandWorkflowName = "post-land"
	postLandJobKey       = "check"
	postLandRunner       = "ci-base"
	postLandGateCommand  = "make gate"
)

// postLandWorkflow is the parsed workflow document plus the bytes it came from.
type postLandWorkflow struct {
	root *yaml.Node
	raw  []byte
}

// readPostLandWorkflow parses the prepared copy. A missing or unparsable file
// is a failure: the file is this bead's deliverable.
func readPostLandWorkflow(t *testing.T) postLandWorkflow {
	t.Helper()

	path := filepath.Join(sourceRepoRoot(t), filepath.FromSlash(postLandWorkflowPath))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("%s: document root is not a mapping", path)
	}
	return postLandWorkflow{root: doc.Content[0], raw: raw}
}

// TestPostLandWorkflowDefinition pins the workflow text the operator commits.
func TestPostLandWorkflowDefinition(t *testing.T) {
	workflow := readPostLandWorkflow(t)

	job := postLandOnlyJob(t, workflow.root)

	t.Run("name is post-land", func(t *testing.T) {
		name := postLandField(workflow.root, "name")
		if name == nil || name.Kind != yaml.ScalarNode || name.Value != postLandWorkflowName {
			t.Errorf("name = %v, want %q", postLandScalar(name), postLandWorkflowName)
		}
	})

	t.Run("runs on ci-base", func(t *testing.T) {
		runsOn := postLandField(job, "runs-on")
		if runsOn == nil || runsOn.Kind != yaml.ScalarNode || runsOn.Value != postLandRunner {
			t.Errorf("job %q runs-on = %v, want %q", postLandJobKey, postLandScalar(runsOn), postLandRunner)
		}
	})

	t.Run("triggers on push to main only", func(t *testing.T) {
		on := postLandField(workflow.root, "on")
		if on == nil || on.Kind != yaml.MappingNode {
			t.Fatalf("workflow has no on mapping")
		}
		if triggers := postLandMappingKeys(on); len(triggers) != 1 || triggers[0] != "push" {
			t.Fatalf("triggers = %v, want exactly [push]", triggers)
		}
		push := on.Content[1]
		if keys := postLandMappingKeys(push); len(keys) != 1 || keys[0] != "branches" {
			t.Fatalf("push keys = %v, want exactly [branches]", keys)
		}
		branches := postLandField(push, "branches")
		if branches == nil || branches.Kind != yaml.SequenceNode {
			t.Fatalf("push has no branches sequence")
		}
		got := make([]string, 0, len(branches.Content))
		for _, node := range branches.Content {
			got = append(got, node.Value)
		}
		if len(got) != 1 || got[0] != "main" {
			t.Errorf("push branches = %v, want [main]", got)
		}
	})

	t.Run("runs make gate", func(t *testing.T) {
		var runs []string
		for _, step := range postLandSteps(job) {
			if run := postLandField(step, "run"); run != nil {
				runs = append(runs, strings.TrimSpace(run.Value))
			}
		}
		count := 0
		for _, run := range runs {
			if run == postLandGateCommand {
				count++
			}
		}
		if count != 1 {
			t.Errorf("job %q runs %q %d times, want exactly once (ran %v)", postLandJobKey, postLandGateCommand, count, runs)
		}
	})

	t.Run("checks out full history and fetches tags", func(t *testing.T) {
		var checkout *yaml.Node
		var fetchesTags bool
		for _, step := range postLandSteps(job) {
			if uses := postLandField(step, "uses"); uses != nil && strings.HasPrefix(uses.Value, "actions/checkout@") {
				checkout = step
			}
			if run := postLandField(step, "run"); run != nil &&
				strings.Contains(run.Value, "git fetch") && strings.Contains(run.Value, "--tags") {
				fetchesTags = true
			}
		}
		if checkout == nil {
			t.Fatalf("job %q has no actions/checkout step", postLandJobKey)
		}
		depth := postLandField(postLandField(checkout, "with"), "fetch-depth")
		if depth == nil || depth.Value != "0" {
			t.Errorf("checkout fetch-depth = %v, want 0 (versions are stamped from git)", postLandScalar(depth))
		}
		if !fetchesTags {
			t.Errorf("no step fetches tags (versions are stamped from git)")
		}
	})

	t.Run("header names the human step and the status context", func(t *testing.T) {
		header := postLandHeader(workflow.raw)
		if header == "" {
			t.Fatalf("%s does not start with a comment", postLandWorkflowPath)
		}
		lowerHeader := strings.ToLower(header)
		for _, want := range []string{
			"human",
			"merge_queue.post_land_status_context",
			postLandWorkflowName + " / " + postLandJobKey,
		} {
			if !strings.Contains(lowerHeader, want) {
				t.Errorf("header comment does not mention %q:\n%s", want, header)
			}
		}
	})

	t.Run("has no concurrency block", func(t *testing.T) {
		if node := postLandField(workflow.root, "concurrency"); node != nil {
			t.Errorf("workflow has a top-level concurrency block; a cancelled or replaced run leaves a false red or no status")
		}
		if node := postLandField(job, "concurrency"); node != nil {
			t.Errorf("job %q has a concurrency block; a cancelled or replaced run leaves a false red or no status", postLandJobKey)
		}
	})

	t.Run("has no continue-on-error", func(t *testing.T) {
		for _, path := range postLandContinueOnErrorPaths(workflow.root) {
			t.Errorf("workflow sets continue-on-error at %s; a failed gate would still post a passing status", path)
		}
	})

	t.Run("holds no secrets or credentials", func(t *testing.T) {
		if bytes.Contains(workflow.raw, []byte("secrets.")) {
			t.Errorf("workflow references a secrets. expression")
		}
		for _, key := range postLandCredentialKeys(workflow.root) {
			t.Errorf("workflow defines credential-shaped key %q", key)
		}
	})
}

// TestPostLandRefusesContinueOnError mutates a scratch copy of the reviewed
// text the way a well-meaning retry step would and expects the pin to name the
// key at each level. A job that may fail without failing the run would let the
// post-land status read green after a red gate, which defeats the verdict.
func TestPostLandRefusesContinueOnError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, root *yaml.Node) []string
	}{
		{
			name: "reviewed text",
			mutate: func(*testing.T, *yaml.Node) []string {
				return nil
			},
		},
		{
			name: "job level",
			mutate: func(t *testing.T, root *yaml.Node) []string {
				postLandSetScalar(postLandOnlyJob(t, root), "continue-on-error", "true")
				return []string{"jobs." + postLandJobKey + ".continue-on-error"}
			},
		},
		{
			name: "step level",
			mutate: func(t *testing.T, root *yaml.Node) []string {
				for i, step := range postLandSteps(postLandOnlyJob(t, root)) {
					if run := postLandField(step, "run"); run != nil && strings.TrimSpace(run.Value) == postLandGateCommand {
						postLandSetScalar(step, "continue-on-error", "true")
						return []string{fmt.Sprintf("jobs.%s.steps[%d].continue-on-error", postLandJobKey, i)}
					}
				}
				t.Fatalf("no step runs %q", postLandGateCommand)
				return nil
			},
		},
		{
			name: "key set twice",
			mutate: func(t *testing.T, root *yaml.Node) []string {
				job := postLandOnlyJob(t, root)
				postLandSetScalar(job, "continue-on-error", "true")
				postLandSetScalar(job, "continue-on-error", "false")
				return []string{"jobs." + postLandJobKey + ".continue-on-error"}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workflow := readPostLandWorkflow(t)
			want := tc.mutate(t, workflow.root)
			if got := postLandContinueOnErrorPaths(workflow.root); !reflect.DeepEqual(got, want) {
				t.Errorf("continue-on-error paths = %v, want %v", got, want)
			}
		})
	}
}

// TestPostLandWorkflowLiveCopyMatches fails once the live workflow exists and
// drifts from the reviewed text. Absence is not a failure: the operator
// commits the live copy by hand after one green run on main.
func TestPostLandWorkflowLiveCopyMatches(t *testing.T) {
	repoRoot := sourceRepoRoot(t)

	prepared, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(postLandWorkflowPath)))
	if err != nil {
		t.Fatalf("read %s: %v", postLandWorkflowPath, err)
	}

	live, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(postLandLivePath)))
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("%s does not exist yet; the operator commits it after one green run on main", postLandLivePath)
	}
	if err != nil {
		t.Fatalf("read %s: %v", postLandLivePath, err)
	}
	if err := postLandCopyMismatch(prepared, live); err != nil {
		t.Error(err)
	}
}

// TestPostLandCopyMismatch drives the refusal branch of the byte-equality
// check, which the live-copy test cannot drive until the operator commits the
// live workflow.
func TestPostLandCopyMismatch(t *testing.T) {
	for _, tc := range []struct {
		name     string
		prepared []byte
		live     []byte
		wantErr  bool
	}{
		{"identical", []byte("name: post-land\n"), []byte("name: post-land\n"), false},
		{"one byte differs", []byte("name: post-land\n"), []byte("name: post-lands\n"), true},
		{"trailing newline differs", []byte("name: post-land\n"), []byte("name: post-land"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := postLandCopyMismatch(tc.prepared, tc.live)
			if (err != nil) != tc.wantErr {
				t.Errorf("postLandCopyMismatch() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

// postLandCopyMismatch returns nil when the prepared and live copies are
// byte-identical, and an error naming both files when they are not.
func postLandCopyMismatch(prepared, live []byte) error {
	if bytes.Equal(prepared, live) {
		return nil
	}
	return fmt.Errorf("%s and %s differ; copy %s over the live file byte for byte",
		postLandWorkflowPath, postLandLivePath, postLandWorkflowPath)
}

// postLandField returns the value mapped to key, or nil when the node is not a
// mapping or the key is absent.
func postLandField(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// postLandOnlyJob returns the workflow's single job, failing the test when the
// jobs mapping does not hold exactly postLandJobKey.
func postLandOnlyJob(t *testing.T, root *yaml.Node) *yaml.Node {
	t.Helper()

	jobs := postLandField(root, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		t.Fatalf("workflow has no jobs mapping")
	}
	if keys := postLandMappingKeys(jobs); len(keys) != 1 || keys[0] != postLandJobKey {
		t.Fatalf("workflow jobs = %v, want exactly one job %q", keys, postLandJobKey)
	}
	return jobs.Content[1]
}

// postLandContinueOnErrorPaths returns the dotted path of every
// continue-on-error key in the workflow, at any depth. A job or step that may
// fail without failing the run would let a failed gate post a passing status,
// so the pin refuses the key wherever it appears.
func postLandContinueOnErrorPaths(node *yaml.Node) []string {
	var paths []string
	var walk func(node *yaml.Node, path string)
	walk = func(node *yaml.Node, path string) {
		if node == nil {
			return
		}
		switch node.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(node.Content); i += 2 {
				key := node.Content[i].Value
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				if key == "continue-on-error" {
					paths = append(paths, childPath)
				}
				walk(node.Content[i+1], childPath)
			}
		case yaml.SequenceNode:
			for i, child := range node.Content {
				walk(child, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(node, "")
	return paths
}

// postLandSetScalar sets key to a string scalar on mapping, replacing an
// existing value so the document never carries a duplicate key.
func postLandSetScalar(mapping *yaml.Node, key, value string) {
	if existing := postLandField(mapping, key); existing != nil {
		*existing = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// postLandMappingKeys returns a mapping's keys in document order.
func postLandMappingKeys(mapping *yaml.Node) []string {
	keys := make([]string, 0, len(mapping.Content)/2)
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keys = append(keys, mapping.Content[i].Value)
	}
	return keys
}

// postLandSteps returns the job's steps, or nil when the job has none.
func postLandSteps(job *yaml.Node) []*yaml.Node {
	steps := postLandField(job, "steps")
	if steps == nil || steps.Kind != yaml.SequenceNode {
		return nil
	}
	return steps.Content
}

// postLandScalar renders a node for an error message.
func postLandScalar(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.ScalarNode {
		return "<absent>"
	}
	return node.Value
}

// postLandHeader returns the leading comment block of the file, or "" when the
// first non-blank line is not a comment.
func postLandHeader(raw []byte) string {
	var header []string
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			header = append(header, line)
			continue
		}
		if len(header) == 0 && trimmed == "" {
			continue
		}
		break
	}
	return strings.Join(header, "\n")
}

// postLandCredentialMarkers are the substrings that make a mapping key read as
// a credential. The post-land check runs on the host runner and needs none.
var postLandCredentialMarkers = []string{"secret", "token", "password", "passwd", "credential"}

// postLandCredentialKeys returns every mapping key, at any depth, that reads as
// a credential.
func postLandCredentialKeys(node *yaml.Node) []string {
	if node == nil {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		var keys []string
		for _, child := range node.Content {
			keys = append(keys, postLandCredentialKeys(child)...)
		}
		return keys
	}

	var keys []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		name := strings.ToLower(node.Content[i].Value)
		for _, marker := range postLandCredentialMarkers {
			if strings.Contains(name, marker) {
				keys = append(keys, name)
				break
			}
		}
		keys = append(keys, postLandCredentialKeys(node.Content[i+1])...)
	}
	return keys
}
