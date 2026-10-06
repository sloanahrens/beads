package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/storage"
)

func TestEffectiveRootStorePolicy(t *testing.T) {
	tests := []struct {
		name             string
		command          string
		strictReadonly   bool
		wantReadOnly     bool
		wantDisableStart bool
		wantMaintenance  bool
	}{
		{
			name:            "ordinary write command",
			command:         "create",
			wantMaintenance: true,
		},
		{
			name:            "classified read keeps compatibility maintenance",
			command:         "search",
			wantReadOnly:    true,
			wantMaintenance: true,
		},
		{
			name:             "strict readonly governs unclassified command",
			command:          "create",
			strictReadonly:   true,
			wantReadOnly:     true,
			wantDisableStart: true,
			wantMaintenance:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			policy := effectiveRootStorePolicy(tc.command, tc.strictReadonly)
			if policy.readOnly != tc.wantReadOnly {
				t.Fatalf("readOnly = %v, want %v", policy.readOnly, tc.wantReadOnly)
			}
			if policy.disableAutoStart != tc.wantDisableStart {
				t.Fatalf("disableAutoStart = %v, want %v", policy.disableAutoStart, tc.wantDisableStart)
			}
			if policy.runMaintenance != tc.wantMaintenance {
				t.Fatalf("runMaintenance = %v, want %v", policy.runMaintenance, tc.wantMaintenance)
			}
		})
	}
}

type strictReadonlyPostRunStore struct {
	storage.DoltStorage
	metadataWrites int
	closeCalls     int
}

func (s *strictReadonlyPostRunStore) SetLocalMetadata(context.Context, string, string) error {
	s.metadataWrites++
	return nil
}

func (s *strictReadonlyPostRunStore) Close() error {
	s.closeCalls++
	return nil
}

type readonlyTreeEntry struct {
	Mode   fs.FileMode
	SHA256 string
}

type readonlyTreeSnapshot struct {
	Exists  bool
	Entries map[string]readonlyTreeEntry
}

func snapshotReadonlyTree(t *testing.T, root string) readonlyTreeSnapshot {
	t.Helper()
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return readonlyTreeSnapshot{}
	} else if err != nil {
		t.Fatalf("stat snapshot root %s: %v", root, err)
	}

	snapshot := readonlyTreeSnapshot{Exists: true, Entries: make(map[string]readonlyTreeEntry)}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		item := readonlyTreeEntry{Mode: info.Mode()}
		switch {
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			item.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			item.SHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(target)))
		}
		snapshot.Entries[rel] = item
		return nil
	}); err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return snapshot
}

func readonlyCanaryEnv(home, beadsDir, circuitDir string, port int) []string {
	replace := map[string]bool{
		"HOME": true, "XDG_CONFIG_HOME": true,
		"BEADS_DIR": true, "BEADS_DB": true, "BD_DB": true,
		"BEADS_DOLT_PORT": true, "BEADS_DOLT_SERVER_PORT": true, "BEADS_DOLT_AUTO_START": true,
		"BEADS_TEST_MODE": true, "BEADS_TEST_CIRCUIT_DIR": true,
		"BD_DISABLE_METRICS": true, "BD_DISABLE_EVENT_FLUSH": true,
		"BD_OTEL_METRICS_URL": true, "BD_OTEL_LOGS_URL": true, "BD_OTEL_STDOUT": true,
	}
	env := make([]string, 0, len(os.Environ())+16)
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !replace[key] {
			env = append(env, value)
		}
	}
	return append(env,
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, "xdg"),
		"BEADS_DIR="+beadsDir,
		"BEADS_DB=", "BD_DB=",
		"BEADS_DOLT_SERVER_PORT="+strconv.Itoa(port),
		"BEADS_DOLT_PORT="+strconv.Itoa(port),
		"BEADS_DOLT_AUTO_START=0",
		"BEADS_TEST_MODE=1",
		"BEADS_TEST_CIRCUIT_DIR="+circuitDir,
		"BD_DISABLE_METRICS=1",
		"BD_DISABLE_EVENT_FLUSH=1",
		"BD_OTEL_METRICS_URL=", "BD_OTEL_LOGS_URL=", "BD_OTEL_STDOUT=false",
		"BEADS_TEST_IGNORE_REPO_CONFIG=1",
	)
}

func TestPersistentPostRunStrictReadonlySuppressesMaintenance(t *testing.T) {
	originalStore := store
	originalReadonly := readonlyMode
	originalRootCtx := rootCtx
	originalRootCancel := rootCancel
	originalCommandSpan := commandSpan
	originalProfileFile := profileFile
	originalTraceFile := traceFile
	storeMutex.Lock()
	originalStoreActive := storeActive
	storeMutex.Unlock()
	originalDoltAutoCommit := doltAutoCommit
	originalDidWrite := commandDidWrite.Load()
	originalDidExplicitCommit := commandDidExplicitDoltCommit
	originalDidWriteTipMetadata := commandDidWriteTipMetadata
	originalTipIDsWereNil := commandTipIDsShown == nil
	originalTipIDsShown := make(map[string]struct{}, len(commandTipIDsShown))
	for id := range commandTipIDsShown {
		originalTipIDsShown[id] = struct{}{}
	}
	originalBackup := runPostRunAutoBackup
	originalExport := runPostRunAutoExport
	originalPush := runPostRunAutoPush
	t.Cleanup(func() {
		store = originalStore
		readonlyMode = originalReadonly
		rootCtx = originalRootCtx
		rootCancel = originalRootCancel
		commandSpan = originalCommandSpan
		profileFile = originalProfileFile
		traceFile = originalTraceFile
		storeMutex.Lock()
		storeActive = originalStoreActive
		storeMutex.Unlock()
		doltAutoCommit = originalDoltAutoCommit
		commandDidWrite.Store(originalDidWrite)
		commandDidExplicitDoltCommit = originalDidExplicitCommit
		commandDidWriteTipMetadata = originalDidWriteTipMetadata
		if originalTipIDsWereNil {
			commandTipIDsShown = nil
		} else {
			commandTipIDsShown = originalTipIDsShown
		}
		runPostRunAutoBackup = originalBackup
		runPostRunAutoExport = originalExport
		runPostRunAutoPush = originalPush
	})

	maintenanceCalls := 0
	runPostRunAutoBackup = func(context.Context) { maintenanceCalls++ }
	runPostRunAutoExport = func(context.Context, bool) error { maintenanceCalls++; return nil }
	runPostRunAutoPush = func(context.Context) { maintenanceCalls++ }

	fake := &strictReadonlyPostRunStore{}
	store = fake
	readonlyMode = true
	rootCtx = context.Background()
	rootCancel = nil
	commandSpan = nil
	profileFile = nil
	traceFile = nil
	commandDidWrite.Store(true)
	commandDidExplicitDoltCommit = false
	commandDidWriteTipMetadata = true
	commandTipIDsShown = map[string]struct{}{"strict-readonly": {}}
	doltAutoCommit = string(doltAutoCommitOn)

	if err := rootCmd.PersistentPostRunE(&cobra.Command{Use: "create"}, nil); err != nil {
		t.Fatalf("PersistentPostRunE: %v", err)
	}
	if maintenanceCalls != 0 {
		t.Fatalf("strict readonly ran %d automatic backup/export/push operation(s)", maintenanceCalls)
	}
	if fake.metadataWrites != 0 {
		t.Fatalf("strict readonly wrote %d tip metadata value(s)", fake.metadataWrites)
	}
	if fake.closeCalls != 1 {
		t.Fatalf("store close calls = %d, want 1", fake.closeCalls)
	}
}
