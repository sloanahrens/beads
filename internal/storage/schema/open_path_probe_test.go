package schema

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestMigrationWorkNeededIssuesOnlyShowProbes is the be-l84 guard for the
// store-open path. bd runs migrationWorkNeeded once per invocation (both the
// main and the ignored series below it), and every catalog question it asks
// must be a single-table SHOW: INFORMATION_SCHEMA.TABLES predicates are not
// pushed down, so Dolt materializes the whole table catalog to answer one
// (~100ms on the town's shared server, the most-sampled query in its
// processlist), and INFORMATION_SCHEMA.COLUMNS has the same shape problem.
//
// sqlmock is ordered and strict: a query with no registered expectation fails
// the call it was issued from. Registering only SHOW-shaped expectations
// therefore makes a regression to INFORMATION_SCHEMA an outright failure here,
// not a silent slow path in production. It also pins the exact per-open
// statement sequence, which is the "statements a connect-and-list run sends"
// count the bead asks about at the unit tier.
func TestMigrationWorkNeededIssuesOnlyShowProbes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// Main series: cursor probe, then the version read.
	expectTableProbe(mock, "schema_migrations", true)
	expectScalar(mock, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations", "version", LatestVersion())
	// Ignored series: cursor probe, version read, then the sentinels that
	// corroborate a non-zero cursor (gh 5033).
	expectTableProbe(mock, "ignored_schema_migrations", true)
	expectScalar(mock, "SELECT COALESCE(MAX(version), 0) FROM ignored_schema_migrations", "version", LatestIgnoredVersion())
	for _, table := range ignoredSource.sentinelTables {
		expectTableProbe(mock, table, true)
	}
	for _, column := range ignoredSource.sentinelColumns {
		expectColumnProbe(mock, column.table, column.column, true)
	}
	// Both series carry the per-migration content_hash column.
	expectColumnProbe(mock, "schema_migrations", "content_hash", true)
	expectColumnProbe(mock, "ignored_schema_migrations", "content_hash", true)
	// Custom type/status backfill checks: plain table reads, no catalog scans.
	expectScalar(mock, "SELECT COUNT(*) FROM custom_types", "count", 0)
	mock.ExpectQuery("SELECT .value. FROM config WHERE .key. = 'types.custom'").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))
	expectScalar(mock, "SELECT COUNT(*) FROM custom_statuses", "count", 0)
	mock.ExpectQuery("SELECT .value. FROM config WHERE .key. = 'status.custom'").
		WillReturnRows(sqlmock.NewRows([]string{"value"}))

	needed, err := migrationWorkNeeded(context.Background(), db)
	if err != nil {
		t.Fatalf("migrationWorkNeeded: %v", err)
	}
	if needed {
		t.Fatal("migrationWorkNeeded = true on an at-latest, fully materialized database")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations (a query deviated from the SHOW-only probe sequence): %v", err)
	}
}
