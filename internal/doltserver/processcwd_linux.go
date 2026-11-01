//go:build linux

package doltserver

import (
	"os"
	"path/filepath"
	"strconv"
)

// processCWD resolves pid's working directory from /proc, the kernel interface
// lsof reads itself. Reading it directly keeps isProcessInDir off a subprocess:
// lsof walks open files before it answers, and on some hosts that walk never
// returns, which hung this package's own test binary for its whole timeout
// (be-x4e8).
//
// The target is returned exactly as the kernel reports it, including the
// " (deleted)" suffix it appends once the directory is gone, so a process
// whose cwd no longer exists never matches a live data directory.
func processCWD(pid int) (cwd string, ok bool) {
	cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
	if err != nil || cwd == "" {
		return "", false
	}
	return cwd, true
}
