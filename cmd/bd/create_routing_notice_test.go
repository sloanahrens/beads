// create_routing_notice_test.go - Tests for the stderr notice printed when
// `bd create` auto-routes a write to a different workspace.
//
// Deliberately untagged (no `cgo` build tag): the routing decision, the notice
// text and the target-open wiring need no store, so these tests stay
// compilable under CGO_ENABLED=0 with the gms_pure_go build tag alongside the
// rest of the pure-Go test suite.

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/routing"
	"github.com/steveyegge/beads/internal/storage"
)

// TestResolveCreateRoute covers the routing decision the create command makes
// before it opens a store: an explicit --repo always wins, auto mode picks the
// role's repo, and a configured default is the last fallback.
func TestResolveCreateRoute(t *testing.T) {
	tests := []struct {
		name            string
		mode            string
		defaultRepo     string
		maintainerRepo  string
		contributorRepo string
		repoOverride    string
		repoFlagChanged bool
		role            routing.UserRole
		wantRepo        string
		wantRule        routing.RoutingRule
	}{
		{
			name:            "explicit --repo wins over auto routing",
			mode:            "auto",
			contributorRepo: "/work/planning",
			repoOverride:    "/work/fork",
			repoFlagChanged: true,
			role:            routing.Contributor,
			wantRepo:        "/work/fork",
			wantRule:        routing.RuleExplicitOverride,
		},
		{
			name:            "explicit empty --repo is still explicit",
			mode:            "auto",
			contributorRepo: "/work/planning",
			repoFlagChanged: true,
			role:            routing.Contributor,
			wantRepo:        "",
			wantRule:        routing.RuleExplicitOverride,
		},
		{
			name:            "auto mode routes a contributor",
			mode:            "auto",
			contributorRepo: "/work/planning",
			role:            routing.Contributor,
			wantRepo:        "/work/planning",
			wantRule:        routing.RuleContributor,
		},
		{
			name:           "auto mode routes a maintainer",
			mode:           "auto",
			maintainerRepo: "/work/upstream",
			role:           routing.Maintainer,
			wantRepo:       "/work/upstream",
			wantRule:       routing.RuleMaintainer,
		},
		{
			name:        "default repo fallback",
			defaultRepo: "/work/shared",
			wantRepo:    "/work/shared",
			wantRule:    routing.RuleDefault,
		},
		{
			name:     "no routing stays local",
			wantRepo: ".",
			wantRule: routing.RuleNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, rule := resolveCreateRoute(tt.mode, tt.defaultRepo, tt.maintainerRepo,
				tt.contributorRepo, tt.repoOverride, tt.repoFlagChanged, tt.role)
			if repo != tt.wantRepo || rule != tt.wantRule {
				t.Errorf("resolveCreateRoute(...) = (%q, %v), want (%q, %v)",
					repo, rule, tt.wantRepo, tt.wantRule)
			}
		})
	}
}

// stubCreateRouteOpeners returns openers that record which branch ran and
// return target without opening a store. wantOpen names the single branch the
// test expects, as "local:<path>" or "remote:<url>".
func stubCreateRouteOpeners(t *testing.T, target createRouteTarget, wantOpen string) createRouteOpeners {
	t.Helper()
	return createRouteOpeners{
		openLocal: func(_ context.Context, repoPath string, _ storage.DoltStorage, _ bool) (createRouteTarget, error) {
			if got := "local:" + repoPath; got != wantOpen {
				t.Errorf("openLocal called as %q, want %q", got, wantOpen)
			}
			return target, nil
		},
		openRemote: func(_ context.Context, remoteURL string) (createRouteTarget, error) {
			if got := "remote:" + remoteURL; got != wantOpen {
				t.Errorf("openRemote called as %q, want %q", got, wantOpen)
			}
			return target, nil
		},
	}
}

