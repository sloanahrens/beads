package embeddeddolt

import (
	"os"
	"path/filepath"
)

// HasRepository reports whether beadsDir contains an embedded Dolt repository.
// It owns the adapter's coarse .dolt marker probe: the marker must be a
// non-symlink directory with at least one entry, but entry names and private
// repository files are never interpreted or opened.
func HasRepository(beadsDir string) bool {
	return len(embeddedRepositories(beadsDir)) > 0
}

// SoleRepository returns the name of the single embedded Dolt repository beneath
// beadsDir, or ("", false) when there is none or more than one.
//
// It shares HasRepository's marker probe rather than restating it, because the
// two callers need the same answer about the same directories: a workspace
// whose metadata.json cannot be read has its database name recovered from this
// directory listing, and recording a candidate HasRepository would not accept
// points metadata.json at a database the adapter cannot open. Two candidates
// name nothing — the unreadable file was the only record of which one this
// workspace used — so the caller must keep refusing to guess.
func SoleRepository(beadsDir string) (string, bool) {
	names := embeddedRepositories(beadsDir)
	if len(names) != 1 {
		return "", false
	}
	return names[0], true
}

// embeddedRepositories returns the names of every usable embedded Dolt
// repository beneath beadsDir, sorted as os.ReadDir reports them. A root that is
// missing or is itself a symlink has none: a workspace symlinking its
// embeddeddolt root is pointing outside the directory whose contents the
// adapter manages, so the entry names there name no repository it owns.
func embeddedRepositories(beadsDir string) []string {
	root := filepath.Join(beadsDir, "embeddeddolt")
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if isRepositoryDirectory(root, entry) {
			names = append(names, entry.Name())
		}
	}
	return names
}

// isRepositoryDirectory reports whether entry names a usable embedded Dolt
// repository under root: a non-symlink directory whose .dolt marker is itself a
// non-symlink directory holding at least one entry. An empty or half-initialized
// marker is not a repository — recording one would leave metadata.json pointing
// at a database that cannot be opened.
func isRepositoryDirectory(root string, entry os.DirEntry) bool {
	if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
		return false
	}
	marker, err := os.Lstat(filepath.Join(root, entry.Name(), ".dolt"))
	if err != nil || !marker.IsDir() || marker.Mode()&os.ModeSymlink != 0 {
		return false
	}
	entries, err := os.ReadDir(filepath.Join(root, entry.Name(), ".dolt"))
	return err == nil && len(entries) > 0
}
