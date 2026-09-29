package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

// HasProjectFiles is the one predicate answering "does a workspace already
// exist here?": a workspace marker (metadata.json or config.yaml) or a
// database (dolt/, embeddeddolt/, or a non-backup *.db). Directories that only
// hold legacy registry files (~/.beads/registry.json) do not qualify.
func HasProjectFiles(beadsDir string) bool {
	return HasWorkspaceMarker(beadsDir) || HasDatabase(beadsDir)
}

// HasWorkspaceMarker reports whether beadsDir carries an explicit workspace
// marker: metadata.json or config.yaml. See beads.HasWorkspaceMarker for why
// creation guards key on this half only (be-n2s).
func HasWorkspaceMarker(beadsDir string) bool {
	for _, name := range []string{"metadata.json", "config.yaml"} {
		if _, err := os.Stat(filepath.Join(beadsDir, name)); err == nil {
			return true
		}
	}
	return false
}

// HasDatabase is the strict half: true only when beadsDir holds an actual
// database directory or *.db file. Inherited tracked artifacts
// (metadata.json, config.yaml, issues.jsonl) do not count.
func HasDatabase(beadsDir string) bool {
	for _, name := range []string{"dolt", "embeddeddolt"} {
		if info, err := os.Stat(filepath.Join(beadsDir, name)); err == nil && info.IsDir() {
			return true
		}
	}
	dbMatches, _ := filepath.Glob(filepath.Join(beadsDir, "*.db"))
	for _, match := range dbMatches {
		baseName := filepath.Base(match)
		if !strings.Contains(baseName, ".backup") && baseName != "vc.db" {
			return true
		}
	}
	return false
}
