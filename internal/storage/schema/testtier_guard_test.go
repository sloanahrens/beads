package schema

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/steveyegge/beads/internal/testtier"
)

// TestMigrateUpRefusesPendingMigrationsInUnitTier pins the be-b23 tripwire:
// in the unit test tier (BD_TEST_TIER=unit) a store with migration work
// pending must fail before the first migration step runs, because each step
// is a fsync'd DOLT_COMMIT (seconds per fresh store).
func TestMigrateUpRefusesPendingMigrationsInUnitTier(t *testing.T) {
	t.Setenv(testtier.EnvVar, "unit")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	expectIgnorePatternSeedNoop(mock, 42)
	expectCursorProbe(mock, "schema_migrations", true)
	expectScalar(mock, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations", "version", 42)

	_, err = MigrateUp(context.Background(), db)
	if !errors.Is(err, testtier.ErrUnitTier) {
		t.Fatalf("MigrateUp() error = %v, want testtier.ErrUnitTier", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
