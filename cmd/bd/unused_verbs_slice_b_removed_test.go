package main

import "testing"

// The eight verbs deleted in be-xu2.6.6 (be-xu2.6 slice B) had no agent
// execution in the 31 days to 2026-11-05 and no program call site
// (engdocs/design/bd-verb-usage-2026-10.md). Guard the removal so a future
// registration is caught.
func TestUnusedSliceBVerbsAreDeleted(t *testing.T) {
	deleted := map[string]bool{
		"conflicts":       true,
		"duplicates":      true,
		"find-duplicates": true,
		"orphans":         true,
		"stale":           true,
		"preflight":       true,
		"rules":           true,
		"batch":           true,
	}
	for _, cmd := range rootCmd.Commands() {
		if deleted[cmd.Name()] {
			t.Errorf("root command still registers the deleted %q verb", cmd.Name())
		}
	}
}

// bd mol stale is a different verb from the deleted top-level bd stale: it
// outlives the deletion of stale.go and must still resolve.
func TestMolStaleVerbStillRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"mol", "stale"})
	if err != nil {
		t.Fatalf("bd mol stale does not resolve: %v", err)
	}
	if cmd == nil || cmd.Parent() == nil || cmd.Parent().Name() != "mol" {
		t.Fatal("bd mol stale is not registered as a mol subcommand")
	}
}
