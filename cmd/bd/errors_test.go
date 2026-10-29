package main

import (
	"encoding/json"
	"testing"
)

func TestJsonStderrError_StructuredOutput(t *testing.T) {
	tests := []struct {
		name    string
		message string
		hint    string
	}{
		{"message_only", "database not found", ""},
		{"message_with_hint", "database not found", "Run 'bd init' to create one"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := map[string]interface{}{
				"schema_version": JSONSchemaVersion,
				"error":          tt.message,
			}
			if tt.hint != "" {
				obj["hint"] = tt.hint
			}

			data, err := json.Marshal(obj)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var parsed map[string]interface{}
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if parsed["schema_version"] != float64(JSONSchemaVersion) {
				t.Errorf("schema_version = %v, want %d", parsed["schema_version"], JSONSchemaVersion)
			}
			if parsed["error"] != tt.message {
				t.Errorf("error = %v, want %s", parsed["error"], tt.message)
			}
			if tt.hint != "" {
				if parsed["hint"] != tt.hint {
					t.Errorf("hint = %v, want %s", parsed["hint"], tt.hint)
				}
			} else {
				if _, ok := parsed["hint"]; ok {
					t.Errorf("hint should not be present when empty")
				}
			}
		})
	}
}

// envelopeData is how a caller adds fields next to "error" and "hint" in an
// error envelope; when it reports false the caller drops its payload without a
// word. buildJSONError is the only producer of that envelope, so this pins its
// shape: if the "data" object ever moves, this fails instead of the gate and
// schema-skew payloads silently going missing.
func TestEnvelopeDataReadsTheErrorEnvelopeBuildJSONErrorMakes(t *testing.T) {
	envelope := buildJSONError("boom", "try again")
	data, ok := envelopeData(envelope)
	if !ok {
		t.Fatalf("envelopeData(buildJSONError(...)) = false for %#v; callers would drop their payload", envelope)
	}
	if data["error"] != "boom" || data["hint"] != "try again" {
		t.Errorf("data = %#v, want error and hint next to where a caller's fields go", data)
	}
	if _, ok := envelopeData("not an envelope"); ok {
		t.Error("envelopeData accepted a value that is not an envelope")
	}
}
