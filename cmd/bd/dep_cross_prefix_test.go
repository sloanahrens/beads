package main

import (
	"strings"
	"testing"
)

// TestDepAddRefusesCrossPrefix locks the plain cross-prefix refusal: a target
// whose prefix differs from the source's is refused and the error names the
// external form, while same-prefix, wisp-id, external: and prefix-less targets
// keep their existing handling.
func TestDepAddRefusesCrossPrefix(t *testing.T) {
	tests := []struct {
		name        string
		fromID      string
		dependsOn   string
		wantRefused bool
	}{
		{name: "cross prefix is refused", fromID: "gt-a", dependsOn: "om-b", wantRefused: true},
		{name: "same prefix is accepted", fromID: "gt-a", dependsOn: "gt-b"},
		{name: "same-prefix wisp id is accepted", fromID: "gt-a", dependsOn: "gt-wisp-c"},
		{name: "external reference is accepted", fromID: "gt-a", dependsOn: "external:om:cap"},
		{name: "prefix-less target is accepted", fromID: "gt-a", dependsOn: "nonexistent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := refuseCrossPrefixDependency(tt.fromID, tt.dependsOn)
			if !tt.wantRefused {
				if err != nil {
					t.Fatalf("refuseCrossPrefixDependency(%q, %q) = %v, want nil", tt.fromID, tt.dependsOn, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("refuseCrossPrefixDependency(%q, %q) = nil, want a refusal", tt.fromID, tt.dependsOn)
			}
			if !strings.Contains(err.Error(), "external:<project>:<capability>") {
				t.Errorf("refusal must name the supported form, got %q", err)
			}
		})
	}
}

// TestDepAddRefusesCrossPrefixBeforeWrite locks the refusal's position: it
// returns before the command reaches the store. The package-level store is
// left at its zero value (nil) and is not assigned here. A refusal that moved
// after a store access reaches the nil store, so the call no longer returns
// the cross-prefix refusal; a nil dereference along the way is recovered and
// reported as a failure rather than crashing the test binary.
func TestDepAddRefusesCrossPrefixBeforeWrite(t *testing.T) {
	if store != nil {
		t.Fatalf("package-level store is not nil at test start; this test needs it at its zero value to detect store access")
	}

	var (
		runErr   error
		panicVal any
	)
	got := captureStderr(t, func() {
		defer func() { panicVal = recover() }()
		runErr = depAddCmd.RunE(depAddCmd, []string{"gt-a", "om-b"})
	})

	if panicVal != nil {
		t.Fatalf("dep add gt-a om-b reached the store before refusing: %v", panicVal)
	}
	if runErr == nil {
		t.Fatal("dep add gt-a om-b = nil, want a refusal")
	}
	if !strings.Contains(got, "external:<project>:<capability>") {
		t.Errorf("dep add gt-a om-b stderr = %q, want the supported form", got)
	}
}

// TestDepAddCommandRefusesCrossPrefix drives the command's RunE so deleting the
// refusal call turns this test red: without the call the cross-prefix add goes
// on to resolve the source against a store.
func TestDepAddCommandRefusesCrossPrefix(t *testing.T) {
	var runErr error
	got := captureStderr(t, func() {
		runErr = depAddCmd.RunE(depAddCmd, []string{"gt-a", "om-b"})
	})

	if runErr == nil {
		t.Fatal("dep add gt-a om-b = nil, want a refusal")
	}
	if !strings.Contains(got, "external:<project>:<capability>") {
		t.Errorf("dep add gt-a om-b stderr = %q, want the supported form", got)
	}
}
