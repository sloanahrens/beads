package beads_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads"
)

func TestOpenBestAvailable_EmbeddedMode_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("failed to create .beads dir: %v", err)
	}

	metadata := `{"backend":"dolt","database":"dolt","dolt_mode":"embedded"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("failed to write metadata.json: %v", err)
	}

	ctx := context.Background()
	_, err := beads.OpenBestAvailable(ctx, beadsDir)
	if err == nil {
		t.Fatal("expected error for embedded mode")
	}
	if !strings.Contains(err.Error(), "embedded Dolt was removed") {
		t.Errorf("expected the embedded-removed error, got: %v", err)
	}
}

func TestOpenBestAvailable_NoMetadata_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("failed to create .beads dir: %v", err)
	}
	// No metadata.json — the default mode is embedded, which was removed.

	ctx := context.Background()
	_, err := beads.OpenBestAvailable(ctx, beadsDir)
	if err == nil {
		t.Fatal("expected error for embedded mode")
	}
	if !strings.Contains(err.Error(), "embedded Dolt was removed") {
		t.Errorf("expected the embedded-removed error, got: %v", err)
	}
}
