//go:build !windows && !linux

package doltserver

import (
	"os/exec"
	"strconv"
	"strings"
)

// processCWD resolves pid's working directory from lsof's machine-readable
// field output: the `n` field after the `cwd` descriptor. These platforms
// expose no /proc, so lsof is the only source.
//
// The selectors are ANDed with -a on purpose: without it "-p <pid>" and
// "-d cwd" can each match on their own and yield cwd entries from unrelated
// processes (a macOS lsof quirk). The path is returned as lsof prints it,
// " (deleted)" suffix included, so a deleted cwd never matches a live one.
func processCWD(pid int) (cwd string, ok bool) {
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil {
		return "", false
	}
	// lsof -Fn output format: "p<pid>\nfcwd\nn<path>"
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		if cwd := strings.TrimSpace(line[1:]); cwd != "" {
			return cwd, true
		}
	}
	return "", false
}
