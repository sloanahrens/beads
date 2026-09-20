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
# Under make test this process already runs inside an outer
# beads_test_env_enter, whose re-entrancy guard (BEADS_TEST_ENV_ACTIVE=1)
# makes a nested enter a no-op. The subject here is the scrub, so start a
# fresh enter: drop the guard and the disable switch, and let its own EXIT
# trap remove the temp root it creates.
unset BEADS_TEST_ENV_ACTIVE BEADS_TEST_ENV_DISABLE BEADS_TEST_ENV_KEEP
# An outer enter (make test) exports BEADS_TEST_MODE=1 and it is not BD_-
# prefixed, so the sweep would not remove it and the assertion below could
# not tell a fresh export from the inherited one. Clear both switches so the
# fresh enter has to set them itself.
unset BD_DISABLE_METRICS BEADS_TEST_MODE
beads_test_env_enter
echo "GT_DOLT_PORT=${GT_DOLT_PORT-<unset>}"
echo "BD_DOLT_AUTO_COMMIT=${BD_DOLT_AUTO_COMMIT-<unset>}"
echo "BD_ALLOW_REMOTE_MIGRATE=${BD_ALLOW_REMOTE_MIGRATE-<unset>}"
echo "BD_ACTOR=${BD_ACTOR-<unset>}"
echo "BD_A_FUTURE_CONFIG_KEY_NOBODY_ENUMERATED_YET=${BD_A_FUTURE_CONFIG_KEY_NOBODY_ENUMERATED_YET-<unset>}"
echo "metrics:BD_DISABLE_METRICS=${BD_DISABLE_METRICS-<unset>}"
echo "metrics:BEADS_TEST_MODE=${BEADS_TEST_MODE-<unset>}"
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
		// The two metrics switches are the one thing enter must SET after
		// the BD_ sweep (be-9zm): with HOME redirected to an empty sandbox
		// and no user config, every bd the suites spawn would otherwise
		// resolve metrics enabled — fork the platform machine-id probe,
		// write event files, and spawn detached send-metrics children.
		if rest, ok := strings.CutPrefix(line, "metrics:"); ok {
			if !strings.HasSuffix(rest, "=1") {
				t.Errorf("beads_test_env_enter left telemetry live for test-spawned bd: %s (want =1)", rest)
			}
			continue
		}
		if !strings.HasSuffix(line, "=<unset>") {
			t.Errorf("beads_test_env_enter left a Dolt-server-reaching env var set: %s (want unset)", line)
		}
	}
}
