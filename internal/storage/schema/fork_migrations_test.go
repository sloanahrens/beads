package schema

import "testing"

// forkMigrationNameAt returns the embedded migration file name the series
// carries at version, or "" when no file has that version. It goes through
// list(), so the names come from the embedded FS the binary runs, never from
// whatever happens to sit in the working directory.
func forkMigrationNameAt(src migrationSource, version int) string {
	for _, f := range src.list() {
		if f.version == version {
			return f.name
		}
	}
	return ""
}

// TestForkOnlyMigrationMainSeries pins the fork's main-series migration 0067.
//
// Why the pin: the schema cursor records only a version number. A merge,
// rebase or rename that gives this fork-only migration a different number or
// file name leaves a database that already applied it silently skipping the
// work (its cursor is already past the version) or re-running it under the new
// number. The file name is asserted, not just the number, because a rename at
// the same number would otherwise pass. See engdocs/fork-upstream-sync.md.
func TestForkOnlyMigrationMainSeries(t *testing.T) {
	const (
		version = 67
		want    = "0067_create_samples.up.sql"
	)
	if got := forkMigrationNameAt(mainSource, version); got != want {
		t.Fatalf("main migration %d is %q, want %q: moving or renaming a fork-only migration breaks every database that already applied it; see engdocs/fork-upstream-sync.md", version, got, want)
	}
}

// TestForkOnlyMigrationIgnoredSeries pins the fork's ignored-series migration
// 0026, for the same reason as TestForkOnlyMigrationMainSeries: the ignored
// cursor also records only a version number, so a moved or renamed file is
// silently skipped or re-applied. See engdocs/fork-upstream-sync.md.
func TestForkOnlyMigrationIgnoredSeries(t *testing.T) {
	const (
		version = 26
		want    = "0026_add_wisp_dependency_target_indexes.up.sql"
	)
	if got := forkMigrationNameAt(ignoredSource, version); got != want {
		t.Fatalf("ignored migration %d is %q, want %q: moving or renaming a fork-only migration breaks every database that already applied it; see engdocs/fork-upstream-sync.md", version, got, want)
	}
}
