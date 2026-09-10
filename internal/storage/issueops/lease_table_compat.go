package issueops

import (
	"errors"
	"strings"

	"github.com/steveyegge/beads/internal/storage/dberrors"
	"github.com/steveyegge/beads/internal/storage/sqlbuild"
)

// ErrLeasesTableMissing is returned by a write that requires the leases
// table (bd-lrgn1: claim/heartbeat/reclaim state) instead of the raw MySQL
// "table not found: leases" it wraps. It means this database predates
// migration 0055 (move claim leases to their own table) and has not had it
// applied yet — every rig's DB is remote-backed on the shared Dolt server,
// so in-place migration cannot be run from here (#4259); the coordinated
// migration is the only remedy (be-cm3, split from be-qah).
var ErrLeasesTableMissing = errors.New(
	"leases table missing: migration 0055 (move claim leases to their own table) has not been applied to this database — run `bd migrate` (or wait for the coordinated migration) before claiming, heartbeating, or reclaiming issues")

// leasesTableMissing reports whether err is specifically the leases-table-
// not-found error, as opposed to some other broken-database error that must
// still fail loudly. Both the read-degrade path (this file's
// degradeLeaseSQL retry) and the write-error path (wrapLeaseTableMissing)
// key off this single classifier so they can never disagree about what
// "the leases table is absent" means.
func leasesTableMissing(err error) bool {
	return dberrors.IsMissingTable(err, "leases")
}

// wrapLeaseTableMissing converts a leases-table-not-found failure on a write
// path into ErrLeasesTableMissing, leaving every other error (including a
// different missing table) unchanged. Every write that mutates the leases
// table routes its error through this before wrapping it with call-site
// context, so a caller never has to pattern-match a raw MySQL 1146 to learn
// the database needs migrating (be-cm3 deliverable 2).
func wrapLeaseTableMissing(err error) error {
	if err != nil && leasesTableMissing(err) {
		return ErrLeasesTableMissing
	}
	return err
}

// degradeLeaseSQL rewrites a rendered query that projected the lease
// overlay (sqlbuild.LeaseSelectColumns, read through joinFragment — exactly
// what sqlbuild.LeaseJoin(...) returned when the caller built the query) into
// the equivalent query with no lease data: the three lease columns become
// NULL literals, which the scan side already treats as "no live lease" (the
// same value a LEFT JOIN produces for an unleased row), and the join itself
// is dropped so the query no longer references the table at all.
//
// Both substrings are exact — the caller passes back its own
// sqlbuild.LeaseSelectColumns/LeaseJoin output, not a guess — so a plain
// string replace is precise rather than a pattern match that could misfire
// on user data.
//
// Read paths call this to retry once after a query fails with
// leasesTableMissing: a database that predates migration 0055 has no leases
// table at all, so every read that would have joined it degrades to
// reporting no live lease for every row instead of failing outright
// (be-cm3 deliverable 1).
func degradeLeaseSQL(query, joinFragment string) string {
	query = strings.ReplaceAll(query, sqlbuild.LeaseSelectColumns, "NULL, NULL, NULL")
	if joinFragment != "" {
		query = strings.ReplaceAll(query, joinFragment, "")
	}
	return query
}
