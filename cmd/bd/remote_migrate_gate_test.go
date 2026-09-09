package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/steveyegge/beads/internal/storage/schema"
)

// TestHandleRemoteMigrateGateJSON_Shape verifies the JSON written to stderr has
// the expected shape: error one-liner, a non-runnable directive hint (NOT the
// escape-hatch command), and a remote_migrate_gate subobject carrying the
// current/latest/pending versions plus the agent-decision fields and options.
func TestHandleRemoteMigrateGateJSON_Shape(t *testing.T) {
	gate := &schema.RemoteMigrateGateError{CurrentVersion: 48, LatestVersion: 50, Pending: 2}

	origStderr := os.Stderr
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	handleRemoteMigrateGateJSON(gate)

	_ = w.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("json.Unmarshal stderr: %v\nstderr was: %s", err, buf.String())
	}

	if got, ok := parsed["error"].(string); !ok || got != gate.Error() {
		t.Errorf("error = %v, want %q", parsed["error"], gate.Error())
	}
	// hint is the non-runnable directive, and must NOT be the runnable escape
	// command — that swap is the whole point of this change (the agent footgun).
	if got, ok := parsed["hint"].(string); !ok || got != gate.AgentDirective() {
		t.Errorf("hint = %v, want directive %q", parsed["hint"], gate.AgentDirective())
	}
	if parsed["hint"] == gate.EscapeHint() {
		t.Errorf("hint must not be the runnable escape command %q (agent footgun)", gate.EscapeHint())
	}

	obj, ok := parsed["remote_migrate_gate"].(map[string]interface{})
	if !ok {
		t.Fatalf("remote_migrate_gate key missing or wrong type: %T", parsed["remote_migrate_gate"])
	}
	if got, ok := obj["current_version"].(float64); !ok || int(got) != 48 {
		t.Errorf("current_version = %v, want 48", obj["current_version"])
	}
	if got, ok := obj["latest_version"].(float64); !ok || int(got) != 50 {
		t.Errorf("latest_version = %v, want 50", obj["latest_version"])
	}
	if got, ok := obj["pending"].(float64); !ok || int(got) != 2 {
		t.Errorf("pending = %v, want 2", obj["pending"])
	}
	if got, ok := obj["human_decision_required"].(bool); !ok || !got {
		t.Errorf("human_decision_required = %v, want true", obj["human_decision_required"])
	}
	if got, ok := obj["severity"].(string); !ok || got != "blocking" {
		t.Errorf("severity = %v, want \"blocking\"", obj["severity"])
	}

	// options must carry both paths, and the runnable migrate command must live
	// ONLY inside the conditional migrate option, never surfaced unconditionally.
	rawOpts, ok := obj["options"].([]interface{})
	if !ok || len(rawOpts) != 2 {
		t.Fatalf("options = %v, want 2 entries", obj["options"])
	}
	ids := map[string]bool{}
	migrateCmdFound := false
	for _, ro := range rawOpts {
		o, ok := ro.(map[string]interface{})
		if !ok {
			t.Fatalf("option wrong type: %T", ro)
		}
		id, _ := o["id"].(string)
		ids[id] = true
		if o["when"] == nil || o["risk"] == nil {
			t.Errorf("option %q missing when/risk: %v", id, o)
		}
		cmds, _ := o["commands"].([]interface{})
		for _, c := range cmds {
			if c == gate.EscapeHint() {
				migrateCmdFound = true
				if id != "migrate" {
					t.Errorf("escape command appears under option %q, want only \"migrate\"", id)
				}
			}
		}
	}
	if !ids["migrate"] || !ids["adopt"] {
		t.Errorf("options ids = %v, want migrate+adopt", ids)
	}
	if !migrateCmdFound {
		t.Errorf("migrate option must contain the escape command %q", gate.EscapeHint())
	}
}

