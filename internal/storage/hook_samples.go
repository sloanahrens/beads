package storage

import (
	"github.com/steveyegge/beads/samplesops"
)

// Samples returns the inner store's generic-samples surface.
//
// IT RECURSES UNWRAPPED, and that is a decision rather than an omission. Add is
// a write, so the reflex is to reach for hook_commenter — but this decorator's
// whole vocabulary is on_create, on_update and on_close, and every one of them
// hands a hook script an ISSUE. A sample is not an issue: it has no id a script
// could read back and no hook name in the vocabulary. Inventing an event so
// this file could fire one would be a hook-vocabulary proposal wearing a role
// commit's clothes. This is the Memories case, not the Commenter case.
//
// So the accessor EXISTS on this decorator — declared, never inherited, which
// the reflection test in role_accessor_decorator_test.go asserts — and adds no
// layer. If a sample event ever joins the hook vocabulary, this file is where
// it lands.
func (h *HookFiringStore) Samples() (samplesops.Samples, error) {
	return h.inner.Samples()
}
