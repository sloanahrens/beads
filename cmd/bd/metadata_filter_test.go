//go:build cgo

package main

import (
	"testing"

	"github.com/steveyegge/beads/internal/storage"
)

func TestValidateMetadataKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key     string
		wantErr bool
	}{
		{"team", false},
		{"story_points", false},
		{"jira.sprint", false},
		{"jira/sprint", false},
		{"a/b/c", false},
		{"_private", false},
		{"CamelCase", false},
		{"a1b2c3", false},
		{"", true},
		{"bad key", true},
		{"bad-key", true},       // hyphens not allowed
		{"123start", true},      // must start with letter/underscore
		{"key=value", true},     // equals not allowed
		{"'; DROP TABLE", true}, // SQL injection
		{"$.path", true},        // JSON path chars not allowed
		{"key\nvalue", true},    // newlines not allowed
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := storage.ValidateMetadataKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMetadataKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}