// TestHandleRemoteMigrateGateJSON_FallbackReason verifies the blunt-block JSON
// carries the machine-readable fallback_reason field (gastownhall/beads#4551
// follow-up), and that the smart-tailored adopt/fork-skew decisions never gain
// it — those already explain themselves via "decision".
func TestHandleRemoteMigrateGateJSON_FallbackReason(t *testing.T) {
	captureJSON := func(gate *schema.RemoteMigrateGateError) map[string]interface{} {
		origStderr := os.Stderr
		r, w, pipeErr := os.Pipe()
		if pipeErr != nil {
			t.Fatal(pipeErr)
		}
		os.Stderr = w
		defer func() { os.Stderr = origStderr }()
		handleRemoteMigrateGateJSON(gate)
		_ = w.Close()

		var buf bytes.Buffer
		if _, err := io.Copy(&buf, r); err != nil {
			t.Fatal(err)
		}
		_ = r.Close()

		var parsed map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("json.Unmarshal stderr: %v\nstderr was: %s", err, buf.String())
		}
		obj, ok := parsed["remote_migrate_gate"].(map[string]interface{})
		if !ok {
			t.Fatalf("remote_migrate_gate key missing or wrong type: %T", parsed["remote_migrate_gate"])
		}
		return obj
	}

	t.Run("blunt block carries fallback_reason", func(t *testing.T) {
		gate := &schema.RemoteMigrateGateError{
			CurrentVersion: 48, LatestVersion: 50, Pending: 2,
			FallbackReason: "below-convergence-floor",
		}
		obj := captureJSON(gate)
		if got, ok := obj["fallback_reason"].(string); !ok || got != "below-convergence-floor" {
			t.Errorf("fallback_reason = %v, want %q", obj["fallback_reason"], "below-convergence-floor")
		}
		if _, ok := obj["decision"]; ok {
			t.Errorf("blunt block must not carry a decision key, got %v", obj["decision"])
		}
	})

	t.Run("adopt decision never carries fallback_reason", func(t *testing.T) {
		gate := &schema.RemoteMigrateGateError{
			CurrentVersion: 48, LatestVersion: 50, Pending: 2,
			Decision: "adopt",
		}
		obj := captureJSON(gate)
		if _, ok := obj["fallback_reason"]; ok {
			t.Errorf("adopt decision must not carry fallback_reason, got %v", obj["fallback_reason"])
		}
	})
}

// TestHandleRemoteMigrateGateJSON_ServerNoRemote covers be-9yi: a server-mode
// database with no Dolt remote configured gets its own "decision" (not the
// generic adopt-or-migrate default text, which wrongly implies a remote to
// adopt from) and its options list must not offer "adopt" — there is nothing
// to re-clone from.
func TestHandleRemoteMigrateGateJSON_ServerNoRemote(t *testing.T) {
	gate := &schema.RemoteMigrateGateError{
		CurrentVersion: 53, LatestVersion: 58, Pending: 5,
		Decision: "server-no-remote",
	}

	origStderr := os.Stderr
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()
	handleRemoteMigrateGateJSON(gate)
	_ = w.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()

	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("json.Unmarshal stderr: %v\nstderr was: %s", err, buf.String())
	}
	obj, ok := parsed["remote_migrate_gate"].(map[string]interface{})
	if !ok {
		t.Fatalf("remote_migrate_gate key missing or wrong type: %T", parsed["remote_migrate_gate"])
	}

	if got, ok := obj["decision"].(string); !ok || got != "server-no-remote" {
		t.Errorf("decision = %v, want %q", obj["decision"], "server-no-remote")
	}
	if _, ok := obj["fallback_reason"]; ok {
		t.Errorf("server-no-remote decision must not carry fallback_reason, got %v", obj["fallback_reason"])
	}

	opts, ok := obj["options"].([]interface{})
	if !ok {
		t.Fatalf("options key missing or wrong type: %T", obj["options"])
	}
	for _, o := range opts {
		opt, ok := o.(map[string]interface{})
		if !ok {
			t.Fatalf("option entry wrong type: %T", o)
		}
		if opt["id"] == "adopt" {
			t.Errorf("options must not offer adopt with no remote to clone from: %v", opt)
		}
	}
}