// TestOpenCreateRoutedTargetNotice drives the production function the create
// command calls after it resolves a route: it opens the selected target and
// emits exactly one stderr line naming the target repo, the workspace it
// resolves to, the matched rule and that rule's fix. Deleting the notice call
// (or dropping the rule on the way through) turns it red.
func TestOpenCreateRoutedTargetNotice(t *testing.T) {
	const (
		localRepo  = "/work/planning"
		localBeads = "/srv/beads-store/planning/.beads"
		remoteURL  = "dolthub://acme/planning"
		cacheDir   = "/home/u/.cache/beads/remotes/abc123"
	)

	tests := []struct {
		name        string
		repoPath    string
		rule        routing.RoutingRule
		target      createRouteTarget
		wantOpen    string
		wantStrings []string
	}{
		{
			name:     "contributor rule",
			repoPath: localRepo,
			rule:     routing.RuleContributor,
			target:   createRouteTarget{RepoPath: localRepo, BeadsDir: localBeads},
			wantOpen: "local:" + localRepo,
			wantStrings: []string{
				localRepo, localBeads, "beads.role=contributor", "git config beads.role maintainer",
			},
		},
		{
			name:     "maintainer rule",
			repoPath: localRepo,
			rule:     routing.RuleMaintainer,
			target:   createRouteTarget{RepoPath: localRepo, BeadsDir: localBeads},
			wantOpen: "local:" + localRepo,
			wantStrings: []string{
				localRepo, localBeads, "routing.maintainer", "bd config unset routing.maintainer",
			},
		},
		{
			name:     "default rule",
			repoPath: localRepo,
			rule:     routing.RuleDefault,
			target:   createRouteTarget{RepoPath: localRepo, BeadsDir: localBeads},
			wantOpen: "local:" + localRepo,
			wantStrings: []string{
				localRepo, localBeads, "routing.default", "bd config unset routing.default",
			},
		},
		{
			// A remote URL write has no repo-local .beads dir; the cache
			// clone is the workspace the bead lands in, and it must be
			// named too rather than leaving the route silent.
			name:     "remote URL target",
			repoPath: remoteURL,
			rule:     routing.RuleDefault,
			target:   createRouteTarget{RepoPath: remoteURL, BeadsDir: cacheDir},
			wantOpen: "remote:" + remoteURL,
			wantStrings: []string{
				remoteURL, cacheDir, "routing.default", "bd config unset routing.default",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			notice := &createRoutingNotice{out: &out}

			target, err := openCreateRoutedTarget(context.Background(), createRouteRequest{
				RepoPath:    tt.repoPath,
				Rule:        tt.rule,
				AllowCreate: true,
			}, stubCreateRouteOpeners(t, tt.target, tt.wantOpen), notice)
			if err != nil {
				t.Fatalf("openCreateRoutedTarget() error = %v", err)
			}
			if target.RepoPath != tt.target.RepoPath || target.BeadsDir != tt.target.BeadsDir {
				t.Errorf("openCreateRoutedTarget() = (%q, %q), want (%q, %q)",
					target.RepoPath, target.BeadsDir, tt.target.RepoPath, tt.target.BeadsDir)
			}

			stderr := out.String()
			if got := strings.Count(stderr, "\n"); got != 1 {
				t.Fatalf("openCreateRoutedTarget(rule=%v) wrote %d stderr lines, want 1: %q",
					tt.rule, got, stderr)
			}
			for _, want := range append(tt.wantStrings, "bd create") {
				if !strings.Contains(stderr, want) {
					t.Errorf("openCreateRoutedTarget(rule=%v) stderr = %q, want substring %q",
						tt.rule, stderr, want)
				}
			}
			// The create notice must describe a write, not reuse the read
			// notice's "bd list/ready" sentence.
			if strings.Contains(stderr, "list/ready") {
				t.Errorf("create notice describes a read: %q", stderr)
			}
		})
	}
}

