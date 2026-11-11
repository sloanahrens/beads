package schema

import (
	"os"
	"strings"
	"testing"
)

// TestSamplesDownMigrationRefusesToDropAPopulatedTable pins the guard in
// migrations/0067_create_samples.down.sql. bd embeds and runs only
// *.up.sql, so the down file has no Go caller to exercise; this test is the
// guard's check, and it fails if the conditional drop is reduced back to a
// bare DROP TABLE.
func TestSamplesDownMigrationRefusesToDropAPopulatedTable(t *testing.T) {
	raw, err := os.ReadFile("migrations/0067_create_samples.down.sql")
	if err != nil {
		t.Fatalf("read 0067 down migration: %v", err)
	}
	sql := string(raw)

	for _, want := range []string{
		"@bd_migrate_force", // the force switch, read in-session
		"FROM samples)",     // the row count that decides refusal
		// The refusal names the variable the guard actually reads — the
		// session variable — not a shell variable nothing in this repo maps
		// onto it.
		"samples_down_refused_rows_present_set_session_bd_migrate_force",
		"IF(@has_samples = 0 OR @samples_rows = 0 OR @bd_migrate_force = 1", // drop only when empty or forced
		"'DROP TABLE IF EXISTS samples'",                                    // the DROP is dynamic, so SQL decides
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("0067 down migration does not contain %q:\n%s", want, sql)
		}
	}

	// The guard is only a guard if the unconditional form is absent: a bare
	// DROP on its own line would run no matter what the row count says.
	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "DROP TABLE IF EXISTS samples;" || trimmed == "DROP TABLE samples;" {
			t.Fatalf("0067 down migration drops the table unconditionally: %q", trimmed)
		}
	}
}
