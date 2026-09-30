//go:build cgo

package main

// `bd serve` against a SERVER-MODE workspace — `bd init --server` pointed at a
// dolt sql-server this process did not start and does not own.
//
// That topology is the reason this file exists rather than another case in the
// proxied test: a server-mode workspace has no proxied-server sidecar to read a
// provider out of, so PersistentPreRunE builds a DoltStore and no unit-of-work
// provider at all. Everything the HTTP surface answers here is answered through
// a provider serve built itself, from the workspace's Dolt connection settings.
