package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// machineEnvelope is the one JSON document machine mode writes to stdout.
type machineEnvelope struct {
	SchemaVersion   int             `json:"schema_version"`
	ContractVersion int             `json:"contract_version"`
	Data            json.RawMessage `json:"data"`
	Error           *struct {
		Kind    string         `json:"kind"`
		Message string         `json:"message"`
		IDs     []machineIDErr `json:"ids"`
		Detail  map[string]any `json:"detail"`
	} `json:"error"`
}

type machineIDErr struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// runMachine runs bd under --machine and returns the parsed envelope and the
// exit code. stdout must be exactly one envelope; stderr is returned for
// failure messages only.
func (w *workspace) runMachine(args ...string) (machineEnvelope, int, string) {
	w.t.Helper()
	cmd := exec.Command(w.bd, append(args, "--machine")...)
	cmd.Dir = w.dir
	cmd.Env = w.env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			w.t.Fatalf("bd %s: %v", strings.Join(args, " "), err)
		}
		code = ee.ExitCode()
	}
	var env machineEnvelope
	if jerr := json.Unmarshal(stdout.Bytes(), &env); jerr != nil {
		w.t.Fatalf("bd %s --machine: stdout is not one envelope: %v\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), jerr, stdout.String(), stderr.String())
	}
	if env.ContractVersion != 1 {
		w.t.Fatalf("contract_version = %d, want 1", env.ContractVersion)
	}
	return env, code, stderr.String()
}

// sqlScalar reads one value from the workspace's embedded database behind
// bd's back (see storeExec).
func (w *workspace) sqlScalar(query string) string {
	w.t.Helper()
	dbDir := filepath.Join(w.dir, ".beads", "embeddeddolt", w.storeDatabase(w.t))
	cmd := exec.Command("dolt", "sql", "-r", "csv", "-q", query)
	cmd.Dir = dbDir
	cmd.Env = append(os.Environ(), "DOLT_ROOT_PATH="+w.t.TempDir())
	raw, err := cmd.CombinedOutput()
	if err != nil {
		w.t.Fatalf("dolt sql -q %q: %v\n%s", query, err, raw)
	}
	out := string(raw)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return ""
	}
	return strings.TrimSpace(lines[len(lines)-1])
}

func errKind(env machineEnvelope) string {
	if env.Error == nil {
		return ""
	}
	return env.Error.Kind
}

// ---------------------------------------------------------------------------
// be-qr3: rename-prefix --config-only
// ---------------------------------------------------------------------------

