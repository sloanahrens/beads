//go:build cgo

package main

import (
	"testing"
)

func TestFilterOutPollution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		title string
		want  bool
	}{
		{"Real feature request", false},
		{"test-something", true},
		{"benchmark-perf test", true},
		{"Actual bug fix", false},
		{"tmp-throwaway", true},
	}

	for _, tt := range tests {
		if got := isTestIssue(tt.title); got != tt.want {
			t.Errorf("isTestIssue(%q) = %v, want %v", tt.title, got, tt.want)
		}
	}
}
