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

// HasDatabase reports whether beadsDir/embeddeddolt/<database> is an existing
// embedded Dolt repository: the per-name form of HasRepository's question.
//
// The per-name question is the one a refuse-to-create open needs. A rename, a
// re-point at a differently-named database, or a bare embeddeddolt/ shell left
// behind by an earlier misdirected open can all leave a directory holding
// repositories while holding NONE under the name the caller is about to open —
// and the engine's answer to "no database by that name" is to create one
// (be-n2s).
//
// It shares embeddedRepositories rather than restating the probe, so a name
// this accepts is exactly a name HasRepository and SoleRepository accept, and
// the three cannot drift apart.
func HasDatabase(beadsDir, database string) bool {
	if !isSinglePathElement(database) {
		return false
	}
	for _, name := range embeddedRepositories(beadsDir) {
		if name == database {
			return true
		}
	}
	return false
}

// isSinglePathElement reports whether name is usable as one path component —
// not empty, not a separator-bearing path, not "." or "..".
func isSinglePathElement(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name
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
