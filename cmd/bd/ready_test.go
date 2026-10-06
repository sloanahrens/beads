//go:build cgo

package main

import (
	"testing"
)

func TestReadyCommandInit(t *testing.T) {
	t.Parallel()
	if readyCmd == nil {
		t.Fatal("readyCmd should be initialized")
	}

	if readyCmd.Use != "ready" {
		t.Errorf("Expected Use='ready', got %q", readyCmd.Use)
	}

	if len(readyCmd.Short) == 0 {
		t.Error("readyCmd should have Short description")
	}

	// Verify --pretty defaults to true
	prettyFlag := readyCmd.Flags().Lookup("pretty")
	if prettyFlag == nil {
		t.Fatal("--pretty flag should exist")
	}
	if prettyFlag.DefValue != "true" {
		t.Errorf("--pretty default should be 'true', got %q", prettyFlag.DefValue)
	}

	// Verify --plain flag exists and defaults to false
	plainFlag := readyCmd.Flags().Lookup("plain")
	if plainFlag == nil {
		t.Fatal("--plain flag should exist")
	}
	if plainFlag.DefValue != "false" {
		t.Errorf("--plain default should be 'false', got %q", plainFlag.DefValue)
	}

	// Verify --sort defaults to "priority"
	sortFlag := readyCmd.Flags().Lookup("sort")
	if sortFlag == nil {
		t.Fatal("--sort flag should exist")
	}
	if sortFlag.DefValue != "priority" {
		t.Errorf("--sort default should be 'priority', got %q", sortFlag.DefValue)
	}

	// Verify --exclude-label flag exists and defaults to empty
	excludeLabelFlag := readyCmd.Flags().Lookup("exclude-label")
	if excludeLabelFlag == nil {
		t.Fatal("--exclude-label flag should exist")
	}
	if excludeLabelFlag.DefValue != "[]" {
		t.Errorf("--exclude-label default should be '[]', got %q", excludeLabelFlag.DefValue)
	}
}
