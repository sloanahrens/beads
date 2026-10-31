package main

import "testing"

// The bd sync verb is deleted: D3 removed every federation remote and agents
// never ran the loop. Guard the removal so a future registration is caught,
// and assert the sibling verbs that shared its group still resolve.
func TestSyncVerbIsDeleted(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "sync" {
			t.Fatalf("root command still registers a %q verb", cmd.Name())
		}
	}

	cases := []struct {
		path   []string
		parent string
	}{
		{[]string{"backup", "sync"}, "backup"},
		{[]string{"dolt", "push"}, "dolt"},
	}
	for _, tc := range cases {
		cmd, _, err := rootCmd.Find(tc.path)
		if err != nil {
			t.Errorf("bd %v does not resolve: %v", tc.path, err)
			continue
		}
		if cmd == nil || cmd.Parent() == nil || cmd.Parent().Name() != tc.parent {
			t.Errorf("bd %v does not resolve to a %q subcommand", tc.path, tc.parent)
		}
	}
}
