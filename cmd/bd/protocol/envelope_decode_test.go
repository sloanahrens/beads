package protocol

import "testing"

// TestParseJSONOutputReadsTheEnvelope pins how the protocol tests read bd
// --json stdout, {"schema_version": N, "data": ...}: an array under data yields
// its elements, an object under data yields one element, and the envelope's
// schema_version stays reachable for the tests that check it.
func TestParseJSONOutputReadsTheEnvelope(t *testing.T) {
	t.Parallel()

	t.Run("array under data yields its elements", func(t *testing.T) {
		t.Parallel()
		out := `{"schema_version":1,"data":[{"id":"a-1"},{"id":"a-2"}]}`
		items := parseJSONOutput(t, out)
		if len(items) != 2 || items[0]["id"] != "a-1" || items[1]["id"] != "a-2" {
			t.Fatalf("parseJSONOutput(%s) = %v, want a-1 and a-2", out, items)
		}
	})

	t.Run("object under data yields one element", func(t *testing.T) {
		t.Parallel()
		out := `{"schema_version":1,"data":{"id":"a-1","title":"t"}}`
		items := parseJSONOutput(t, out)
		if len(items) != 1 || items[0]["id"] != "a-1" || items[0]["title"] != "t" {
			t.Fatalf("parseJSONOutput(%s) = %v, want one element a-1", out, items)
		}
	})

	t.Run("empty array under data yields nothing", func(t *testing.T) {
		t.Parallel()
		if items := parseJSONOutput(t, `{"schema_version":1,"data":[]}`); len(items) != 0 {
			t.Fatalf("parseJSONOutput of an empty data array = %v, want none", items)
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
