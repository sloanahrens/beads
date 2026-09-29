//go:build cgo

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// be-h0k regression: `bd config get issue-prefix` run inside a polecat-style
// git worktree (its .beads holds only a redirect to the rig; an unrelated
// town workspace with issue-prefix hq sits above both) printed
// "issue-prefix (not set)". It must print the prefix the rig's commands
// actually use, the same one `bd create` uses: config.yaml issue-prefix,
// else the database's issue_prefix.
func TestEmbeddedConfigGetIssuePrefixThroughRedirect(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	t.Parallel()
	bd := buildEmbeddedBD(t)

	town, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mustWrite := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(town, ".beads", "config.yaml"), "issue-prefix: hq\nrouting:\n  mode: explicit\n")

	// HOME must not be the town: <home>/.beads/config.yaml is the legacy
	// user-level config location and would load the town config on purpose.
	env := append(bdEnv(t.TempDir()), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	run := func(dir, name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = env
		stdout, stderr, err := runCommandBuffers(t, cmd)
		if err != nil {
			t.Fatalf("%s %s in %s: %v\nstdout:\n%s\nstderr:\n%s", filepath.Base(name), strings.Join(args, " "), dir, err, stdout.String(), stderr.String())
		}
		return stdout.String()
	}

	rig := filepath.Join(town, "rig", "mayor", "rig")
	if err := os.MkdirAll(rig, 0o755); err != nil {
		t.Fatal(err)
	}
	run(rig, "git", "init", "-q", "-b", "main")
	mustWrite(filepath.Join(rig, "README"), "rig\n")
	run(rig, "git", "add", "README")
	run(rig, "git", "commit", "-q", "-m", "init")
	run(rig, bd, "init", "--quiet", "--prefix", "zz")

	worktree := filepath.Join(town, "rig", "polecats", "amber", "rig")
	run(rig, "git", "worktree", "add", "-q", "-b", "polecat-amber", worktree)
	// Gastown hides every tracked .beads file in polecat worktrees and leaves
	// only the redirect.
	if err := os.RemoveAll(filepath.Join(worktree, ".beads")); err != nil {
		t.Fatal(err)
	}
	mustWrite(filepath.Join(worktree, ".beads", "redirect"), "../../../mayor/rig/.beads\n")

	for _, dir := range []string{worktree, rig} {
		if got := strings.TrimSpace(run(dir, bd, "config", "get", "issue-prefix")); got != "zz" {
			t.Errorf("bd config get issue-prefix in %s = %q, want %q", dir, got, "zz")
		}
		var res map[string]any
		out := run(dir, bd, "config", "get", "issue-prefix", "--json")
		if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &res); err != nil {
			t.Fatalf("parse --json output %q: %v", out, err)
		}
		if res["value"] != "zz" {
			t.Errorf("bd config get issue-prefix --json in %s: value = %q, want %q (%s)", dir, res["value"], "zz", out)
		}
		// The raw database key keeps its meaning.
		if got := strings.TrimSpace(run(dir, bd, "config", "get", "issue_prefix")); got != "zz" {
			t.Errorf("bd config get issue_prefix in %s = %q, want %q", dir, got, "zz")
		}
	}

	// BD_ISSUE_PREFIX overrides both, and --json says so.
	env = append(env, "BD_ISSUE_PREFIX=ee")
	out := run(worktree, bd, "config", "get", "issue-prefix", "--json")
	var envRes map[string]any
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &envRes); err != nil {
		t.Fatalf("parse --json output %q: %v", out, err)
	}
	if envRes["value"] != "ee" || envRes["location"] != "env var" {
		t.Errorf("with BD_ISSUE_PREFIX=ee: got %v, want value ee from env var", envRes)
	}
	env = env[:len(env)-1]

	// config.yaml wins over the database, as in bd create. Set it in the rig
	// only: the worktree must read it through the redirect, never the town's.
	rigConfig := filepath.Join(rig, ".beads", "config.yaml")
	existing, _ := os.ReadFile(rigConfig)
	mustWrite(rigConfig, string(existing)+"\nissue-prefix: yy\n")
	if got := strings.TrimSpace(run(worktree, bd, "config", "get", "issue-prefix")); got != "yy" {
		t.Errorf("bd config get issue-prefix in worktree after rig config.yaml set = %q, want %q", got, "yy")
	}
}
