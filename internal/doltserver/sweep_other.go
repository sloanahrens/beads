//go:build !linux && !darwin

package doltserver

// SweepOrphanedTestServers is a no-op on platforms where process command
// lines and working directories cannot be inspected by an implementation in
// this package. The stub keeps callers (test TestMains) portable.
func SweepOrphanedTestServers(_ ...string) []int {
	return nil
}

// CountOrphanedTestServers is a no-op on platforms where process command
// lines and working directories cannot be inspected by an implementation in
// this package (see SweepOrphanedTestServers).
func CountOrphanedTestServers() int {
	return 0
}
