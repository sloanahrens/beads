//go:build !linux

package hooks

import "time"

// pidfdExitTimeout mirrors the Linux bound so the descendants test compiles
// here; the test skips on its platform check before reaching the wait.
const pidfdExitTimeout = 10 * time.Second

// awaitChildExit has no pidfd to wait on off Linux, so it reports a skip: the
// descendants test then skips with a message instead of passing without a wait.
func awaitChildExit(int, time.Duration, func() (bool, error)) childExit {
	return childExit{SkipReason: "pidfd_open is only available on Linux; cannot wait for the child's exit"}
}
