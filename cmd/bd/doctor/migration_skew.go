package doctor

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/steveyegge/beads/internal/storage/dberrors"
	"github.com/steveyegge/beads/internal/storage/schema"
)

const migrationContentSkewCheckName = "Migration Content Skew"

// CheckMigrationContentSkew detects when this database and its remote applied
// DIFFERENT content for the same migration version — the silent schema fork from
// gastownhall/beads#4259. It compares the local schema_migrations content hashes
// against the already-cached remote-tracking ref (no network fetch) and warns on
// any divergence.
//
// This is a read-only diagnostic; it never gates push/pull. It skips cleanly
// when there is no remote, no cached remote ref, or no recorded hashes to
// compare.
func CheckMigrationContentSkew(ss *SharedStore) DoctorCheck {
	if store := ss.Store(); store != nil {
		return checkMigrationContentSkew(context.Background(), store.DB(), store.RemoteName())
	}
	return DoctorCheck{
		Name:     migrationContentSkewCheckName,
		Status:   StatusOK,
		Message:  "N/A (no database)",
		Category: CategoryData,
	}
}

// checkMigrationContentSkew compares local migration content hashes against the
// configured sync remote's cached tracking ref. remote is the sync remote name
// (DoltStore.RemoteName(), "origin" by default) — NOT whichever remote happens
// to sort first in dolt_remotes.
func checkMigrationContentSkew(ctx context.Context, db *sql.DB, remote string) DoctorCheck {
	ok := func(msg string) DoctorCheck {
		return DoctorCheck{Name: migrationContentSkewCheckName, Status: StatusOK, Message: msg, Category: CategoryData}
	}
	// "Cannot check" is NOT "checked and matches": surface unexpected failures
	// as a warning instead of swallowing them as OK (bd-6dnrw.27 — a broken
	// query made this check a silent permanent no-op).
	cannot := func(stage string, err error) DoctorCheck {
		return DoctorCheck{
			Name:     migrationContentSkewCheckName,
			Status:   StatusWarning,
			Message:  fmt.Sprintf("Could not check migration content skew (%s): %v", stage, err),
			Detail:   "The skew check failed to run; this does not mean skew exists. Re-run `bd doctor` and report the error if it persists.",
			Category: CategoryData,
		}
	}

	if remote == "" {
		remote = "origin"
	}

	// Without the configured sync remote there is nothing to compare against.
	var remoteCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM dolt_remotes WHERE name = ?", remote).Scan(&remoteCount); err != nil {
		if dberrors.IsTableNotExist(err) {
			return ok("No remote configured — nothing to compare")
		}
		return cannot("read dolt_remotes", err)
	}
	if remoteCount == 0 {
		return ok(fmt.Sprintf("Sync remote %q not configured — nothing to compare", remote))
	}

	branch := "main"
	var active string
	if err := db.QueryRowContext(ctx, "SELECT active_branch()").Scan(&active); err == nil && active != "" {
		branch = active
	}

	local, err := schema.ReadMigrationContentHashes(ctx, db, "")
	if err != nil {
		if schema.MissingMigrationObjectErr(err) {
			return ok("No local migration content hashes recorded yet")
		}
		return cannot("read local schema_migrations", err)
	}
	if len(local) == 0 {
		return ok("No local migration content hashes recorded yet")
	}

	ref := "remotes/" + remote + "/" + branch
	remoteHashes, err := schema.ReadMigrationContentHashes(ctx, db, ref)
	if err != nil {
		// The remote-tracking ref is not cached yet (e.g. never pushed/pulled).
		if schema.RemoteRefUnavailableErr(err) {
			return ok(fmt.Sprintf("No cached remote ref %q to compare", ref))
		}
		// The cached ref exists but predates schema_migrations/content_hash.
		if schema.MissingMigrationObjectErr(err) {
			return ok(fmt.Sprintf("Cached remote ref %q predates migration content hashes — nothing to compare", ref))
		}
		return cannot(fmt.Sprintf("read schema_migrations at %q", ref), err)
	}

	skewed := schema.ContentHashSkew(local, remoteHashes)
	if len(skewed) == 0 {
		return ok(fmt.Sprintf("Applied migrations match remote %q", remote))
	}

	versions := make([]string, len(skewed))
	for i, v := range skewed {
		versions[i] = fmt.Sprintf("%04d", v)
	}
	return DoctorCheck{
		Name:     migrationContentSkewCheckName,
		Status:   StatusWarning,
		Message:  fmt.Sprintf("This database and remote %q applied different content for migration(s) %s", remote, strings.Join(versions, ", ")),
		Detail:   "Two clones ran different migration content for the same version number — a silent schema fork (gastownhall/beads#4259). `bd dolt pull` may fail to merge cryptically.",
		Fix:      "Upgrade every clone to a bd version that carries the schema-convergence migration. If a merge already fails, make one clone canonical and re-bootstrap the others from the remote.",
		Category: CategoryData,
	}
}
