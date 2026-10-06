//go:build cgo

package main

import (
	"strings"
	"testing"
)

func TestKVPrefix(t *testing.T) {
	// Verify the kvPrefix constant matches expected value
	if kvPrefix != "kv." {
		t.Errorf("Expected kvPrefix to be 'kv.', got %q", kvPrefix)
	}
}

func TestValidateKVKey(t *testing.T) {
	testCases := []struct {
		name    string
		key     string
		wantErr bool
		errMsg  string
	}{
		// Valid keys
		{"simple key", "mykey", false, ""},
		{"key with underscore", "my_key", false, ""},
		{"key with dots", "my.key.name", false, ""},
		{"key with numbers", "key123", false, ""},

		// Invalid keys
		{"empty key", "", true, "cannot be empty"},
		{"whitespace only", "   ", true, "cannot be only whitespace"},
		{"kv prefix", "kv.nested", true, "cannot start with 'kv.'"},
		{"sync prefix", "sync.remote", true, "reserved prefix"},
		{"conflict prefix", "conflict.strategy", true, "reserved prefix"},
		{"federation prefix", "federation.remote", true, "reserved prefix"},
		{"jira prefix", "jira.url", true, "reserved prefix"},
		{"linear prefix", "linear.key", true, "reserved prefix"},
		{"export prefix", "export.path", true, "reserved prefix"},
		{"import prefix", "import.path", true, "reserved prefix"},

		// memory.* is reserved for `bd remember`: a generic kv.memory.* key is
		// indistinguishable from a memory and the merge resolver auto-resolves it
		// with --theirs (GH#2474), so it must not be settable via `bd kv set`.
		{"memory prefix", "memory.foo", true, "reserved for persistent memories"},
		{"memory prefix slug", "memory.test-wedge", true, "reserved for persistent memories"},
		// A key that merely contains "memory" but does not start with the prefix
		// is still valid — only the actual namespace is reserved.
		{"memory not a prefix", "my.memory.note", false, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateKVKey(tc.key)
			if tc.wantErr {
				if err == nil {
					t.Errorf("Expected error for key %q, got nil", tc.key)
				} else if tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("Expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for key %q: %v", tc.key, err)
				}
			}
		})
	}
}
