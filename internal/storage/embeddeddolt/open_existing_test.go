//go:build cgo

package embeddeddolt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/storage/embeddeddolt"
)

// TestOpenExistingRefusesToCreateDatabase pins the refuse-to-create open.
// embeddeddolt.Open's contract is create-or-open, which is what makes a
// misresolved directory turn into a phantom database; OpenExisting is the
// variant a caller uses when it knows the workspace is already initialized, so
// the two properties that matter are that it still opens a database that is
// on disk and that it fabricates nothing when one is not (be-n2s).
func TestOpenExistingRefusesToCreateDatabase(t *testing.T) {
	template := pristineEmbeddedDoltTemplateForTest(t)

	// clonePristineWorkspace hands each subtest its own .beads/ holding a real
	// embedded database, never opened in this process, so the assertions below
	// observe the open's own behaviour rather than the Open cache's.
	clonePristineWorkspace := func(t *testing.T) string {
		t.Helper()
		beadsDir := filepath.Join(t.TempDir(), ".beads")
		if err := clonePristineEmbeddedDoltTemplate(template, beadsDir); err != nil {
			t.Fatalf("clone pristine embedded Dolt template: %v", err)
		}
		return beadsDir
	}

	t.Run("opens a database that is on disk", func(t *testing.T) {
		beadsDir := clonePristineWorkspace(t)

		store, err := embeddeddolt.OpenExisting(t.Context(), beadsDir, pristineEmbeddedDoltDatabase, "main")
		if err != nil {
			t.Fatalf("OpenExisting on an on-disk database: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	t.Run("refuses a database that is not, and creates nothing", func(t *testing.T) {
		beadsDir := clonePristineWorkspace(t)
		dataDir := filepath.Join(beadsDir, "embeddeddolt")
		before := directoryDigest(t, dataDir)

		if _, err := embeddeddolt.OpenExisting(t.Context(), beadsDir, "absent", "main"); err == nil {
			t.Fatal("OpenExisting opened a database that is not on disk; want a refusal")
		}

		if _, err := os.Stat(filepath.Join(dataDir, "absent")); !os.IsNotExist(err) {
			t.Errorf("refusal left a database directory behind (stat err = %v); it must create nothing", err)
		}
		if after := directoryDigest(t, dataDir); after != before {
			t.Errorf("refusal modified %s:\n before %s\n after  %s", dataDir, before, after)
		}
	})

	t.Run("refuses a directory with no embeddeddolt tree, and creates none", func(t *testing.T) {
		beadsDir := t.TempDir()

		if _, err := embeddeddolt.OpenExisting(t.Context(), beadsDir, pristineEmbeddedDoltDatabase, "main"); err == nil {
			t.Fatal("OpenExisting fabricated a database in a bare directory; want a refusal")
		}

		if _, err := os.Stat(filepath.Join(beadsDir, "embeddeddolt")); !os.IsNotExist(err) {
			t.Errorf("refusal created embeddeddolt/ in a bare directory (stat err = %v)", err)
		}
	})
}
