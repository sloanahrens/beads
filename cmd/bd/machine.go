package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/steveyegge/beads/internal/ui"
)

// Machine mode is the non-interactive surface a program drives bd through
// (BD_MACHINE=1 or --machine). It never blocks and never probes the terminal:
// JSON output is implied, colors, metrics, tips, the molecules scan and Dolt
// auto-push are skipped, and stdin is closed unless an argument asks for it.
// See engdocs/design/d1-machine-surface.md.

// machineEnvVar turns machine mode on for every bd call a program makes.
const machineEnvVar = ui.MachineEnvVar

// machineMode is decided once, in main(), before cobra parses anything, so
// every hook and command sees the same answer. Tests set it directly.
var machineMode bool

// machineFlag backs the persistent --machine flag. Its value is read from
// os.Args in main() (see machineRequested); the flag exists so cobra accepts
// it and lists it in help and capabilities.
var machineFlag bool

// machineModeActive reports whether this process runs in machine mode.
func machineModeActive() bool {
	return machineMode
}

// machineRequested decides machine mode from the arguments and environment.
// An explicit --machine or --machine=<bool> wins over the environment, so a
// caller can opt a single call out with --machine=false.
func machineRequested(args []string, getenv func(string) string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--machine" {
			return true
		}
		if v, ok := strings.CutPrefix(a, "--machine="); ok {
			return envTruthyValue(v)
		}
	}
	return envTruthyValue(getenv(machineEnvVar))
}

// machineStdinAllowed reports whether an argument asks bd to read stdin: a
// bare "-", a "--flag=-", or --stdin. Anything else means no read may happen
// in machine mode, so stdin is swapped for /dev/null before any command runs.
func machineStdinAllowed(args []string) bool {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "-" || a == "--stdin" || strings.HasSuffix(a, "=-") {
			return true
		}
	}
	return false
}

// applyMachineProcessSetup does the process-level part of machine mode that
// has to happen before cobra runs: closing stdin (unless asked for) and
// dropping terminal styling. It is idempotent.
func applyMachineProcessSetup(args []string) {
	ui.DisableColors()
	if !machineStdinAllowed(args) {
		if devNull, err := os.Open(os.DevNull); err == nil {
			os.Stdin = devNull
		}
	}
}

// applyMachineCommandSetup runs at the top of the root pre-run: it forces
// JSON output. Config rebinds of jsonOutput later in the pre-run OR machine
// mode back in (see jsonFromConfig).
func applyMachineCommandSetup() {
	if !machineModeActive() {
		return
	}
	jsonOutput = true
}

// jsonFromConfig is the value jsonOutput takes when neither --json nor
// --format was given: the config default, or true in machine mode.
func jsonFromConfig(configured bool) bool {
	return configured || machineModeActive()
}

// machineRefusesInteractive returns an error when a streaming or interactive
// mode is requested under machine mode, which must never block. what names the
// flag or command for the message.
func machineRefusesInteractive(what string) error {
	if !machineModeActive() {
		return nil
	}
	return HandleError("%s is interactive or streaming and is refused in machine mode (%s / --machine)", what, machineEnvVar)
}

// shouldLoadMolecules reports whether the root pre-run scans molecule
// catalogs into the store. The scan runs on every command today and nothing a
// machine caller asks for depends on it, so machine mode skips it.
func shouldLoadMolecules(cmd *cobra.Command) bool {
	return cmd.Name() != "import" && !machineModeActive()
}
