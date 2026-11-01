package main

import (
	"strings"
	"testing"
)

func TestDoltStopHelpStatesForceExecutableSafety(t *testing.T) {
	normalizedHelp := strings.Join(strings.Fields(strings.ToLower(doltStopCmd.Long)), " ")
	for _, want := range []string{
		"force still refuses to signal",
		"executable identity cannot be matched to bd or dolt",
	} {
		if !strings.Contains(normalizedHelp, want) {
			t.Errorf("dolt stop long help missing %q:\n%s", want, doltStopCmd.Long)
		}
	}
	flag := doltStopCmd.Flags().Lookup("force")
	if flag == nil || !strings.Contains(flag.Usage, "bd/dolt executable match") {
		t.Errorf("dolt stop --force usage does not state executable match: %+v", flag)
	}
}
