//go:build cgo

package main

import (
	"testing"
)

// TestRelateCommandInit tests that the relate and unrelate commands are properly initialized.
func TestRelateCommandInit(t *testing.T) {
	if relateCmd == nil {
		t.Fatal("relateCmd should be initialized")
	}
	if relateCmd.Use != "relate <id1> <id2>" {
		t.Errorf("Expected Use='relate <id1> <id2>', got %q", relateCmd.Use)
	}

	if unrelateCmd == nil {
		t.Fatal("unrelateCmd should be initialized")
	}
	if unrelateCmd.Use != "unrelate <id1> <id2>" {
		t.Errorf("Expected Use='unrelate <id1> <id2>', got %q", unrelateCmd.Use)
	}
}
