//go:build cgo && integration

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSampleRoundTrip drives `bd sample add` and `bd sample list` against a
// real store: the refusals are unit-tier (sample_test.go), and this is the
// round trip they sit in front of.
//
// Payloads are asserted as DECODED JSON values here. Byte fidelity is the
// role's promise and is pinned by the samples contract; the CLI's --json
// output embeds the stored payload as a JSON value, and re-indentation is the
// envelope's, not a change to the value.
func TestSampleRoundTrip(t *testing.T) {
	env := newParityEnv(t)
	// newParityEnv installs a real store as the global; the sample verbs open
	// it through ensureDirectMode, which requires the active flag too.
	setSampleGlobalFlags(t)

	balance := writePayload(t, []byte(`{"balance": 12}`))
	usage := writePayload(t, []byte(`{"used": 1200}`))
	usageLater := writePayload(t, []byte(`{"used": 1300}`))

	env.setFlags(sampleAddCmd, map[string]string{
		"kind": "deepseek_balance", "at": "2026-11-11T09:00:00Z", "json": balance,
	})
	if result := env.run(sampleAddCmd); result.exitCode != 0 {
		t.Fatalf("bd sample add (balance) exited %d: %s", result.exitCode, result.stderr)
	}
	env.setFlags(sampleAddCmd, map[string]string{
		"kind": "usage_record", "at": "2026-11-11T10:00:00Z", "json": usage,
	})
	if result := env.run(sampleAddCmd); result.exitCode != 0 {
		t.Fatalf("bd sample add (usage) exited %d: %s", result.exitCode, result.stderr)
	}
	env.setFlags(sampleAddCmd, map[string]string{
		"kind": "usage_record", "at": "2026-11-11T11:00:00Z", "json": usageLater,
	})
	if result := env.run(sampleAddCmd); result.exitCode != 0 {
		t.Fatalf("bd sample add (usage later) exited %d: %s", result.exitCode, result.stderr)
	}

	// One kind's rows, newest first, payloads intact.
	env.setFlags(sampleListCmd, map[string]string{"kind": "usage_record", "since": "2026-11-11T00:00:00Z"})
	requireInheritedSampleListJSONFlag(t)
	jsonOutput = true
	result := env.run(sampleListCmd)
	if result.exitCode != 0 {
		t.Fatalf("bd sample list exited %d: %s", result.exitCode, result.stderr)
	}
	rows := decodeSampleList(t, result.stdout)
	if len(rows) != 2 {
		t.Fatalf("bd sample list returned %d rows, want 2: %s", len(rows), result.stdout)
	}
	if got := rows[0]["payload"]; !jsonEqual(got, map[string]any{"used": float64(1300)}) {
		t.Errorf("newest row payload = %#v, want {\"used\":1300}", got)
	}
	if got := rows[1]["payload"]; !jsonEqual(got, map[string]any{"used": float64(1200)}) {
		t.Errorf("second row payload = %#v, want {\"used\":1200}", got)
	}
	for _, row := range rows {
		if row["kind"] != "usage_record" {
			t.Errorf("row kind = %v, want usage_record", row["kind"])
		}
	}

	// A later --since keeps only the newer row, and it is inclusive at the
	// boundary the caller passes.
	env.setFlags(sampleListCmd, map[string]string{"kind": "usage_record", "since": "2026-11-11T11:00:00Z"})
	result = env.run(sampleListCmd)
	if result.exitCode != 0 {
		t.Fatalf("bd sample list --since exited %d: %s", result.exitCode, result.stderr)
	}
	if rows := decodeSampleList(t, result.stdout); len(rows) != 1 {
		t.Fatalf("bd sample list --since 11:00 returned %d rows, want 1", len(rows))
	}

	// --limit takes from the front of the newest-first order.
	env.setFlags(sampleListCmd, map[string]string{"kind": "usage_record", "since": "2026-11-11T00:00:00Z", "limit": "1"})
	result = env.run(sampleListCmd)
	if result.exitCode != 0 {
		t.Fatalf("bd sample list --limit exited %d: %s", result.exitCode, result.stderr)
	}
	if rows := decodeSampleList(t, result.stdout); len(rows) != 1 {
		t.Fatalf("bd sample list --limit 1 returned %d rows, want 1", len(rows))
	}

	// The other kind is untouched.
	env.setFlags(sampleListCmd, map[string]string{"kind": "deepseek_balance"})
	result = env.run(sampleListCmd)
	if result.exitCode != 0 {
		t.Fatalf("bd sample list (balance) exited %d: %s", result.exitCode, result.stderr)
	}
	if rows := decodeSampleList(t, result.stdout); len(rows) != 1 {
		t.Fatalf("bd sample list (balance) returned %d rows, want 1", len(rows))
	}
}

