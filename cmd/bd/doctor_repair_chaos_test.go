//go:build chaos

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDoctorRepair_CorruptDatabase_NotADatabase_RebuildFromJSONL(t *testing.T) {
	t.Skip("SQLite file corruption chaos test; not applicable to Dolt backend (bd-o0u)")
}

func TestDoctorRepair_CorruptDatabase_NoJSONL_FixFails(t *testing.T) {
	t.Skip("SQLite file corruption chaos test; not applicable to Dolt backend (bd-o0u)")
}

func TestDoctorRepair_CorruptDatabase_BacksUpSidecars(t *testing.T) {
	t.Skip("SQLite sidecar (-wal/-shm/-journal) backup test; Dolt has no sidecars (bd-o0u)")
}

func TestDoctorRepair_CorruptDatabase_WithRunningDaemon_FixSucceeds(t *testing.T) {
	t.Skip("SQLite file corruption with daemon test; not applicable to Dolt backend (bd-o0u)")
}

func TestDoctorRepair_JSONLIntegrity_MalformedLine_ReexportFromDB(t *testing.T) {
	t.Skip("SQLite JSONL re-export chaos test; not applicable to Dolt backend (bd-o0u)")
}

func TestDoctorRepair_DatabaseIntegrity_DBWriteLocked_ImportFailsFast(t *testing.T) {
	t.Skip("SQLite write-lock chaos test; Dolt uses server connections, not file locks (bd-o0u)")
}

func TestDoctorRepair_CorruptDatabase_ReadOnlyBeadsDir_PermissionsFixMakesWritable(t *testing.T) {
	t.Skip("SQLite file corruption + read-only dir chaos test; not applicable to Dolt backend (bd-o0u)")
}

func runBDWithEnv(ctx context.Context, exe, dir, dbPath string, env map[string]string, args ...string) (string, error) {
	fullArgs := []string{"--db", dbPath}
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, exe, fullArgs...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"BEADS_DIR="+filepath.Join(dir, ".beads"),
	)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}