func TestProtocol_RenamePrefixConfigOnly(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	w.create("--title", "row one", "--type", "task")
	storedPrefix := func() string {
		return w.sqlScalar("SELECT value FROM config WHERE `key` = 'issue_prefix'")
	}

	// Equal prefix: success, nothing changes.
	env, code, stderr := w.runMachine("rename-prefix", w.prefix, "--config-only")
	if code != 0 {
		t.Fatalf("equal prefix: exit %d, error %+v\n%s", code, env.Error, stderr)
	}
	var data struct {
		OldPrefix string `json:"old_prefix"`
		NewPrefix string `json:"new_prefix"`
		Changed   bool   `json:"changed"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("data: %v (%s)", err, env.Data)
	}
	if data.Changed || data.NewPrefix != w.prefix {
		t.Fatalf("equal prefix: data = %+v", data)
	}

	// Unset prefix with matching rows: the cell is written, no id rewritten.
	w.storeExec(t, "DELETE FROM config WHERE `key` = 'issue_prefix'")
	env, code, stderr = w.runMachine("rename-prefix", w.prefix, "--config-only")
	if code != 0 {
		t.Fatalf("unset prefix: exit %d, error %+v\n%s", code, env.Error, stderr)
	}
	if got := storedPrefix(); got != w.prefix {
		t.Fatalf("unset prefix: stored = %q, want %q", got, w.prefix)
	}

	// Stale prefix with matching rows: repaired.
	w.storeExec(t, "UPDATE config SET value = 'stale' WHERE `key` = 'issue_prefix'")
	env, code, _ = w.runMachine("rename-prefix", w.prefix, "--config-only")
	if code != 0 {
		t.Fatalf("stale prefix: exit %d, error %+v", code, env.Error)
	}
	if got := storedPrefix(); got != w.prefix {
		t.Fatalf("stale prefix: stored = %q, want %q", got, w.prefix)
	}

	// A prefix the rows do not carry would need id rewriting: refused, nothing written.
	env, code, _ = w.runMachine("rename-prefix", "other", "--config-only")
	if code != 21 || errKind(env) != "refused" {
		t.Fatalf("mismatch: exit %d kind %q, want 21 refused", code, errKind(env))
	}
	if got := storedPrefix(); got != w.prefix {
		t.Fatalf("mismatch wrote the prefix: stored = %q", got)
	}
}

// ---------------------------------------------------------------------------
// be-cgr: dep prune-orphans
// ---------------------------------------------------------------------------

func TestProtocol_DepPruneOrphans(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	a := w.create("--title", "keeps its edge", "--type", "task")
	b := w.create("--title", "live target", "--type", "task")
	c := w.create("--title", "loses its target", "--type", "task")
	wisp := w.create("--title", "wisp target", "--type", "task", "--ephemeral")
	w.run("dep", "add", a, b)
	w.run("dep", "add", c, wisp)
	// Remove the wisp row underneath the edge: dependencies.depends_on_wisp_id
	// has no foreign key, so this leaves exactly one orphan.
	w.storeExec(t, "DELETE FROM wisps WHERE id = '"+wisp+"'")
	// A wisp-to-wisp edge lives in wisp_dependencies, whose foreign keys
	// cascade; turn them off so the delete leaves the edge behind, the way
	// rows orphaned while the constraints were missing look.
	w1 := w.create("--title", "wisp source", "--type", "task", "--ephemeral")
	w2 := w.create("--title", "wisp target gone", "--type", "task", "--ephemeral")
	w.run("dep", "add", w1, w2)
	w.storeExec(t, "SET FOREIGN_KEY_CHECKS = 0; DELETE FROM wisps WHERE id = '"+w2+"'")
	if got := w.sqlScalar("SELECT COUNT(*) FROM wisp_dependencies WHERE issue_id = '" + w1 + "'"); got != "1" {
		t.Fatalf("setup: %s wisp_dependencies rows for %s, want 1", got, w1)
	}

	countDeps := func() string { return w.sqlScalar("SELECT COUNT(*) FROM dependencies") }
	if got := countDeps(); got != "2" {
		t.Fatalf("setup: %s dependency rows, want 2", got)
	}

	type counts struct {
		DryRun           bool `json:"dry_run"`
		Dependencies     int  `json:"dependencies"`
		WispDependencies int  `json:"wisp_dependencies"`
		Total            int  `json:"total"`
	}
	env, code, stderr := w.runMachine("dep", "prune-orphans", "--dry-run")
	if code != 0 {
		t.Fatalf("dry run: exit %d error %+v\n%s", code, env.Error, stderr)
	}
	var got counts
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("data: %v (%s)", err, env.Data)
	}
	if !got.DryRun || got.Dependencies != 1 || got.WispDependencies != 1 || got.Total != 2 {
		t.Fatalf("dry run counts = %+v, want 1 + 1 orphans", got)
	}
	if n := countDeps(); n != "2" {
		t.Fatalf("dry run deleted rows: %s left", n)
	}

	env, code, _ = w.runMachine("dep", "prune-orphans")
	if code != 0 {
		t.Fatalf("prune: exit %d error %+v", code, env.Error)
	}
	got = counts{}
	_ = json.Unmarshal(env.Data, &got)
	if got.DryRun || got.Dependencies != 1 || got.WispDependencies != 1 || got.Total != 2 {
		t.Fatalf("prune counts = %+v", got)
	}
	if n := w.sqlScalar("SELECT COUNT(*) FROM wisp_dependencies WHERE issue_id = '" + w1 + "'"); n != "0" {
		t.Fatalf("after prune: %s wisp_dependencies rows for %s, want 0", n, w1)
	}
	if n := countDeps(); n != "1" {
		t.Fatalf("after prune: %s rows, want the live edge only", n)
	}

	env, code, _ = w.runMachine("dep", "prune-orphans")
	got = counts{}
	_ = json.Unmarshal(env.Data, &got)
	if code != 0 || got.Total != 0 {
		t.Fatalf("second prune: exit %d counts %+v, want 0", code, got)
	}
}

// ---------------------------------------------------------------------------
// be-pgd: close/delete --if-status / --if-assignee
// ---------------------------------------------------------------------------

func TestProtocol_CloseWriteTimeGuards(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	open := w.create("--title", "still open", "--type", "task")
	moved := w.create("--title", "moved on", "--type", "task")
	w.run("update", moved, "--status", "in_progress", "--assignee", "someone")

	// One matches, one does not: the match closes, the mismatch is untouched.
	// A batch whose only failures are guard refusals keeps guard_not_held
	// (exit 13) even beside successes: the d1 batch rule bd update has.
	env, code, _ := w.runMachine("close", open, moved, "--if-status", "open")
	if code != 13 || errKind(env) != "guard_not_held" {
		t.Fatalf("mixed batch: exit %d kind %q, want 13 guard_not_held", code, errKind(env))
	}
	if len(env.Error.IDs) != 1 || env.Error.IDs[0].ID != moved || env.Error.IDs[0].Kind != "guard_not_held" {
		t.Fatalf("mixed batch ids = %+v", env.Error.IDs)
	}
	if s := w.showJSON(open)["status"]; s != "closed" {
		t.Fatalf("%s status %v, want closed", open, s)
	}
	if s := w.showJSON(moved)["status"]; s != "in_progress" {
		t.Fatalf("%s status %v, want in_progress (guard must write nothing)", moved, s)
	}

	// Every id refused by a guard: guard_not_held, exit 13, like bd update.
	env, code, _ = w.runMachine("close", moved, "--if-assignee", "nobody")
	if code != 13 || errKind(env) != "guard_not_held" {
		t.Fatalf("assignee mismatch: exit %d kind %q, want 13 guard_not_held", code, errKind(env))
	}

	// Guards that hold do not waive close policy (another actor's claim), so
	// the reaper's shape is --force plus guards.
	env, code, _ = w.runMachine("close", moved, "--if-assignee", "someone", "--if-status", "in_progress", "--force")
	if code != 0 {
		t.Fatalf("matching guards: exit %d error %+v", code, env.Error)
	}
	if s := w.showJSON(moved)["status"]; s != "closed" {
		t.Fatalf("%s status %v, want closed", moved, s)
	}

	// A typo in --if-status fails fast.
	if _, code, _ := w.runMachine("close", open, "--if-status", "opne"); code != 27 {
		t.Fatalf("invalid --if-status: exit %d, want 27", code)
	}
}

func TestProtocol_DeleteWriteTimeGuards(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	a := w.create("--title", "open a", "--type", "task")
	b := w.create("--title", "pinned b", "--type", "task")
	w.run("update", b, "--status", "in_progress")

	// Delete is all-or-nothing: one mismatch refuses the request.
	env, code, _ := w.runMachine("delete", a, b, "--force", "--if-status", "open")
	if code != 13 || errKind(env) != "guard_not_held" {
		t.Fatalf("mismatch: exit %d kind %q, want 13 guard_not_held", code, errKind(env))
	}
	if len(env.Error.IDs) != 1 || env.Error.IDs[0].ID != b {
		t.Fatalf("mismatch ids = %+v, want only %s", env.Error.IDs, b)
	}
	w.showJSON(a) // still there
	w.showJSON(b)

	env, code, _ = w.runMachine("delete", a, "--force", "--if-status", "open", "--if-assignee", "")
	if code != 0 {
		t.Fatalf("matching guard: exit %d error %+v", code, env.Error)
	}
	if _, err := w.tryRun("show", a, "--json"); err == nil {
		t.Fatalf("%s still exists after guarded delete", a)
	}

	if _, code, _ := w.runMachine("delete", b, "--force", "--cascade", "--if-status", "in_progress"); code != 27 {
		t.Fatalf("guards with --cascade: exit %d, want 27", code)
	}
}

// ---------------------------------------------------------------------------
// be-u20: land-record
// ---------------------------------------------------------------------------

func TestProtocol_LandRecord(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	id := w.create("--title", "work bead", "--type", "task")

	env, code, stderr := w.runMachine("land-record", id, "--reject", "--kind", "gate_failed",
		"--gate-tail", "FAIL pkg/x", "--om-findings", `[{"severity":"major","text":"no test"}]`,
		"--conflicting-file", "a.go", "--conflicting-file", "b.go")
	if code != 0 {
		t.Fatalf("reject: exit %d error %+v\n%s", code, env.Error, stderr)
	}
	shown := w.showJSON(id)
	meta, _ := shown["metadata"].(map[string]any)
	rej, _ := meta["landing_rejection"].(map[string]any)
	if rej["kind"] != "gate_failed" || rej["gate_tail"] != "FAIL pkg/x" {
		t.Fatalf("landing_rejection = %v", rej)
	}
	if files, _ := rej["conflicting_files"].([]any); len(files) != 2 {
		t.Fatalf("conflicting_files = %v", rej["conflicting_files"])
	}
	if f, _ := rej["om_findings"].([]any); len(f) != 1 {
		t.Fatalf("om_findings = %v", rej["om_findings"])
	}
	if !hasLabel(shown, "rework") {
		t.Fatalf("rejection did not add rework label: %v", shown["labels"])
	}

	env, code, stderr = w.runMachine("land-record", id, "--patch-id", "abc123", "--landed-commit", "def456",
		"--gate-result", "pass", "--om-verdict", "approve", "--om-score", "8.5", "--route", "merge-queue")
	if code != 0 {
		t.Fatalf("land: exit %d error %+v\n%s", code, env.Error, stderr)
	}
	shown = w.showJSON(id)
	meta, _ = shown["metadata"].(map[string]any)
	land, _ := meta["landing"].(map[string]any)
	want := map[string]any{"patch_id": "abc123", "landed_commit": "def456", "gate_result": "pass",
		"om_verdict": "approve", "om_score": 8.5, "route": "merge-queue"}
	for k, v := range want {
		if land[k] != v {
			t.Fatalf("landing[%s] = %v, want %v (landing = %v)", k, land[k], v, land)
		}
	}
	if hasLabel(shown, "rework") {
		t.Fatalf("landing did not clear rework label")
	}

	if _, code, _ := w.runMachine("land-record", id, "--patch-id", "x"); code != 27 {
		t.Fatalf("incomplete landing: exit %d, want 27", code)
	}
	if _, code, _ := w.runMachine("land-record", w.prefix+"-nope", "--reject", "--kind", "x"); code != 20 {
		t.Fatalf("unknown id: exit %d, want 20", code)
	}
}

// TestProtocol_LandingRecordViaUpdate pins the generic surface that can carry
// the same record without land-record: one bd update merges a metadata key
// and edits a label in one write.
func TestProtocol_LandingRecordViaUpdate(t *testing.T) {
	t.Parallel()
	w := newWorkspace(t)
	id := w.create("--title", "work bead", "--type", "task")
	env, code, stderr := w.runMachine("update", id,
		"--metadata", `{"landing_rejection":{"kind":"conflict","conflicting_files":["a.go"]}}`,
		"--add-label", "rework")
	if code != 0 {
		t.Fatalf("update: exit %d error %+v\n%s", code, env.Error, stderr)
	}
	shown := w.showJSON(id)
	meta, _ := shown["metadata"].(map[string]any)
	rej, _ := meta["landing_rejection"].(map[string]any)
	if rej["kind"] != "conflict" || !hasLabel(shown, "rework") {
		t.Fatalf("metadata = %v labels = %v", meta, shown["labels"])
	}
}

func hasLabel(issue map[string]any, label string) bool {
	labels, _ := issue["labels"].([]any)
	for _, l := range labels {
		if l == label {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// capabilities lists the new verbs
// ---------------------------------------------------------------------------

func TestProtocol_CapabilitiesListMachineVerbs(t *testing.T) {
	t.Parallel()
	bd := buildBD(t)
	out, err := exec.Command(bd, "capabilities", "--json").Output()
	if err != nil {
		t.Fatalf("capabilities: %v", err)
	}
	var caps struct {
		Commands []struct {
			Path  string `json:"path"`
			Flags []struct {
				Name string `json:"name"`
			} `json:"flags"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(out, &caps); err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := map[string][]string{
		"rename-prefix":     {"config-only"},
		"dep prune-orphans": {"dry-run"},
		"close":             {"if-status", "if-assignee"},
		"delete":            {"if-status", "if-assignee"},
		"land-record":       {"patch-id", "landed-commit", "gate-result", "om-verdict", "om-score", "route", "reject", "kind", "gate-tail", "gate-tail-file", "om-findings", "conflicting-file"},
	}
	for path, flags := range want {
		found := false
		for _, c := range caps.Commands {
			if c.Path != path {
				continue
			}
			found = true
			have := map[string]bool{}
			for _, f := range c.Flags {
				have[f.Name] = true
			}
			for _, f := range flags {
				if !have[f] {
					t.Errorf("%s lacks --%s", path, f)
				}
			}
		}
		if !found {
			t.Errorf("capabilities lacks %s", path)
		}
	}
}
