// Tests for bd-5247: `bd update --set-metadata a=1 b=2` silently turns `b=2`
// into a positional issue id (--set-metadata takes one key=value per flag).
// Before this guard, `a=1` was written and only the unbound pairs failed, so a
// caller could not tell a full write from a 1-of-N write. The guard refuses a
// `=`-bearing positional before ANY write, so no partial update lands.
//
// This file MUST NOT carry a cgo build tag: it exercises the default sqlite
// backend via a bd binary built with the gms_pure_go tag, reusing the helpers
// in update_multi_id_exit_test.go (same package).

package main

import (
	"testing"
)

func TestErrStrayFlagValuePositional(t *testing.T) {
	// A positional carrying '=' is a mis-typed flag value and must be refused.
	if err := errStrayFlagValuePositional([]string{"test-abc", "probe_b=2"}); err == nil {
		t.Fatal("errStrayFlagValuePositional with a '='-bearing positional = nil, want error")
	}
	// Plain issue ids (no '=') must pass through untouched.
	if err := errStrayFlagValuePositional([]string{"test-abc", "test-def"}); err != nil {
		t.Fatalf("errStrayFlagValuePositional with plain ids = %v, want nil", err)
	}
	if err := errStrayFlagValuePositional(nil); err != nil {
		t.Fatalf("errStrayFlagValuePositional(nil) = %v, want nil", err)
	}
}
