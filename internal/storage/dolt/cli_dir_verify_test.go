package dolt

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseDoltLogHash covers the `dolt log` output shapes this parser must
// handle: ANSI-colored (be-aru — dolt colors even non-tty output), plain, and
// malformed.
func TestParseDoltLogHash(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		out     string
		want    string
		wantErr bool
	}{
		{
			name: "ansi-colored commit line (observed dolt 2.3.2 output)",
			out:  "\x1b[33mcommit a0qnu7rg47sr7vmcuj8t23o6s2d8hlhc \x1b[0mInitialize data repository\n",
			want: "a0qnu7rg47sr7vmcuj8t23o6s2d8hlhc",
		},
		{
			name: "plain commit line",
			out:  "commit abcdefg1234567890\nAuthor: test <test@test.com>\n",
			want: "abcdefg1234567890",
		},
		{
			name:    "missing commit prefix",
			out:     "error: something went wrong\n",
			wantErr: true,
		},
		{
			name:    "empty output",
			out:     "",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDoltLogHash([]byte(tc.out))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got hash %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStaleLocalCLIDirError verifies the be-aru guard: identical hashes are
// accepted (CLI directory is provably the server's own store), and any
// mismatch — including either side being unreadable (empty) — is refused
// with an error naming the directory, the database, and both hashes, per the
// be-aru deliverable ("refuse with an explicit error naming both").
func TestStaleLocalCLIDirError(t *testing.T) {
	t.Parallel()
	const cliDir = "/beads/polecats/dust/beads/.beads/dolt/gt"
	const database = "gt"
	const branch = "main"

	t.Run("matching hashes are accepted", func(t *testing.T) {
		t.Parallel()
		if err := staleLocalCLIDirError(cliDir, database, branch, "abc123", "abc123"); err != nil {
			t.Fatalf("expected nil for matching hashes, got %v", err)
		}
	})

	t.Run("mismatched hashes are refused, naming both", func(t *testing.T) {
		t.Parallel()
		err := staleLocalCLIDirError(cliDir, database, branch, "serverhash1", "stalehash2")
		if err == nil {
			t.Fatal("expected error for mismatched hashes, got nil")
		}
		msg := err.Error()
		for _, want := range []string{cliDir, database, branch, "serverhash1", "stalehash2"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error message missing %q; got: %s", want, msg)
			}
		}
	})

	t.Run("empty server hash is refused", func(t *testing.T) {
		t.Parallel()
		err := staleLocalCLIDirError(cliDir, database, branch, "", "stalehash2")
		if err == nil {
			t.Fatal("expected error when server hash is empty, got nil")
		}
	})

	t.Run("empty local hash is refused", func(t *testing.T) {
		t.Parallel()
		err := staleLocalCLIDirError(cliDir, database, branch, "serverhash1", "")
		if err == nil {
			t.Fatal("expected error when local hash is empty, got nil")
		}
	})

	t.Run("both empty is refused, not treated as vacuously matching", func(t *testing.T) {
		t.Parallel()
		err := staleLocalCLIDirError(cliDir, database, branch, "", "")
		if err == nil {
			t.Fatal("expected error when both hashes are empty, got nil")
		}
	})
}

// TestLocalBranchHash_RealDoltDir exercises localBranchHash against a real,
// locally dolt-init'd directory — a "dummy .beads/dolt" standing in for the
// be-aru repro's leftover embedded copy — with no sql-server and no network
// involved at all. It never touches a Dolt server or any port.
func TestLocalBranchHash_RealDoltDir(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("dolt not in PATH")
	}

	tmp := t.TempDir()
	dbDir := filepath.Join(tmp, "gt")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	initCmd := exec.Command("dolt", "init", "--name", "test", "--email", "test@example.com")
	initCmd.Dir = dbDir
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("dolt init: %v\n%s", err, out)
	}

	hash, err := localBranchHash(context.Background(), dbDir, "main")
	if err != nil {
		t.Fatalf("localBranchHash: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash from a freshly initialized dolt repo")
	}

	// staleLocalCLIDirError must accept this directory against its own real
	// hash (the healthy case: CLI directory IS the store) ...
	if err := staleLocalCLIDirError(dbDir, "gt", "main", hash, hash); err != nil {
		t.Errorf("expected nil when local hash matches itself, got %v", err)
	}
	// ... and refuse it against a different hash, simulating a connected
	// server (be-aru: dolt_mode=server) whose live branch head does not
	// match this on-disk directory — e.g. a stale embedded copy.
	if err := staleLocalCLIDirError(dbDir, "gt", "main", "some-other-servers-head", hash); err == nil {
		t.Error("expected refusal when local directory diverges from the server's reported head")
	}

	// A branch that does not exist locally must error, not silently return
	// an empty/zero hash that could compare equal to another empty value.
	if _, err := localBranchHash(context.Background(), dbDir, "does-not-exist"); err == nil {
		t.Error("expected error for a nonexistent branch, got nil")
	}
}
