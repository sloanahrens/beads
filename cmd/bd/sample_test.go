//go:build cgo

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveyegge/beads/samplesops"
)

// The store-backed round trip for bd sample lives in
// sample_integration_test.go. The tests here stay in the unit tier: every
// refusal happens inside the verb before it opens the samples role, so driving
// RunE directly needs no Dolt server and no database.

// newSampleUnitEnv pins the globals the sample verbs read and deliberately
// leaves `store` nil: a refusal that reached the store would fail differently,
// which is exactly what the tests below must not allow.
func newSampleUnitEnv(t *testing.T) *parityEnv {
	t.Helper()
	saveAndRestoreGlobals(t)
	ensureCleanGlobalState(t)

	savedCtx, savedJSON, savedReadonly := rootCtx, jsonOutput, readonlyMode
	t.Cleanup(func() {
		rootCtx, jsonOutput, readonlyMode = savedCtx, savedJSON, savedReadonly
	})

	rootCtx = context.Background()
	jsonOutput = false
	readonlyMode = false
	store = nil
	storeActive = false
	return &parityEnv{t: t}
}

// writePayload writes payload under the test's temp dir and returns its path.
func writePayload(t *testing.T, payload []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	return path
}

func TestSampleAddRefusesBadInputBeforeOpeningTheStore(t *testing.T) {
	oversize := append([]byte(`{"k":"`), make([]byte, samplesops.MaxPayloadBytes)...)
	oversize = append(oversize, []byte(`"}`)...)

	tests := []struct {
		name    string
		payload []byte
		flags   map[string]string
		want    []string
	}{
		{
			name:    "bad at",
			payload: []byte(`{"ok":true}`),
			flags:   map[string]string{"kind": "usage_record", "at": "yesterday"},
			want:    []string{"--at", "RFC3339"},
		},
		{
			name:    "missing at",
			payload: []byte(`{"ok":true}`),
			flags:   map[string]string{"kind": "usage_record", "at": ""},
			want:    []string{"--at", "required"},
		},
		{
			name:    "non-object payload",
			payload: []byte(`[1,2,3]`),
			flags:   map[string]string{"kind": "usage_record", "at": "2026-11-11T09:30:00Z"},
			want:    []string{"payload", "JSON object"},
		},
		{
			name:    "oversize payload",
			payload: oversize,
			flags:   map[string]string{"kind": "usage_record", "at": "2026-11-11T09:30:00Z"},
			want:    []string{"payload", "65536-byte cap"},
		},
		{
			name:    "empty kind",
			payload: []byte(`{"ok":true}`),
			flags:   map[string]string{"kind": "", "at": "2026-11-11T09:30:00Z"},
			want:    []string{"kind", "required"},
		},
		{
			name:    "empty payload source",
			payload: []byte(`{"ok":true}`),
			flags:   map[string]string{"kind": "usage_record", "at": "2026-11-11T09:30:00Z", "json": ""},
			want:    []string{"--json", "required"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newSampleUnitEnv(t)
			flags := map[string]string{"json": writePayload(t, tt.payload)}
			for k, v := range tt.flags {
				flags[k] = v
			}
			env.setFlags(sampleAddCmd, flags)

			result := env.run(sampleAddCmd)
			if result.exitCode == 0 {
				t.Fatalf("bd sample add exited 0, want a refusal (stdout: %q)", result.stdout)
			}
			for _, want := range tt.want {
				if !strings.Contains(result.stderr, want) {
					t.Errorf("stderr %q does not name %q", result.stderr, want)
				}
			}
		})
	}
}

func TestSampleListRefusesBadInputBeforeOpeningTheStore(t *testing.T) {
	tests := []struct {
		name  string
		flags map[string]string
		want  []string
	}{
		{
			name:  "bad since",
			flags: map[string]string{"kind": "usage_record", "since": "2026-11-11 09:30"},
			want:  []string{"--since", "RFC3339"},
		},
		{
			name:  "negative limit",
			flags: map[string]string{"kind": "usage_record", "limit": "-1"},
			want:  []string{"limit", "negative"},
		},
		{
			name:  "empty kind",
			flags: map[string]string{"kind": ""},
			want:  []string{"kind", "required"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newSampleUnitEnv(t)
			env.setFlags(sampleListCmd, tt.flags)

			result := env.run(sampleListCmd)
			if result.exitCode == 0 {
				t.Fatalf("bd sample list exited 0, want a refusal (stdout: %q)", result.stdout)
			}
			for _, want := range tt.want {
				if !strings.Contains(result.stderr, want) {
					t.Errorf("stderr %q does not name %q", result.stderr, want)
				}
			}
		})
	}
}

// TestSampleCommandDocumentsBothVerbs pins acceptance box 4's CLI half: the
// parent help lists add and list, and each documents its flags. The docs page
// is generated from this text, so a missing sentence here is a missing page.
func TestSampleCommandDocumentsBothVerbs(t *testing.T) {
	wanted := map[string][]string{
		"add":  {"--kind", "--at", "--json", "RFC3339", "verbatim"},
		"list": {"--kind", "--since", "--limit", "newest first", "payload"},
	}
	found := map[string]bool{}
	for _, sub := range sampleCmd.Commands() {
		help, ok := wanted[sub.Name()]
		if !ok {
			continue
		}
		found[sub.Name()] = true
		for _, want := range help {
			if !strings.Contains(sub.Long, want) {
				t.Errorf("bd sample %s help does not mention %q:\n%s", sub.Name(), want, sub.Long)
			}
		}
	}
	for name := range wanted {
		if !found[name] {
			t.Errorf("bd sample has no %q subcommand", name)
		}
	}
}

// TestSampleAddJsonFlagIsThePayloadNotMachineOutput pins the deliberate
// shadowing: add takes --json <file|->, so the inherited bool must not answer
// for it. list keeps the inherited bool, so it declares no local flag.
func TestSampleAddJsonFlagIsThePayloadNotMachineOutput(t *testing.T) {
	flag := sampleAddCmd.Flags().Lookup("json")
	if flag == nil {
		t.Fatal("bd sample add has no --json flag")
	}
	if got := flag.Value.Type(); got != "string" {
		t.Errorf("bd sample add --json is a %s, want a string payload source", got)
	}
	if flag := sampleListCmd.Flags().Lookup("json"); flag != nil {
		t.Errorf("bd sample list declares a local --json flag (%s); it must take the inherited output flag", flag.Value.Type())
	}
}
