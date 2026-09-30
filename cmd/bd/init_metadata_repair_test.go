package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func assertMetadataUnchanged(t *testing.T, beadsDir string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json"))
	if err != nil {
		t.Fatalf("read metadata after the repair declined: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("repair declined but rewrote metadata:\nbefore: %q\nafter:  %q", before, after)
	}
}

// repairGateCommand builds the flag surface explicitRepairConflict inspects,
// marking exactly the selectors the caller passes.
func repairGateCommand(t *testing.T, selectors map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("database", "", "")
	cmd.Flags().String("prefix", "", "")
	for name, value := range selectors {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	return cmd
}

// TestExplicitRepairConflict pins the gate that keeps the rewrite from silently
// swallowing an explicit selector: the repair records the database name the disk
// evidence names, so a request for a different one is refused rather than
// ignored. bd init --prefix cm is the documented repair, so a selector that
// agrees — or was never passed — must not be treated as a conflict.
func TestExplicitRepairConflict(t *testing.T) {
	tests := []struct {
		name       string
		selectors  map[string]string
		prefix     string
		discovered string
		want       bool
	}{
		{name: "no selectors", discovered: "cm"},
		{name: "matching database", selectors: map[string]string{"database": "cm"}, discovered: "cm"},
		{name: "case-insensitive matching database", selectors: map[string]string{"database": "CM"}, discovered: "cm"},
		{name: "matching prefix", selectors: map[string]string{"prefix": "cm"}, prefix: "cm", discovered: "cm"},
		{name: "hyphenated prefix matches underscore database", selectors: map[string]string{"prefix": "my-proj"}, prefix: "my-proj", discovered: "my_proj"},

		{name: "different database", selectors: map[string]string{"database": "foo"}, discovered: "cm", want: true},
		{name: "different prefix", selectors: map[string]string{"prefix": "foo"}, prefix: "foo", discovered: "cm", want: true},
		{name: "database requested with nothing discoverable", selectors: map[string]string{"database": "cm"}, discovered: "", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := repairGateCommand(t, tt.selectors)
			if got := explicitRepairConflict(cmd, tt.prefix, cmd.Flag("database").Value.String(), tt.discovered); got != tt.want {
				t.Fatalf("explicitRepairConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExplicitRepairConflictWithoutFlagsRegistered proves the predicate is safe
// on a command that never declared the selectors: a missing flag is not a
// conflict, so the repair is not skipped for a reason that does not exist.
func TestExplicitRepairConflictWithoutFlagsRegistered(t *testing.T) {
	if explicitRepairConflict(&cobra.Command{}, "", "", "cm") {
		t.Fatal("explicitRepairConflict() = true with no selectors registered")
	}
}
