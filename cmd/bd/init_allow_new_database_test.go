package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestAllowNewDatabase(t *testing.T) {
	tests := []struct {
		name string
		flag bool
		env  string
		want bool
	}{
		{"neither", false, "", false},
		{"flag", true, "", true},
		{"env 1", false, "1", true},
		{"env true", false, "TRUE", true},
		{"env padded with whitespace", false, " 1\n", true},
		{"env 0", false, "0", false},
		{"env other text", false, "yes", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BD_ALLOW_NEW_DATABASE", tc.env)
			// A throwaway command keeps the flag state out of initCmd.
			cmd := &cobra.Command{}
			cmd.Flags().Bool("allow-new-database", false, "")
			if tc.flag {
				if err := cmd.Flags().Set("allow-new-database", "true"); err != nil {
					t.Fatal(err)
				}
			}
			if got := allowNewDatabase(cmd); got != tc.want {
				t.Errorf("allowNewDatabase = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInitCmdDeclaresAllowNewDatabaseFlag(t *testing.T) {
	if initCmd.Flags().Lookup("allow-new-database") == nil {
		t.Fatal("bd init must declare --allow-new-database: the refusal message tells callers to pass it")
	}
}
