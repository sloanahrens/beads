package protocol

import "testing"

// TestParseJSONOutputReadsTheEnvelope pins how the protocol tests read bd
// --json stdout, {"schema_version": N, "data": ...}: one row per shape of data.
func TestParseJSONOutputReadsTheEnvelope(t *testing.T) {
	t.Parallel()

	rows := []struct {
		name    string
		data    any
		wantIDs []string
		wantErr bool
	}{
		{"array of objects yields its elements", []any{map[string]any{"id": "a-1"}, map[string]any{"id": "a-2"}}, []string{"a-1", "a-2"}, false},
		{"object yields one element", map[string]any{"id": "a-1"}, []string{"a-1"}, false},
		{"empty array yields nothing", []any{}, nil, false},
		{"null yields nothing", nil, nil, false},
		{"array holding a non-object is an error", []any{map[string]any{"id": "a-1"}, "x"}, nil, true},
		{"string is an error", "a-1", nil, true},
		{"number is an error", float64(3), nil, true},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			items, err := envelopeElements(row.data)
			if (err != nil) != row.wantErr {
				t.Fatalf("envelopeElements(%v) error = %v, want error %v", row.data, err, row.wantErr)
			}
			var got []string
			for _, it := range items {
				id, _ := it["id"].(string)
				got = append(got, id)
			}
			if len(got) != len(row.wantIDs) {
				t.Fatalf("envelopeElements(%v) ids = %v, want %v", row.data, got, row.wantIDs)
			}
			for i := range got {
				if got[i] != row.wantIDs[i] {
					t.Fatalf("envelopeElements(%v) ids = %v, want %v", row.data, got, row.wantIDs)
				}
			}
		})
	}

	t.Run("parseJSONOutput unwraps the envelope", func(t *testing.T) {
		t.Parallel()
		items := parseJSONOutput(t, `{"schema_version":1,"data":[{"id":"a-1"},{"id":"a-2"}]}`)
		if len(items) != 2 || items[0]["id"] != "a-1" || items[1]["id"] != "a-2" {
			t.Fatalf("parseJSONOutput = %v, want a-1 and a-2", items)
		}
	})

	t.Run("schema_version stays reachable on the envelope", func(t *testing.T) {
		t.Parallel()
		env, items := parseEnvelope(t, `{"schema_version":1,"data":{"id":"a-1"}}`)
		assertSchemaVersion(t, env, "envelope")
		if _, ok := items[0]["schema_version"]; ok {
			t.Errorf("schema_version leaked into the data element: %v", items[0])
		}
	})
}
