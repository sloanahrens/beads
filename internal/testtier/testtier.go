// Package testtier enforces the beads test tiers (wayfinder D9, be-b23).
//
// The unit tier (make test) runs with BD_TEST_TIER=unit and must never pay
// for a real Dolt store: every fresh store runs the full migration chain,
// one DOLT_COMMIT per migration, each fsync'd (F_FULLFSYNC on macOS, about
// four seconds per store). Tests that need a real store belong to the
// integration tier (make test-integration). The tripwires in schema.MigrateUp
// and doltserver.Start call Refuse, so a unit-tier test that opens a fresh
// store or auto-starts a Dolt server fails with a message naming the tier,
// in-process or in a spawned bd subprocess alike.
//
// Some cmd/bd subprocess helpers strip every BEADS_* and BD_* variable from
// the child environment, so the variable alone cannot reach every spawned bd.
// scripts/test.sh therefore also links the tier into the bd it prebuilds for
// subprocess tests (-X .../testtier.buildTier=unit); that value applies when
// the variable is absent. Production builds never set either, so Refuse is a
// no-op outside the test tiers.
package testtier

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// EnvVar names the test tier. "unit" arms the tripwires; any other value,
// including unset, leaves them off.
const EnvVar = "BD_TEST_TIER"

// buildTier is the tier linked into a test-only bd binary by scripts/test.sh.
// Empty in every production build.
var buildTier string

// tier returns the active tier: the environment variable when present (even
// empty), otherwise the linked value.
func tier() string {
	if v, ok := os.LookupEnv(EnvVar); ok {
		return strings.TrimSpace(v)
	}
	return buildTier
}

// ErrUnitTier is returned by Refuse in the unit tier.
var ErrUnitTier = errors.New("operation not allowed in the unit test tier")

// Unit reports whether the process runs in the unit test tier.
func Unit() bool {
	return tier() == "unit"
}

// Integration reports whether the process runs in the integration test tier,
// where infra-unavailable skips become failures (require-mode).
func Integration() bool {
	return tier() == "integration"
}

// Refuse returns an error wrapping ErrUnitTier when running in the unit tier,
// and nil otherwise. op names what was refused, for the failure message.
func Refuse(op string) error {
	if !Unit() {
		return nil
	}
	return fmt.Errorf("%w: %s needs a real Dolt store; %s=unit forbids it. "+
		"Move the test to the integration tier (//go:build integration, run by make test-integration) "+
		"or use a fixture that does not migrate", ErrUnitTier, op, EnvVar)
}
