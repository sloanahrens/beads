package main

import (
	"strings"
	"testing"
)

// be-078: the concept agents search for is "ephemeral"/"wisp"; the flag is
// named --include-infra, so its help text must carry both words or
// `bd list --help | grep -i wisp` finds nothing and "0 results" is read as
// "the data is gone".
func TestIncludeInfraHelpNamesEphemeralAndWisp(t *testing.T) {
	for _, cmd := range []struct {
		name string
		use  string
	}{{"list", listCmd.Flags().Lookup("include-infra").Usage}} {
		lower := strings.ToLower(cmd.use)
		for _, word := range []string{"ephemeral", "wisp"} {
			if !strings.Contains(lower, word) {
				t.Errorf("bd %s --include-infra help %q does not mention %q", cmd.name, cmd.use, word)
			}
		}
	}
}

// be-3qk: --repo takes a filesystem PATH, not a rig/repository name; the
// wording that said "Target repository" sent an agent's bead into a
// bootstrapped local database named after a rig (be-z03).
func TestCreateRepoFlagHelpSaysPath(t *testing.T) {
	use := createCmd.Flags().Lookup("repo").Usage
	if !strings.Contains(strings.ToLower(use), "path") {
		t.Errorf("bd create --repo help %q does not say it takes a path", use)
	}
}
