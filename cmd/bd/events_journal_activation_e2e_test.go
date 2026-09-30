//go:build cgo && unix

package main

// The two plumbings the root pre-run never reached. Both are end-to-end on
// purpose: activation failing is INVISIBLE from inside the process — the
// command succeeds, the write lands, and only a later read of the journal shows
// that nothing was recorded. Nothing short of mutating and then reading back
// can tell the difference.