// TestOpenCreateRoutedTargetNoticeSuppressed covers the branches that must
// stay silent and the once-per-process guard.
func TestOpenCreateRoutedTargetNoticeSuppressed(t *testing.T) {
	repoTarget := createRouteTarget{RepoPath: "/work/planning", BeadsDir: "/work/planning/.beads"}
	okOpeners := func() createRouteOpeners {
		return createRouteOpeners{
			openLocal: func(_ context.Context, _ string, _ storage.DoltStorage, _ bool) (createRouteTarget, error) {
				return repoTarget, nil
			},
			openRemote: func(_ context.Context, _ string) (createRouteTarget, error) {
				return repoTarget, nil
			},
		}
	}
	open := func(t *testing.T, req createRouteRequest, notice *createRoutingNotice, openers createRouteOpeners) error {
		t.Helper()
		_, err := openCreateRoutedTarget(context.Background(), req, openers, notice)
		return err
	}

	t.Run("explicit --repo is not auto-routed", func(t *testing.T) {
		var out bytes.Buffer
		notice := &createRoutingNotice{out: &out}
		err := open(t, createRouteRequest{
			RepoPath: "/work/fork", Rule: routing.RuleExplicitOverride, AllowCreate: true,
		}, notice, okOpeners())
		if err != nil {
			t.Fatalf("openCreateRoutedTarget() error = %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("explicit --repo wrote stderr = %q, want empty", out.String())
		}
	})

	t.Run("unrouted local target", func(t *testing.T) {
		var out bytes.Buffer
		notice := &createRoutingNotice{out: &out}
		err := open(t, createRouteRequest{
			RepoPath: ".", Rule: routing.RuleNone, AllowCreate: true,
		}, notice, okOpeners())
		if err != nil {
			t.Fatalf("openCreateRoutedTarget() error = %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("unrouted create wrote stderr = %q, want empty", out.String())
		}
	})

	t.Run("quiet", func(t *testing.T) {
		var out bytes.Buffer
		notice := &createRoutingNotice{out: &out}
		err := open(t, createRouteRequest{
			RepoPath: "/work/planning", Rule: routing.RuleContributor, AllowCreate: true, Quiet: true,
		}, notice, okOpeners())
		if err != nil {
			t.Fatalf("openCreateRoutedTarget() error = %v", err)
		}
		if out.Len() != 0 {
			t.Errorf("quiet create wrote stderr = %q, want empty", out.String())
		}

		// A suppressed call must not consume the once guard: a later
		// non-quiet routed create is still announced.
		err = open(t, createRouteRequest{
			RepoPath: "/work/planning", Rule: routing.RuleContributor, AllowCreate: true,
		}, notice, okOpeners())
		if err != nil {
			t.Fatalf("openCreateRoutedTarget() error = %v", err)
		}
		if out.Len() == 0 {
			t.Error("routed create after a quiet call wrote no stderr, want the notice")
		}
	})

	t.Run("open error", func(t *testing.T) {
		var out bytes.Buffer
		notice := &createRoutingNotice{out: &out}
		errOpen := errors.New("open failed")
		openers := createRouteOpeners{
			openLocal: func(_ context.Context, _ string, _ storage.DoltStorage, _ bool) (createRouteTarget, error) {
				return createRouteTarget{}, errOpen
			},
			openRemote: func(_ context.Context, _ string) (createRouteTarget, error) {
				return createRouteTarget{}, errOpen
			},
		}
		err := open(t, createRouteRequest{
			RepoPath: "/work/planning", Rule: routing.RuleContributor, AllowCreate: true,
		}, notice, openers)
		if !errors.Is(err, errOpen) {
			t.Fatalf("openCreateRoutedTarget() error = %v, want %v", err, errOpen)
		}
		if out.Len() != 0 {
			t.Errorf("failed open wrote stderr = %q, want empty", out.String())
		}
	})

	t.Run("once per process", func(t *testing.T) {
		var out bytes.Buffer
		notice := &createRoutingNotice{out: &out}
		req := createRouteRequest{
			RepoPath: "/work/planning", Rule: routing.RuleMaintainer, AllowCreate: true,
		}
		for i := 0; i < 2; i++ {
			if err := open(t, req, notice, okOpeners()); err != nil {
				t.Fatalf("openCreateRoutedTarget() call %d error = %v", i+1, err)
			}
		}
		if got := strings.Count(out.String(), "note: auto-routed create"); got != 1 {
			t.Errorf("two routed creates wrote the notice %d times, want 1: %q", got, out.String())
		}
	})
}
