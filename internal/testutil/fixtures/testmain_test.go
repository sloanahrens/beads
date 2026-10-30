package fixtures

import (
	"os"
	"testing"
)

// TestMain opts this package's tests into the Dolt test container that
// testutil.RequireDoltContainer starts: the storage layer refuses to connect a
// test database to a server unless BEADS_TEST_SERVER=1 (see
// internal/storage/dolt/store.go).
func TestMain(m *testing.M) {
	os.Setenv("BEADS_TEST_MODE", "1")
	os.Setenv("BEADS_TEST_SERVER", "1")
	os.Exit(m.Run())
}
