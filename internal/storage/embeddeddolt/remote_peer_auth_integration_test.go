//go:build cgo && integration

package embeddeddolt

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/steveyegge/beads/internal/storage"
)

// Regression test for the GH#5080 remainder: the non-federation verbs read
// credentials from the environment alone, so a peer reached as an ordinary
// remote (`bd sync --remote <peer>`, `bd dolt push|pull --remote <peer>`, or a
// peer registered as origin) authenticated as whatever the environment held.
// Every verb must present the credentials add-peer stored for the remote it
// operates on. PushRemote appears twice because it has two entry points.
func TestRemoteVerbsPresentStoredPeerCredentials(t *testing.T) {
	ctx := t.Context()
	store := newPeerAuthTestStore(t)

	t.Setenv("DOLT_REMOTE_USER", "envuser")
	t.Setenv("DOLT_REMOTE_PASSWORD", "envpass")

	// "origin" covers the verbs that resolve the default remote internally;
	// "team" covers the verbs that take the remote as a parameter.
	for _, name := range []string{defaultRemote, "team"} {
		if err := store.AddFederationPeer(ctx, &storage.FederationPeer{
			Name: name, RemoteURL: "https://peer.example/" + name,
			Username: name + "user", Password: name + "pass",
		}); err != nil {
			t.Fatalf("AddFederationPeer(%s): %v", name, err)
		}
	}

	cases := []struct {
		name   string
		remote string
		call   func(*EmbeddedDoltStore, context.Context) error
	}{
		{"Push", defaultRemote, func(s *EmbeddedDoltStore, ctx context.Context) error {
			return s.Push(ctx)
		}},
		{"Pull", defaultRemote, func(s *EmbeddedDoltStore, ctx context.Context) error {
			return s.Pull(ctx)
		}},
		{"PullWithStrategy", defaultRemote, func(s *EmbeddedDoltStore, ctx context.Context) error {
			return s.PullWithStrategy(ctx, "ours")
		}},
		{"ForcePush", defaultRemote, func(s *EmbeddedDoltStore, ctx context.Context) error {
			return s.ForcePush(ctx)
		}},
		{"PullRemoteWithStrategy", "team", func(s *EmbeddedDoltStore, ctx context.Context) error {
			return s.PullRemoteWithStrategy(ctx, "team", "ours")
		}},
		{"PushRemote", "team", func(s *EmbeddedDoltStore, ctx context.Context) error {
			return s.PushRemote(ctx, "team", false)
		}},
		{"PushRemote force", "team", func(s *EmbeddedDoltStore, ctx context.Context) error {
			return s.PushRemote(ctx, "team", true)
		}},
		{"PullRemote", "team", func(s *EmbeddedDoltStore, ctx context.Context) error {
			return s.PullRemote(ctx, "team")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got presentedAuth
			captureRemoteEntryPoints(t, &got)

			if err := tc.call(store, ctx); !errors.Is(err, errStopRemote) {
				t.Fatalf("%s error = %v, want errStopRemote (the verb never reached the remote entry point)", tc.name, err)
			}
			if got.remote != tc.remote {
				t.Errorf("remote = %q, want %q", got.remote, tc.remote)
			}
			if got.branch != store.branch {
				t.Errorf("branch = %q, want %q", got.branch, store.branch)
			}
			wantUser, wantPwd := tc.remote+"user", tc.remote+"pass"
			if got.user != wantUser {
				t.Errorf("user = %q, want stored %q (not the ambient %q)", got.user, wantUser, "envuser")
			}
			if !got.usrSet || got.envUsr != wantUser {
				t.Errorf("DOLT_REMOTE_USER during op = %q (set=%v), want %q", got.envUsr, got.usrSet, wantUser)
			}
			if !got.pwdSet || got.envPwd != wantPwd {
				t.Errorf("DOLT_REMOTE_PASSWORD during op = %q (set=%v), want %q", got.envPwd, got.pwdSet, wantPwd)
			}
			if v := os.Getenv("DOLT_REMOTE_USER"); v != "envuser" {
				t.Errorf("DOLT_REMOTE_USER after op = %q, want restored %q", v, "envuser")
			}
			if v := os.Getenv("DOLT_REMOTE_PASSWORD"); v != "envpass" {
				t.Errorf("DOLT_REMOTE_PASSWORD after op = %q, want restored %q", v, "envpass")
			}
		})
	}
}

