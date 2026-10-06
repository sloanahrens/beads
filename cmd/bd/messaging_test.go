//go:build cgo

package main

import (
	"os"
	"testing"
)

// TestFindMailDelegate tests the mail delegate resolution logic.
// Kept separate because it modifies global state (env vars, store).
func TestFindMailDelegate(t *testing.T) {
	origBeads := os.Getenv("BEADS_MAIL_DELEGATE")
	origBD := os.Getenv("BD_MAIL_DELEGATE")
	defer func() {
		os.Setenv("BEADS_MAIL_DELEGATE", origBeads)
		os.Setenv("BD_MAIL_DELEGATE", origBD)
	}()

	t.Run("BEADS_MAIL_DELEGATE takes priority", func(t *testing.T) {
		os.Setenv("BEADS_MAIL_DELEGATE", "gt mail")
		os.Setenv("BD_MAIL_DELEGATE", "other mail")
		defer func() {
			os.Unsetenv("BEADS_MAIL_DELEGATE")
			os.Unsetenv("BD_MAIL_DELEGATE")
		}()

		got := findMailDelegate()
		if got != "gt mail" {
			t.Errorf("findMailDelegate() = %q, want \"gt mail\"", got)
		}
	})

	t.Run("BD_MAIL_DELEGATE fallback", func(t *testing.T) {
		os.Unsetenv("BEADS_MAIL_DELEGATE")
		os.Setenv("BD_MAIL_DELEGATE", "custom mail")
		defer os.Unsetenv("BD_MAIL_DELEGATE")

		got := findMailDelegate()
		if got != "custom mail" {
			t.Errorf("findMailDelegate() = %q, want \"custom mail\"", got)
		}
	})

	t.Run("no delegate returns empty", func(t *testing.T) {
		os.Unsetenv("BEADS_MAIL_DELEGATE")
		os.Unsetenv("BD_MAIL_DELEGATE")

		oldStore := store
		store = nil
		defer func() { store = oldStore }()

		got := findMailDelegate()
		if got != "" {
			t.Errorf("findMailDelegate() = %q, want empty string", got)
		}
	})
}
