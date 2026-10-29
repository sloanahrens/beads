package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestWrapWithSchemaVersion_Envelope_Object(t *testing.T) {
	input := map[string]string{"id": "beads-123", "title": "Test"}
	result := wrapWithSchemaVersion(input)

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}
	if m["schema_version"] != JSONSchemaVersion {
		t.Errorf("schema_version = %v, want %d", m["schema_version"], JSONSchemaVersion)
	}
	data, ok := m["data"]
	if !ok {
		t.Fatal("missing 'data' key in envelope")
	}
	inner, ok := data.(map[string]string)
	if !ok {
		t.Fatalf("data type = %T, want map[string]string", data)
	}
	if inner["id"] != "beads-123" {
		t.Errorf("data.id = %v, want beads-123", inner["id"])
	}
}

func TestWrapWithSchemaVersion_Envelope_Slice(t *testing.T) {
	input := []string{"a", "b", "c"}
	result := wrapWithSchemaVersion(input)

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected envelope map, got %T", result)
	}
	if m["schema_version"] != JSONSchemaVersion {
		t.Errorf("schema_version = %v, want %d", m["schema_version"], JSONSchemaVersion)
	}
	data, ok := m["data"]
	if !ok {
		t.Fatal("missing 'data' key in envelope")
	}
	arr, ok := data.([]string)
	if !ok {
		t.Fatalf("data type = %T, want []string", data)
	}
	if len(arr) != 3 {
		t.Errorf("data length = %d, want 3", len(arr))
	}
}

func TestWrapWithSchemaVersion_Envelope_Nil(t *testing.T) {
	result := wrapWithSchemaVersion(nil)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected envelope map, got %T", result)
	}
	if m["schema_version"] != JSONSchemaVersion {
		t.Errorf("schema_version = %v, want %d", m["schema_version"], JSONSchemaVersion)
	}
	if m["data"] != nil {
		t.Errorf("data = %v, want nil", m["data"])
	}
}

func TestWrapWithSchemaVersion_Envelope_RoundTrip(t *testing.T) {
	input := map[string]interface{}{"count": 42, "name": "test"}
	result := wrapWithSchemaVersion(input)

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["schema_version"] != float64(JSONSchemaVersion) {
		t.Errorf("schema_version = %v, want %v", parsed["schema_version"], float64(JSONSchemaVersion))
	}
	innerData, ok := parsed["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data type = %T, want map[string]interface{}", parsed["data"])
	}
	if innerData["count"] != float64(42) {
		t.Errorf("data.count = %v, want 42", innerData["count"])
	}
}

func TestOutputJSONWithPagination_NoTruncation(t *testing.T) {
	items := []string{"a", "b"}
	out := captureStdout(t, func() error {
		return outputJSONWithPagination(items, nil)
	})

	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(bytes.TrimSpace([]byte(out))), &m); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	var arr []string
	if err := json.Unmarshal(m["data"], &arr); err != nil {
		t.Fatalf("unmarshal data: %v\noutput: %s", err, out)
	}
	if len(arr) != 2 {
		t.Errorf("len = %d, want 2", len(arr))
	}
}

func TestOutputJSONWithPagination_EnvelopeTruncated(t *testing.T) {
	items := []string{"a", "b", "c"}
	pag := &PaginationMeta{Returned: 3, Total: 10, Truncated: true}
	out := captureStdout(t, func() error {
		return outputJSONWithPagination(items, pag)
	})

	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(bytes.TrimSpace([]byte(out))), &m); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if _, ok := m["pagination"]; !ok {
		t.Fatal("missing 'pagination' key in envelope")
	}
	var got PaginationMeta
	if err := json.Unmarshal(m["pagination"], &got); err != nil {
		t.Fatalf("unmarshal pagination: %v", err)
	}
	if !got.Truncated {
		t.Error("pagination.truncated = false, want true")
	}
	if got.Returned != 3 {
		t.Errorf("pagination.returned = %d, want 3", got.Returned)
	}
	if got.Total != 10 {
		t.Errorf("pagination.total = %d, want 10", got.Total)
	}
}

// A truncated page must carry the pagination block on stdout, and nothing
// may be written to stderr: the old deprecation notice is gone.
func TestOutputJSONWithPagination_TruncatedPageAlwaysPaginated_NoStderrNotice(t *testing.T) {
	items := []string{"a", "b", "c"}
	pag := paginationMetaFor(true, 3)

	// One call, both streams: a notice written once per process would be
	// spent by an earlier call, so a second call under the stderr capture could
	// not show it. captureStderr holds stdioMutex, so captureStdout cannot nest
	// inside it; stdout goes to a file the test reads back.
	stdoutFile, err := os.CreateTemp(t.TempDir(), "stdout-*.json")
	if err != nil {
		t.Fatalf("create stdout file: %v", err)
	}
	defer stdoutFile.Close()
	stderr := captureStderr(t, func() {
		oldStdout := os.Stdout
		os.Stdout = stdoutFile
		defer func() { os.Stdout = oldStdout }()
		if err := outputJSONWithPagination(items, pag); err != nil {
			t.Errorf("outputJSONWithPagination: %v", err)
		}
	})
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
	raw, err := os.ReadFile(stdoutFile.Name())
	if err != nil {
		t.Fatalf("read stdout file: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, raw)
	}
	if _, ok := m["pagination"]; !ok {
		t.Fatalf("truncated page has no 'pagination' key; output: %s", raw)
	}
}

func TestPaginationMetaFor(t *testing.T) {
	if got := paginationMetaFor(false, 5); got != nil {
		t.Errorf("hasMore=false: got %+v, want nil", got)
	}
	got := paginationMetaFor(true, 5)
	if got == nil || !got.Truncated || got.Returned != 5 {
		t.Errorf("hasMore=true, returned=5: got %+v, want {Returned:5 Truncated:true}", got)
	}
	if got.Total != 0 {
		t.Errorf("Total = %d, want 0 (unset/omitempty — list/query have no full-count probe)", got.Total)
	}
}

func TestOutputJSONWithPagination_EnvelopeNotTruncated(t *testing.T) {
	items := []string{"a", "b"}
	out := captureStdout(t, func() error {
		return outputJSONWithPagination(items, nil)
	})

	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(bytes.TrimSpace([]byte(out))), &m); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if _, ok := m["pagination"]; ok {
		t.Error("unexpected 'pagination' key when not truncated")
	}
}