// A remote with no stored peer must behave exactly as it did before the verbs
// were wrapped: the ambient environment pair reaches Dolt untouched, and
// withPeerAuth neither rewrites nor restores anything around the operation.
func TestRemoteVerbPlainRemoteKeepsAmbientEnv(t *testing.T) {
	ctx := t.Context()
	store := newPeerAuthTestStore(t)

	t.Setenv("DOLT_REMOTE_USER", "envuser")
	t.Setenv("DOLT_REMOTE_PASSWORD", "envpass")

	var got presentedAuth
	captureRemoteEntryPoints(t, &got)

	if err := store.PushRemote(ctx, "plain", false); !errors.Is(err, errStopRemote) {
		t.Fatalf("PushRemote error = %v, want errStopRemote", err)
	}
	if got.remote != "plain" {
		t.Errorf("remote = %q, want %q", got.remote, "plain")
	}
	if got.user != "envuser" {
		t.Errorf("user = %q, want the ambient %q", got.user, "envuser")
	}
	if !got.usrSet || got.envUsr != "envuser" {
		t.Errorf("DOLT_REMOTE_USER during op = %q (set=%v), want ambient %q", got.envUsr, got.usrSet, "envuser")
	}
	if !got.pwdSet || got.envPwd != "envpass" {
		t.Errorf("DOLT_REMOTE_PASSWORD during op = %q (set=%v), want ambient %q", got.envPwd, got.pwdSet, "envpass")
	}
	if v := os.Getenv("DOLT_REMOTE_USER"); v != "envuser" {
		t.Errorf("DOLT_REMOTE_USER after op = %q, want %q", v, "envuser")
	}
	if v := os.Getenv("DOLT_REMOTE_PASSWORD"); v != "envpass" {
		t.Errorf("DOLT_REMOTE_PASSWORD after op = %q, want %q", v, "envpass")
	}
}

// The unauthenticated case (git+ssh, file://, no environment pair) must stay
// unauthenticated: no stored peer and no ambient credentials means the verb
// presents no user and leaves both variables unset.
func TestRemoteVerbPlainRemoteWithoutAuthPresentsNone(t *testing.T) {
	ctx := t.Context()
	store := newPeerAuthTestStore(t)

	t.Setenv("DOLT_REMOTE_USER", "scratch")
	_ = os.Unsetenv("DOLT_REMOTE_USER")
	t.Setenv("DOLT_REMOTE_PASSWORD", "scratch")
	_ = os.Unsetenv("DOLT_REMOTE_PASSWORD")

	var got presentedAuth
	captureRemoteEntryPoints(t, &got)

	if err := store.PullRemote(ctx, "plain"); !errors.Is(err, errStopRemote) {
		t.Fatalf("PullRemote error = %v, want errStopRemote", err)
	}
	if got.user != "" {
		t.Errorf("user = %q, want empty", got.user)
	}
	if got.usrSet {
		t.Errorf("DOLT_REMOTE_USER during op = %q, want unset", got.envUsr)
	}
	if got.pwdSet {
		t.Errorf("DOLT_REMOTE_PASSWORD during op = %q, want unset", got.envPwd)
	}
	if v, set := os.LookupEnv("DOLT_REMOTE_USER"); set {
		t.Errorf("DOLT_REMOTE_USER after op = %q, want unset", v)
	}
	if v, set := os.LookupEnv("DOLT_REMOTE_PASSWORD"); set {
		t.Errorf("DOLT_REMOTE_PASSWORD after op = %q, want unset", v)
	}
}
