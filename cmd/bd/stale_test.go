//go:build cgo

package main

import (
	"testing"
)

func TestStaleCommandInit(t *testing.T) {
	// Not parallel: InheritedFlags mutates Cobra flag state on the global command tree.
	if staleCmd == nil {
		t.Fatal("staleCmd should be initialized")
	}

	if staleCmd.Use != "stale" {
		t.Errorf("Expected Use='stale', got %q", staleCmd.Use)
	}

	if len(staleCmd.Short) == 0 {
		t.Error("staleCmd should have Short description")
	}

	// Check flags are defined
	flags := staleCmd.Flags()
	if flags.Lookup("days") == nil {
		t.Error("staleCmd should have --days flag")
	}
	if flags.Lookup("status") == nil {
		t.Error("staleCmd should have --status flag")
	}
	if flags.Lookup("limit") == nil {
		t.Error("staleCmd should have --limit flag")
	}
	// --json is inherited from rootCmd as a persistent flag
	if staleCmd.InheritedFlags().Lookup("json") == nil {
		t.Error("staleCmd should inherit --json flag from rootCmd")
	}
}
