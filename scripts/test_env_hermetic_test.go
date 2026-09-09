package scripts_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestMakeTestHermeticEnv locks in be-9yi: scripts/ci/lib/test-env.sh's
// beads_test_env_enter (sourced by `make test` via scripts/test.sh) must
// strip every env var that could route a test process at a real Dolt server.
//
// A 2026-09-09 incident: beads/refinery ran the full suite with
// GT_DOLT_PORT=3307 (Gas Town's env var for the town's live, shared,
// production `hq` dolt sql-server) inherited from its shell. Nothing in the
// test harness stripped it, and a downstream auto-migration path used it to
// reach and migrate hq mid test-run, bricking every other bd client
// town-wide until the mayor rolled the cursor back.
//
// This test asserts two things: GT_DOLT_PORT is explicitly unset (it is not
// BD_-prefixed, so nothing else here would catch it), and EVERY BD_-prefixed
// env var is unset, not just a hand-enumerated few — internal/config/config.go
// binds v.SetEnvPrefix("BD") + v.AutomaticEnv(), so any "BD_<KEY>" env var
// can silently override any config key (including the remote-migrate escape
// hatch BD_ALLOW_REMOTE_MIGRATE itself), and a hand-enumerated unset list
// drifts out of sync with new config keys exactly the way this incident's
// list did.
func TestMakeTestHermeticEnv(t *testing.T) {
	const script = `
set -euo pipefail
source ci/lib/test-env.sh
beads_test_env_enter
echo "GT_DOLT_PORT=${GT_DOLT_PORT-<unset>}"
echo "BD_DOLT_AUTO_COMMIT=${BD_DOLT_AUTO_COMMIT-<unset>}"
echo "BD_ALLOW_REMOTE_MIGRATE=${BD_ALLOW_REMOTE_MIGRATE-<unset>}"
echo "BD_ACTOR=${BD_ACTOR-<unset>}"
echo "BD_A_FUTURE_CONFIG_KEY_NOBODY_ENUMERATED_YET=${BD_A_FUTURE_CONFIG_KEY_NOBODY_ENUMERATED_YET-<unset>}"
`
	cmd := exec.Command("bash", "-c", script)
	// cwd is the "scripts" package directory (go test convention), so
	// "ci/lib/test-env.sh" resolves without hard-coding the repo root.
	cmd.Env = append(os.Environ(),
		"GT_DOLT_PORT=3307",
		"BD_DOLT_AUTO_COMMIT=off",
		"BD_ALLOW_REMOTE_MIGRATE=1",
		"BD_ACTOR=beads/polecats/nitro",
		"BD_A_FUTURE_CONFIG_KEY_NOBODY_ENUMERATED_YET=leaked",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("beads_test_env_enter failed: %v\n%s", err, out)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasSuffix(line, "=<unset>") {
			t.Errorf("beads_test_env_enter left a Dolt-server-reaching env var set: %s (want unset)", line)
		}
	}
}
