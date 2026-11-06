package main

import "testing"

// The singular bd comment verb is deleted; "comments add" is the verb agents
// mean. Guard the removal so a future registration is caught, and assert the
// plural verb still resolves.
func TestCommentVerbIsDeleted(t *testing.T) {
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "comment" {
			t.Fatalf("root command still registers a %q verb", cmd.Name())
		}
	}

	cmd, _, err := rootCmd.Find([]string{"comments", "add"})
	if err != nil {
		t.Fatalf("bd comments add does not resolve: %v", err)
	}
	if cmd == nil || cmd.Name() != "add" || cmd.Parent() == nil || cmd.Parent().Name() != "comments" {
		t.Errorf("bd comments add does not resolve to the comments add subcommand")
	}
}
