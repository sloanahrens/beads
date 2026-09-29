package main

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestMachineRequested(t *testing.T) {
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == machineEnvVar {
				return v
			}
			return ""
		}
	}
	cases := []struct {
		name string
		args []string
		env  string
		want bool
	}{
		{"off by default", []string{"show", "x"}, "", false},
		{"env on", []string{"show", "x"}, "1", true},
		{"env true word", []string{"show"}, "true", true},
		{"env zero", []string{"show"}, "0", false},
		{"env false", []string{"show"}, "false", false},
		{"flag on", []string{"show", "--machine", "x"}, "", true},
		{"flag explicit true", []string{"--machine=true", "list"}, "", true},
		{"flag false beats env", []string{"list", "--machine=false"}, "1", false},
		{"after double dash is not a flag", []string{"create", "--", "--machine"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := machineRequested(tc.args, env(tc.env)); got != tc.want {
				t.Fatalf("machineRequested(%v, BD_MACHINE=%q) = %v, want %v", tc.args, tc.env, got, tc.want)
			}
		})
	}
}

func TestMachineStdinAllowed(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"show", "x"}, false},
		{[]string{"create", "t", "--body-file=-"}, true},
		{[]string{"close", "x", "--reason-file", "-"}, true},
		{[]string{"import", "--stdin"}, true},
		{[]string{"create", "--", "-"}, false},
		{[]string{"update", "x", "--notes=a-b"}, false},
	}
	for _, tc := range cases {
		if got := machineStdinAllowed(tc.args); got != tc.want {
			t.Errorf("machineStdinAllowed(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

// withMachineMode runs fn with machine mode forced on and restores the
// globals it touches.
func withMachineMode(t *testing.T, on bool) {
	t.Helper()
	oldMachine, oldJSON := machineMode, jsonOutput
	machineMode = on
	t.Cleanup(func() { machineMode, jsonOutput = oldMachine, oldJSON })
}

func TestMachineModeImpliesJSON(t *testing.T) {
	withMachineMode(t, true)
	jsonOutput = false
	applyMachineCommandSetup()
	if !jsonOutput {
		t.Fatal("machine mode did not turn on JSON output")
	}
	if !jsonFromConfig(false) {
		t.Fatal("a config rebind (json: false) turned JSON off again under machine mode")
	}
}

func TestJSONFromConfigOutsideMachineMode(t *testing.T) {
	withMachineMode(t, false)
	if jsonFromConfig(false) || !jsonFromConfig(true) {
		t.Fatal("outside machine mode jsonFromConfig must return the configured value")
	}
}

func TestMachineModeDisablesMetrics(t *testing.T) {
	// TestMain disables metrics through the env; clear that so the machine
	// check is what is being measured.
	for _, k := range []string{"BD_DISABLE_METRICS", "DO_NOT_TRACK"} {
		if v, ok := os.LookupEnv(k); ok {
			_ = os.Unsetenv(k)
			t.Cleanup(func() { _ = os.Setenv(k, v) })
		}
	}
	withMachineMode(t, true)
	if resolveMetricsEnabled() {
		t.Fatal("metrics resolved enabled under machine mode")
	}
}

func TestMachineModeSkipsMoleculeScan(t *testing.T) {
	show := &cobra.Command{Use: "show"}
	withMachineMode(t, false)
	if !shouldLoadMolecules(show) {
		t.Fatal("molecule scan skipped outside machine mode")
	}
	machineMode = true
	if shouldLoadMolecules(show) {
		t.Fatal("molecule scan still runs under machine mode")
	}
}

func TestMachineModeRefusesInteractive(t *testing.T) {
	withMachineMode(t, false)
	if err := machineRefusesInteractive("bd edit"); err != nil {
		t.Fatalf("refused outside machine mode: %v", err)
	}
	machineMode = true
	if err := machineRefusesInteractive("bd edit"); err == nil {
		t.Fatal("bd edit allowed under machine mode")
	}
}

func TestMachineModeSkipsAutoPush(t *testing.T) {
	withMachineMode(t, true)
	if autoPushAllowedByMode() {
		t.Fatal("auto-push allowed under machine mode")
	}
}
