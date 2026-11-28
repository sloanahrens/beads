package types

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestExternalDependenciesJSON pins both members: a zero view emits neither
// key, and a filled one emits both with each edge's target id and type.
func TestExternalDependenciesJSON(t *testing.T) {
	t.Parallel()

	t.Run("zero view omits both", func(t *testing.T) {
		t.Parallel()
		raw, err := json.Marshal(IssueDetails{})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(raw), "external_dependencies") {
			t.Errorf("zero view emits external dependencies: %s", raw)
		}
	})

	t.Run("filled view carries both", func(t *testing.T) {
		t.Parallel()
		unread := true
		raw, err := json.Marshal(IssueDetails{
			ExternalDependencies:       []ExternalDependency{{ID: "external:other#bd-2", Type: DepBlocks}, {ID: "bd-remote-2", Type: DepRelated}},
			ExternalDependenciesUnread: &unread,
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		for _, want := range []string{
			`"external_dependencies":[{"id":"external:other#bd-2","type":"blocks"},{"id":"bd-remote-2","type":"related"}]`,
			`"external_dependencies_unread":true`,
		} {
			if !strings.Contains(string(raw), want) {
				t.Errorf("view JSON lacks %s: %s", want, raw)
			}
		}
	})
}
