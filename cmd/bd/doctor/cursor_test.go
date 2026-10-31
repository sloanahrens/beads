package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCursorHooksFile(t *testing.T, repoDir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(repoDir, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".cursor", "hooks.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCursorSettingsHealth(t *testing.T) {
	t.Run("malformed is error", func(t *testing.T) {
		repo := t.TempDir()
		t.Setenv("HOME", t.TempDir())
		t.Setenv("USERPROFILE", t.TempDir())
		writeCursorHooksFile(t, repo, `{not valid json`)

		dc := CheckCursorSettingsHealth(repo)
		if dc.Status != StatusError {
			t.Fatalf("status = %q, want error (%#v)", dc.Status, dc)
		}
	})

	t.Run("valid is ok", func(t *testing.T) {
		repo := t.TempDir()
		t.Setenv("HOME", t.TempDir())
		t.Setenv("USERPROFILE", t.TempDir())
		writeCursorHooksFile(t, repo, `{"version":1,"hooks":{"sessionStart":[{"command":"bd cursor-hook sessionStart"}]}}`)

		dc := CheckCursorSettingsHealth(repo)
		if dc.Status != StatusOK {
			t.Fatalf("status = %q, want ok (%#v)", dc.Status, dc)
		}
	})

	t.Run("absent is ok", func(t *testing.T) {
		repo := t.TempDir()
		t.Setenv("HOME", t.TempDir())
		t.Setenv("USERPROFILE", t.TempDir())

		dc := CheckCursorSettingsHealth(repo)
		if dc.Status != StatusOK {
			t.Fatalf("status = %q, want ok (%#v)", dc.Status, dc)
		}
	})
}
