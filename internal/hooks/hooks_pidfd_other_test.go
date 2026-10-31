//go:build !linux

package hooks

import (
	"errors"
	"fmt"
	"time"
)

// pidfdExitTimeout mirrors the Linux bound so the descendants test compiles
// here; it skips on its platform check before the wait is reached.
const pidfdExitTimeout = 10 * time.Second

// errPidfdPlatform stands in for pidfd_open off Linux: there is no pidfd here,
// so the descendants test has no kernel wait to make.
var errPidfdPlatform = errors.New("pidfd_open is only available on Linux")

// pidfdUnsupported reports the pidfd wait as unavailable off Linux, so a caller
// that reached the wait skips rather than asserting without one.
func pidfdUnsupported(err error) bool {
	return errors.Is(err, errPidfdPlatform)
}

// pidfdExitWait reports that no pidfd can be opened off Linux.
func pidfdExitWait(pid int, timeout time.Duration) (func() (bool, error), func(), error) {
	return nil, nil, fmt.Errorf("pidfd_open %d: %w", pid, errPidfdPlatform)
}