// TestSampleAddReportsTheStoredID pins the human-facing half: add prints the
// id of the row it stored, so a caller can name it later.
func TestSampleAddReportsTheStoredID(t *testing.T) {
	env := newParityEnv(t)
	setSampleGlobalFlags(t)

	env.setFlags(sampleAddCmd, map[string]string{
		"kind": "usage_record", "at": "2026-11-11T09:00:00Z", "json": writePayload(t, []byte(`{"used":1}`)),
	})
	result := env.run(sampleAddCmd)
	if result.exitCode != 0 {
		t.Fatalf("bd sample add exited %d: %s", result.exitCode, result.stderr)
	}
	if !strings.HasPrefix(result.stdout, "Added sample ") {
		t.Fatalf("bd sample add printed %q, want an 'Added sample <id>' line", result.stdout)
	}
	id := strings.TrimSpace(strings.TrimPrefix(result.stdout, "Added sample "))
	if id == "" {
		t.Fatal("bd sample add printed no id")
	}
}

// TestSampleAddRefusalWritesNothing pins the write side of a CLI refusal
// against a real store: the unit tests prove the refusal, and this proves it
// left no row behind.
func TestSampleAddRefusalWritesNothing(t *testing.T) {
	env := newParityEnv(t)
	setSampleGlobalFlags(t)

	env.setFlags(sampleAddCmd, map[string]string{
		"kind": "usage_record", "at": "2026-11-11T09:00:00Z", "json": writePayload(t, []byte(`[1,2]`)),
	})
	if result := env.run(sampleAddCmd); result.exitCode == 0 {
		t.Fatalf("bd sample add with an array payload exited 0, want a refusal")
	}

	env.setFlags(sampleListCmd, map[string]string{"kind": "usage_record"})
	jsonOutput = true
	result := env.run(sampleListCmd)
	if result.exitCode != 0 {
		t.Fatalf("bd sample list exited %d: %s", result.exitCode, result.stderr)
	}
	if rows := decodeSampleList(t, result.stdout); len(rows) != 0 {
		t.Fatalf("a refused add wrote %d rows: %s", len(rows), result.stdout)
	}
}

// decodeSampleList decodes the --json envelope's data array.
func decodeSampleList(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var envelope struct {
		SchemaVersion int              `json:"schema_version"`
		Data          []map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decoding bd sample list --json: %v\n%s", err, stdout)
	}
	if envelope.SchemaVersion != JSONSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", envelope.SchemaVersion, JSONSchemaVersion)
	}
	return envelope.Data
}

func jsonEqual(got any, want map[string]any) bool {
	gotMap, ok := got.(map[string]any)
	if !ok {
		return false
	}
	if len(gotMap) != len(want) {
		return false
	}
	for k, v := range want {
		if gotMap[k] != v {
			return false
		}
	}
	return true
}

// setSampleGlobalFlags turns on the two package globals these verbs read for
// one test and restores both when it ends. newParityEnv snapshots globals too,
// but a direct assignment here would make the restore a fact about a helper in
// another file; this keeps it a fact about this test.
func setSampleGlobalFlags(t *testing.T) {
	t.Helper()
	savedStoreActive, savedJSON := storeActive, jsonOutput
	storeActive, jsonOutput = true, false
	t.Cleanup(func() { storeActive, jsonOutput = savedStoreActive, savedJSON })
}

// requireInheritedSampleListJSONFlag keeps the list verb on its inherited
// --json bool. It exists so a future local flag on the verb cannot make these
// assertions pass vacuously against a non-JSON stdout. LocalFlags is what
// separates a local declaration from root's inherited bool: Flags reports the
// inherited one too once any test has executed the command tree.
func requireInheritedSampleListJSONFlag(t *testing.T) {
	t.Helper()
	if flag := sampleListCmd.LocalFlags().Lookup("json"); flag != nil {
		t.Fatalf("bd sample list declares a local --json flag (%s); this test sets the inherited bool", flag.Value.Type())
	}
}
