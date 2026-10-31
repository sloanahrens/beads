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

// pidfdKernel has no usable form off Linux; the descendants test skips before
// one can be passed on.
type pidfdKernel struct{}

// realPidfdKernel returns the zero kernel: there is no pidfd without Linux.
func realPidfdKernel() pidfdKernel { return pidfdKernel{} }

// pidfdUnsupported reports the platform error as an unusable pidfd wait, so a
// caller that reached it skips rather than asserting without one.
func pidfdUnsupported(err error) bool {
	return errors.Is(err, errPidfdPlatform)
}

// waitForChildExit reports that no pidfd can be opened off Linux.
func waitForChildExit(pidfdKernel, int, time.Duration) (func() (bool, error), func(), error) {
	return nil, nil, fmt.Errorf("pidfd_open: %w", errPidfdPlatform)
}
