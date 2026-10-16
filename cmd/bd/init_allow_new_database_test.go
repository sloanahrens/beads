package main

import "testing"

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
		{"env 0", false, "0", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BD_ALLOW_NEW_DATABASE", tc.env)
			if tc.flag {
				if err := initCmd.Flags().Set("allow-new-database", "true"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = initCmd.Flags().Set("allow-new-database", "false") })
			}
			if got := allowNewDatabase(initCmd); got != tc.want {
				t.Errorf("allowNewDatabase = %v, want %v", got, tc.want)
			}
		})
	}
}
