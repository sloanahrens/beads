//go:build cgo

package embeddeddolt

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/steveyegge/beads/internal/storage/versioncontrolops"
)

// presentedAuth is what a remote entry point was handed, plus the environment
// pair Dolt itself would read at that moment.
type presentedAuth struct {
	remote string
	branch string
	user   string
	envUsr string
	usrSet bool
	envPwd string
	pwdSet bool
}

// errStopRemote aborts a verb at the remote entry point. Credentials are fully
// resolved by then, so stopping there records everything the test needs while
// keeping it off the network and out of the post-pull recompute.
var errStopRemote = errors.New("stop before remote io")

// captureRemoteEntryPoints swaps the remote entry points for recorders and
// restores them when the test ends.
func captureRemoteEntryPoints(t *testing.T, got *presentedAuth) {
	t.Helper()

	record := func(remote, branch, user string) error {
		got.remote, got.branch, got.user = remote, branch, user
		got.envUsr, got.usrSet = os.LookupEnv("DOLT_REMOTE_USER")
		got.envPwd, got.pwdSet = os.LookupEnv("DOLT_REMOTE_PASSWORD")
		return errStopRemote
	}

	prevPush, prevForce := vcPush, vcForcePush
	prevPull, prevPullStrategy := vcPull, vcPullWithStrategy
	t.Cleanup(func() {
		vcPush, vcForcePush = prevPush, prevForce
		vcPull, vcPullWithStrategy = prevPull, prevPullStrategy
	})

	vcPush = func(_ context.Context, _ versioncontrolops.DBConn, remote, branch, user string) error {
		return record(remote, branch, user)
	}
	vcForcePush = func(_ context.Context, _ versioncontrolops.DBConn, remote, branch, user string) error {
		return record(remote, branch, user)
	}
	vcPull = func(_ context.Context, _ versioncontrolops.DBConn, remote, branch, user string) error {
		return record(remote, branch, user)
	}
	vcPullWithStrategy = func(_ context.Context, _ versioncontrolops.DBConn, remote, branch, user, _ string) error {
		return record(remote, branch, user)
	}
}

// The seam is only sound while the variables still hold the real entry points.
func TestRemoteEntryPointsUseVersionControlOps(t *testing.T) {
	cases := []struct {
		name      string
		got, want any
	}{
		{"vcPush", vcPush, versioncontrolops.Push},
		{"vcForcePush", vcForcePush, versioncontrolops.ForcePush},
		{"vcPull", vcPull, versioncontrolops.Pull},
		{"vcPullWithStrategy", vcPullWithStrategy, versioncontrolops.PullWithStrategy},
	}
	for _, tc := range cases {
		if reflect.ValueOf(tc.got).Pointer() != reflect.ValueOf(tc.want).Pointer() {
			t.Errorf("%s is not bound to the versioncontrolops entry point", tc.name)
		}
	}
}
