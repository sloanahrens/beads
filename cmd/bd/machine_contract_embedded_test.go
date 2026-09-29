//go:build cgo

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

// binaryHotReads serves the contract through the real bd binary against a
// throwaway embedded Dolt workspace (no server, no Docker).
type binaryHotReads struct {
	bd, dir string
}

func (b *binaryHotReads) run(t *testing.T, args ...string) (decodedEnvelope, int) {
	t.Helper()
	cmd := exec.Command(b.bd, args...)
	cmd.Dir = b.dir
	cmd.Env = append(bdEnv(b.dir), "BD_MACHINE=1", "BD_EVENTS_JOURNAL=1")
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("bd %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	var env decodedEnvelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("bd %v: stdout is not one envelope: %v\n%s\nstderr: %s", args, err, out.String(), errOut.String())
	}
	return env, code
}

func (b *binaryHotReads) seed(t *testing.T, n int) {
	for i := 0; i < n; i++ {
		env, code := b.run(t, "create", "contract row "+strconv.Itoa(i), "-p", strconv.Itoa((n-i)%3))
		if code != 0 {
			t.Fatalf("create: exit %d error %+v", code, env.Error)
		}
	}
}

func (b *binaryHotReads) ready(t *testing.T, after string, limit int) (decodedEnvelope, int) {
	args := []string{"ready", "--sort", "priority", "--limit", strconv.Itoa(limit)}
	if after != "" {
		args = append(args, "--after", after)
	}
	return b.run(t, args...)
}

func (b *binaryHotReads) events(t *testing.T, since int64, limit int) (decodedEnvelope, int) {
	return b.run(t, "events", "tail", "--since", strconv.FormatInt(since, 10), "--limit", strconv.Itoa(limit))
}

func TestHotReadContract_EmbeddedDolt(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}
	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "hr", "--skip-hooks", "--skip-agents")
	runHotReadContract(t, &binaryHotReads{bd: bd, dir: dir})
}
