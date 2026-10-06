//go:build cgo

package main

import (
	"testing"
)

func TestSqlCommandInit(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "sql <query>" {
			found = true
			if cmd.GroupID != "maint" {
				t.Errorf("Expected GroupID 'maint', got %q", cmd.GroupID)
			}
			break
		}
	}
	if !found {
		t.Error("sql command not registered with rootCmd")
	}
}
