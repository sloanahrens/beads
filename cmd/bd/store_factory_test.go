//go:build cgo

package main

import (
	"strings"
	"testing"
)

// TestNewDoltStoreFromConfig_NoMetadata pins the fail-closed result of the
// embedded-Dolt removal (be-xu2.2) in the cgo build: an empty beads directory
// has no server-mode metadata, so newDoltStoreFromConfig refuses instead of
// opening an embedded store.
//
// It replaces three tests this file used to carry — the GH#2988 "no database
// selected" regression (an empty dir got the default embedded store) and the
// GH#3231 hyphen/dot database-name sanitization pair — none of which can hold
// now: normalizeLoadedConfig no longer sanitizes anything, and there is no
// embedded default store to configure. The !cgo build's
// store_factory_nocgo_test.go covers the same refusal.
func TestNewDoltStoreFromConfig_NoMetadata(t *testing.T) {
	beadsDir := t.TempDir()

	_, err := newDoltStoreFromConfig(t.Context(), beadsDir)
	if err == nil {
		t.Fatal("newDoltStoreFromConfig opened an empty beads dir; want the embedded-removal refusal")
	}
	if !strings.Contains(err.Error(), embeddedRemovedErrMsg) {
		t.Errorf("want the embedded-removal refusal, got: %v", err)
	}
}
