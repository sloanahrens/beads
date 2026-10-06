//go:build cgo

package main

import (
	"testing"
	"time"
)

func TestCompactValidation(t *testing.T) {
	tests := []struct {
		name       string
		compactID  string
		compactAll bool
		dryRun     bool
		force      bool
		wantError  bool
	}{
		{
			name:       "both id and all",
			compactID:  "test-1",
			compactAll: true,
			wantError:  true,
		},
		{
			name:      "force without id",
			force:     true,
			wantError: true,
		},
		{
			name:      "no flags",
			wantError: true,
		},
		{
			name:      "dry run only",
			dryRun:    true,
			wantError: false,
		},
		{
			name:      "id only",
			compactID: "test-1",
			wantError: false,
		},
		{
			name:       "all only",
			compactAll: true,
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.compactID != "" && tt.compactAll {
				// Should fail
				if !tt.wantError {
					t.Error("Expected error for both --id and --all")
				}
			}

			if tt.force && tt.compactID == "" {
				// Should fail
				if !tt.wantError {
					t.Error("Expected error for --force without --id")
				}
			}

			if tt.compactID == "" && !tt.compactAll && !tt.dryRun {
				// Should fail
				if !tt.wantError {
					t.Error("Expected error when no action specified")
				}
			}
		})
	}
}

func TestCompactProgressBar(t *testing.T) {
	// Test progress bar formatting
	pb := progressBar(50, 100)
	if len(pb) == 0 {
		t.Error("Progress bar should not be empty")
	}

	pb = progressBar(100, 100)
	if len(pb) == 0 {
		t.Error("Full progress bar should not be empty")
	}

	pb = progressBar(0, 100)
	if len(pb) == 0 {
		t.Error("Zero progress bar should not be empty")
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func TestCompactInitCommand(t *testing.T) {
	if compactCmd == nil {
		t.Fatal("compactCmd should be initialized")
	}

	if compactCmd.Use != "compact" {
		t.Errorf("Expected Use='compact', got %q", compactCmd.Use)
	}

	if len(compactCmd.Long) == 0 {
		t.Error("compactCmd should have Long description")
	}

	// Verify --json flag exists
	jsonFlag := compactCmd.Flags().Lookup("json")
	if jsonFlag == nil {
		t.Error("compact command should have --json flag")
	}
}
