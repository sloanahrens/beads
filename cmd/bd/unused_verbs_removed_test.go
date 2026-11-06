package main

import "testing"

// The ten verbs deleted in be-xu2.6.1 (be-xu2.6 slice 10) had no agent
// execution in the 31 days to 2026-11-05 and no program call site
// (engdocs/design/bd-verb-usage-2026-10.md). Guard the removal so a future
// registration is caught.
func TestUnusedVerbsAreDeleted(t *testing.T) {
	deleted := map[string]bool{
		"assign":    true,
		"diff":      true,
		"duplicate": true,
		"link":      true,
		"lint":      true,
		"ping":      true,
		"promote":   true,
		"ship":      true,
		"tag":       true,
		"types":     true,
	}
	for _, cmd := range rootCmd.Commands() {
		if deleted[cmd.Name()] {
			t.Errorf("root command still registers the deleted %q verb", cmd.Name())
		}
	}
}

// supersede was declared in duplicate.go and outlived the verb deleted with
// that file: the count gives it 4 agent executions and the verdict keep. It
// must still resolve at the root.
func TestSupersedeVerbStillRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"supersede"})
	if err != nil {
		t.Fatalf("bd supersede does not resolve: %v", err)
	}
	if cmd == nil || cmd.Parent() == nil || cmd.Parent().Name() != "bd" {
		t.Fatal("bd supersede is not registered as a top-level command")
	}
	if flag := cmd.Flags().Lookup("with"); flag == nil {
		t.Fatal("bd supersede lost its --with flag")
	}
}
