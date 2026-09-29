// Package ui provides terminal styling and output helpers for beads CLI.
package ui

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// MachineEnvVar is the environment switch for bd's machine mode. It is read
// here, at package init time, so a machine caller never pays for terminal
// probing (color detection, the OSC 11 background query) before main runs.
const MachineEnvVar = "BD_MACHINE"

// MachineModeFromEnv reports whether BD_MACHINE asks for machine mode. Any
// value other than empty, "0" or "false" (case-insensitive) turns it on.
func MachineModeFromEnv() bool {
	v := strings.ToLower(os.Getenv(MachineEnvVar))
	return v != "" && v != "0" && v != "false"
}

// IsTerminal returns true if stdout is connected to a terminal (TTY).
// Always false in machine mode: a program is on the other end.
func IsTerminal() bool {
	if MachineModeFromEnv() {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsStderrTerminal returns true if stderr is connected to a terminal (TTY).
// Used to suppress advisory messages (e.g. deprecation notices) when stderr
// is captured by test harnesses or piped to another process.
func IsStderrTerminal() bool {
	if MachineModeFromEnv() {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// ShouldUseColor determines if ANSI color codes should be used.
// Respects standard conventions:
//   - BD_GIT_HOOK=1: disables color in git hook context (prevents OSC 11 queries, GH#1303)
//   - NO_COLOR: https://no-color.org/ - disables color if set
//   - CLICOLOR=0: disables color
//   - CLICOLOR_FORCE: forces color even in non-TTY
//   - TERM=dumb: disables color unless explicitly forced
//   - Falls back to TTY detection
func ShouldUseColor() bool {
	if MachineModeFromEnv() {
		return false
	}
	// Git hook context - disable color to prevent termenv OSC 11 terminal
	// background queries that leak escape sequences to the terminal (GH#1303).
	// Set by bd hook shim templates before calling 'bd hooks run'.
	if os.Getenv("BD_GIT_HOOK") == "1" {
		return false
	}

	// NO_COLOR standard - any value disables color
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// CLICOLOR=0 disables color
	if os.Getenv("CLICOLOR") == "0" {
		return false
	}

	// CLICOLOR_FORCE forces color even in non-TTY
	if os.Getenv("CLICOLOR_FORCE") != "" {
		return true
	}

	// TERM is a terminfo terminal type string (for example "xterm-256color",
	// "xterm-kitty", "screen-256color", "tmux-256color", or "vt100").
	// "dumb" explicitly means ANSI controls are unsupported.
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}

	// Default: use color only if stdout is a TTY
	return IsTerminal()
}

// ShouldUseHyperlinks determines if OSC 8 terminal hyperlinks should be emitted.
// OSC 8 support is not implied by ANSI color support, so this intentionally uses
// a narrower allowlist plus FORCE_HYPERLINK for users who know their terminal.
func ShouldUseHyperlinks() bool {
	return shouldUseHyperlinks(IsTerminal())
}

// shouldUseHyperlinks is the testable implementation behind ShouldUseHyperlinks.
// It takes the stdout TTY result as a parameter so tests can cover known terminal
// capability markers (for example Windows Terminal's WT_SESSION) without needing
// to run inside those terminals.
func shouldUseHyperlinks(stdoutIsTerminal bool) bool {
	if os.Getenv("BD_GIT_HOOK") == "1" {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICOLOR") == "0" {
		return false
	}
	// TERM is a terminfo terminal type string (for example "xterm-256color",
	// "xterm-kitty", "screen-256color", "tmux-256color", or "vt100"), not a
	// boolean toggle. "dumb" explicitly means OSC 8 hyperlinks are unsupported.
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	if force := os.Getenv("FORCE_HYPERLINK"); force != "" && force != "0" {
		return true
	}
	if !stdoutIsTerminal {
		return false
	}

	if os.Getenv("WT_SESSION") != "" ||
		os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("WEZTERM_EXECUTABLE") != "" ||
		os.Getenv("KONSOLE_VERSION") != "" ||
		os.Getenv("DOMTERM") != "" ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return true
	}

	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "iterm.app", "wezterm", "vscode", "apple_terminal", "tabby", "hyper", "ghostty":
		return true
	}

	termName := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(termName, "xterm-kitty") ||
		strings.Contains(termName, "wezterm") ||
		strings.Contains(termName, "foot") ||
		strings.Contains(termName, "contour") ||
		strings.Contains(termName, "ghostty") {
		return true
	}

	if vte := os.Getenv("VTE_VERSION"); vte != "" {
		version, err := strconv.Atoi(vte)
		return err == nil && version >= 5000
	}

	return false
}

// ShouldUseEmoji determines if emoji decorations should be used.
// Disabled in non-TTY mode to keep output machine-readable.
// Can be controlled with BD_NO_EMOJI environment variable.
func ShouldUseEmoji() bool {
	// Explicit disable
	if os.Getenv("BD_NO_EMOJI") != "" {
		return false
	}

	// Default: use emoji only if stdout is a TTY
	return IsTerminal()
}
