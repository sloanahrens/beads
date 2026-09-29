//go:build cgo

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// machineRun is one bd invocation's observable result.
type machineRun struct {
	stdout, stderr string
	code           int
	env            decodedEnvelope
}

// runBD runs the built binary in dir. machine adds BD_MACHINE=1 and decodes
// stdout as the envelope, failing if stdout is anything but one document.
func runBD(t *testing.T, bd, dir string, machine bool, args ...string) machineRun {
	t.Helper()
	cmd := exec.Command(bd, args...)
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	if machine {
		cmd.Env = append(cmd.Env, "BD_MACHINE=1")
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	r := machineRun{stdout: out.String(), stderr: errOut.String()}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		r.code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("bd %v: %v", args, err)
	}
	if machine {
		dec := json.NewDecoder(strings.NewReader(r.stdout))
		if derr := dec.Decode(&r.env); derr != nil {
			t.Fatalf("bd %v: stdout is not a JSON envelope: %v\nstdout=%s\nstderr=%s", args, derr, r.stdout, r.stderr)
		}
		if dec.More() {
			t.Fatalf("bd %v: stdout carries more than one JSON document:\n%s", args, r.stdout)
		}
		if r.env.SchemaVersion == nil || r.env.ContractVersion == nil || *r.env.ContractVersion != JSONContractVersion {
			t.Fatalf("bd %v: envelope versions missing or wrong: %s", args, r.stdout)
		}
	}
	return r
}

func createID(t *testing.T, bd, dir string, extra ...string) string {
	t.Helper()
	r := runBD(t, bd, dir, true, append([]string{"create"}, extra...)...)
	if r.code != 0 {
		t.Fatalf("create %v: exit %d\n%s\n%s", extra, r.code, r.stdout, r.stderr)
	}
	var issue struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(r.env.Data, &issue); err != nil || issue.ID == "" {
		t.Fatalf("create data has no id: %s (%v)", r.env.Data, err)
	}
	return issue.ID
}

// TestMachineSurfaceEndToEnd drives the machine contract through the binary
// against a throwaway embedded Dolt workspace: no server, no Docker.
func TestMachineSurfaceEndToEnd(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "mc", "--skip-hooks", "--skip-agents")

	closeable := createID(t, bd, dir, "closeable")
	blocker := createID(t, bd, dir, "blocker")
	blocked := createID(t, bd, dir, "blocked")
	if r := runBD(t, bd, dir, true, "dep", "add", blocked, blocker, "--type=blocks"); r.code != 0 {
		t.Fatalf("dep add: exit %d %s", r.code, r.stderr)
	}

	t.Run("show success", func(t *testing.T) {
		r := runBD(t, bd, dir, true, "show", closeable)
		if r.code != 0 || r.env.Error != nil || !strings.Contains(string(r.env.Data), closeable) {
			t.Fatalf("exit %d error %+v data %s", r.code, r.env.Error, r.env.Data)
		}
	})

	t.Run("show missing is not_found", func(t *testing.T) {
		r := runBD(t, bd, dir, true, "show", "mc-nope")
		if r.code != 20 || r.env.Error == nil || r.env.Error.Kind != "not_found" {
			t.Fatalf("exit %d error %+v", r.code, r.env.Error)
		}
		if len(r.env.Error.IDs) != 1 || r.env.Error.IDs[0].ID != "mc-nope" {
			t.Fatalf("ids = %+v", r.env.Error.IDs)
		}
	})

	t.Run("show some missing is partial", func(t *testing.T) {
		r := runBD(t, bd, dir, true, "show", closeable, "mc-nope")
		if r.code != 22 || r.env.Error == nil || r.env.Error.Kind != "partial" || !strings.Contains(string(r.env.Data), closeable) {
			t.Fatalf("exit %d error %+v data %s", r.code, r.env.Error, r.env.Data)
		}
	})

	t.Run("list carries pagination", func(t *testing.T) {
		r := runBD(t, bd, dir, true, "list", "--limit", "1")
		if r.code != 0 || r.env.Pagination == nil || !r.env.Pagination.Truncated || r.env.Pagination.Returned != 1 {
			t.Fatalf("exit %d pagination %+v error %+v", r.code, r.env.Pagination, r.env.Error)
		}
		r = runBD(t, bd, dir, true, "list", "--limit", "0")
		if r.code != 0 || r.env.Pagination == nil || r.env.Pagination.Truncated {
			t.Fatalf("unlimited list: exit %d pagination %+v", r.code, r.env.Pagination)
		}
	})

	t.Run("unknown command is invalid_args", func(t *testing.T) {
		r := runBD(t, bd, dir, true, "shwo", closeable)
		if r.code != 27 || r.env.Error == nil || r.env.Error.Kind != "invalid_args" {
			t.Fatalf("exit %d error %+v", r.code, r.env.Error)
		}
	})

	t.Run("readonly refusal is refused", func(t *testing.T) {
		r := runBD(t, bd, dir, true, "--readonly", "update", closeable, "--notes", "x")
		if r.code != 21 || r.env.Error == nil || r.env.Error.Kind != "refused" {
			t.Fatalf("exit %d error %+v stderr %s", r.code, r.env.Error, r.stderr)
		}
	})

	t.Run("partial close exits non-zero in both modes", func(t *testing.T) {
		r := runBD(t, bd, dir, true, "close", closeable, blocked)
		if r.code != 22 || r.env.Error == nil || r.env.Error.Kind != "partial" {
			t.Fatalf("machine: exit %d error %+v", r.code, r.env.Error)
		}
		if len(r.env.Error.IDs) != 1 || r.env.Error.IDs[0].ID != blocked || r.env.Error.IDs[0].Kind != kindRefused {
			t.Fatalf("machine: ids = %+v", r.env.Error.IDs)
		}
		// Legacy: a partial batch exits 1 (it exited 0 before), and the
		// closeable id still closes.
		another := createID(t, bd, dir, "another closeable")
		r = runBD(t, bd, dir, false, "close", another, blocked)
		if r.code != 1 {
			t.Fatalf("legacy partial close: exit %d, want 1", r.code)
		}
		r = runBD(t, bd, dir, false, "show", another, "--json")
		if !strings.Contains(r.stdout, `"status": "closed"`) {
			t.Fatalf("legacy partial close did not close the closeable id:\n%s", r.stdout)
		}
	})

	t.Run("undefer of an open issue is refused", func(t *testing.T) {
		r := runBD(t, bd, dir, true, "undefer", blocker)
		if r.code != 21 || r.env.Error == nil || r.env.Error.Kind != "refused" {
			t.Fatalf("exit %d error %+v", r.code, r.env.Error)
		}
		r = runBD(t, bd, dir, false, "undefer", blocker)
		if r.code != 1 {
			t.Fatalf("legacy undefer failure: exit %d, want 1", r.code)
		}
	})

	t.Run("legacy json shape is unchanged", func(t *testing.T) {
		r := runBD(t, bd, dir, false, "show", blocker, "--json")
		if r.code != 0 || !strings.HasPrefix(strings.TrimSpace(r.stdout), "[") {
			t.Fatalf("legacy show --json is no longer a bare array: exit %d\n%s", r.code, r.stdout)
		}
	})

	t.Run("no workspace is store_unavailable", func(t *testing.T) {
		empty := t.TempDir()
		r := runBD(t, bd, empty, true, "list")
		if r.code != 25 || r.env.Error == nil || r.env.Error.Kind != "store_unavailable" {
			t.Fatalf("exit %d error %+v stderr %s", r.code, r.env.Error, r.stderr)
		}
	})
}
