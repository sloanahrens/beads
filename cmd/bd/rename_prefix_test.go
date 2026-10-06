//go:build cgo

package main

import (
	"testing"
)

func TestValidatePrefix(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
	}{
		{"valid lowercase", "kw-", false},
		{"valid with numbers", "work1-", false},
		{"valid with hyphen", "my-work-", false},
		{"empty", "", true},
		{"long prefix ok", "verylongprefix-", false}, // No length limit (GH#770)
		{"starts with number", "1work-", true},
		{"uppercase", "KW-", true},
		{"no hyphen", "kw", false},
		{"just hyphen", "-", true},
		{"starts with hyphen", "-work", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePrefix(tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePrefix(%q) error = %v, wantErr %v", tt.prefix, err, tt.wantErr)
			}
		})
	}
}
